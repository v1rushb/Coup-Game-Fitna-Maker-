package server

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/v1rushb/neoncoup/engine"
	"github.com/v1rushb/neoncoup/protocol"
	"github.com/v1rushb/neoncoup/transport"
)

// tclient is a scripted protocol client over an in-memory pipe.
type tclient struct {
	t      *testing.T
	conn   transport.Conn
	frames chan protocol.Envelope
	done   chan struct{}
	seat   int
	ver    uint64
	token  string
}

func dial(t *testing.T, s *Server, h protocol.Hello) *tclient {
	t.Helper()
	a, b := transport.Pipe()
	go s.ServeConn(b)
	c := &tclient{t: t, conn: a, frames: make(chan protocol.Envelope, 4096), done: make(chan struct{}), seat: -1}
	go func() {
		defer close(c.done)
		for {
			f, err := a.Recv(context.Background())
			if err != nil {
				return
			}
			env, err := protocol.Decode(f)
			if err != nil {
				t.Errorf("bad frame from server: %v", err)
				return
			}
			c.frames <- env
		}
	}()
	if h.Proto == 0 {
		h.Proto = protocol.Version
	}
	c.send(protocol.THello, h)
	return c
}

func (c *tclient) send(typ string, payload any) {
	c.t.Helper()
	if err := c.conn.Send(context.Background(), protocol.MustEncode(typ, payload)); err != nil {
		c.t.Fatalf("send %s: %v", typ, err)
	}
}

// expect reads frames until one of the given type arrives.
func (c *tclient) expect(typ string) protocol.Envelope {
	c.t.Helper()
	timeout := time.After(5 * time.Second)
	for {
		select {
		case env := <-c.frames:
			c.track(env)
			if env.Type == typ {
				return env
			}
		case <-timeout:
			c.t.Fatalf("timed out waiting for %q", typ)
		}
	}
}

func (c *tclient) track(env protocol.Envelope) {
	switch env.Type {
	case protocol.TWelcome:
		var w protocol.Welcome
		_ = env.Into(&w)
		c.token = w.Token
	case protocol.TGameStart:
		var g protocol.GameStart
		_ = env.Into(&g)
		c.seat = g.YouSeat
	case protocol.TState:
		var s protocol.State
		_ = env.Into(&s)
		c.ver = s.View.Version
	}
}

