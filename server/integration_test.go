package server_test

import (
	"context"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/v1rushb/neoncoup/bot"
	"github.com/v1rushb/neoncoup/client/headless"
	"github.com/v1rushb/neoncoup/engine"
	"github.com/v1rushb/neoncoup/protocol"
	"github.com/v1rushb/neoncoup/server"
	"github.com/v1rushb/neoncoup/transport"
)

// TestNetworkGameWithThreeHeadlessClients runs a real WebSocket server and
// three headless clients that play a complete game against each other.
func TestNetworkGameWithThreeHeadlessClients(t *testing.T) {
	srv := server.New(server.Options{Seed: 99})
	defer srv.Close()
	settings := protocol.DefaultSettings()
	settings.Speed = protocol.SpeedInstant
	settings.FillBots = false
	settings.MaxPlayers = 3
	settings.TurnMS = 0
	room, err := srv.CreateRoom("", settings)
	if err != nil {
		t.Fatal(err)
	}
	hs := httptest.NewServer(srv.Handler())
	defer hs.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	levels := []bot.Level{bot.Hard, bot.Normal, bot.Easy}
	results := make([]headless.Result, 3)
	errs := make([]error, 3)
	var wg sync.WaitGroup
	for i := range 3 {
		conn, err := transport.Dial(ctx, hs.URL)
		if err != nil {
			t.Fatal(err)
		}
		opts := headless.Options{Name: []string{"ALPHA", "BRAVO", "CHARLIE"}[i], Room: room.Code(), Level: levels[i], Seed: uint64(i + 1)}
		if i == 0 {
			opts.StartWhen = 3
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i], errs[i] = headless.Play(ctx, conn, opts)
		}()
		// Join in order so ALPHA is the host.
		time.Sleep(50 * time.Millisecond)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("client %d: %v", i, err)
		}
	}
	w := results[0].Winner
	seats := map[int]bool{}
	for i, r := range results {
		if r.Winner != w {
			t.Fatalf("client %d saw winner %d, client 0 saw %d", i, r.Winner, w)
		}
		if r.Seat < 0 || seats[r.Seat] {
			t.Fatalf("bad seat assignment %+v", results)
		}
		seats[r.Seat] = true
	}
	wins := 0
	for _, r := range results {
		if r.Won() {
			wins++
		}
	}
	if wins != 1 {
		t.Fatalf("%d winners", wins)
	}

	// Every client got the same replay, and it reproduces the game.
	rep := results[1].Replay
	g, err := engine.New(rep.Config)
	if err != nil {
		t.Fatal(err)
	}
	for _, st := range rep.Steps {
		if _, err := g.Apply(st.Seat, st.Intent); err != nil {
			t.Fatalf("replay diverged: %v", err)
		}
		if err := g.CheckInvariants(); err != nil {
			t.Fatal(err)
		}
	}
	if g.Winner() != w {
		t.Fatalf("replay winner %d, want %d", g.Winner(), w)
	}
	t.Logf("winner %s after %d intents (%d turns)", results[0].Names[w], len(rep.Steps), g.Turn())
}

func TestWebSocketVersionMismatchIsRejected(t *testing.T) {
	srv := server.New(server.Options{})
	defer srv.Close()
	room, _ := srv.CreateRoom("", instantSettings())
	hs := httptest.NewServer(srv.Handler())
	defer hs.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := transport.Dial(ctx, hs.URL)
	if err != nil {
		t.Fatal(err)
	}
	_ = conn.Send(ctx, protocol.MustEncode(protocol.THello, protocol.Hello{Proto: 0, Name: "old", Room: room.Code()}))
	f, err := conn.Recv(ctx)
	if err != nil {
		t.Fatal(err)
	}
	env, _ := protocol.Decode(f)
	var rej protocol.Reject
	_ = env.Into(&rej)
	if env.Type != protocol.TReject || rej.Code != protocol.RejectVersion {
		t.Fatalf("%s %+v", env.Type, rej)
	}
}

// instantSettings are room settings with presentation pacing off.
func instantSettings() protocol.Settings {
	st := protocol.DefaultSettings()
	st.Speed = protocol.SpeedInstant
	return st
}
