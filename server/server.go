// Package server is the authoritative NEON//COUP host. Clients send intents;
// rooms validate them against the engine and send every member a state
// redacted for their seat. Each room is an actor: one goroutine owns all of
// its state, and everything else talks to it through its inbox.
package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/v1rushb/neoncoup/protocol"
	"github.com/v1rushb/neoncoup/transport"
)

// Options configure a Server.
type Options struct {
	Logger   *slog.Logger
	Defaults protocol.Settings
	// ThinkMin and ThinkMax bound the bots' human-like delay. Both zero
	// means bots act as soon as they are asked.
	ThinkMin, ThinkMax time.Duration
	// MaxRooms caps concurrent rooms (0 = 64).
	MaxRooms int
	// DefaultRoom receives hellos that name no room.
	DefaultRoom string
	// Seed makes seat order, engine seeds and bots deterministic (tests). 0 = random.
	Seed uint64
	// MaxSpectators per room (0 = 10).
	MaxSpectators int
	// HelloTimeout bounds how long a connection may take to say hello (0 = 10s).
	HelloTimeout time.Duration
	// IdleClose closes a room once nobody has been connected to it for this
	// long (0 = rooms stay open until the server closes). Shared hubs such
	// as the SSH server set it.
	IdleClose time.Duration
}

// Server owns rooms and accepts connections.
type Server struct {
	opts   Options
	log    *slog.Logger
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	conns  sync.WaitGroup // live ServeConn calls, so Close can let farewells flush

	mu    sync.Mutex
	rooms map[string]*Room
	rng   *rand.Rand
}

// New creates a server.
func New(opts Options) *Server {
	if opts.Logger == nil {
		opts.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	if opts.Defaults.MaxPlayers == 0 {
		opts.Defaults = protocol.DefaultSettings()
	}
	if opts.MaxRooms == 0 {
		opts.MaxRooms = 64
	}
	if opts.MaxSpectators == 0 {
		opts.MaxSpectators = 10
	}
	if opts.HelloTimeout == 0 {
		opts.HelloTimeout = 10 * time.Second
	}
	seed := opts.Seed
	if seed == 0 {
		seed = rand.Uint64()
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Server{
		opts:   opts,
		log:    opts.Logger,
		ctx:    ctx,
		cancel: cancel,
		rooms:  map[string]*Room{},
		rng:    rand.New(rand.NewPCG(seed, 0xdecafbad)),
	}
}

// ErrRoomExists is returned when a room code is taken.
var ErrRoomExists = errors.New("room code already in use")

// CreateRoom opens a room. An empty code picks a fresh one.
func (s *Server) CreateRoom(code string, settings protocol.Settings) (*Room, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.rooms) >= s.opts.MaxRooms {
		return nil, errors.New("too many rooms")
	}
	if code == "" {
		for range 100 {
			code = protocol.NewRoomCode()
			if s.rooms[code] == nil {
				break
			}
		}
	}
	if s.rooms[code] != nil {
		return nil, ErrRoomExists
	}
	if settings.MaxPlayers == 0 {
		settings = s.opts.Defaults
	}
	r := newRoom(s, code, settings.Clamp(), s.rng.Uint64())
	s.rooms[code] = r
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		r.run()
		s.mu.Lock()
		if s.rooms[code] == r {
			delete(s.rooms, code)
		}
		s.mu.Unlock()
	}()
	s.log.Info("room created", "room", code)
	return r, nil
}

// Room returns an open room by code.
func (s *Server) Room(code string) *Room {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.rooms[code]
}

// Rooms lists open room codes.
func (s *Server) Rooms() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, 0, len(s.rooms))
	for c := range s.rooms {
		out = append(out, c)
	}
	sort.Strings(out)
	return out
}

// Close shuts every room, tells connected players why, and waits (briefly)
// for that to reach them.
func (s *Server) Close() {
	s.cancel()
	s.wg.Wait()
	done := make(chan struct{})
	go func() { s.conns.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
	}
}

