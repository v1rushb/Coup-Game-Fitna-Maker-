package client

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/v1rushb/neoncoup/engine"
	"github.com/v1rushb/neoncoup/protocol"
)

func plain(s string) string { return ansi.Strip(s) }

func TestBannerAnnouncesTheDeclaredAction(t *testing.T) {
	sc := newScripted(t, false)
	sc.s.speed = protocol.SpeedRelaxed
	sc.queueFrames(sc.apply(0, engine.ActionIntent(engine.Tax, -1)))
	sc.s.pump(sc.a)
	sc.a.now = sc.a.now.Add(500 * time.Millisecond) // past the glitch-in
	out := plain(sc.a.View().Content)
	if !strings.Contains(out, "VEX ▸ CLAIMS DUKE ▸ TAX +3¢") {
		t.Fatalf("no action banner:\n%s", out)
	}
	if bar := plain(sc.s.contextLine(sc.a, 200)); !strings.Contains(bar, "VEX ▸ CLAIMS DUKE") {
		t.Fatalf("context bar during the beat: %q", bar)
	}
}

func TestTargetedActionHighlightsTheTarget(t *testing.T) {
	sc := newScripted(t, false)
	sc.s.speed = protocol.SpeedRelaxed
	// VEX steals from MOTH (seat 2).
	sc.queueFrames(sc.apply(0, engine.ActionIntent(engine.Steal, 2)))
	sc.s.pump(sc.a)
	sc.a.now = sc.a.now.Add(500 * time.Millisecond)
	out := plain(sc.a.View().Content)
	if !strings.Contains(out, "VEX ▸ CLAIMS CAPTAIN ▸ STEAL 2¢ ▸ MOTH") {
		t.Fatalf("banner should name the target:\n%s", out)
	}
	if !strings.Contains(out, "◀ TARGET") {
		t.Fatalf("target panel not marked:\n%s", out)
	}
	if !sc.s.dimmed(3) || sc.s.dimmed(0) || sc.s.dimmed(2) {
		t.Fatal("only the actor and the target should stay lit during the beat")
	}
}

func TestContextBarExplainsChoicesAndRisks(t *testing.T) {
	sc := newScripted(t, false)
	sc.do(0, engine.ActionIntent(engine.Tax, -1))
	sc.s.deadline, sc.s.timerTotal = sc.a.now.Add(9*time.Second), 12*time.Second
	bar := plain(sc.s.contextLine(sc.a, 200))
	for _, want := range []string{"VEX claims Duke to take 3 coins.", "[C] Challenge (if VEX has a Duke, you lose an influence)", "[P] Pass", "9s"} {
		if !strings.Contains(bar, want) {
			t.Fatalf("context bar %q lacks %q", bar, want)
		}
	}
	// Narrow terminals keep the choice and the countdown.
	if narrow := plain(sc.s.contextLine(sc.a, 60)); !strings.Contains(narrow, "[C] Challenge") || !strings.Contains(narrow, "9s") || len([]rune(narrow)) > 60 {
		t.Fatalf("narrow context bar %q", narrow)
	}
}

func TestYourTurnListsWhyActionsAreDimmed(t *testing.T) {
	sc := newScripted(t, false)
	sc.do(0, engine.ActionIntent(engine.Income, -1)) // VEX, then it's your turn
	if !sc.s.awaitingMe() || sc.s.v.Phase != engine.PhaseTurnStart {
		t.Fatal("expected your turn")
	}
	bar := plain(sc.s.contextLine(sc.a, 200))
	for _, want := range []string{"YOUR TURN", "Coup needs 7¢", "Assassinate needs 3¢"} {
		if !strings.Contains(bar, want) {
			t.Fatalf("context bar %q lacks %q", bar, want)
		}
	}
	if sc.s.turnAlert.IsZero() {
		t.Fatal("your turn should raise the alert")
	}
	alert := sc.s.turnAlert
	sc.s.onState(sc.a, protocol.State{View: sc.g.View(1)}, sc.a.now)
	if sc.s.turnAlert != alert {
		t.Fatal("the alert fired twice for one turn")
	}
}

func TestOpponentProvingIsExplained(t *testing.T) {
	sc := newScripted(t, false)
	sc.do(0, engine.ActionIntent(engine.Tax, -1))
	sc.do(1, engine.ChallengeIntent())
	bar := plain(sc.s.contextLine(sc.a, 200))
	if !strings.Contains(bar, "You challenged VEX's Duke.") || !strings.Contains(bar, "VEX must flip a card") {
		t.Fatalf("context bar %q", bar)
	}
}
