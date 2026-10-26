package client

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/v1rushb/neoncoup/discovery"
	"github.com/v1rushb/neoncoup/protocol"
	"github.com/v1rushb/neoncoup/server"
	"github.com/v1rushb/neoncoup/transport"
)

// LocalBackend runs servers inside this process: solo games, hosted rooms,
// and joins to other hosts.
type LocalBackend struct {
	Logger *slog.Logger
	// Relay is the default relay URL for hosting and joining by code.
	Relay string
	// LANTimeout bounds LAN discovery before falling back to the relay.
	LANTimeout time.Duration

	mu    sync.Mutex
	srv   *server.Server
	stops []func()
}

func (b *LocalBackend) logger() *slog.Logger {
	if b.Logger == nil {
		return slog.New(slog.DiscardHandler)
	}
	return b.Logger
}

// Solo starts a private game against bots.
func (b *LocalBackend) Solo(o SoloOptions) (*Connection, error) {
	b.Close()
	srv, conn, err := soloServer(o)
	if err != nil {
		return nil, err
	}
	b.mu.Lock()
	b.srv = srv
	b.mu.Unlock()
	return conn, nil
}

// Hosted is a running room server.
type Hosted struct {
	Server *server.Server
	Room   string
	Port   int
	Info   *HostInfo
	stops  []func()
}

// Close stops the room server, its listener and its discovery responder.
func (h *Hosted) Close() {
	for i := len(h.stops) - 1; i >= 0; i-- {
		h.stops[i]()
	}
}

// OpenRoom opens a room server: a WebSocket listener, LAN discovery and,
// when relay is set, a relay registration. It is shared by the TUI host and
// the headless host.
func OpenRoom(o HostOptions, log *slog.Logger) (*Hosted, error) {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	code := protocol.NewRoomCode()
	srv := server.New(server.Options{Logger: log, DefaultRoom: code, ThinkMin: 800 * time.Millisecond, ThinkMax: 2500 * time.Millisecond})
	h := &Hosted{Server: srv, Room: code}
	h.stops = append(h.stops, srv.Close)
	if _, err := srv.CreateRoom(code, o.Settings); err != nil {
		h.Close()
		return nil, err
	}
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", o.Port))
	if err != nil {
		h.Close()
		return nil, fmt.Errorf("can't listen on port %d (%w). Pick another with --port", o.Port, err)
	}
	h.Port = ln.Addr().(*net.TCPAddr).Port
	hs := &http.Server{Handler: srv.Handler(), ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = hs.Serve(ln) }()
	h.stops = append(h.stops, func() { _ = hs.Close() })

	info := &HostInfo{Room: code}
	info.Lines = append(info.Lines, "same network: neoncoup join "+code)
	for _, ip := range discovery.LocalIPs() {
		info.Lines = append(info.Lines, fmt.Sprintf("direct:       neoncoup join %s:%d", ip, h.Port))
		break
	}
	resp, err := discovery.Listen(func(room string) (int, bool) { return h.Port, room == code })
	if err != nil {
		log.Warn("LAN discovery unavailable", "err", err)
		info.Lines = append(info.Lines, "(LAN discovery is unavailable here; use the direct command)")
	} else {
		h.stops = append(h.stops, func() { _ = resp.Close() })
	}
	if o.Relay != "" {
		if err := h.registerRelay(o.Relay, log); err != nil {
			h.Close()
			return nil, err
		}
		info.Lines = append(info.Lines, fmt.Sprintf("internet:     neoncoup join %s --relay %s", code, o.Relay))
	} else {
		info.Lines = append(info.Lines, fmt.Sprintf("internet:     forward TCP port %d, or host with --relay URL", h.Port))
	}
	h.Info = info
	log.Info("hosting", "room", code, "port", h.Port, "relay", o.Relay)
	return h, nil
}

// Host opens a room that others can join, with the local player in it.
func (b *LocalBackend) Host(o HostOptions) (*Connection, error) {
	b.Close()
	if o.Relay == "" {
		o.Relay = b.Relay
	}
	h, err := OpenRoom(o, b.logger())
	if err != nil {
		return nil, err
	}
	b.mu.Lock()
	b.stops = append(b.stops, h.Close)
	b.mu.Unlock()
	a, c := transport.Pipe()
	go h.Server.ServeConn(c)
	return &Connection{Conn: a, Room: h.Room, Via: "you are hosting", Host: h.Info, Target: h.Room}, nil
}

