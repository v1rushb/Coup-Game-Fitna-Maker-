package server

import (
	"slices"
	"testing"
	"time"

	"github.com/v1rushb/neoncoup/engine"
	"github.com/v1rushb/neoncoup/protocol"
)

// pacedRoom opens a room at the given speed. The named member moves first.
func pacedRoom(t *testing.T, speed string, players int, bots bool, first string) (*Server, *Room) {
	t.Helper()
	s := New(Options{Seed: 11})
	t.Cleanup(s.Close)
	st := protocol.DefaultSettings()
	st.Speed, st.TurnMS, st.MaxPlayers, st.FillBots = speed, 0, players, bots
	r, err := s.CreateRoom("NEON-PACE", st)
	if err != nil {
		t.Fatal(err)
	}
	r.Do(func(r *Room) { r.lastWinner = first })
	return s, r
}

// awaitTurn reads until a state that waits on c for its turn.
func (c *tclient) awaitTurn() protocol.State {
	c.t.Helper()
	for {
		st := into[protocol.State](c.t, c.expect(protocol.TState))
		if st.View.Phase == engine.PhaseTurnStart && slices.Contains(st.View.Awaiting, c.seat) {
			return st
		}
	}
}

// nextEvent reads until an event matching ok arrives and returns it with its
// arrival time.
func (c *tclient) nextEvent(ok func(engine.Event) bool) (engine.Event, time.Time) {
	c.t.Helper()
	timeout := time.After(10 * time.Second)
	for {
		select {
		case env := <-c.frames:
			c.track(env)
			if env.Type != protocol.TEvents {
				continue
			}
			at := time.Now()
			for _, e := range into[protocol.Events](c.t, env).Events {
				if ok(e) {
					return e, at
				}
			}
		case <-timeout:
			c.t.Fatal("timed out waiting for an event")
		}
	}
}

func TestBeatTableMatchesTheSpec(t *testing.T) {
	ev := func(k engine.EventKind) engine.Event {
		return engine.Event{Kind: k, Amount: 3, Reason: engine.ReasonTax}
	}
	want := map[engine.EventKind]int{
		engine.EvActionDeclared: 1500, engine.EvChallenge: 2000, engine.EvBlock: 2000,
		engine.EvInfluenceLost: 2000, engine.EvCardReplaced: 2500, engine.EvEliminated: 3000,
		engine.EvPass: 0, engine.EvWindowOpen: 0,
	}
	for k, ms := range want {
		if got := protocol.BeatMS(ev(k), protocol.SpeedRelaxed); got != ms {
			t.Errorf("%s at relaxed = %d ms, want %d", k, got, ms)
		}
		if got := protocol.BeatMS(ev(k), protocol.SpeedNormal); got != int(float64(ms)*0.6+0.5) {
			t.Errorf("%s at normal = %d ms", k, got)
		}
		if got := protocol.BeatMS(ev(k), protocol.SpeedFast); got != int(float64(ms)*0.3+0.5) {
			t.Errorf("%s at fast = %d ms", k, got)
		}
		if got := protocol.BeatMS(ev(k), protocol.SpeedInstant); got != 0 {
			t.Errorf("%s at instant = %d ms", k, got)
		}
	}
	if w := protocol.DefaultSettings().Window(); w != 12*time.Second {
		t.Fatalf("default window at relaxed is %v, want 12s", w)
	}
}

func TestReactionWindowOpensOnlyAfterTheAnnouncement(t *testing.T) {
	s, _ := pacedRoom(t, protocol.SpeedRelaxed, 3, true, "TESTER")
	c := dial(t, s, protocol.Hello{Name: "TESTER", Room: "NEON-PACE"})
	c.expect(protocol.TWelcome)
	c.send(protocol.TStart, nil)
	c.expect(protocol.TGameStart)
	c.awaitTurn()
	c.send(protocol.TIntent, protocol.IntentMsg{Ver: c.ver, Intent: engine.ActionIntent(engine.Tax, -1)})

	var st protocol.State
	for {
		st = into[protocol.State](t, c.expect(protocol.TState))
		if st.View.Phase == engine.PhaseChallengeWindow {
			break
		}
	}
	got := time.Now()
	announce := protocol.BeatMS(engine.Event{Kind: engine.EvActionDeclared}, protocol.SpeedRelaxed)
	if st.TimerMS != 12000 {
		t.Fatalf("window is %d ms, want the 12 s relaxed default", st.TimerMS)
	}
	if st.OpensInMS < int64(announce)-100 {
		t.Fatalf("window opens in %d ms, before the %d ms announcement ends", st.OpensInMS, announce)
	}
	if d := st.DeadlineMS - st.OpensInMS; d < 11990 || d > 12010 {
		t.Fatalf("deadline %d ms after opening, want the full 12 s window", d)
	}
	// Bots answer the claim the moment the window opens, never before.
	_, at := c.nextEvent(func(e engine.Event) bool {
		return (e.Kind == engine.EvPass || e.Kind == engine.EvChallenge) && e.Seat != c.seat
	})
	if early := time.Duration(st.OpensInMS)*time.Millisecond - at.Sub(got); early > 50*time.Millisecond {
		t.Fatalf("a bot reacted %v before the window opened", early)
	}
}