func into[T any](t *testing.T, env protocol.Envelope) T {
	t.Helper()
	var v T
	if err := env.Into(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

func newTestServer(t *testing.T) (*Server, *Room) {
	t.Helper()
	s := New(Options{Seed: 42})
	t.Cleanup(s.Close)
	settings := protocol.DefaultSettings()
	settings.Speed = protocol.SpeedInstant
	settings.TurnMS = 0
	r, err := s.CreateRoom("NEON-TEST", settings)
	if err != nil {
		t.Fatal(err)
	}
	return s, r
}

func TestRejectsWrongProtocolVersion(t *testing.T) {
	s, _ := newTestServer(t)
	c := dial(t, s, protocol.Hello{Proto: protocol.Version + 1, Name: "old", Room: "NEON-TEST"})
	rej := into[protocol.Reject](t, c.expect(protocol.TReject))
	if rej.Code != protocol.RejectVersion || !strings.Contains(rej.Reason, "Update") {
		t.Fatalf("%+v", rej)
	}
	<-c.done // server closes the connection
}

func TestRejectsUnknownRoom(t *testing.T) {
	s, _ := newTestServer(t)
	c := dial(t, s, protocol.Hello{Name: "x", Room: "NEON-ZZZZ"})
	if rej := into[protocol.Reject](t, c.expect(protocol.TReject)); rej.Code != protocol.RejectNotFound {
		t.Fatalf("%+v", rej)
	}
}

func TestRejectsNonHelloFirstFrame(t *testing.T) {
	s, _ := newTestServer(t)
	a, b := transport.Pipe()
	go s.ServeConn(b)
	_ = a.Send(context.Background(), protocol.MustEncode(protocol.TStart, nil))
	f, err := a.Recv(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	env, _ := protocol.Decode(f)
	if env.Type != protocol.TReject {
		t.Fatalf("got %s", env.Type)
	}
}

func TestLobbyHostAndReadyRules(t *testing.T) {
	s, r := newTestServer(t)
	r.Do(func(r *Room) { r.settings.FillBots = false; r.settings.MaxPlayers = 3 })
	a := dial(t, s, protocol.Hello{Name: "\x1b[31mALICE\x1b[0m", Room: "NEON-TEST"})
	if w := into[protocol.Welcome](t, a.expect(protocol.TWelcome)); !w.Host {
		t.Fatal("first member should host")
	}
	b := dial(t, s, protocol.Hello{Name: "alice", Room: "NEON-TEST"})
	if w := into[protocol.Welcome](t, b.expect(protocol.TWelcome)); w.Host {
		t.Fatal("second member is not host")
	}
	lobby := into[protocol.Lobby](t, b.expect(protocol.TLobby))
	if len(lobby.Members) != 2 || lobby.Members[0].Name != "ALICE" || lobby.Members[1].Name != "alice-2" {
		t.Fatalf("names not sanitized/deduped: %+v", lobby.Members)
	}

	b.send(protocol.TSettings, protocol.Settings{MaxPlayers: 6})
	if e := into[protocol.Error](t, b.expect(protocol.TError)); e.Code != protocol.ErrNotHost {
		t.Fatalf("%+v", e)
	}
	b.send(protocol.TStart, nil)
	if e := into[protocol.Error](t, b.expect(protocol.TError)); e.Code != protocol.ErrNotHost {
		t.Fatalf("%+v", e)
	}
	a.send(protocol.TStart, nil)
	if e := into[protocol.Error](t, a.expect(protocol.TError)); e.Code != protocol.ErrNotReady {
		t.Fatalf("start with unready member: %+v", e)
	}
	b.send(protocol.TReady, protocol.Ready{Ready: true})
	// Wait until the room has processed the ready flag (frames from
	// different connections are not ordered relative to each other).
	for {
		l := into[protocol.Lobby](t, a.expect(protocol.TLobby))
		if len(l.Members) == 2 && l.Members[1].Ready {
			break
		}
	}
	a.send(protocol.TStart, nil)
	gs := into[protocol.GameStart](t, a.expect(protocol.TGameStart))
	if len(gs.Seats) != 2 {
		t.Fatalf("seats %+v", gs.Seats)
	}
}

func TestSoloGameRunsToCompletionWithReplay(t *testing.T) {
	s, r := newTestServer(t)
	r.Do(func(r *Room) { r.settings.MaxPlayers = 4; r.settings.FillBots = true })
	c := dial(t, s, protocol.Hello{Name: "solo", Room: "NEON-TEST"})
	c.expect(protocol.TWelcome)
	c.send(protocol.TStart, nil)
	gs := into[protocol.GameStart](t, c.expect(protocol.TGameStart))
	if len(gs.Seats) != 4 || gs.YouSeat < 0 {
		t.Fatalf("%+v", gs)
	}
	bots := 0
	for _, st := range gs.Seats {
		if st.Bot {
			bots++
		}
	}
	if bots != 3 {
		t.Fatalf("want 3 bots, got %d", bots)
	}
	over := playUntilOver(t, c)
	// The replay must reproduce the game exactly.
	g, err := engine.New(over.Replay.Config)
	if err != nil {
		t.Fatal(err)
	}
	for _, st := range over.Replay.Steps {
		if _, err := g.Apply(st.Seat, st.Intent); err != nil {
			t.Fatalf("replay step refused: %v", err)
		}
	}
	if !g.Over() || g.Winner() != over.Winner {
		t.Fatalf("replay winner %d, game said %d", g.Winner(), over.Winner)
	}
	// Back in the lobby, ready for a rematch.
	lobby := into[protocol.Lobby](t, c.expect(protocol.TLobby))
	if lobby.InGame {
		t.Fatal("room should return to lobby")
	}
}

// playUntilOver answers decisions with the first legal intent, checking
// redaction on every state it receives. Like a real client it only answers
// the newest state, once the frames queued so far are drained.
func playUntilOver(t *testing.T, c *tclient) protocol.GameOver {
	t.Helper()
	deadline := time.After(20 * time.Second)
	var latest *protocol.State
	answered := uint64(0)
	answer := func() {
		if latest != nil && len(c.frames) == 0 && len(latest.View.Legal) > 0 && latest.View.Version+1 != answered {
			answered = latest.View.Version + 1
			c.send(protocol.TIntent, protocol.IntentMsg{Ver: latest.View.Version, Intent: latest.View.Legal[0]})
		}
	}
	for {
		select {
		case env := <-c.frames:
			c.track(env)
			if debugPlay {
				t.Logf("%s %s", time.Now().Format("05.000"), env.Data)
			}
			switch env.Type {
			case protocol.TState:
				st := into[protocol.State](t, env)
				checkRedacted(t, st.View, c.seat)
				latest = &st
			case protocol.TEvents:
				for _, e := range into[protocol.Events](t, env).Events {
					if len(e.Secret) > 0 && e.Seat != c.seat {
						t.Fatalf("received another seat's secret: %+v", e)
					}
				}
			case protocol.TGameOver:
				return into[protocol.GameOver](t, env)
			}
			answer()
		case <-c.done:
			t.Fatal("server closed the connection mid-game")
		case <-deadline:
			t.Fatal("game did not finish")
		}
	}
}

var debugPlay = false

func checkRedacted(t *testing.T, v engine.PlayerView, seat int) {
	t.Helper()
	if v.Seat != seat {
		t.Fatalf("view for seat %d sent to seat %d", v.Seat, seat)
	}
	if seat < 0 && (v.Hand != nil || v.Legal != nil) {
		t.Fatal("spectator got a hand")
	}
	if len(v.ExchangeOptions) > 0 && (v.Pending == nil || v.Pending.Actor != seat) {
		t.Fatal("exchange options leaked")
	}
}

func TestStaleIntentRejected(t *testing.T) {
	s, r := newTestServer(t)
	r.Do(func(r *Room) { r.settings.MaxPlayers = 2 })
	c := dial(t, s, protocol.Hello{Name: "solo", Room: "NEON-TEST"})
	c.send(protocol.TStart, nil)
	c.expect(protocol.TGameStart)
	st := into[protocol.State](t, c.expect(protocol.TState))
	c.send(protocol.TIntent, protocol.IntentMsg{Ver: st.View.Version + 5, Intent: engine.PassIntent()})
	if e := into[protocol.Error](t, c.expect(protocol.TError)); e.Code != protocol.ErrStale {
		t.Fatalf("%+v", e)
	}
}

func TestIllegalIntentRejected(t *testing.T) {
	s, r := newTestServer(t)
	r.Do(func(r *Room) { r.settings.MaxPlayers = 2 })
	c := dial(t, s, protocol.Hello{Name: "solo", Room: "NEON-TEST"})
	c.send(protocol.TStart, nil)
	c.expect(protocol.TGameStart)
	st := into[protocol.State](t, c.expect(protocol.TState))
	// Coup with two coins is never legal.
	c.send(protocol.TIntent, protocol.IntentMsg{Ver: st.View.Version, Intent: engine.ActionIntent(engine.Coup, 1-c.seat)})
	if e := into[protocol.Error](t, c.expect(protocol.TError)); e.Code != protocol.ErrIllegal {
		t.Fatalf("%+v", e)
	}
}

func TestChatIsSanitizedAndRateLimited(t *testing.T) {
	s, _ := newTestServer(t)
	a := dial(t, s, protocol.Hello{Name: "a", Room: "NEON-TEST"})
	a.expect(protocol.TWelcome)
	a.send(protocol.TChat, protocol.Chat{Text: "\x1b]0;pwned\x07hello \x1b[2Jworld"})
	var got protocol.Chat
	for {
		got = into[protocol.Chat](t, a.expect(protocol.TChat))
		if !got.System {
			break
		}
	}
	if got.Text != "hello world" || got.Name != "a" {
		t.Fatalf("%+v", got)
	}
	limited := false
	for range 20 {
		a.send(protocol.TChat, protocol.Chat{Text: "spam"})
	}
	timeout := time.After(3 * time.Second)
	for !limited {
		select {
		case env := <-a.frames:
			if env.Type == protocol.TError && into[protocol.Error](t, env).Code == protocol.ErrRateLimit {
				limited = true
			}
		case <-timeout:
			t.Fatal("chat flood was not rate limited")
		}
	}
}

func TestSpectatorSeesNoHands(t *testing.T) {
	s, r := newTestServer(t)
	r.Do(func(r *Room) { r.settings.MaxPlayers = 3 })
	host := dial(t, s, protocol.Hello{Name: "host", Room: "NEON-TEST"})
	host.expect(protocol.TWelcome) // join first, so this member is the host
	spec := dial(t, s, protocol.Hello{Name: "eye", Room: "NEON-TEST", Spectate: true})
	spec.expect(protocol.TWelcome)
	host.send(protocol.TStart, nil)
	gs := into[protocol.GameStart](t, spec.expect(protocol.TGameStart))
	if gs.YouSeat != -1 || len(gs.Seats) != 3 {
		t.Fatalf("%+v", gs)
	}
	st := into[protocol.State](t, spec.expect(protocol.TState))
	raw, _ := json.Marshal(st)
	if strings.Contains(string(raw), `"hand"`) {
		t.Fatalf("spectator state has a hand: %s", raw)
	}
}

func TestDisconnectHoldsSeatThenBotTakesOverAndRejoinReclaims(t *testing.T) {
	// Slow bots down so the game is still running when we come back.
	s := New(Options{Seed: 7, ThinkMin: 150 * time.Millisecond, ThinkMax: 200 * time.Millisecond})
	t.Cleanup(s.Close)
	settings := protocol.DefaultSettings()
	settings.Speed = protocol.SpeedInstant
	settings.TurnMS = 0
	r, _ := s.CreateRoom("NEON-TEST", settings)
	r.Do(func(r *Room) {
		r.settings.MaxPlayers = 2
		r.settings.ReconnectS = 0 // take over immediately
		r.settings.WindowMS = 60000
	})
	c := dial(t, s, protocol.Hello{Name: "flaky", Room: "NEON-TEST"})
	c.expect(protocol.TWelcome)
	token := c.token
	watcher := dial(t, s, protocol.Hello{Name: "eye", Room: "NEON-TEST", Spectate: true})
	watcher.expect(protocol.TWelcome)
	c.send(protocol.TStart, nil)
	c.expect(protocol.TGameStart)
	c.expect(protocol.TState)
	seat := c.seat
	c.conn.Close()
	<-c.done

	// The watcher hears the hold notice, then the takeover.
	sawHold, sawTakeover := false, false
	timeout := time.After(5 * time.Second)
	for !sawTakeover {
		select {
		case env := <-watcher.frames:
			if env.Type != protocol.TChat {
				continue
			}
			msg := into[protocol.Chat](t, env)
			if msg.System && strings.Contains(msg.Text, "lost the uplink") {
				sawHold = true
			}
			if msg.System && strings.Contains(msg.Text, "bot is running their seat") {
				sawTakeover = true
			}
		case <-timeout:
			t.Fatal("no bot takeover announced")
		}
	}
	if !sawHold {
		t.Fatal("seat hold was not announced")
	}

	c2 := dial(t, s, protocol.Hello{Name: "flaky", Room: "NEON-TEST", Token: token})
	w := into[protocol.Welcome](t, c2.expect(protocol.TWelcome))
	if !w.Rejoined {
		t.Fatal("token rejoin not recognized")
	}
	gs := into[protocol.GameStart](t, c2.expect(protocol.TGameStart))
	if gs.YouSeat != seat {
		t.Fatalf("rejoined into seat %d, want %d", gs.YouSeat, seat)
	}
	r.Do(func(r *Room) {
		if r.game == nil {
			t.Error("game ended unexpectedly")
		} else if r.seats[seat].takeover || r.seats[seat].bot != nil {
			t.Error("bot still controls the reclaimed seat")
		}
	})
}

func TestWindowTimeoutAutoPasses(t *testing.T) {
	s, r := newTestServer(t)
	r.Do(func(r *Room) { r.settings.MaxPlayers = 2; r.settings.WindowMS = 2000 })
	c := dial(t, s, protocol.Hello{Name: "afk", Room: "NEON-TEST"})
	c.send(protocol.TStart, nil)
	c.expect(protocol.TGameStart)
	// Force a window that waits on the human, then never answer it.
	start := time.Now()
	var timed bool
	for time.Since(start) < 15*time.Second && !timed {
		env := c.expect(protocol.TState)
		st := into[protocol.State](t, env)
		v := st.View
		if v.Phase.Timed() && slicesContains(v.Awaiting, c.seat) {
			if st.DeadlineMS <= 0 || st.TimerMS != 2000 {
				t.Fatalf("window state without a deadline: %+v", st)
			}
			timed = true
			break
		}
		if len(v.Legal) > 0 {
			c.send(protocol.TIntent, protocol.IntentMsg{Ver: v.Version, Intent: v.Legal[0]})
		}
	}
	if !timed {
		t.Skip("no window reached the human in time")
	}
	// Silence: the host must pass for us after ~2s.
	waitStart := time.Now()
	for {
		st := into[protocol.State](t, c.expect(protocol.TState))
		if !st.View.Phase.Timed() || !slicesContains(st.View.Awaiting, c.seat) {
			break
		}
	}
	if el := time.Since(waitStart); el < 1500*time.Millisecond {
		t.Fatalf("window closed after %v, before its timer", el)
	}
}

func slicesContains(s []int, v int) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

func TestIdleRoomCloses(t *testing.T) {
	s := New(Options{IdleClose: 100 * time.Millisecond})
	t.Cleanup(s.Close)
	r, _ := s.CreateRoom("NEON-IDLE", protocol.DefaultSettings())
	c := dial(t, s, protocol.Hello{Name: "x", Room: "NEON-IDLE"})
	c.expect(protocol.TWelcome)
	time.Sleep(250 * time.Millisecond)
	if s.Room("NEON-IDLE") == nil {
		t.Fatal("room closed while someone was connected")
	}
	c.send(protocol.TLeave, nil)
	select {
	case <-r.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("empty room did not close")
	}
	deadline := time.Now().Add(time.Second)
	for s.Room("NEON-IDLE") != nil && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if s.Room("NEON-IDLE") != nil {
		t.Fatal("closed room still listed")
	}
}

func TestShutdownTellsPlayersWhy(t *testing.T) {
	s := New(Options{})
	r, _ := s.CreateRoom("NEON-BYEB", protocol.DefaultSettings())
	_ = r
	c := dial(t, s, protocol.Hello{Name: "x", Room: "NEON-BYEB"})
	c.expect(protocol.TWelcome)
	s.Close()
	rej := into[protocol.Reject](t, c.expect(protocol.TReject))
	if rej.Code != protocol.RejectClosed {
		t.Fatalf("%+v", rej)
	}
}

func TestJunkFloodIsDisconnected(t *testing.T) {
	s, _ := newTestServer(t)
	c := dial(t, s, protocol.Hello{Name: "spam", Room: "NEON-TEST"})
	c.expect(protocol.TWelcome)
	for range 300 {
		if err := c.conn.Send(context.Background(), []byte("{not json")); err != nil {
			break
		}
	}
	select {
	case <-c.done:
	case <-time.After(5 * time.Second):
		t.Fatal("junk flood was not disconnected")
	}
}

func TestLateJoinersCannotOverflowTheTable(t *testing.T) {
	s, r := newTestServer(t)
	r.Do(func(r *Room) { r.settings.MaxPlayers = 2; r.settings.WindowMS = 60000 })
	host := dial(t, s, protocol.Hello{Name: "host", Room: "NEON-TEST"})
	host.send(protocol.TStart, nil)
	host.expect(protocol.TGameStart)
	for i := range 4 {
		c := dial(t, s, protocol.Hello{Name: fmt.Sprintf("late%d", i), Room: "NEON-TEST"})
		c.expect(protocol.TWelcome)
	}
	var players int
	r.Do(func(r *Room) {
		r.game = nil
		r.resetToLobby()
		for _, m := range r.members {
			if !m.spectate {
				players++
			}
		}
	})
	if players > 2 {
		t.Fatalf("%d players for 2 seats", players)
	}
}

func TestHostRoleSurvivesEveryoneDropping(t *testing.T) {
	s, r := newTestServer(t)
	a := dial(t, s, protocol.Hello{Name: "a", Room: "NEON-TEST"})
	a.expect(protocol.TWelcome)
	b := dial(t, s, protocol.Hello{Name: "b", Room: "NEON-TEST"})
	b.expect(protocol.TWelcome)
	r.Do(func(r *Room) { r.settings.ReconnectS = 1 })
	tokenB := b.token
	a.conn.Close()
	<-a.done
	time.Sleep(400 * time.Millisecond)
	b.conn.Close()
	<-b.done
	time.Sleep(800 * time.Millisecond) // a's hold has expired, b's has not
	b2 := dial(t, s, protocol.Hello{Name: "b", Room: "NEON-TEST", Token: tokenB})
	if w := into[protocol.Welcome](t, b2.expect(protocol.TWelcome)); !w.Host {
		t.Fatal("the last runner standing should host")
	}
}
