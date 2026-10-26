// Package relay is a tiny matchmaking and forwarding service for players
// behind NAT. A host registers its room code over /host; players connect to
// /join/<ROOM> and speak the normal game protocol, which the relay forwards
// to the host without reading it. All game logic stays on the host.
package relay

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/v1rushb/neoncoup/protocol"
	"github.com/v1rushb/neoncoup/transport"
)

// Options configure a relay.
type Options struct {
	Logger *slog.Logger
	// MaxRooms caps registered hosts (0 = 1000).
	MaxRooms int
	// MaxPlayers caps relayed connections per room (0 = 16).
	MaxPlayers int
	// FrameRate caps frames per second per player (0 = 20).
	FrameRate float64
	// ByteRate caps bytes per second per player (0 = 32 KiB).
	ByteRate float64
	// MaxRoomsPerIP caps rooms registered from one address (0 = 8).
	MaxRoomsPerIP int
	// Reserve keeps a dropped host's room code for its key (0 = 2 minutes).
	Reserve time.Duration
	// HelloWait is how long a player may stay silent after connecting (0 = 10s).
	HelloWait time.Duration
}

// Relay pairs hosts and players.
type Relay struct {
	opts Options
	log  *slog.Logger
	mu   sync.Mutex
	// rooms maps a room code to its host link.
	rooms map[string]*hostLink
	// reserved holds codes of recently dropped hosts for their keys.
	reserved map[string]reservation
	perIP    map[string]int
	start    time.Time
}

type reservation struct {
	key   string
	until time.Time
}

type hostLink struct {
	ws      *websocket.Conn
	room    string
	mu      sync.Mutex
	next    uint32
	players map[uint32]*player
	done    chan struct{}
}

// player is one relayed connection. Frames from the host go through its own
// queue and writer, so one slow player can never stall the room.
type player struct {
	ws   *websocket.Conn
	out  chan []byte
	once sync.Once
	gone chan struct{}
}

func newPlayer(ws *websocket.Conn) *player {
	p := &player{ws: ws, out: make(chan []byte, 256), gone: make(chan struct{})}
	go p.writeLoop()
	return p
}

func (p *player) writeLoop() {
	for {
		select {
		case <-p.gone:
			return
		case f := <-p.out:
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			err := p.ws.Write(ctx, websocket.MessageText, f)
			cancel()
			if err != nil {
				p.close(websocket.StatusGoingAway, "write failed")
				return
			}
		}
	}
}

// send queues a frame; a player too far behind is disconnected.
func (p *player) send(f []byte) {
	select {
	case p.out <- f:
	case <-p.gone:
	default:
		p.close(websocket.StatusPolicyViolation, "too slow")
	}
}

// close ends the connection without blocking the caller on the handshake.
func (p *player) close(code websocket.StatusCode, reason string) {
	p.once.Do(func() {
		close(p.gone)
		go func() { _ = p.ws.Close(code, reason) }()
	})
}

// New creates a relay.
func New(o Options) *Relay {
	if o.Logger == nil {
		o.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	if o.MaxRooms == 0 {
		o.MaxRooms = 1000
	}
	if o.MaxPlayers == 0 {
		o.MaxPlayers = 16
	}
	if o.FrameRate == 0 {
		o.FrameRate = 20
	}
	if o.ByteRate == 0 {
		o.ByteRate = 32 << 10
	}
	if o.MaxRoomsPerIP == 0 {
		o.MaxRoomsPerIP = 8
	}
	if o.Reserve == 0 {
		o.Reserve = 2 * time.Minute
	}
	if o.HelloWait == 0 {
		o.HelloWait = 10 * time.Second
	}
	return &Relay{opts: o, log: o.Logger, rooms: map[string]*hostLink{}, reserved: map[string]reservation{}, perIP: map[string]int{}, start: time.Now()}
}

// Handler serves /host, /join/<ROOM>, /healthz and a status page.
func (r *Relay) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/host", r.serveHost)
	mux.HandleFunc("/join/", r.serveJoin)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "ok\n") })
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		r.mu.Lock()
		n := len(r.rooms)
		r.mu.Unlock()
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = fmt.Fprintf(w, "NEON//COUP relay (protocol v%d)\nrooms: %d\nuptime: %s\n", protocol.Version, n, time.Since(r.start).Round(time.Second))
	})
	return mux
}