// Join connects to a room by code (LAN first, then relay) or by address.
func (b *LocalBackend) Join(ctx context.Context, target string) (*Connection, error) {
	b.Close()
	target = strings.TrimSpace(target)
	if code, ok := protocol.ParseRoomCode(target); ok {
		lanWait := b.LANTimeout
		if lanWait == 0 {
			lanWait = 1500 * time.Millisecond
		}
		lctx, cancel := context.WithTimeout(ctx, lanWait)
		addr, lanErr := discovery.Find(lctx, code)
		cancel()
		if lanErr == nil {
			if conn, err := transport.Dial(ctx, addr); err == nil {
				return &Connection{Conn: conn, Room: code, Via: "LAN " + addr, Dial: dialer(addr), Target: code}, nil
			}
		}
		if b.Relay != "" {
			u, err := RelayJoinURL(b.Relay, code)
			if err != nil {
				return nil, err
			}
			conn, err := transport.Dial(ctx, u)
			if err != nil {
				return nil, fmt.Errorf("room %s is not on your LAN, and the relay at %s did not answer: %w", code, b.Relay, err)
			}
			return &Connection{Conn: conn, Room: code, Via: "relay " + b.Relay, Dial: dialer(u), Target: code}, nil
		}
		return nil, fmt.Errorf("room %s is not on your LAN and no relay is configured. "+
			"Ask the host for their address (neoncoup join HOST:PORT), or set a relay with --relay or in settings", code)
	}
	if target == "" {
		return nil, errors.New("nothing to join")
	}
	conn, err := transport.Dial(ctx, target)
	if err != nil {
		return nil, err
	}
	return &Connection{Conn: conn, Via: "direct " + target, Dial: dialer(target), Target: target}, nil
}

func dialer(addr string) Dialer {
	return func(ctx context.Context) (transport.Conn, error) { return transport.Dial(ctx, addr) }
}

// RelayJoinURL is where players connect to reach a room through a relay.
func RelayJoinURL(relay, room string) (string, error) {
	return transport.WSURL(strings.TrimRight(relay, "/")+"/join/"+room, "/join/"+room)
}

// Close stops anything this backend started.
func (b *LocalBackend) Close() {
	b.mu.Lock()
	srv, stops := b.srv, b.stops
	b.srv, b.stops = nil, nil
	b.mu.Unlock()
	for _, s := range stops {
		s()
	}
	if srv != nil {
		srv.Close()
	}
}

// registerRelay announces the room on a relay and serves every player who
// arrives through it. If the relay link drops, it re-registers with backoff;
// relayed players then rejoin with their tokens.
func (h *Hosted) registerRelay(url string, log *slog.Logger) error {
	key := protocol.NewToken() // proves to the relay that re-registrations are ours
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	rh, err := transport.DialRelayHost(ctx, url, h.Room, key)
	cancel()
	if err != nil {
		return fmt.Errorf("relay %s: %w", url, err)
	}
	stop := make(chan struct{})
	var once sync.Once
	h.stops = append(h.stops, func() { once.Do(func() { close(stop) }) })
	go func() {
		for {
			go func(rh *transport.RelayHost) {
				for {
					c, err := rh.Accept(context.Background())
					if err != nil {
						return
					}
					go h.Server.ServeConn(c)
				}
			}(rh)
			select {
			case <-stop:
				rh.Close()
				return
			case <-rh.Done():
			}
			log.Warn("relay link dropped; re-registering", "relay", url)
			wait := 250 * time.Millisecond
			for {
				select {
				case <-stop:
					return
				case <-time.After(wait):
				}
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				next, err := transport.DialRelayHost(ctx, url, h.Room, key)
				cancel()
				if err == nil {
					rh = next
					log.Info("relay link restored", "relay", url)
					break
				}
				wait = min(wait*2, 30*time.Second)
			}
		}
	}()
	return nil
}
