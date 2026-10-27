package client

import (
	"fmt"
	"image/color"
	"slices"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/v1rushb/neoncoup/client/art"
	"github.com/v1rushb/neoncoup/client/theme"
	"github.com/v1rushb/neoncoup/engine"
)

// Focus: what matters right now. The banner announces each beat, the context
// bar says in one sentence what is happening, what you can do and what each
// choice risks, and panels outside the current beat are dimmed.

const turnAlertDur = 2 * time.Second

// charName is a character's name in sentence case, e.g. "Duke".
func charName(c engine.Character) string {
	s := c.String()
	if s == "" || c == engine.NoCharacter {
		return "?"
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// actName is an action's name in sentence case, e.g. "Foreign Aid".
func actName(a engine.Action) string {
	words := strings.Fields(strings.ToLower(a.Title()))
	for i, w := range words {
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	return strings.Join(words, " ")
}

// who names a seat in a sentence: "you" for the viewer.
func (g *gameScreen) who(seat int) string {
	if seat == g.me && seat >= 0 {
		return "you"
	}
	return g.name(seat)
}

// Who is who() at the start of a sentence.
func (g *gameScreen) Who(seat int) string {
	if seat == g.me && seat >= 0 {
		return "You"
	}
	return g.name(seat)
}

// effect describes what an action does, e.g. "take 3 coins".
func (g *gameScreen) effect(a *App, p *engine.PendingView) string {
	coin := a.t.Coin()
	switch p.Action {
	case engine.Income:
		return "take 1 coin"
	case engine.ForeignAid:
		return "take 2 coins of foreign aid"
	case engine.Coup:
		return "coup " + g.who(p.Target)
	case engine.Tax:
		return "take 3 coins"
	case engine.Assassinate:
		return "assassinate " + g.who(p.Target)
	case engine.Steal:
		return "steal 2 coins from " + g.who(p.Target)
	case engine.Exchange:
		return "swap cards with the court deck"
	case engine.Audit:
		return fmt.Sprintf("audit %s (-2%s)", g.who(p.Target), coin)
	}
	return strings.ToLower(p.Action.Title())
}

// claimSentence is "VEX claims Duke to take 3 coins.", or a plain action.
func (g *gameScreen) claimSentence(a *App, p *engine.PendingView) string {
	if p.Claim != engine.NoCharacter {
		return fmt.Sprintf("%s claims %s to %s.", g.Who(p.Actor), charName(p.Claim), g.effect(a, p))
	}
	verb := map[engine.Action]string{engine.Income: "takes income", engine.ForeignAid: "asks for foreign aid (+2" + a.t.Coin() + ")",
		engine.Coup: "launches a coup on " + g.who(p.Target)}[p.Action]
	if verb == "" {
		verb = g.effect(a, p)
	}
	return g.Who(p.Actor) + " " + verb + "."
}

// bannerText is the headline for a beat, segments joined by ▸.
func (g *gameScreen) bannerText(a *App, e engine.Event) (string, color.Color) {
	t := a.t
	sep := " " + t.G("▸", ">") + " "
	coin := t.Coin()
	up := func(seat int) string { return strings.ToUpper(g.name(seat)) }
	// v conjugates for the subject: VEX SHOWS, YOU SHOW.
	v := func(seat int, verb string) string {
		if seat == g.me && seat >= 0 {
			return strings.TrimSuffix(verb, "S")
		}
		return verb
	}
	poss := func(seat int) string {
		if seat == g.me && seat >= 0 {
			return "YOUR"
		}
		return up(seat) + "'S"
	}
	switch e.Kind {
	case engine.EvActionDeclared:
		parts := []string{up(e.Seat)}
		if e.Claim != engine.NoCharacter {
			parts = append(parts, v(e.Seat, "CLAIMS")+" "+e.Claim.Title())
		}
		act := e.Action.Title()
		switch e.Action {
		case engine.Income:
			act += " +1" + coin
		case engine.ForeignAid:
			act += " +2" + coin
		case engine.Tax:
			act += " +3" + coin
		case engine.Steal:
			act += " 2" + coin
		case engine.Audit:
			act += " -2" + coin
		case engine.Coup:
			act += " -7" + coin
		case engine.Assassinate:
			act += " -3" + coin
		}
		parts = append(parts, act)
		if e.Other >= 0 {
			parts = append(parts, up(e.Other))
		}
		col := theme.Magenta
		if e.Claim != engine.NoCharacter {
			col = theme.CharColor(e.Claim)
		} else if e.Action == engine.Coup {
			col = theme.Red
		}
		return strings.Join(parts, sep), col
	case engine.EvChallenge:
		return up(e.Seat) + sep + v(e.Seat, "CHALLENGES") + sep + poss(e.Other) + " " + e.Claim.Title(), theme.Red
	case engine.EvBlock:
		return up(e.Seat) + sep + v(e.Seat, "BLOCKS") + " WITH " + e.Claim.Title(), theme.CharColor(e.Claim)
	case engine.EvClaimProven:
		return up(e.Seat) + " " + v(e.Seat, "SHOWS") + " " + e.Card.Title() + sep + "CLAIM VERIFIED", theme.Green
	case engine.EvClaimFailed:
		return "BLUFF CAUGHT" + sep + up(e.Seat) + " " + v(e.Seat, "FLIPS") + " " + e.Card.Title(), theme.Red
	case engine.EvCardReplaced:
		return up(e.Seat) + " " + v(e.Seat, "SHUFFLES") + " IT BACK" + sep + v(e.Seat, "DRAWS") + " A NEW CARD", theme.Purple
	case engine.EvInfluenceLost:
		return up(e.Seat) + " " + v(e.Seat, "LOSES") + " AN INFLUENCE" + sep + e.Card.Title() + " EXPOSED", theme.Red
	case engine.EvEliminated:
		return up(e.Seat) + sep + "FLATLINED", theme.Red
	case engine.EvActionBlocked:
		return "BLOCK HOLDS" + sep + poss(e.Seat) + " " + e.Action.Title() + " FIZZLES", theme.Acid
	case engine.EvActionFailed:
		if e.Reason == engine.ReasonNoTarget {
			return e.Action.Title() + " HAS NO TARGET LEFT", theme.Dim
		}
		return poss(e.Seat) + " " + e.Action.Title() + " IS VOIDED", theme.Red
	case engine.EvExchangeDraw:
		return up(e.Seat) + " " + v(e.Seat, "DRAWS") + fmt.Sprintf(" %d FROM THE COURT", e.Amount), theme.Green
	case engine.EvExchangeDone:
		return up(e.Seat) + " " + v(e.Seat, "RETURNS") + fmt.Sprintf(" %d CARDS", e.Amount), theme.Green
	case engine.EvCoins:
		amt := fmt.Sprintf("%+d%s", e.Amount, coin)
		if e.Reason == engine.ReasonSteal {
			return up(e.Seat) + " " + v(e.Seat, "LIFTS") + " " + fmt.Sprintf("%d%s", e.Amount, coin) + " FROM " + up(e.Other), theme.Acid
		}
		return up(e.Seat) + " " + amt, theme.Acid
	case engine.EvTurnStart:
		if e.Seat == g.me {
			return fmt.Sprintf("TURN %d", e.Turn) + sep + "YOUR MOVE", theme.Cyan
		}
		return fmt.Sprintf("TURN %d", e.Turn) + sep + up(e.Seat), theme.Magenta
	}
	return "", nil
}

// bigBeat reports whether a beat gets the large banner.
func bigBeat(k engine.EventKind) bool {
	switch k {
	case engine.EvActionDeclared, engine.EvChallenge, engine.EvBlock, engine.EvClaimFailed, engine.EvEliminated:
		return true
	}
	return false
}

// banner renders the current beat's headline, or "" when nothing is playing.
func (g *gameScreen) banner(a *App, maxW int) string {
	b := g.beat
	if b == nil {
		return ""
	}
	text, col := g.bannerText(a, b.ev)
	switch b.ev.Kind {
	case engine.EvCoins, engine.EvCardReplaced, engine.EvExchangeDraw, engine.EvExchangeDone:
		return "" // these play out across the table; the banner would cover them
	}
	if text == "" {
		return ""
	}
	t := a.t
	text = t.A(text)
	if !a.cfg.ReduceMotion {
		if p := b.progress(g.clock(a)); p < 0.12 {
			text = art.Glitch(text, 0.6*(1-p/0.12), a.rng)
		}
	}
	st := lipgloss.NewStyle().Foreground(col).Bold(true)
	box := lipgloss.NewStyle().Border(t.RoundBorder).BorderForeground(col).Background(theme.Panel).Padding(0, 2)
	if bigBeat(b.ev.Kind) {
		box = lipgloss.NewStyle().Border(t.ThickBorder).BorderForeground(col).Background(theme.Panel).Padding(1, 4)
	}
	inner := maxW - 2 - box.GetHorizontalPadding()
	if inner < 10 {
		return ""
	}
	return box.Render(st.Render(trunc(text, inner)))
}

// focusSeats are the seats a beat is about.
func focusSeats(e engine.Event) []int {
	var out []int
	for _, s := range []int{e.Seat, e.Other} {
		if s >= 0 {
			out = append(out, s)
		}
	}
	return out
}

// dimmed reports whether a seat's panel is outside the current beat.
func (g *gameScreen) dimmed(seat int) bool {
	if g.beat == nil {
		return false
	}
	return !slices.Contains(focusSeats(g.beat.ev), seat)
}

// actionReason explains why an action is not available on your turn, or "".
func (g *gameScreen) actionReason(a *App, act engine.Action) string {
	if g.me < 0 || g.me >= len(g.v.Players) {
		return ""
	}
	me := g.v.Players[g.me]
	coin := a.t.Coin()
	switch {
	case me.Coins >= engine.MustCoupAt && act != engine.Coup:
		return fmt.Sprintf("must Coup at %d%s", engine.MustCoupAt, coin)
	case me.Coins < act.Cost():
		return fmt.Sprintf("%s needs %d%s", actName(act), act.Cost(), coin)
	case act.Targeted() && len(g.targets(act)) == 0:
		if act == engine.Audit {
			return fmt.Sprintf("no target with %d+%s", engine.AuditThreshold, coin)
		}
		return "no target"
	}
	return ""
}

// option is one choice in the context bar, with what it risks in full and
// in a few words.
type option struct {
	key, label, risk, short string
}

// risk levels for renderOptions.
const (
	riskNone = iota
	riskShort
	riskFull
)

func (g *gameScreen) renderOptions(a *App, opts []option, level int) string {
	t := a.t
	var parts []string
	for _, o := range opts {
		s := t.Key.Render("["+o.key+"]") + " " + t.Text.Render(o.label)
		r := ""
		switch level {
		case riskFull:
			r = o.risk
		case riskShort:
			r = o.short
		}
		if r != "" {
			s += " " + t.Muted.Render("("+r+")")
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, "  ")
}

// choices lays out a sentence and options, dropping detail until it fits.
func (g *gameScreen) choices(a *App, w int, sentence string, opts []option) string {
	timer := func(cells int) string {
		if c := g.timerChip(a, cells); c != "" {
			return "  " + c
		}
		return ""
	}
	var cands []string
	for _, lvl := range []int{riskFull, riskShort, riskNone} {
		cands = append(cands, sentence+"  "+g.renderOptions(a, opts, lvl)+timer(9))
	}
	for _, lvl := range []int{riskFull, riskShort, riskNone} {
		cands = append(cands, g.renderOptions(a, opts, lvl)+timer(6))
	}
	for _, c := range cands {
		if lipgloss.Width(c) <= w {
			return c
		}
	}
	return cands[len(cands)-1]
}

// article is "a" or "an" for a character's name.
func article(c engine.Character) string {
	if strings.ContainsRune("AEIOU", rune(charName(c)[0])) {
		return "an"
	}
	return "a"
}

// timerChip is the context bar's countdown: a draining bar and seconds left.
func (g *gameScreen) timerChip(a *App, cells int) string {
	if g.deadline.IsZero() || g.timerTotal <= 0 || g.beat != nil {
		return ""
	}
	left := max(0, g.deadline.Sub(g.clock(a)))
	frac := min(1, left.Seconds()/g.timerTotal.Seconds())
	filled := int(frac*float64(cells) + 0.5)
	col := theme.Cyan
	switch {
	case frac < 0.35:
		col = theme.Red
	case frac < 0.6:
		col = theme.Acid
	}
	return lipgloss.NewStyle().Foreground(col).Render(strings.Repeat(a.t.G("█", "#"), filled)) +
		a.t.Faint.Render(strings.Repeat(a.t.G("░", "."), cells-filled)) +
		a.t.Text.Render(fmt.Sprintf(" %ds", int(left.Seconds()+0.99)))
}

// contextLine builds the context bar sentence for width w, trying the
// fullest wording first.
func (g *gameScreen) contextLine(a *App, w int) string {
	t := a.t
	v := g.v
	if !g.have {
		return t.Muted.Render("syncing...")
	}
	timer := func(cells int) string {
		if c := g.timerChip(a, cells); c != "" {
			return "  " + c
		}
		return ""
	}
	pick := func(cands ...string) string {
		for _, c := range cands {
			if lipgloss.Width(c) <= w {
				return c
			}
		}
		return cands[len(cands)-1]
	}
	if g.paused {
		return t.Warn.Render("PAUSED") + t.Muted.Render(" // bots and timers are frozen. P to resume.")
	}
	if b := g.beat; b != nil {
		text, _ := g.bannerText(a, b.ev)
		line := t.Bold.Render(t.A(text))
		if g.soloControls(a) {
			return pick(line+t.Faint.Render("   [space] skip  [P] pause"), line)
		}
		return line
	}
	if g.presenting() {
		return t.Muted.Render("...")
	}
	if g.me < 0 {
		return t.Accent.Render("SPECTATING") + t.Muted.Render(" // the table can't see you") + timer(10)
	}
	me := v.Players[g.me]
	if !me.Alive {
		return t.Loss.Render("FLATLINED") + t.Muted.Render(" // you're watching the rest of the run")
	}
	pend := v.Pending
	if !g.awaitingMe() {
		var names []string
		for _, s := range v.Awaiting {
			names = append(names, g.name(s))
		}
		waiting := t.Muted.Render("waiting on "+strings.Join(names, ", ")+" ") + a.spinner()
		if g.sent {
			waiting = t.Muted.Render("sent. waiting for the table ") + a.spinner()
		}
		if pend != nil && v.Phase.Timed() {
			s := t.Text.Render(g.claimSentence(a, pend)) + "  "
			return pick(s+waiting+timer(10), waiting+timer(10), waiting)
		}
		switch {
		case v.Phase == engine.PhaseTurnStart && len(v.Awaiting) == 1:
			waiting = t.Muted.Render(g.name(v.Current)+" is choosing a move ") + a.spinner()
		case v.Phase == engine.PhaseProveClaim && v.Challenge != nil:
			c := v.Challenge
			s := t.Warn.Render(fmt.Sprintf("%s challenged %s %s.", g.Who(c.Challenger), possessive(g, c.Claimant), charName(c.Claim)))
			return pick(s+t.Muted.Render(fmt.Sprintf(" %s must flip a card: the %s proves it. ", g.Who(c.Claimant), charName(c.Claim)))+a.spinner(), s+" "+a.spinner())
		case v.Phase == engine.PhaseLoseInfluence && v.Loser >= 0:
			waiting = t.Loss.Render(g.Who(v.Loser)+" is choosing a card to lose ") + a.spinner()
		case v.Phase == engine.PhaseExchange:
			waiting = t.Accent.Render(g.Who(v.Current)+" is swapping cards with the deck ") + a.spinner()
		}
		return pick(waiting+timer(10), waiting)
	}

	switch v.Phase {
	case engine.PhaseTurnStart:
		head := t.Hot.Render("YOUR TURN") + t.Text.Render(fmt.Sprintf(" with %d%s.", me.Coins, t.Coin()))
		if g.mode == modeTarget {
			return t.Hot.Render("TARGET FOR "+g.pending.Title()+":") + t.Muted.Render(" pick a runner below, esc cancels")
		}
		if me.Coins >= engine.MustCoupAt {
			return head + t.Warn.Render(" 10+ coins: you must Coup (3).")
		}
		var why []string
		for _, act := range actionKeys {
			if act == engine.Audit && !v.Magistrate {
				continue
			}
			if r := g.actionReason(a, act); r != "" {
				why = append(why, r)
			}
		}
		legend := t.Muted.Render(" Green: you hold it. Magenta~: a bluff.")
		dim := t.Faint.Render(" " + strings.Join(why, " · "))
		return pick(head+legend+dim, head+dim, head+legend, head)
	case engine.PhaseChallengeWindow:
		s := t.Warn.Render(g.claimSentence(a, pend))
		if g.noCopiesLeft(pend.Claim) {
			s += t.Loss.Render(" No copies left: it's a bluff!")
		}
		risk := fmt.Sprintf("if %s %s %s %s, you lose an influence", g.who(pend.Actor), has(g, pend.Actor), article(pend.Claim), charName(pend.Claim))
		opts := []option{{"C", "Challenge", risk, "wrong: you lose 1"}, {"P", "Pass", "", ""}}
		return g.choices(a, w, s, opts)
	case engine.PhaseBlockWindow:
		var s string
		if pend.Target == g.me {
			s = t.Warn.Render(fmt.Sprintf("%s's %s is aimed at you.", g.Who(pend.Actor), actName(pend.Action)))
		} else {
			s = t.Warn.Render(g.claimSentence(a, pend))
		}
		var opts []option
		for _, c := range pend.Action.Blockers(v.Magistrate) {
			risk := fmt.Sprintf("if challenged without %s %s, you lose an influence", article(c), charName(c))
			short := "caught: you lose 1"
			if pend.Action == engine.Assassinate {
				risk = fmt.Sprintf("caught without %s %s, you lose both", article(c), charName(c))
				short = "caught: you lose both"
			}
			label := "Block with " + charName(c)
			if !handHas(v.Hand, c) {
				label += t.G("~", "?")
			}
			opts = append(opts, option{"B", label, risk, short})
		}
		allow := ""
		switch {
		case pend.Action == engine.Assassinate && pend.Target == g.me:
			allow = "you lose an influence"
		case pend.Action == engine.Steal && pend.Target == g.me:
			allow = "they take 2" + t.Coin()
		}
		opts = append(opts, option{"P", "Allow", allow, allow})
		if len(opts) > 2 {
			// Several blockers share one key; the card is picked next.
			opts = []option{{"B", "Block", "pick the card next; if challenged without it, you lose an influence", "caught: you lose 1"}, opts[len(opts)-1]}
		}
		if g.mode == modeBlock {
			return t.Warn.Render("BLOCK WITH:") + t.Muted.Render(" pick the card below, esc cancels")
		}
		return g.choices(a, w, s, opts)
	case engine.PhaseBlockChallengeWindow:
		s := t.Warn.Render(fmt.Sprintf("%s blocks with %s.", g.Who(pend.Blocker), charName(pend.BlockClaim)))
		if g.noCopiesLeft(pend.BlockClaim) {
			s += t.Loss.Render(" No copies left: it's a bluff!")
		}
		risk := fmt.Sprintf("if %s %s %s %s, you lose an influence", g.who(pend.Blocker), has(g, pend.Blocker), article(pend.BlockClaim), charName(pend.BlockClaim))
		opts := []option{{"C", "Challenge the block", risk, "wrong: you lose 1"}, {"P", "Accept", "", ""}}
		return g.choices(a, w, s, opts)
	case engine.PhaseProveClaim:
		c := v.Challenge
		if c == nil {
			break
		}
		s := t.Loss.Render(fmt.Sprintf("%s challenges your %s.", g.Who(c.Challenger), charName(c.Claim)))
		help := t.Text.Render(fmt.Sprintf(" Flip the %s to prove it (%s loses an influence), or any other card to concede.", charName(c.Claim), g.who(c.Challenger)))
		return pick(s+help, s+t.Text.Render(" Flip a card: the "+charName(c.Claim)+" proves it."), s)
	case engine.PhaseLoseInfluence:
		return t.Loss.Render("You lose an influence.") + t.Text.Render(" Pick the card to give up; it stays face up for everyone.")
	case engine.PhaseExchange:
		return t.Accent.Render(fmt.Sprintf("EXCHANGE: keep %d of %d.", me.Hidden, len(v.ExchangeOptions))) +
			t.Muted.Render(" The rest go back into the deck. Only you see these.")
	}
	return ""
}

// has is "has" for others and "have" for you.
func has(g *gameScreen, seat int) string {
	if seat == g.me {
		return "have"
	}
	return "has"
}

// noCopiesLeft reports whether a claim can only be a bluff from where you sit.
func (g *gameScreen) noCopiesLeft(c engine.Character) bool {
	if c == engine.NoCharacter || !g.have {
		return false
	}
	return courtCount(g.v)[c].Unseen == 0
}

// alerting reports whether the your-turn pulse is running.
func (g *gameScreen) alerting(a *App) bool {
	return !g.turnAlert.IsZero() && a.now.Sub(g.turnAlert) < turnAlertDur && !a.cfg.ReduceMotion
}

// possessive is "VEX's" or "your".
func possessive(g *gameScreen, seat int) string {
	if seat == g.me {
		return "your"
	}
	return g.name(seat) + "'s"
}