// Rooms reports how many hosts are registered.
func (r *Relay) Rooms() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.rooms)
}

func accept(w http.ResponseWriter, req *http.Request) (*websocket.Conn, error) {
	// Terminal clients send no Origin and pass the default same-origin check;
	// web pages from other sites do not.
	c, err := websocket.Accept(w, req, nil)
	if err != nil {
		return nil, err
	}
	c.SetReadLimit(protocol.MaxFrame + 16)
	return c, nil
}

func rejectText(ctx context.Context, c *websocket.Conn, code, reason string) {
	_ = c.Write(ctx, websocket.MessageText, protocol.MustEncode(protocol.TReject, protocol.Reject{Code: code, Reason: reason}))
	_ = c.Close(websocket.StatusPolicyViolation, reason)
}

func (r *Relay) serveHost(w http.ResponseWriter, req *http.Request) {
	ws, err := accept(w, req)
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(req.Context(), 10*time.Second)
	_, data, err := ws.Read(ctx)
	cancel()
	if err != nil {
		_ = ws.CloseNow()
		return
	}
	env, err := protocol.Decode(data)
	var reg transport.RelayRegister
	if err == nil && env.Type == "register" {
		err = env.Into(&reg)
	} else if err == nil {
		err = fmt.Errorf("expected register")
	}
	if err != nil {
		rejectText(req.Context(), ws, protocol.RejectBadHello, err.Error())
		return
	}
	if reg.Proto != protocol.Version {
		rejectText(req.Context(), ws, protocol.RejectVersion, fmt.Sprintf("relay speaks protocol v%d, host speaks v%d", protocol.Version, reg.Proto))
		return
	}
	room, ok := protocol.ParseRoomCode(reg.Room)
	if !ok {
		rejectText(req.Context(), ws, protocol.RejectBadHello, "bad room code")
		return
	}
	if len(reg.Key) < 16 {
		rejectText(req.Context(), ws, protocol.RejectBadHello, "missing room key")
		return
	}
	ip := clientIP(req)
	link := &hostLink{ws: ws, room: room, players: map[uint32]*player{}, done: make(chan struct{})}
	r.mu.Lock()
	res, held := r.reserved[room]
	if held && time.Now().After(res.until) {
		delete(r.reserved, room)
		held = false
	}
	switch {
	case r.rooms[room] != nil:
		r.mu.Unlock()
		rejectText(req.Context(), ws, protocol.RejectFull, "room code already registered")
		return
	case held && res.key != reg.Key:
		r.mu.Unlock()
		rejectText(req.Context(), ws, protocol.RejectFull, "room code is reserved for its host")
		return
	case len(r.rooms) >= r.opts.MaxRooms:
		r.mu.Unlock()
		rejectText(req.Context(), ws, protocol.RejectFull, "relay is full")
		return
	case r.perIP[ip] >= r.opts.MaxRoomsPerIP:
		r.mu.Unlock()
		rejectText(req.Context(), ws, protocol.RejectFull, "too many rooms from your address")
		return
	}
	delete(r.reserved, room)
	r.rooms[room] = link
	r.perIP[ip]++
	r.mu.Unlock()
	r.log.Info("host registered", "room", room, "remote", req.RemoteAddr)
	defer func() {
		r.mu.Lock()
		if r.rooms[room] == link {
			delete(r.rooms, room)
			r.reserved[room] = reservation{key: reg.Key, until: time.Now().Add(r.opts.Reserve)}
		}
		if r.perIP[ip]--; r.perIP[ip] <= 0 {
			delete(r.perIP, ip)
		}
		r.mu.Unlock()
		close(link.done)
		link.mu.Lock()
		for _, p := range link.players {
			p.close(websocket.StatusGoingAway, "host left")
		}
		link.mu.Unlock()
		r.log.Info("host gone", "room", room)
	}()
	if err := ws.Write(req.Context(), websocket.MessageText, protocol.MustEncode("registered", nil)); err != nil {
		return
	}
	stopPing := keepalive(ws)
	defer stopPing()
	for {
		typ, data, err := ws.Read(context.Background())
		if err != nil {
			return
		}
		if typ != websocket.MessageBinary {
			continue
		}
		op, id, payload, err := transport.DecodeMux(data)
		if err != nil {
			continue
		}
		link.mu.Lock()
		p := link.players[id]
		if op == transport.OpClose {
			delete(link.players, id)
		}
		link.mu.Unlock()
		if p == nil {
			continue
		}
		switch op {
		case transport.OpData:
			p.send(append([]byte(nil), payload...))
		case transport.OpClose:
			p.close(websocket.StatusNormalClosure, "")
		}
	}
}

