package client

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/v1rushb/neoncoup/client/theme"
	"github.com/v1rushb/neoncoup/engine"
)

// The chronicle: what each player has claimed and done this game, for the
// claim badges on their panel, the inspect overlay and the "last round"
// summary when your turn comes around.

type chronicle struct {
	claims  map[int]map[engine.Character]int
	history map[int][]string
	round   []string // what happened since your last turn, one entry per action
}

func newChronicle() chronicle {
	return chronicle{claims: map[int]map[engine.Character]int{}, history: map[int][]string{}}
}

func (c *chronicle) claim(seat int, ch engine.Character) {
	if c.claims[seat] == nil {
		c.claims[seat] = map[engine.Character]int{}
	}
	c.claims[seat][ch]++
}

func (c *chronicle) note(seat int, s string) {
	c.history[seat] = append(c.history[seat], s)
	if n := len(c.history[seat]); n > 60 {
		c.history[seat] = c.history[seat][n-60:]
	}
}

// annotate adds an outcome to the last round entry.
func (c *chronicle) annotate(s string) {
	if n := len(c.round); n > 0 {
		c.round[n-1] += " " + s
	}
}

// record files one event. It runs as each beat starts.
func (g *gameScreen) record(a *App, e engine.Event) {
	c := &g.chron
	coin := a.t.Coin()
	turn := fmt.Sprintf("T%-2d ", e.Turn)
	switch e.Kind {
	case engine.EvActionDeclared:
		what := e.Action.Title()
		if e.Other >= 0 {
			what += " " + g.arrow(a) + " " + g.name(e.Other)
		}
		if e.Claim != engine.NoCharacter {
			c.claim(e.Seat, e.Claim)
			what += " (claims " + e.Claim.Title() + ")"
		}
		c.note(e.Seat, turn+what)
		short := g.name(e.Seat) + " " + strings.ToLower(e.Action.Title())
		if e.Other >= 0 {
			short += g.arrow(a) + g.name(e.Other)
		}
		c.round = append(c.round, short)
	case engine.EvBlock:
		c.claim(e.Seat, e.Claim)
		c.note(e.Seat, fmt.Sprintf("%sBLOCKS %s (claims %s)", turn, e.Action.Title(), e.Claim.Title()))
		c.annotate("(blocked by " + g.name(e.Seat) + ")")
	case engine.EvChallenge:
		c.note(e.Seat, fmt.Sprintf("%sCHALLENGES %s's %s", turn, g.name(e.Other), e.Claim.Title()))
		c.note(e.Other, fmt.Sprintf("%sCHALLENGED by %s", turn, g.name(e.Seat)))
	case engine.EvClaimProven:
		c.note(e.Seat, "    "+a.t.G("✓", "v")+" PROVED IT: showed "+e.Card.Title())
		c.annotate("(challenge failed)")
	case engine.EvClaimFailed:
		c.note(e.Seat, "    "+a.t.G("✖", "x")+" CAUGHT BLUFFING")
		c.annotate("(bluff caught)")
	case engine.EvInfluenceLost:
		c.note(e.Seat, turn+"LOST "+e.Card.Title())
	case engine.EvEliminated:
		c.note(e.Seat, turn+"FLATLINED")
	case engine.EvCoins:
		if e.Reason == engine.ReasonSteal && e.Amount > 0 {
			c.annotate(fmt.Sprintf("+%d%s", e.Amount, coin))
		}
	case engine.EvTurnStart:
		if e.Seat == g.me && g.me >= 0 && len(c.round) > 0 {
			g.addLog(logLine{text: "LAST ROUND // " + strings.Join(c.round, " · "), kind: logSummary, turn: e.Turn})
			c.round = nil
		}
	}
}

func (g *gameScreen) arrow(a *App) string { return a.t.G("→", ">") }

// badges renders a seat's claim history, most-claimed first: DUKE×2 CAPT×1.
func (g *gameScreen) badges(a *App, seat int) string {
	m := g.chron.claims[seat]
	if len(m) == 0 {
		return a.t.Faint.Render("no claims yet")
	}
	type kv struct {
		c engine.Character
		n int
	}
	var list []kv
	for c, n := range m {
		list = append(list, kv{c, n})
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].n != list[j].n {
			return list[i].n > list[j].n
		}
		return list[i].c < list[j].c
	})
	var parts []string
	for _, x := range list {
		parts = append(parts, lipgloss.NewStyle().Foreground(theme.CharColor(x.c)).Render(fmt.Sprintf("%s%s%d", abbr(x.c), a.t.G("×", "x"), x.n)))
	}
	return strings.Join(parts, " ")
}

// --- inspect ---------------------------------------------------------------

// inspectOrder is the order Tab cycles through: opponents clockwise, then you.
func (g *gameScreen) inspectOrder() []int {
	order := g.seatOrder()
	if g.me >= 0 {
		order = append(order, g.me)
	}
	return order
}

// nextInspect advances inspect mode; past the last player it closes.
func (g *gameScreen) nextInspect() {
	order := g.inspectOrder()
	if g.inspect < 0 {
		g.inspect = order[0]
		return
	}
	i := slices.Index(order, g.inspect)
	if i < 0 || i+1 >= len(order) {
		g.inspect = -1
		return
	}
	g.inspect = order[i+1]
}

