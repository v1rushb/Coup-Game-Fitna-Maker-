package client

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/v1rushb/neoncoup/client/theme"
	"github.com/v1rushb/neoncoup/engine"
	"github.com/v1rushb/neoncoup/protocol"
)

func key(sc *scripted, k string) {
	switch k {
	case "tab":
		sc.a.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	case "esc":
		sc.a.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	default:
		sc.a.Update(tea.KeyPressMsg{Code: []rune(k)[0], Text: k})
	}
}

func TestClaimBadgesAndInspect(t *testing.T) {
	sc := newScripted(t, false)
	sc.do(0, engine.ActionIntent(engine.Tax, -1)) // VEX claims Duke...
	sc.passAll()
	for _, s := range []int{1, 2, 3, 4} {
		sc.do(s, engine.ActionIntent(engine.Income, -1))
	}
	sc.do(0, engine.ActionIntent(engine.Tax, -1)) // ...twice
	if b := plain(sc.s.badges(sc.a, 0)); b != "DUKE×2" {
		t.Fatalf("VEX badges %q", b)
	}
	if !strings.Contains(shot(sc.a), "DUKE×2") {
		t.Fatal("badges not on VEX's panel")
	}
	key(sc, "tab")
	if sc.s.inspect != sc.s.inspectOrder()[0] {
		t.Fatal("tab should open inspect on the first rival")
	}
	for sc.s.inspect != 0 {
		key(sc, "tab")
	}
	out := shot(sc.a)
	if !strings.Contains(out, "INSPECT // VEX") || !strings.Contains(out, "TAX (claims DUKE)") {
		t.Fatalf("inspect view:\n%s", out)
	}
	for range len(sc.s.seats) {
		if key(sc, "tab"); sc.s.inspect == -1 {
			break
		}
	}
	if sc.s.inspect != -1 {
		t.Fatal("tab past the last player should close inspect")
	}
}

func TestLastRoundSummaryWhenYourTurnStarts(t *testing.T) {
	sc := newScripted(t, false)
	sc.do(0, engine.ActionIntent(engine.Steal, 2)) // VEX steals from MOTH
	sc.passAll()
	sc.passAll()
	var last string
	for _, l := range sc.s.log {
		if l.kind == logSummary {
			last = l.text
		}
	}
	if !strings.Contains(last, "LAST ROUND // VEX steal→MOTH +2¢") {
		t.Fatalf("summary %q", last)
	}
}

func TestLogLinesCarryIcons(t *testing.T) {
	n := func(seat int) string { return []string{"VEX", "YOU"}[seat] }
	for _, tc := range []struct {
		e    engine.Event
		icon string
		kind logKind
	}{
		{engine.Event{Kind: engine.EvActionDeclared, Seat: 0, Other: -1, Action: engine.Tax, Claim: engine.Duke}, "▸", logAction},
		{engine.Event{Kind: engine.EvChallenge, Seat: 1, Other: 0, Claim: engine.Duke}, "!", logChallenge},
		{engine.Event{Kind: engine.EvBlock, Seat: 1, Other: 0, Claim: engine.Duke, Action: engine.ForeignAid}, "■", logBlock},
		{engine.Event{Kind: engine.EvClaimFailed, Seat: 0, Other: 1, Card: engine.Contessa}, "✖", logLoss},
	} {
		l, ok := narrate(tc.e, n, 1, "¢")
		if !ok || l.icon != tc.icon || l.kind != tc.kind || strings.HasPrefix(l.text, "> ") {
			t.Errorf("%s: %+v", tc.e.Kind, l)
		}
	}
}

// replayOf records a rigged game's intents as a replay.
func replayOf(t *testing.T, cfg engine.Config, steps []protocol.Step) protocol.Replay {
	t.Helper()
	return protocol.Replay{Format: protocol.ReplayFormat, Config: cfg, Steps: steps}
}