func (r *Relay) serveJoin(w http.ResponseWriter, req *http.Request) {
	room, ok := protocol.ParseRoomCode(strings.TrimPrefix(req.URL.Path, "/join/"))
	ws, err := accept(w, req)
	if err != nil {
		return
	}
	if !ok {
		rejectText(req.Context(), ws, protocol.RejectNotFound, "bad room code")
		return
	}
	r.mu.Lock()
	link := r.rooms[room]
	r.mu.Unlock()
	if link == nil {
		rejectText(req.Context(), ws, protocol.RejectNotFound, fmt.Sprintf("no host has registered %s with this relay", room))
		return
	}
	link.mu.Lock()
	if len(link.players) >= r.opts.MaxPlayers {
		link.mu.Unlock()
		rejectText(req.Context(), ws, protocol.RejectFull, "room is full on this relay")
		return
	}
	link.next++
	id := link.next
	pl := newPlayer(ws)
	link.players[id] = pl
	link.mu.Unlock()

	// No write timeout on the shared host link: with coder/websocket a timeout
	// closes the whole connection, which would let one player cut off the
	// room. A dead link is caught by its keepalive instead.
	toHost := func(op byte, payload []byte) error {
		return link.ws.Write(context.Background(), websocket.MessageBinary, transport.EncodeMux(op, id, payload))
	}
	defer func() {
		link.mu.Lock()
		_, live := link.players[id]
		delete(link.players, id)
		link.mu.Unlock()
		if live {
			_ = toHost(transport.OpClose, nil)
		}
		pl.close(websocket.StatusNormalClosure, "")
	}()
	if err := toHost(transport.OpOpen, []byte(req.RemoteAddr)); err != nil {
		return
	}
	stopPing := keepalive(ws)
	defer stopPing()
	ws.SetReadLimit(maxPlayerFrame)
	frames := protocol.NewLimiter(r.opts.FrameRate, int(r.opts.FrameRate*2))
	bytes := 0.0
	last := time.Now()
	for first := true; ; first = false {
		ctx, cancel := context.Background(), context.CancelFunc(func() {})
		if first {
			// A connection that never says anything only holds a slot.
			ctx, cancel = context.WithTimeout(context.Background(), r.opts.HelloWait)
		}
		typ, data, err := ws.Read(ctx)
		cancel()
		if err != nil {
			return
		}
		now := time.Now()
		bytes = max(0, bytes-now.Sub(last).Seconds()*r.opts.ByteRate) + float64(len(data))
		last = now
		if typ != websocket.MessageText || !frames.Allow(now) || bytes > r.opts.ByteRate*4 {
			continue
		}
		select {
		case <-link.done:
			return
		case <-pl.gone:
			return
		default:
		}
		if err := toHost(transport.OpData, data); err != nil {
			return
		}
	}
}

// maxPlayerFrame bounds what a player may send through the relay; game
// frames from clients are tiny.
const maxPlayerFrame = 16 << 10

// clientIP is the remote address without its port.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// keepalive pings a connection until the returned stop function is called.
func keepalive(ws *websocket.Conn) func() {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		t := time.NewTicker(transport.KeepaliveInterval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				pctx, pcancel := context.WithTimeout(ctx, transport.KeepaliveInterval)
				err := ws.Ping(pctx)
				pcancel()
				if err != nil && ctx.Err() == nil {
					_ = ws.CloseNow()
					return
				}
			}
		}
	}()
	return cancel
}