func (g *gameScreen) inspectView(a *App) string {
	t := a.t
	seat := g.inspect
	if seat < 0 || seat >= len(g.v.Players) {
		return ""
	}
	p := g.v.Players[seat]
	name := g.seats[seat].Name
	tag := "human"
	if g.seats[seat].Bot {
		tag = strings.ToUpper(g.seats[seat].Level) + " bot"
	}
	if seat == g.me {
		tag = "you"
	}
	var rev []string
	for _, c := range p.Revealed {
		rev = append(rev, lipgloss.NewStyle().Foreground(theme.CharColor(c)).Strikethrough(true).Render(c.Title()))
	}
	revealed := t.Faint.Render("none")
	if len(rev) > 0 {
		revealed = strings.Join(rev, " ")
	}
	status := fmt.Sprintf("%d%s · influence %d", p.Coins, t.Coin(), p.Hidden)
	if !p.Alive {
		status = "[ DISCONNECTED ]"
	}
	lines := []string{
		t.Title.Render("INSPECT // "+name) + t.Muted.Render("  "+tag),
		t.Text.Render(status),
		t.Muted.Render("revealed  ") + revealed,
		t.Muted.Render("claims    ") + g.badges(a, seat),
		"",
		t.Accent.Render("HISTORY"),
	}
	hist := g.chron.history[seat]
	room := max(3, a.h-len(lines)-10)
	if len(hist) > room {
		lines = append(lines, t.Faint.Render(fmt.Sprintf("  (%d earlier)", len(hist)-room+1)))
		hist = hist[len(hist)-room+1:]
	}
	if len(hist) == 0 {
		lines = append(lines, t.Faint.Render("  nothing yet"))
	}
	for _, h := range hist {
		lines = append(lines, t.Text.Render(trunc(h, 64)))
	}
	lines = append(lines, "", t.Faint.Render("tab: next player · esc: close"))
	return lipgloss.NewStyle().Border(t.RoundBorder).BorderForeground(theme.Cyan).Background(theme.Panel).Padding(1, 3).
		Render(t.A(strings.Join(lines, "\n")))
}

// --- first-game hints --------------------------------------------------------

// hints are shown the first time each situation comes up.
var hints = map[string]string{
	"turn":           "Your turn: pick an action with 1-8. Green ones you can back up; magenta~ ones would be bluffs. Anyone may challenge a claim.",
	"challenge":      "Someone claimed a character. Challenge (c) if you think they're bluffing: if they can show it, YOU lose an influence. The tracker's UNSEEN 0 means a sure bluff.",
	"block":          "You may block (b) by claiming the blocking character, held or not. Blocks can be challenged like any claim.",
	"targeted":       "This is aimed at you. Block it if you can (or dare); otherwise it goes through.",
	"blockchallenge": "Someone blocked. Challenge (c) the block if you think it's a bluff; if they have the card, you lose an influence.",
	"prove":          "You've been challenged. Flip the claimed card to win (the challenger loses an influence), or flip another card to concede.",
	"lose":           "Pick the card to give up. It stays face up for the rest of the game.",
	"exchange":       "Exchange: choose which cards to keep (1-4, then enter). Nobody else sees what you drew or returned.",
	"coup7":          "You have 7 coins: Coup (3) removes an influence and can't be blocked or challenged.",
	"coup10":         "At 10 coins you must Coup this turn.",
}

// hintFor picks the hint for the decision in front of you, if it's new.
func (g *gameScreen) hintFor(a *App) string {
	if a.cfg.NoHints || g.tutor != nil || g.me < 0 || !g.awaitingMe() {
		return ""
	}
	v := g.v
	var key string
	switch v.Phase {
	case engine.PhaseTurnStart:
		switch me := v.Players[g.me]; {
		case me.Coins >= engine.MustCoupAt:
			key = "coup10"
		case me.Coins >= engine.CoupCost:
			key = "coup7"
		default:
			key = "turn"
		}
	case engine.PhaseChallengeWindow:
		key = "challenge"
	case engine.PhaseBlockWindow:
		key = "block"
		if v.Pending != nil && v.Pending.Target == g.me {
			key = "targeted"
		}
	case engine.PhaseBlockChallengeWindow:
		key = "blockchallenge"
	case engine.PhaseProveClaim:
		key = "prove"
	case engine.PhaseLoseInfluence:
		key = "lose"
	case engine.PhaseExchange:
		key = "exchange"
	}
	if key == "" || a.cfg.HintsSeen[key] {
		return ""
	}
	return key
}

// updateHint shows a new hint when a decision opens, and remembers it.
func (g *gameScreen) updateHint(a *App) {
	if g.hintVer == g.v.Version && g.hint != "" {
		return
	}
	g.hint = ""
	key := g.hintFor(a)
	if key == "" {
		return
	}
	g.hint, g.hintVer = key, g.v.Version
	if a.cfg.HintsSeen == nil {
		a.cfg.HintsSeen = map[string]bool{}
	}
	a.cfg.HintsSeen[key] = true
	a.saveConfig()
}

// hintBox draws the current hint, or "" when there is none to show.
func (g *gameScreen) hintBox(a *App) string {
	if g.hint == "" || g.hintVer != g.v.Version || !g.awaitingMe() {
		return ""
	}
	t := a.t
	body := t.Warn.Render("HINT ") + t.Text.Render(wrap(hints[g.hint], 52)) + "\n" + t.Faint.Render("(hints: on/off in settings, esc)")
	return lipgloss.NewStyle().Border(t.RoundBorder).BorderForeground(theme.Acid).Background(theme.Panel).Padding(0, 1).Render(t.A(body))
}
