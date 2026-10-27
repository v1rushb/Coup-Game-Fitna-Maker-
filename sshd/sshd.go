// Package sshd serves NEON//COUP over SSH: `ssh -p 2222 host` opens the
// full TUI with no install. Every session shares one in-process hub, so SSH
// players can host rooms and join each other by room code. There is no audio
// in this mode; the terminal is the only channel.
package sshd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/ssh"
	"charm.land/wish/v2"
	"charm.land/wish/v2/activeterm"
	wishtea "charm.land/wish/v2/bubbletea"

	"github.com/v1rushb/neoncoup/audio"
	"github.com/v1rushb/neoncoup/client"
	"github.com/v1rushb/neoncoup/config"
	"github.com/v1rushb/neoncoup/discovery"
	"github.com/v1rushb/neoncoup/protocol"
	"github.com/v1rushb/neoncoup/server"
	"github.com/v1rushb/neoncoup/transport"
)

// Options configure the SSH server.
type Options struct {
	Addr        string
	HostKeyPath string
	Logger      *slog.Logger
	Version     string
	// HTTPAddr, when set, also serves the hub over WebSockets so CLI players
	// can join SSH-hosted rooms (with LAN discovery).
	HTTPAddr string
	// MaxSessions caps concurrent SSH sessions (0 = 64).
	MaxSessions int
	// Instant removes the bots' think delay (tests).
	Instant bool
}

// Server is a running SSH front end.
type Server struct {
	opts     Options
	log      *slog.Logger
	hub      *server.Server
	ssh      *ssh.Server
	http     *http.Server
	disc     *discovery.Responder
	mu       sync.Mutex
	sessions int
	httpPort int
}

