package relay

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/v1rushb/neoncoup/protocol"
	"github.com/v1rushb/neoncoup/transport"
)

func newRelay(t *testing.T) (*Relay, string) {
	r := New(Options{MaxPlayers: 2})
	s := httptest.NewServer(r.Handler())
	t.Cleanup(s.Close)
	return r, s.URL
}

func TestHostRegistersAndPlayersAreForwarded(t *testing.T) {
	r, url := newRelay(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	h, err := transport.DialRelayHost(ctx, url, "NEON-AB23", "host-key-0123456789")
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	if r.Rooms() != 1 {
		t.Fatal("not registered")
	}
	// Duplicate registrations are refused.
	if _, err := transport.DialRelayHost(ctx, url, "neon-ab23", "someone-else-key-0123"); err == nil || !strings.Contains(err.Error(), "already") {
		t.Fatalf("duplicate: %v", err)
	}

	p, err := transport.Dial(ctx, url+"/join/NEON-AB23")
	if err != nil {
		t.Fatal(err)
	}
	hc, err := h.Accept(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// Player -> host.
	if err := p.Send(ctx, []byte(`{"t":"ping","d":{"t":1}}`)); err != nil {
		t.Fatal(err)
	}
	f, err := hc.Recv(ctx)
	if err != nil || string(f) != `{"t":"ping","d":{"t":1}}` {
		t.Fatalf("%s %v", f, err)
	}
	// Host -> player.
	if err := hc.Send(ctx, []byte(`{"t":"pong","d":{"t":1}}`)); err != nil {
		t.Fatal(err)
	}
	f, err = p.Recv(ctx)
	if err != nil || string(f) != `{"t":"pong","d":{"t":1}}` {
		t.Fatalf("%s %v", f, err)
	}
	// Host closes the stream: the player is disconnected.
	_ = hc.Close()
	if _, err := p.Recv(ctx); err == nil {
		t.Fatal("player still connected after the host closed the stream")
	}
	// Player leaves: the host stream ends.
	p2, _ := transport.Dial(ctx, url+"/join/NEON-AB23")
	hc2, _ := h.Accept(ctx)
	_ = p2.Close()
	if _, err := hc2.Recv(ctx); err == nil {
		t.Fatal("host stream alive after the player left")
	}
}

func TestJoinUnknownRoomIsRejected(t *testing.T) {
	_, url := newRelay(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	p, err := transport.Dial(ctx, url+"/join/NEON-ZZZZ")
	if err != nil {
		t.Fatal(err)
	}
	f, err := p.Recv(ctx)
	if err != nil {
		t.Fatal(err)
	}
	env, _ := protocol.Decode(f)
	var rej protocol.Reject
	_ = env.Into(&rej)
	if env.Type != protocol.TReject || rej.Code != protocol.RejectNotFound {
		t.Fatalf("%s %+v", env.Type, rej)
	}
	_ = p.Close()
}

func TestRoomCapacityAndHostLeaving(t *testing.T) {
	r, url := newRelay(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	h, err := transport.DialRelayHost(ctx, url, "NEON-CD45", "host-key-0123456789")
	if err != nil {
		t.Fatal(err)
	}
	var players []transport.Conn
	for range 2 {
		p, err := transport.Dial(ctx, url+"/join/NEON-CD45")
		if err != nil {
			t.Fatal(err)
		}
		players = append(players, p)
		if _, err := h.Accept(ctx); err != nil {
			t.Fatal(err)
		}
	}
	extra, _ := transport.Dial(ctx, url+"/join/NEON-CD45")
	f, _ := extra.Recv(ctx)
	if env, _ := protocol.Decode(f); env.Type != protocol.TReject {
		t.Fatal("over-capacity player accepted")
	}
	_ = extra.Close()
	h.Close()
	for _, p := range players {
		if _, err := p.Recv(ctx); err == nil {
			t.Fatal("player kept after host left")
		}
	}
	deadline := time.Now().Add(2 * time.Second)
	for r.Rooms() != 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if r.Rooms() != 0 {
		t.Fatal("room not released")
	}
}

func TestBadRegistrations(t *testing.T) {
	_, url := newRelay(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := transport.DialRelayHost(ctx, url, "not-a-code", "host-key-0123456789"); err == nil {
		t.Fatal("bad code accepted")
	}
}

func TestMuxFraming(t *testing.T) {
	b := transport.EncodeMux(transport.OpData, 0xdeadbeef, []byte("hi"))
	op, id, p, err := transport.DecodeMux(b)
	if err != nil || op != transport.OpData || id != 0xdeadbeef || string(p) != "hi" {
		t.Fatal(op, id, p, err)
	}
	if _, _, _, err := transport.DecodeMux([]byte{1, 2}); err == nil {
		t.Fatal("short frame accepted")
	}
}

func TestDroppedHostsCodeIsReservedForItsKey(t *testing.T) {
	r := New(Options{Reserve: time.Minute})
	s := httptest.NewServer(r.Handler())
	t.Cleanup(s.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	h, err := transport.DialRelayHost(ctx, s.URL, "NEON-EF67", "owner-key-0123456789")
	if err != nil {
		t.Fatal(err)
	}
	h.Close()
	deadline := time.Now().Add(2 * time.Second)
	for r.Rooms() != 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := transport.DialRelayHost(ctx, s.URL, "NEON-EF67", "squatter-key-0123456"); err == nil || !strings.Contains(err.Error(), "reserved") {
		t.Fatalf("squatter took a reserved code: %v", err)
	}
	h2, err := transport.DialRelayHost(ctx, s.URL, "NEON-EF67", "owner-key-0123456789")
	if err != nil {
		t.Fatalf("owner could not re-register: %v", err)
	}
	h2.Close()
	if _, err := transport.DialRelayHost(ctx, s.URL, "NEON-GH89", "short"); err == nil {
		t.Fatal("registration without a proper key accepted")
	}
}

func TestSilentJoinersAreDropped(t *testing.T) {
	r := New(Options{HelloWait: 200 * time.Millisecond})
	s := httptest.NewServer(r.Handler())
	t.Cleanup(s.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	h, err := transport.DialRelayHost(ctx, s.URL, "NEON-JK23", "owner-key-0123456789")
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	p, _ := transport.Dial(ctx, s.URL+"/join/NEON-JK23")
	hc, _ := h.Accept(ctx)
	start := time.Now()
	if _, err := hc.Recv(ctx); err == nil {
		t.Fatal("expected the silent player's stream to end")
	}
	if time.Since(start) > 2*time.Second {
		t.Fatal("silent player held the slot too long")
	}
	_ = p.Close()
}
