package client

import (
	"testing"
	"time"

	"github.com/v1rushb/neoncoup/engine"
	"github.com/v1rushb/neoncoup/protocol"
)

// queueFrames queues a batch the way the host sends it: events, then the
// state, without playing anything yet.
func (sc *scripted) queueFrames(evs []engine.Event) {
	for _, e := range engine.RedactAll(evs, 1) {
		sc.s.enqueue(sc.a, qItem{ev: &e})
	}
	st := protocol.State{View: sc.g.View(1)}
	sc.s.enqueue(sc.a, qItem{state: &st})
}

func (sc *scripted) apply(seat int, in engine.Intent) []engine.Event {
	sc.t.Helper()
	evs, err := sc.g.Apply(seat, in)
	if err != nil {
		sc.t.Fatal(err)
	}
	return evs
}

func (sc *scripted) advance(d time.Duration) {
	sc.a.now = sc.a.now.Add(d)
	sc.s.pump(sc.a)
}

func TestStateWaitsForItsBeats(t *testing.T) {
	sc := newScripted(t, false)
	sc.s.speed = protocol.SpeedRelaxed
	// VEX (seat 0) is first and claims Duke.
	sc.queueFrames(sc.apply(0, engine.ActionIntent(engine.Tax, -1)))
	sc.s.pump(sc.a)
	if sc.s.v.Phase == engine.PhaseChallengeWindow || sc.s.awaitingMe() {
		t.Fatal("the challenge window showed before the announcement played")
	}
	if sc.s.beat == nil || sc.s.beat.ev.Kind != engine.EvActionDeclared {
		t.Fatalf("expected the announcement beat, got %+v", sc.s.beat)
	}
	if p := sc.s.v.Pending; p == nil || p.Actor != 0 || p.Claim != engine.Duke {
		t.Fatalf("the announcement should show the claim on the table: %+v", p)
	}
	sc.advance(1400 * time.Millisecond)
	if sc.s.v.Phase == engine.PhaseChallengeWindow {
		t.Fatal("state applied 100 ms before its beat ended")
	}
	sc.advance(150 * time.Millisecond)
	if sc.s.v.Phase != engine.PhaseChallengeWindow || !sc.s.awaitingMe() {
		t.Fatalf("after the beat the window should be open for you: phase %s", sc.s.v.Phase)
	}
}

func TestSpaceSkipsTheCurrentBeat(t *testing.T) {
	sc := newScripted(t, false)
	sc.s.speed = protocol.SpeedRelaxed
	sc.queueFrames(sc.apply(0, engine.ActionIntent(engine.Tax, -1)))
	sc.s.pump(sc.a)
	sc.s.skipBeat(sc.a)
	if sc.s.v.Phase != engine.PhaseChallengeWindow || sc.s.presenting() {
		t.Fatal("skipping the announcement should open the window at once")
	}
}

func TestPauseHoldsThePresentation(t *testing.T) {
	sc := newScripted(t, false)
	sc.s.speed = protocol.SpeedRelaxed
	sc.queueFrames(sc.apply(0, engine.ActionIntent(engine.Tax, -1)))
	sc.s.pump(sc.a)
	sc.advance(500 * time.Millisecond)
	sc.s.setPaused(sc.a, true)
	sc.advance(10 * time.Second)
	if sc.s.v.Phase == engine.PhaseChallengeWindow {
		t.Fatal("the beat kept playing while paused")
	}
	sc.s.setPaused(sc.a, false)
	sc.advance(900 * time.Millisecond)
	if sc.s.v.Phase == engine.PhaseChallengeWindow {
		t.Fatal("resume lost the rest of the beat")
	}
	sc.advance(200 * time.Millisecond)
	if sc.s.v.Phase != engine.PhaseChallengeWindow {
		t.Fatal("the beat never finished after resuming")
	}
}

func TestFallingBehindPlaysBeatsFaster(t *testing.T) {
	sc := newScripted(t, false)
	sc.s.speed = protocol.SpeedRelaxed
	sc.queueFrames(sc.apply(0, engine.ActionIntent(engine.Income, -1)))
	sc.queueFrames(sc.apply(1, engine.ActionIntent(engine.Income, -1)))
	sc.s.pump(sc.a)
	b := sc.s.beat
	if b == nil {
		t.Fatal("no beat playing")
	}
	if d := b.end.Sub(b.start); d >= 1500*time.Millisecond {
		t.Fatalf("two states behind, the announcement still takes %v", d)
	}
}

func TestCoinsTickDuringTheirBeat(t *testing.T) {
	sc := newScripted(t, false)
	sc.s.speed = protocol.SpeedRelaxed
	before := sc.s.v.Players[0].Coins
	sc.queueFrames(sc.apply(0, engine.ActionIntent(engine.Income, -1)))
	sc.s.pump(sc.a)
	sc.advance(1600 * time.Millisecond) // past the announcement, into the coin beat
	if sc.s.beat == nil || sc.s.beat.ev.Kind != engine.EvCoins {
		t.Fatalf("expected the coin beat, got %+v", sc.s.beat)
	}
	if got := sc.s.v.Players[0].Coins; got != before+1 {
		t.Fatalf("coins should move as the coin beat starts: %d -> %d", before, got)
	}
	sc.s.drain(sc.a)
	if sc.s.v.Players[0].Coins != sc.g.View(1).Players[0].Coins || sc.s.v.Treasury != sc.g.View(1).Treasury {
		t.Fatal("projected view disagrees with the host's state")
	}
}