func TestEarlyIntentsWaitForTheWindowToOpen(t *testing.T) {
	s, _ := pacedRoom(t, protocol.SpeedRelaxed, 2, false, "ALICE")
	a := dial(t, s, protocol.Hello{Name: "ALICE", Room: "NEON-PACE"})
	a.expect(protocol.TWelcome)
	b := dial(t, s, protocol.Hello{Name: "BOB", Room: "NEON-PACE"})
	b.expect(protocol.TWelcome)
	b.send(protocol.TReady, protocol.Ready{Ready: true})
	for {
		l := into[protocol.Lobby](t, a.expect(protocol.TLobby))
		if len(l.Members) == 2 && l.Members[1].Ready {
			break
		}
	}
	a.send(protocol.TStart, nil)
	a.expect(protocol.TGameStart)
	b.expect(protocol.TGameStart)
	a.awaitTurn()
	a.send(protocol.TIntent, protocol.IntentMsg{Ver: a.ver, Intent: engine.ActionIntent(engine.Tax, -1)})

	_, declared := b.nextEvent(func(e engine.Event) bool { return e.Kind == engine.EvActionDeclared })
	var st protocol.State
	for {
		st = into[protocol.State](t, b.expect(protocol.TState))
		if st.View.Phase == engine.PhaseChallengeWindow {
			break
		}
	}
	// BOB's client skips the presentation and challenges at once. The host
	// holds the challenge until the announcement is over.
	b.send(protocol.TIntent, protocol.IntentMsg{Ver: st.View.Version, Intent: engine.ChallengeIntent()})
	_, applied := b.nextEvent(func(e engine.Event) bool { return e.Kind == engine.EvChallenge })
	announce := time.Duration(protocol.BeatMS(engine.Event{Kind: engine.EvActionDeclared}, protocol.SpeedRelaxed)) * time.Millisecond
	if gap := applied.Sub(declared); gap < announce-50*time.Millisecond {
		t.Fatalf("challenge applied %v after the declaration, inside the %v announcement", gap, announce)
	}
}

func TestSoloPauseFreezesBotsAndTimers(t *testing.T) {
	s, _ := pacedRoom(t, protocol.SpeedFast, 3, true, "TESTER")
	c := dial(t, s, protocol.Hello{Name: "TESTER", Room: "NEON-PACE"})
	c.expect(protocol.TWelcome)
	c.send(protocol.TStart, nil)
	c.expect(protocol.TGameStart)
	c.awaitTurn()
	// Income has no window, so the next turn is a bot's.
	c.send(protocol.TIntent, protocol.IntentMsg{Ver: c.ver, Intent: engine.ActionIntent(engine.Income, -1)})
	c.nextEvent(func(e engine.Event) bool { return e.Kind == engine.EvTurnStart && e.Seat != c.seat })
	paused := true
	c.send(protocol.TControl, protocol.Control{Pause: &paused})
	for {
		if st := into[protocol.State](t, c.expect(protocol.TState)); st.Paused {
			break
		}
	}
	deadline := time.After(2 * time.Second)
	for waiting := true; waiting; {
		select {
		case env := <-c.frames:
			if env.Type != protocol.TEvents {
				continue
			}
			for _, e := range into[protocol.Events](t, env).Events {
				if e.Kind == engine.EvActionDeclared {
					t.Fatalf("a bot moved while the table was paused: %v", e)
				}
			}
		case <-deadline:
			waiting = false
		}
	}
	paused = false
	c.send(protocol.TControl, protocol.Control{Pause: &paused})
	c.nextEvent(func(e engine.Event) bool { return e.Kind == engine.EvActionDeclared && e.Seat != c.seat })
}

func TestSkipOpensTheDecisionSooner(t *testing.T) {
	s, _ := pacedRoom(t, protocol.SpeedRelaxed, 3, true, "TESTER")
	c := dial(t, s, protocol.Hello{Name: "TESTER", Room: "NEON-PACE"})
	c.expect(protocol.TWelcome)
	c.send(protocol.TStart, nil)
	c.expect(protocol.TGameStart)
	c.awaitTurn()
	c.send(protocol.TIntent, protocol.IntentMsg{Ver: c.ver, Intent: engine.ActionIntent(engine.Tax, -1)})
	for {
		if st := into[protocol.State](t, c.expect(protocol.TState)); st.View.Phase == engine.PhaseChallengeWindow {
			break
		}
	}
	c.send(protocol.TControl, protocol.Control{SkipMS: 1500})
	st := into[protocol.State](t, c.expect(protocol.TState))
	if st.OpensInMS > 200 {
		t.Fatalf("after skipping the announcement the window still opens in %d ms", st.OpensInMS)
	}
	if st.DeadlineMS < 11800 {
		t.Fatalf("skipping shortened the window itself: %d ms left", st.DeadlineMS)
	}
}

func TestControlNeedsASoloTable(t *testing.T) {
	s, _ := pacedRoom(t, protocol.SpeedRelaxed, 2, false, "ALICE")
	a := dial(t, s, protocol.Hello{Name: "ALICE", Room: "NEON-PACE"})
	a.expect(protocol.TWelcome)
	b := dial(t, s, protocol.Hello{Name: "BOB", Room: "NEON-PACE"})
	b.expect(protocol.TWelcome)
	b.send(protocol.TReady, protocol.Ready{Ready: true})
	for {
		l := into[protocol.Lobby](t, a.expect(protocol.TLobby))
		if len(l.Members) == 2 && l.Members[1].Ready {
			break
		}
	}
	a.send(protocol.TStart, nil)
	a.expect(protocol.TGameStart)
	paused := true
	a.send(protocol.TControl, protocol.Control{Pause: &paused})
	if e := into[protocol.Error](t, a.expect(protocol.TError)); e.Code != protocol.ErrIllegal {
		t.Fatalf("pause at a two-human table: %+v", e)
	}
}
