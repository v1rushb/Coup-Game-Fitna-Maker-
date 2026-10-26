package client

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/v1rushb/neoncoup/bot"
	"github.com/v1rushb/neoncoup/client/headless"
	"github.com/v1rushb/neoncoup/protocol"
	"github.com/v1rushb/neoncoup/relay"
	"github.com/v1rushb/neoncoup/transport"
)

func hostSettings() protocol.Settings {
	st := protocol.DefaultSettings()
	st.MaxPlayers = 2
	st.FillBots = false
	st.TurnMS = 0
	return st
}

// playPair runs the host's local seat and one remote seat to the end.
func playPair(t *testing.T, h *Hosted, remote transport.Conn) {
	t.Helper()
	// Bots in these tests act immediately.
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	a, b := transport.Pipe()
	go h.Server.ServeConn(b)
	var wg sync.WaitGroup
	var errHost, errRemote error
	var rHost, rRemote headless.Result
	wg.Add(2)
	go func() {
		defer wg.Done()
		rHost, errHost = headless.Play(ctx, a, headless.Options{Name: "HOST", Level: bot.Normal, StartWhen: 2})
	}()
	time.Sleep(50 * time.Millisecond)
	go func() {
		defer wg.Done()
		rRemote, errRemote = headless.Play(ctx, remote, headless.Options{Name: "GUEST", Level: bot.Normal, Seed: 2})
	}()
	wg.Wait()
	if errHost != nil || errRemote != nil {
		t.Fatalf("host: %v, remote: %v", errHost, errRemote)
	}
	if rHost.Winner != rRemote.Winner || rHost.Winner < 0 {
		t.Fatalf("winners disagree: %d vs %d", rHost.Winner, rRemote.Winner)
	}
}