// New prepares a server; call ListenAndServe to run it.
func New(o Options) (*Server, error) {
	if o.Logger == nil {
		o.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	if o.MaxSessions == 0 {
		o.MaxSessions = 64
	}
	think := server.Options{ThinkMin: 800 * time.Millisecond, ThinkMax: 2500 * time.Millisecond}
	if o.Instant {
		think = server.Options{}
	}
	hub := server.New(server.Options{Logger: o.Logger, ThinkMin: think.ThinkMin, ThinkMax: think.ThinkMax, IdleClose: 2 * time.Minute, MaxRooms: 256})
	s := &Server{opts: o, log: o.Logger, hub: hub}
	srv, err := wish.NewServer(
		wish.WithAddress(o.Addr),
		wish.WithHostKeyPath(o.HostKeyPath),
		wish.WithIdleTimeout(30*time.Minute),
		wish.WithMiddleware(
			wishtea.Middleware(s.handler),
			activeterm.Middleware(),
			s.limit,
		),
	)
	if err != nil {
		hub.Close()
		return nil, err
	}
	s.ssh = srv
	return s, nil
}

// limit caps concurrent sessions. Middlewares run last-to-first, so this
// runs before the TUI starts.
func (s *Server) limit(next ssh.Handler) ssh.Handler {
	return func(sess ssh.Session) {
		s.mu.Lock()
		if s.sessions >= s.opts.MaxSessions {
			s.mu.Unlock()
			wish.Fatalln(sess, "NEON//COUP is full right now. Try again in a bit.")
			return
		}
		s.sessions++
		s.mu.Unlock()
		defer func() {
			s.mu.Lock()
			s.sessions--
			s.mu.Unlock()
		}()
		s.log.Info("ssh session", "user", sess.User(), "remote", sess.RemoteAddr().String())
		next(sess)
	}
}

func (s *Server) handler(sess ssh.Session) (tea.Model, []tea.ProgramOption) {
	cfg := config.Default()
	if n := protocol.CleanName(sess.User()); n != "" {
		cfg.Name = strings.ToUpper(n)
	}
	cfg.SkipBoot = false
	ascii := false
	for _, e := range sess.Environ() {
		if e == "NEONCOUP_ASCII=1" {
			ascii = true
		}
	}
	be := &hubBackend{s: s}
	app := client.New(client.Options{
		Backend: be,
		Config:  cfg,
		Sound:   audio.Nop{},
		ASCII:   ascii,
		Version: s.opts.Version,
		Remote:  true,
	})
	return app, []tea.ProgramOption{tea.WithFPS(20)}
}

// ListenAndServe runs until ctx is canceled.
func (s *Server) ListenAndServe(ctx context.Context) error {
	if s.opts.HTTPAddr != "" {
		ln, err := net.Listen("tcp", s.opts.HTTPAddr)
		if err != nil {
			return fmt.Errorf("http listener: %w", err)
		}
		s.httpPort = ln.Addr().(*net.TCPAddr).Port
		s.http = &http.Server{Handler: s.hub.Handler(), ReadHeaderTimeout: 10 * time.Second}
		go func() { _ = s.http.Serve(ln) }()
		if d, err := discovery.Listen(func(room string) (int, bool) { return s.httpPort, s.hub.Room(room) != nil }); err == nil {
			s.disc = d
		}
	}
	errc := make(chan error, 1)
	go func() { errc <- s.ssh.ListenAndServe() }()
	select {
	case err := <-errc:
		s.close()
		if errors.Is(err, ssh.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		err := s.ssh.Shutdown(sctx)
		s.close()
		if errors.Is(err, ssh.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func (s *Server) close() {
	if s.disc != nil {
		_ = s.disc.Close()
	}
	if s.http != nil {
		_ = s.http.Close()
	}
	s.hub.Close()
}

// Hub exposes the shared game server (tests).
func (s *Server) Hub() *server.Server { return s.hub }

// hubBackend gives one SSH session access to the shared hub.
type hubBackend struct {
	s    *Server
	mu   sync.Mutex
	owns []*server.Room // private solo rooms this session opened
}

func (b *hubBackend) pipe(room *server.Room) transport.Conn {
	a, c := transport.Pipe()
	go b.s.hub.ServeConn(c)
	return a
}

func (b *hubBackend) Solo(o client.SoloOptions) (*client.Connection, error) {
	b.Close()
	st := protocol.DefaultSettings()
	st.MaxPlayers = 1 + o.Bots
	st.FillBots = true
	st.BotLevel = o.Level
	st.Magistrate = o.Magistrate
	st.TurnMS = 0
	st.WindowMS = o.WindowMS
	st.Speed = protocol.ParseSpeed(o.Speed)
	if o.Instant || b.s.opts.Instant {
		st.Speed = protocol.SpeedInstant
	}
	room, err := b.s.hub.CreateRoom("", st.Clamp())
	if err != nil {
		return nil, err
	}
	b.mu.Lock()
	b.owns = append(b.owns, room)
	b.mu.Unlock()
	return &client.Connection{Conn: b.pipe(room), Room: room.Code(), Via: "ssh"}, nil
}

func (b *hubBackend) Host(o client.HostOptions) (*client.Connection, error) {
	b.Close()
	room, err := b.s.hub.CreateRoom("", o.Settings)
	if err != nil {
		return nil, err
	}
	code := room.Code()
	info := &client.HostInfo{Room: code, Lines: []string{"friends: ssh in to this server, pick JOIN ROOM, enter " + code}}
	if b.s.httpPort != 0 {
		info.Lines = append(info.Lines, "or with the app on this LAN: neoncoup join "+code)
	}
	return &client.Connection{Conn: b.pipe(room), Room: code, Via: "ssh", Host: info, Target: code}, nil
}

func (b *hubBackend) Join(ctx context.Context, target string) (*client.Connection, error) {
	b.Close()
	code, ok := protocol.ParseRoomCode(target)
	if !ok {
		return nil, errors.New("over SSH you join rooms on this server by code, e.g. NEON-7K2Q")
	}
	room := b.s.hub.Room(code)
	if room == nil {
		return nil, fmt.Errorf("no room %s on this server", code)
	}
	return &client.Connection{Conn: b.pipe(room), Room: code, Via: "ssh", Target: code}, nil
}

// Close shuts the session's private solo rooms; shared rooms live on until
// they are empty.
func (b *hubBackend) Close() {
	b.mu.Lock()
	owns := b.owns
	b.owns = nil
	b.mu.Unlock()
	for _, r := range owns {
		r.Close()
	}
}
