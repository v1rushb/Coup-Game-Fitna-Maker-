package client

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/v1rushb/neoncoup/client/art"
	"github.com/v1rushb/neoncoup/engine"
	"github.com/v1rushb/neoncoup/protocol"
)

// playTo queues a batch and advances until a beat of kind k is at progress p.
func (sc *scripted) playTo(evs []engine.Event, k engine.EventKind, p float64) bool {
	sc.queueFrames(evs)
	sc.s.pump(sc.a)
	for range 200 {
		b := sc.s.beat
		if b == nil {
			return false
		}
		if b.ev.Kind == k {
			sc.a.now = b.start.Add(time.Duration(float64(b.end.Sub(b.start)) * p))
			return true
		}
		sc.a.now = b.end
		sc.s.pump(sc.a)
	}
	return false
}

// animations are setups that stop inside an animated beat.
var animations = []struct {
	name string
	kind engine.EventKind
	run  func(sc *scripted) []engine.Event
}{
	{"coins", engine.EvCoins, func(sc *scripted) []engine.Event {
		return sc.apply(0, engine.ActionIntent(engine.Income, -1))
	}},
	{"opponent redraw", engine.EvCardReplaced, func(sc *scripted) []engine.Event {
		sc.do(0, engine.ActionIntent(engine.Income, -1))
		sc.do(1, engine.ActionIntent(engine.Income, -1))
		sc.do(2, engine.ActionIntent(engine.Exchange, -1)) // MOTH holds an Ambassador
		sc.do(1, engine.ChallengeIntent())
		return sc.apply(2, engine.RevealIntent(0))
	}},
	{"your redraw", engine.EvCardReplaced, func(sc *scripted) []engine.Event {
		sc.do(0, engine.ActionIntent(engine.Income, -1))
		sc.do(1, engine.ActionIntent(engine.Tax, -1)) // you hold the Duke
		sc.do(0, engine.ChallengeIntent())
		return sc.apply(1, engine.RevealIntent(0))
	}},
	{"influence loss", engine.EvInfluenceLost, func(sc *scripted) []engine.Event {
		sc.do(0, engine.ActionIntent(engine.Tax, -1)) // VEX bluffs
		sc.do(1, engine.ChallengeIntent())
		return sc.apply(0, engine.RevealIntent(0))
	}},
	{"exchange", engine.EvExchangeDraw, func(sc *scripted) []engine.Event {
		sc.do(0, engine.ActionIntent(engine.Income, -1))
		sc.do(1, engine.ActionIntent(engine.Income, -1))
		sc.do(2, engine.ActionIntent(engine.Exchange, -1))
		for _, s := range []int{0, 1, 3} {
			sc.do(s, engine.PassIntent())
		}
		return sc.apply(4, engine.PassIntent())
	}},
	{"elimination", engine.EvEliminated, func(sc *scripted) []engine.Event {
		sc.do(0, engine.ActionIntent(engine.Tax, -1))
		sc.do(1, engine.ChallengeIntent())
		sc.do(0, engine.RevealIntent(0))
		for _, s := range []int{1, 2, 3, 4} {
			sc.do(s, engine.ActionIntent(engine.Income, -1))
		}
		sc.do(0, engine.ActionIntent(engine.Tax, -1))
		return sc.apply(1, engine.ChallengeIntent())
	}},
}

func TestAnimationFramesStayInBounds(t *testing.T) {
	for _, ascii := range []bool{false, true} {
		for _, an := range animations {
			for _, p := range []float64{0.05, 0.3, 0.6, 0.95} {
				sc := newScripted(t, ascii)
				sc.s.speed = protocol.SpeedRelaxed
				if !sc.playTo(an.run(sc), an.kind, p) {
					t.Fatalf("%s: never reached a %s beat", an.name, an.kind)
				}
				checkFrame(t, sc.a, fmt.Sprintf("%s at %.0f%% ascii=%v", an.name, p*100, ascii))
			}
		}
	}
}

func TestOpponentsOnlySeeCardBacksMove(t *testing.T) {
	sc := newScripted(t, false)
	sc.s.speed = protocol.SpeedRelaxed
	an := animations[1] // an opponent's redraw
	if !sc.playTo(an.run(sc), an.kind, 0.7) {
		t.Fatal("no redraw beat")
	}
	if sc.s.beat.ev.Secret != nil {
		t.Fatal("an opponent's replacement card reached this client")
	}
	sc.a.w, sc.a.h = 160, 45
	if n := strings.Count(shot(sc.a), "NEON//COUP"); n < 2 {
		t.Fatal("the replacement should travel as a card back")
	}
	sc.a.cfg.ReduceMotion = true
	if n := strings.Count(shot(sc.a), "NEON//COUP"); n != 1 {
		t.Fatalf("reduced motion still draws moving cards (%d backs)", n-1)
	}
}

func TestYourReplacementFlipsOnlyAtTheEnd(t *testing.T) {
	sc := newScripted(t, false)
	sc.s.speed = protocol.SpeedRelaxed
	an := animations[2]
	if !sc.playTo(an.run(sc), an.kind, 0.5) {
		t.Fatal("no redraw beat")
	}
	newCard := sc.s.beat.ev.Secret[0]
	if card, ok := sc.s.beatSlot(sc.a, 0, artOpts(), 18); !ok || strings.TrimSpace(card) != "" {
		t.Fatal("your slot should be empty while the card is in the deck")
	}
	sc.s.drain(sc.a)
	if sc.s.v.Hand[0].Char != newCard {
		t.Fatalf("hand slot holds %s after the redraw, want %s", sc.s.v.Hand[0].Char, newCard)
	}
}

func artOpts() art.CardOpts { return art.CardOpts{} }