func TestJoinByRoomCodeOnTheLAN(t *testing.T) {
	h, err := OpenRoom(HostOptions{Settings: hostSettings(), Port: 0}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	b := &LocalBackend{}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := b.Join(ctx, strings.ToLower(h.Room))
	if err != nil {
		if strings.Contains(err.Error(), "not on your LAN") {
			t.Skip("no multicast route in this environment")
		}
		t.Fatal(err)
	}
	if !strings.HasPrefix(conn.Via, "LAN ") || conn.Dial == nil {
		t.Fatalf("connected via %q", conn.Via)
	}
	playPair(t, h, conn.Conn)
}

func TestJoinDirectByAddress(t *testing.T) {
	h, err := OpenRoom(HostOptions{Settings: hostSettings(), Port: 0}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	b := &LocalBackend{}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := b.Join(ctx, fmt.Sprintf("127.0.0.1:%d", h.Port))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(conn.Via, "direct") {
		t.Fatalf("via %q", conn.Via)
	}
	playPair(t, h, conn.Conn)
}

func TestJoinThroughTheRelay(t *testing.T) {
	rl := relay.New(relay.Options{})
	rs := httptest.NewServer(rl.Handler())
	defer rs.Close()
	h, err := OpenRoom(HostOptions{Settings: hostSettings(), Port: 0, Relay: rs.URL}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	if !strings.Contains(strings.Join(h.Info.Lines, "\n"), "--relay "+rs.URL) {
		t.Fatalf("join instructions lack the relay: %v", h.Info.Lines)
	}
	// Go straight to the relay, as a player behind NAT would.
	u, _ := RelayJoinURL(rs.URL, h.Room)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := transport.Dial(ctx, u)
	if err != nil {
		t.Fatal(err)
	}
	playPair(t, h, conn)
	if rl.Rooms() != 1 {
		t.Fatalf("relay rooms %d", rl.Rooms())
	}
}

func TestJoinFallsBackToRelay(t *testing.T) {
	rl := relay.New(relay.Options{})
	rs := httptest.NewServer(rl.Handler())
	defer rs.Close()
	h, err := OpenRoom(HostOptions{Settings: hostSettings(), Port: 0, Relay: rs.URL}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	// A code nobody on the LAN serves, but the relay knows: unknown rooms
	// come back as a reject frame from the relay.
	b := &LocalBackend{Relay: rs.URL, LANTimeout: 200 * time.Millisecond}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := b.Join(ctx, "NEON-ZZZZ")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(conn.Via, "relay") {
		t.Fatalf("via %q", conn.Via)
	}
	_ = conn.Conn.Send(ctx, protocol.MustEncode(protocol.THello, protocol.Hello{Proto: protocol.Version, Name: "x", Room: "NEON-ZZZZ"}))
	f, err := conn.Conn.Recv(ctx)
	if err != nil {
		t.Fatal(err)
	}
	env, _ := protocol.Decode(f)
	if env.Type != protocol.TReject {
		t.Fatalf("got %s", env.Type)
	}
}

func TestJoinUnknownCodeWithoutRelayExplains(t *testing.T) {
	b := &LocalBackend{LANTimeout: 200 * time.Millisecond}
	_, err := b.Join(context.Background(), "NEON-ZZZZ")
	if err == nil || !strings.Contains(err.Error(), "no relay is configured") {
		t.Fatalf("err %v", err)
	}
}

func TestHostPortInUse(t *testing.T) {
	h, err := OpenRoom(HostOptions{Settings: hostSettings(), Port: 0}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	if _, err := OpenRoom(HostOptions{Settings: hostSettings(), Port: h.Port}, nil); err == nil || !strings.Contains(err.Error(), "--port") {
		t.Fatalf("err %v", err)
	}
}

func TestSessionReconnectsWithItsToken(t *testing.T) {
	h, err := OpenRoom(HostOptions{Settings: hostSettings(), Port: 0}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	addr := fmt.Sprintf("127.0.0.1:%d", h.Port)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, err := transport.Dial(ctx, addr)
	if err != nil {
		t.Fatal(err)
	}
	sess := NewSession(conn, protocol.Hello{Name: "FLAKY"}, dialer(addr))
	defer sess.Close()
	next := func() any {
		ch := make(chan any, 1)
		go func() { ch <- sess.Wait()() }()
		select {
		case m := <-ch:
			return m
		case <-ctx.Done():
			t.Fatal("timed out waiting for the session")
			return nil
		}
	}
	welcome := func() protocol.Welcome {
		for {
			if f, ok := next().(frameMsg); ok && f.env.Type == protocol.TWelcome {
				var w protocol.Welcome
				_ = f.env.Into(&w)
				return w
			}
		}
	}
	first := welcome()
	// Yank the cable.
	_ = conn.Close()
	sawRetry, sawBack := false, false
	for !sawBack {
		switch next().(type) {
		case sessionRetryMsg:
			sawRetry = true
		case sessionBackMsg:
			sawBack = true
		case sessionDownMsg:
			t.Fatal("session gave up")
		}
	}
	second := welcome()
	if !sawRetry || second.You != first.You || second.Token != first.Token {
		t.Fatalf("rejoined as %+v, first was %+v", second, first)
	}
}

// blipProxy forwards TCP to a target and can sever every connection at once.
type blipProxy struct {
	ln    net.Listener
	mu    sync.Mutex
	conns []net.Conn
}

func newBlipProxy(t *testing.T, target string) *blipProxy {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	p := &blipProxy{ln: ln}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			up, err := net.Dial("tcp", target)
			if err != nil {
				_ = c.Close()
				continue
			}
			p.mu.Lock()
			p.conns = append(p.conns, c, up)
			p.mu.Unlock()
			go func() { _, _ = io.Copy(up, c); _ = up.Close() }()
			go func() { _, _ = io.Copy(c, up); _ = c.Close() }()
		}
	}()
	t.Cleanup(func() { _ = ln.Close(); p.sever() })
	return p
}

func (p *blipProxy) url() string { return "ws://" + p.ln.Addr().String() }

func (p *blipProxy) sever() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, c := range p.conns {
		_ = c.Close()
	}
	p.conns = nil
}

func TestRelayedPlayerSurvivesARelayBlip(t *testing.T) {
	rl := relay.New(relay.Options{})
	rs := httptest.NewServer(rl.Handler())
	defer rs.Close()
	px := newBlipProxy(t, strings.TrimPrefix(rs.URL, "http://"))
	h, err := OpenRoom(HostOptions{Settings: hostSettings(), Port: 0, Relay: px.url()}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	u, _ := RelayJoinURL(px.url(), h.Room)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	conn, err := transport.Dial(ctx, u)
	if err != nil {
		t.Fatal(err)
	}
	sess := NewSession(conn, protocol.Hello{Name: "ROAMER", Room: h.Room}, dialer(u))
	defer sess.Close()
	next := func() any {
		ch := make(chan any, 1)
		go func() { ch <- sess.Wait()() }()
		select {
		case m := <-ch:
			return m
		case <-ctx.Done():
			t.Fatal("timed out")
			return nil
		}
	}
	welcome := func() protocol.Welcome {
		for {
			switch m := next().(type) {
			case frameMsg:
				if m.env.Type == protocol.TWelcome {
					var w protocol.Welcome
					_ = m.env.Into(&w)
					return w
				}
			case sessionDownMsg:
				t.Fatalf("session gave up: %v", m.err)
			}
		}
	}
	first := welcome()
	// Cut every connection through the relay: host link and player alike.
	// The player's first retries land before the host has re-registered.
	px.sever()
	second := welcome()
	if second.You != first.You {
		t.Fatalf("came back as %s, was %s", second.You, first.You)
	}
}