// ServeConn speaks the protocol on one connection until it closes. It is
// safe to call from many goroutines.
func (s *Server) ServeConn(conn transport.Conn) {
	s.conns.Add(1)
	defer s.conns.Done()
	defer func() { _ = conn.Close() }()
	ctx, cancel := context.WithTimeout(s.ctx, s.opts.HelloTimeout)
	frame, err := conn.Recv(ctx)
	cancel()
	if err != nil {
		return
	}
	reject := func(code, reason string) {
		sctx, scancel := context.WithTimeout(s.ctx, 2*time.Second)
		_ = conn.Send(sctx, protocol.MustEncode(protocol.TReject, protocol.Reject{Code: code, Reason: reason}))
		scancel()
	}
	env, err := protocol.Decode(frame)
	var hello protocol.Hello
	if err == nil && env.Type == protocol.THello {
		err = env.Into(&hello)
	} else if err == nil {
		err = fmt.Errorf("expected hello, got %q", env.Type)
	}
	if err != nil {
		reject(protocol.RejectBadHello, err.Error())
		return
	}
	if hello.Proto != protocol.Version {
		reject(protocol.RejectVersion, fmt.Sprintf(
			"protocol mismatch: host speaks v%d, your client speaks v%d. Update NEON//COUP so both sides match.",
			protocol.Version, hello.Proto))
		return
	}
	code := s.opts.DefaultRoom
	if hello.Room != "" {
		if c, ok := protocol.ParseRoomCode(hello.Room); ok {
			code = c
		} else {
			code = hello.Room
		}
	}
	room := s.Room(code)
	if room == nil {
		reject(protocol.RejectNotFound, fmt.Sprintf("no room %q on this host", code))
		return
	}

	cl := newClient(conn)
	go cl.writeLoop()
	defer func() {
		if s.ctx.Err() != nil {
			// The host is shutting down: let the room queue its farewell,
			// then give it a moment to go out before hanging up.
			select {
			case <-room.Done():
			case <-time.After(time.Second):
			}
			cl.flush(time.Second)
		}
		cl.close()
	}()

	reply := make(chan string, 1)
	if !room.post(joinMsg{cl: cl, hello: hello, reply: reply}) {
		reject(protocol.RejectClosed, "room closed")
		return
	}
	select {
	case rej := <-reply:
		if rej != "" {
			// The room already queued a Reject frame; let it flush.
			cl.flush(2 * time.Second)
			return
		}
	case <-room.Done():
		// The room closed with our join still queued.
		cl.send(protocol.MustEncode(protocol.TReject, protocol.Reject{Code: protocol.RejectClosed, Reason: "room closed"}))
		cl.flush(time.Second)
		return
	case <-s.ctx.Done():
		return
	}

	for {
		frame, err := conn.Recv(s.ctx)
		if err != nil {
			room.post(dropMsg{cl: cl})
			return
		}
		now := time.Now()
		strike := func(code, msg string) bool {
			cl.strikes++
			cl.send(protocol.MustEncode(protocol.TError, protocol.Error{Code: code, Msg: msg}))
			if cl.strikes > 100 {
				room.post(dropMsg{cl: cl})
				return false
			}
			return true
		}
		// Every frame, well-formed or not, spends from the overall budget
		// before we pay to parse it.
		if !cl.frameLimiter.Allow(now) {
			if !strike(protocol.ErrRateLimit, "slow down") {
				return
			}
			continue
		}
		env, err := protocol.Decode(frame)
		if err != nil {
			if !strike(protocol.ErrBadMsg, err.Error()) {
				return
			}
			continue
		}
		// Intents are paced by the game itself (one per decision), so only
		// the other messages get the tighter budget.
		if env.Type != protocol.TIntent && !cl.limiter.Allow(now) {
			if !strike(protocol.ErrRateLimit, "slow down") {
				return
			}
			continue
		}
		if env.Type == protocol.TChat || env.Type == protocol.TEmote {
			if !cl.chatLimiter.Allow(now) {
				cl.send(protocol.MustEncode(protocol.TError, protocol.Error{Code: protocol.ErrRateLimit, Msg: "chat is cooling down"}))
				continue
			}
		}
		if env.Type == protocol.TLeave {
			room.post(dropMsg{cl: cl, leave: true})
			return
		}
		if !room.post(frameMsg{cl: cl, env: env}) {
			return
		}
	}
}

// Handler serves the WebSocket endpoint at /ws plus a small status page.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		conn, err := transport.Accept(w, r)
		if err != nil {
			s.log.Warn("websocket upgrade failed", "remote", r.RemoteAddr, "err", err)
			return
		}
		s.ServeConn(conn)
	})
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "ok\n")
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		// Room codes are how people get in, so the status page only counts them.
		_, _ = fmt.Fprintf(w, "NEON//COUP host (protocol v%d)\nrooms: %d\njoin with: neoncoup join <room-code>\n",
			protocol.Version, len(s.Rooms()))
	})
	return mux
}

// client is one live connection.
type client struct {
	conn         transport.Conn
	out          chan []byte
	done         chan struct{}
	once         sync.Once
	frameLimiter *protocol.Limiter // everything
	limiter      *protocol.Limiter // everything but intents
	chatLimiter  *protocol.Limiter
	strikes      int
}

func newClient(conn transport.Conn) *client {
	return &client{
		conn: conn,
		// A whole game is a few hundred frames; a client that falls this far
		// behind is gone, not just slow.
		out:          make(chan []byte, 4096),
		done:         make(chan struct{}),
		frameLimiter: protocol.NewLimiter(60, 120),
		limiter:      protocol.NewLimiter(20, 40),
		chatLimiter:  protocol.NewLimiter(1.5, 5),
	}
}

// send queues a frame without blocking. A client that cannot keep up is
// disconnected rather than allowed to stall the room.
func (c *client) send(frame []byte) {
	select {
	case <-c.done:
		return
	default:
	}
	select {
	case c.out <- frame:
	default:
		c.close()
	}
}

func (c *client) close() {
	c.once.Do(func() {
		close(c.done)
		_ = c.conn.Close()
	})
}

// flush waits until queued frames are written or the timeout passes.
func (c *client) flush(timeout time.Duration) {
	deadline := time.After(timeout)
	for {
		if len(c.out) == 0 {
			// Give the writer a moment to finish the frame it holds.
			time.Sleep(20 * time.Millisecond)
			return
		}
		select {
		case <-deadline:
			return
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// writeLoop sends queued frames until the client closes. It outlives server
// shutdown on purpose, so farewell frames still go out.
func (c *client) writeLoop() {
	for {
		select {
		case <-c.done:
			return
		case f := <-c.out:
			sctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			err := c.conn.Send(sctx, f)
			cancel()
			if err != nil {
				c.close()
				return
			}
		}
	}
}