func TestPostGameCountsBluffs(t *testing.T) {
	D, A, C, B, T := engine.Duke, engine.Assassin, engine.Captain, engine.Ambassador, engine.Contessa
	cfg := engine.Config{Players: 2, First: 0, Rig: &engine.Rig{
		Hands: [][]engine.Character{{A, T}, {D, C}}, Deck: []engine.Character{D, D, A, A, C, C, B, B, B, T, T}, Coins: []int{2, 2}}}
	steps := []protocol.Step{
		{Seat: 0, Intent: engine.ActionIntent(engine.Tax, -1)}, // a bluff that gets away
		{Seat: 1, Intent: engine.PassIntent()},
		{Seat: 1, Intent: engine.ActionIntent(engine.Income, -1)},
		{Seat: 0, Intent: engine.ActionIntent(engine.Tax, -1)}, // a bluff that gets caught
		{Seat: 1, Intent: engine.ChallengeIntent()},
		{Seat: 0, Intent: engine.RevealIntent(0)},
		{Seat: 1, Intent: engine.ActionIntent(engine.Tax, -1)}, // honest
		{Seat: 0, Intent: engine.PassIntent()},
	}
	name := func(s int) string { return []string{"VEX", "YOU"}[s] }
	pg, err := analyze(replayOf(t, cfg, steps), name)
	if err != nil {
		t.Fatal(err)
	}
	if pg.bluffs[0] != (bluffCount{ok: 1, caught: 1}) || pg.bluffs[1] != (bluffCount{}) {
		t.Fatalf("bluffs %+v", pg.bluffs)
	}
	if len(pg.hands) != 2 || pg.hands[0][0].Char != A || !pg.hands[0][0].Revealed || pg.hands[0][1].Char != T {
		t.Fatalf("final hands %+v", pg.hands)
	}
	found := false
	for _, m := range pg.moments {
		found = found || strings.Contains(m, "YOU caught VEX bluffing DUKE")
	}
	if !found {
		t.Fatalf("moments %v", pg.moments)
	}
}

func TestHintsShowOnceAndCanBeTurnedOff(t *testing.T) {
	sc := newScripted(t, false)
	sc.do(0, engine.ActionIntent(engine.Tax, -1)) // a challenge window for you
	if sc.s.hint != "challenge" || !strings.Contains(shot(sc.a), "HINT") {
		t.Fatalf("first challenge window showed hint %q", sc.s.hint)
	}
	if !sc.a.cfg.HintsSeen["challenge"] {
		t.Fatal("hint not remembered")
	}
	sc.passAll()
	sc.do(1, engine.ActionIntent(engine.Income, -1))
	sc.do(2, engine.ActionIntent(engine.Tax, -1)) // another challenge window
	if sc.s.hintBox(sc.a) != "" {
		t.Fatal("the same hint showed twice")
	}
	sc2 := newScripted(t, false)
	sc2.a.cfg.NoHints = true
	sc2.do(0, engine.ActionIntent(engine.Tax, -1))
	if sc2.s.hint != "" {
		t.Fatal("hints showed while turned off")
	}
}

func TestEscOpensSettingsAndPausesSolo(t *testing.T) {
	sc := newScripted(t, false)
	sc.a.mode = "solo"
	for i := range sc.s.seats {
		if i != 1 {
			sc.s.seats[i].Bot = true
		}
	}
	key(sc, "esc")
	if sc.s.prefs == nil || !sc.s.paused {
		t.Fatal("esc should open settings and pause the solo game")
	}
	if out := shot(sc.a); !strings.Contains(out, "SETTINGS") || !strings.Contains(out, "Palette") || !strings.Contains(out, "Music volume") {
		t.Fatalf("settings overlay:\n%s", out)
	}
	key(sc, "esc")
	if sc.s.prefs != nil || sc.s.paused {
		t.Fatal("closing settings should resume")
	}
}

func TestPalettesSwitchColors(t *testing.T) {
	defer theme.Use("neon")
	neonBg := theme.Bg
	theme.Use("contrast")
	if r, g, b, _ := theme.Bg.RGBA(); r|g|b != 0 {
		t.Fatal("high contrast background is not black")
	}
	theme.Use("colorblind")
	if theme.Red == theme.Green {
		t.Fatal("colorblind palette lost the red/green distinction")
	}
	theme.Use("neon")
	if theme.Bg != neonBg {
		t.Fatal("neon palette not restored")
	}
	// Every palette renders a clean frame.
	for _, p := range theme.PaletteNames {
		sc := newScripted(t, false)
		sc.a.applyPalette(p)
		sc.do(0, engine.ActionIntent(engine.Tax, -1))
		checkFrame(t, sc.a, "palette "+p)
	}
}
