package client

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/v1rushb/neoncoup/protocol"
	"github.com/v1rushb/neoncoup/transport"
)

// Dialer reopens a connection to the same host, for token rejoins.
type Dialer func(ctx context.Context) (transport.Conn, error)

// Session is the client's link to a host. Frames arrive as frameMsg values
// through Wait; sends never block the UI.
type Session struct {
	hello  protocol.Hello
	dial   Dialer
	ctx    context.Context
	cancel context.CancelFunc

	frames chan protocol.Envelope
	out    chan []byte
	events chan tea.Msg

	mu       sync.Mutex
	conn     transport.Conn
	token    string
	gen      int
	welcomed bool
	rejoin   time.Duration // how long to keep trying to reconnect
}

// Session messages delivered to the UI.
type (
	frameMsg struct {
		s   *Session
		env protocol.Envelope
	}
	sessionDownMsg struct {
		s   *Session
		err error
	}
	sessionRetryMsg struct {
		s       *Session
		attempt int
	}
	sessionBackMsg struct{ s *Session }
)

// NewSession sends hello on conn and starts pumping frames. dial may be nil,
// in which case a dropped connection ends the session.
func NewSession(conn transport.Conn, hello protocol.Hello, dial Dialer) *Session {
	ctx, cancel := context.WithCancel(context.Background())
	hello.Proto = protocol.Version
	if hello.Client == "" {
		hello.Client = "neoncoup"
	}
	s := &Session{
		hello:  hello,
		dial:   dial,
		ctx:    ctx,
		cancel: cancel,
		frames: make(chan protocol.Envelope, 1024),
		out:    make(chan []byte, 256),
		events: make(chan tea.Msg, 16),
		conn:   conn,
		token:  hello.Token,
		rejoin: 60 * time.Second,
	}
	go s.writeLoop()
	s.start(conn)
	return s
}

func (s *Session) start(conn transport.Conn) {
	s.mu.Lock()
	h := s.hello
	h.Token = s.token
	s.mu.Unlock()
	// Hello goes out first, before the new connection is published to the
	// writer, so nothing queued can beat it onto the wire.
	sctx, cancel := context.WithTimeout(s.ctx, 5*time.Second)
	_ = conn.Send(sctx, protocol.MustEncode(protocol.THello, h))
	cancel()
	s.mu.Lock()
	s.conn = conn
	s.gen++
	gen := s.gen
	s.mu.Unlock()
	go s.readLoop(conn, gen)
}

// SetRejoin sets how long a dropped session keeps trying to reconnect. The
// app matches it to the table's seat hold.
func (s *Session) SetRejoin(d time.Duration) {
	s.mu.Lock()
	s.rejoin = d
	s.mu.Unlock()
}

func (s *Session) readLoop(conn transport.Conn, gen int) {
	for {
		f, err := conn.Recv(s.ctx)
		if err != nil {
			s.lost(gen, err)
			return
		}
		env, err := protocol.Decode(f)
		if err != nil {
			continue
		}
		if env.Type == protocol.TWelcome {
			var w protocol.Welcome
			if env.Into(&w) == nil {
				s.mu.Lock()
				s.token = w.Token
				s.welcomed = true
				s.mu.Unlock()
			}
		}
		if env.Type == protocol.TReject {
			var rej protocol.Reject
			_ = env.Into(&rej)
			s.mu.Lock()
			retry := s.welcomed && s.dial != nil && (rej.Code == protocol.RejectNotFound || rej.Code == protocol.RejectClosed)
			if !retry {
				// Version mismatches, full rooms and bad hellos are final.
				s.dial = nil
			}
			s.mu.Unlock()
			if retry {
				// Mid-rejoin, "no such room" usually means the host is still
				// re-registering with its relay: keep trying.
				_ = conn.Close()
				s.lost(gen, fmt.Errorf("host unavailable: %s", rej.Reason))
				return
			}
		}
		select {
		case s.frames <- env:
		case <-s.ctx.Done():
			return
		}
	}
}

func (s *Session) lost(gen int, err error) {
	s.mu.Lock()
	if gen != s.gen {
		s.mu.Unlock()
		return
	}
	dial := s.dial
	s.mu.Unlock()
	if s.ctx.Err() != nil {
		return
	}
	if dial == nil {
		s.push(sessionDownMsg{s: s, err: err})
		return
	}
	go s.reconnect(dial, err)
}

// reconnect retries with backoff until the rejoin window runs out.
func (s *Session) reconnect(dial Dialer, cause error) {
	s.mu.Lock()
	deadline := time.Now().Add(s.rejoin)
	s.mu.Unlock()
	wait := 500 * time.Millisecond
	for attempt := 1; time.Now().Before(deadline); attempt++ {
		s.push(sessionRetryMsg{s: s, attempt: attempt})
		ctx, cancel := context.WithTimeout(s.ctx, 5*time.Second)
		conn, err := dial(ctx)
		cancel()
		if err == nil {
			s.start(conn)
			s.push(sessionBackMsg{s: s})
			return
		}
		if s.ctx.Err() != nil {
			return
		}
		select {
		case <-time.After(wait):
		case <-s.ctx.Done():
			return
		}
		wait = min(wait*2, 5*time.Second)
	}
	s.push(sessionDownMsg{s: s, err: cause})
}

func (s *Session) push(m tea.Msg) {
	select {
	case s.events <- m:
	case <-s.ctx.Done():
	}
}

func (s *Session) writeLoop() {
	for {
		select {
		case <-s.ctx.Done():
			return
		case f := <-s.out:
			s.mu.Lock()
			conn := s.conn
			s.mu.Unlock()
			ctx, cancel := context.WithTimeout(s.ctx, 5*time.Second)
			_ = conn.Send(ctx, f)
			cancel()
		}
	}
}

// Send queues a frame. It drops the frame if the queue is full, which only
// happens when the connection is gone.
func (s *Session) Send(typ string, payload any) {
	f, err := protocol.Encode(typ, payload)
	if err != nil {
		return
	}
	select {
	case s.out <- f:
	default:
	}
}

// Wait returns a command that delivers the next frame or session event.
func (s *Session) Wait() tea.Cmd {
	return func() tea.Msg {
		select {
		case env := <-s.frames:
			return frameMsg{s: s, env: env}
		case m := <-s.events:
			return m
		case <-s.ctx.Done():
			return sessionDownMsg{s: s, err: errSessionClosed}
		}
	}
}

// Token is the current rejoin token.
func (s *Session) Token() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.token
}

// Close ends the session.
func (s *Session) Close() {
	s.mu.Lock()
	conn := s.conn
	s.mu.Unlock()
	f, _ := protocol.Encode(protocol.TLeave, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	_ = conn.Send(ctx, f)
	cancel()
	s.cancel()
	_ = conn.Close()
}

var errSessionClosed = errors.New("session closed")
