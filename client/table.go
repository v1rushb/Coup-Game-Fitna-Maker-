package client

import (
	"fmt"
	"image/color"
	"math"
	"slices"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/v1rushb/neoncoup/client/art"
	"github.com/v1rushb/neoncoup/client/theme"
	"github.com/v1rushb/neoncoup/engine"
	"github.com/v1rushb/neoncoup/protocol"
)

const (
	oppH     = 6
	maxTopW  = 34
	chatRows = 7
	// wantCenterW fits the card tracker's full row of mini cards; side
	// panels narrow before the center does.
	wantCenterW = 78
)

// layout picks panel widths for the terminal size.
func layout(w int) (logW, sideW int) {
	logW = max(30, min(44, w*28/100))
	sideW = 24
	if w-logW-2*sideW < wantCenterW {
		sideW = max(18, (w-logW-wantCenterW)/2)
	}
	return logW, sideW
}

func (g *gameScreen) view(a *App) string {
	W, H := a.w, a.h
	if !g.have || len(g.v.Players) != len(g.seats) {
		return a.centerBox("SYNCING", a.spinner()+" "+a.t.Muted.Render("pulling the table state..."))
	}
	logW, _ := layout(W)
	mainW := W - logW
	bodyH := H - 3
	top := g.topBar(a, W)
	main := g.tableArea(a, mainW, bodyH)
	side := g.sidePanel(a, logW, bodyH)
	out := lipgloss.JoinVertical(lipgloss.Left, top, lipgloss.JoinHorizontal(lipgloss.Top, main, side), g.actionBar(a, W))
	out = fit(out, W, H)
	if bn := g.banner(a, mainW-2); bn != "" {
		// Centered over the middle of the table, between the rivals and your hand.
		bw, bh := lipgloss.Size(bn)
		handH := handHeight(bodyH)
		midH := bodyH - oppH - handH
		out = overlayAt(out, bn, max(0, (mainW-bw)/2), 1+oppH+max(0, (midH-bh)/2))
	}
	out = g.sprites(a, out)
	if !g.myDeath.IsZero() && a.now.Sub(g.myDeath) < deathDur && g.over == nil {
		out = g.deathScreen(a, out)
	}
	if g.mode == modeTaunt {
		out = overlay(out, g.tauntMenu(a), W, H)
	}
	if hb := g.hintBox(a); hb != "" && g.inspect < 0 && g.prefs == nil {
		_, hh := lipgloss.Size(hb)
		out = overlayAt(out, hb, 1, H-2-hh)
	}
	if g.inspect >= 0 {
		out = overlay(out, g.inspectView(a), W, H)
	}
	if g.prefs != nil {
		out = overlay(out, g.prefs.view(a), W, H)
	}
	if g.paused {
		box := lipgloss.NewStyle().Border(a.t.ThickBorder).BorderForeground(theme.Acid).Background(theme.Panel).Padding(1, 4).
			Render(a.t.Warn.Render("PAUSED") + "\n\n" + a.t.Muted.Render("bots and timers are frozen") + "\n" + a.t.Faint.Render("P to resume"))
		out = overlay(out, box, W, H)
	}
	if g.tutor != nil {
		out = g.tutor.overlay(a, g, out)
	}
	if g.over != nil {
		out = overlay(out, g.resultsView(a), W, H)
	}
	return out
}

// --- top bar -----------------------------------------------------------------

func (g *gameScreen) topBar(a *App, w int) string {
	t := a.t
	room := ""
	if a.conn != nil && a.mode != "solo" && a.mode != "tutorial" {
		room = "  ROOM " + a.conn.Room
	}
	left := t.Title.Render(" NEON//COUP ") + t.Muted.Render(fmt.Sprintf("%s  GAME %d  TURN %d", room, g.gs.Game, g.v.Turn))
	if g.v.Phase != engine.PhaseGameOver && g.v.Current >= 0 && g.v.Current < len(g.seats) {
		if g.v.Current == g.me {
			st := lipgloss.NewStyle().Foreground(theme.Bg).Background(theme.Cyan).Bold(true)
			if g.alerting(a) && (a.now.Sub(g.turnAlert)/(250*time.Millisecond))%2 == 1 {
				st = st.Background(theme.White)
			}
			left += "  " + st.Render(" "+t.G("▶", ">")+" YOUR TURN ")
		} else {
			left += "  " + t.Hot.Render(t.G("▶ ", "> ")+g.name(g.v.Current))
		}
	}
	if g.v.Magistrate {
		left += t.Warn.Render("  MAGISTRATE")
	}
	if g.me < 0 {
		left += t.Accent.Render("  SPECTATING")
	}
	right := a.muteIcon() + t.Faint.Render("  ? help  q leave ")
	gap := max(1, w-lipgloss.Width(left)-lipgloss.Width(right))
	return lipgloss.NewStyle().Background(theme.Panel).Render(left + strings.Repeat(" ", gap) + right)
}

// --- table -------------------------------------------------------------------

// seatOrder lists the other seats clockwise starting after the viewer.
func (g *gameScreen) seatOrder() []int {
	n := len(g.seats)
	var out []int
	start := g.me
	if start < 0 {
		start = n - 1
	}
	for i := 1; i <= n; i++ {
		s := (start + i) % n
		if s != g.me {
			out = append(out, s)
		}
	}
	return out
}

func (g *gameScreen) tableArea(a *App, w, h int) string {
	_, sideW := layout(a.w)
	others := g.seatOrder()
	handH := handHeight(h)
	compact := handH == art.CompactH
	midH := h - oppH - handH

	left, right := -1, -1
	topSeats := others
	if len(others) >= 4 {
		left, right = others[0], others[len(others)-1]
		topSeats = others[1 : len(others)-1]
	}
	var boxes []string
	if len(topSeats) > 0 {
		bw := min(maxTopW, w/len(topSeats))
		for _, s := range topSeats {
			boxes = append(boxes, g.oppBox(a, s, bw, oppH))
		}
	}
	topRow := lipgloss.PlaceHorizontal(w, lipgloss.Center, lipgloss.JoinHorizontal(lipgloss.Top, boxes...))
	topRow = fit(topRow, w, oppH)

	centerW := w
	var mid []string
	if left >= 0 {
		mid = append(mid, lipgloss.PlaceVertical(midH, lipgloss.Center, g.oppBox(a, left, sideW, min(midH, oppH+2))))
		centerW -= sideW
	}
	if right >= 0 {
		centerW -= sideW
	}
	mid = append(mid, g.terminal(a, centerW, midH))
	if right >= 0 {
		mid = append(mid, lipgloss.PlaceVertical(midH, lipgloss.Center, g.oppBox(a, right, sideW, min(midH, oppH+2))))
	}
	midRow := fit(lipgloss.JoinHorizontal(lipgloss.Top, mid...), w, midH)
	handRow := fit(g.handRow(a, w, handH, compact), w, handH)
	return lipgloss.JoinVertical(lipgloss.Left, topRow, midRow, handRow)
}

// handHeight is the hand row's height for a table area h rows tall.
func handHeight(h int) int {
	if h-oppH-art.CardH < 9 {
		return art.CompactH
	}
	return art.CardH
}

func (g *gameScreen) passed(seat int) bool {
	if !g.v.Phase.Timed() {
		return false
	}
	// A seat that was asked and is no longer awaited in this window passed.
	return !slices.Contains(g.v.Awaiting, seat) && g.askedThisWindow(seat)
}

func (g *gameScreen) askedThisWindow(seat int) bool {
	p := g.v.Pending
	if p == nil || !g.v.Players[seat].Alive {
		return false
	}
	switch g.v.Phase {
	case engine.PhaseChallengeWindow:
		return seat != p.Actor
	case engine.PhaseBlockChallengeWindow:
		return seat != p.Blocker
	case engine.PhaseBlockWindow:
		if p.Action.BlockedByTargetOnly() {
			return seat == p.Target
		}
		return seat != p.Actor
	}
	return false
}

func (g *gameScreen) oppBox(a *App, seat, w, h int) string {
	t := a.t
	if seat >= len(g.v.Players) {
		return ""
	}
	p := g.v.Players[seat]
	if g.eliminating(a, seat) {
		p.Alive = false // the glitch is over: show what's left of them
	}
	inner := w - 4
	now := a.now
	awaited := slices.Contains(g.v.Awaiting, seat)

	pend := g.v.Pending
	border := theme.Faint
	switch {
	case !p.Alive:
		border = theme.Faint
	case g.offline[seat]:
		border = theme.Red
	case pend != nil && pend.Target == seat && pend.Action.Targeted():
		border = theme.Red // the action is aimed here
	case awaited:
		border = theme.Cyan
	case g.v.Current == seat:
		border = theme.Magenta
	}
	dim := g.dimmed(seat)
	if dim {
		border = theme.Faint
	}
	if ts, ok := g.oppFx[seat]; ok && now.Sub(ts) < flashDur && (now.Sub(ts)/(150*time.Millisecond))%2 == 0 {
		border = theme.White
	}

	// Line 1: marker + name + tag.
	// The mover always wears the ▶; anyone else being waited on spins.
	marker := "  "
	if g.v.Current == seat && p.Alive {
		marker = t.Hot.Render(t.G("▶ ", "> "))
	} else if awaited && p.Alive {
		marker = a.spinner() + " "
	}
	nameSt := t.Bold
	if !p.Alive {
		nameSt = t.Faint.Strikethrough(true)
	}
	tagTxt := ""
	if seat < len(g.seats) && g.seats[seat].Bot {
		tagTxt = strings.ToUpper(g.seats[seat].Level)
	}
	if g.offline[seat] {
		tagTxt = "OFFLINE"
	}
	if inner-2-len(tagTxt) < 9 {
		tagTxt = "" // narrow box: the name matters more
	}
	nameW := inner - 2 - len(tagTxt) - 1
	line1 := marker + nameSt.Render(trunc(g.seats[seat].Name, max(4, nameW)))
	line1 = padTo(line1, inner-len(tagTxt)) + t.Faint.Render(tagTxt)

	// Line 2: coins, face-up cards and influence pips; a flatlined runner
	// is just gone.
	coins := g.coinText(a, seat)
	if !p.Alive {
		coins = t.Faint.Render("[ DISCONNECTED ]")
		if inner < 18 {
			coins = t.Faint.Render("[DISCONNECTED]")
		}
	} else {
		for _, c := range p.Revealed {
			tag := " " + lipgloss.NewStyle().Foreground(theme.Red).Strikethrough(true).Render(t.G("✖", "x")+abbr(c))
			if lipgloss.Width(coins+tag)+3 <= inner {
				coins += tag
			}
		}
	}
	pips := t.G(strings.Repeat("●", p.Hidden), strings.Repeat("o", p.Hidden)) + t.G(strings.Repeat("○", len(p.Revealed)), strings.Repeat(".", len(p.Revealed)))
	pipSt := lipgloss.NewStyle().Foreground(theme.Magenta)
	if !p.Alive {
		pipSt = t.Faint
	}
	line2 := padTo(coins, inner-lipgloss.Width(pips)) + pipSt.Render(pips)

	// Line 3: what they've claimed this game (their face-up cards once
	// they're out).
	line3 := g.badges(a, seat)
	if !p.Alive {
		var rev []string
		for _, c := range p.Revealed {
			rev = append(rev, lipgloss.NewStyle().Foreground(theme.CharColor(c)).Strikethrough(true).Render(t.G("✖", "x")+abbr(c)))
		}
		line3 = strings.Join(rev, " ")
	}

	// Line 4: status.
	line4 := g.seatStatus(a, seat, inner)

	content := strings.Join([]string{line1, line2, line3, line4}, "\n")
	glitching := false
	if b := g.beat; b != nil && b.ev.Kind == engine.EvEliminated && b.ev.Seat == seat && !a.cfg.ReduceMotion {
		// The panel tears apart for most of the elimination beat.
		if amt := 1 - span(b.progress(g.clock(a)), 0, 0.75); amt > 0 {
			content = lipgloss.NewStyle().Foreground(glitchColor(a)).Render(art.Glitch(ansi.Strip(content), amt, a.rng))
			border, glitching = glitchColor(a), true
		}
	} else if ts, ok := g.glitch[seat]; ok && now.Sub(ts) < glitchDur && g.beat == nil {
		amt := 1 - now.Sub(ts).Seconds()/glitchDur.Seconds()
		content = lipgloss.NewStyle().Foreground(glitchColor(a)).Render(art.Glitch(ansi.Strip(content), amt, a.rng))
		border, glitching = glitchColor(a), true
	}
	if dim && !glitching {
		content = t.Faint.Render(ansi.Strip(content))
	}
	st := lipgloss.NewStyle().Border(t.RoundBorder).BorderForeground(border).Padding(0, 1).Width(w).Height(h).MaxHeight(h)
	if g.v.Current == seat && p.Alive && !dim {
		st = st.Border(t.ThickBorder) // the active player is unmistakable
	}
	return st.Render(fitLines(content, inner, h-2))
}

func glitchColor(a *App) color.Color {
	return []color.Color{theme.Red, theme.Magenta, theme.Cyan, theme.White}[a.rng.IntN(4)]
}

func (g *gameScreen) seatStatus(a *App, seat, w int) string {
	t := a.t
	p := g.v.Players[seat]
	if g.eliminating(a, seat) {
		return ""
	}
	pend := g.v.Pending
	switch {
	case !p.Alive:
		return ""
	case g.v.Challenge != nil && g.v.Challenge.Claimant == seat:
		return t.Warn.Render("CHALLENGED! proving...")
	case g.v.Phase == engine.PhaseLoseInfluence && g.v.Loser == seat:
		return t.Loss.Render("losing influence...")
	case pend != nil && pend.Blocker == seat:
		return t.Warn.Render("blocks: " + pend.BlockClaim.Title())
	case pend != nil && pend.Actor == seat && pend.Claim != engine.NoCharacter:
		return t.Hot.Render("claims " + pend.Claim.Title())
	case pend != nil && pend.Target == seat:
		if w < 16 {
			return t.Loss.Render(t.G("◀ ", "< ") + "TARGET")
		}
		return t.Loss.Render(t.G("◀ ", "< ") + "TARGET: " + pend.Action.Title())
	case g.passed(seat):
		return t.Faint.Render("pass")
	case slices.Contains(g.v.Awaiting, seat):
		return t.Accent.Render("thinking...")
	case g.v.Current == seat:
		return t.Hot.Render("on the move")
	}
	return ""
}

func (g *gameScreen) coinText(a *App, seat int) string {
	t := a.t
	shown := int(math.Round(g.shown[seat]))
	st := lipgloss.NewStyle().Foreground(theme.Acid).Bold(true)
	if ts, ok := g.coinFx[seat]; ok && a.now.Sub(ts) < flashDur {
		if g.coinUp[seat] {
			st = t.Gain
		} else {
			st = t.Loss
		}
	}
	if !g.v.Players[seat].Alive {
		st = t.Faint
	}
	return st.Render(fmt.Sprintf("%s%d", t.Coin(), shown))
}

// --- center terminal -------------------------------------------------------

// terminal is the center of the table: what is on the table now, above the
// card tracker.
func (g *gameScreen) terminal(a *App, w, h int) string {
	t := a.t
	inner, innerH := w-4, h-2
	v := g.v
	var top []string
	add := func(st lipgloss.Style, text string) {
		top = append(top, wrapStyled(st, text, inner)...)
	}
	pend := v.Pending
	switch {
	case v.Phase == engine.PhaseGameOver:
		add(t.Gain, "GAME OVER")
	case pend == nil:
		if v.Current == g.me {
			add(t.Hot, "> YOUR TURN · choose an action below")
		} else {
			add(t.Text, "> "+g.name(v.Current)+" is choosing a move")
		}
	default:
		add(t.Bold, "> "+g.actionLine(a, pend))
		if pend.Blocker >= 0 {
			add(t.Warn, "  BLOCK: "+g.name(pend.Blocker)+" claims "+pend.BlockClaim.Title())
		}
		if c := v.Challenge; c != nil {
			add(t.Loss, fmt.Sprintf("  %s CHALLENGES %s's %s", g.name(c.Challenger), g.name(c.Claimant), c.Claim.Title()))
		}
	}
	var phase, bar []string
	if pl := g.phaseLine(a); pl != "" {
		phase = []string{pl}
	}
	if b := g.timerBar(a, inner); b != "" {
		bar = []string{b}
	}
	label := []string{"", t.Faint.Render(trunc("COURT TRACKER // revealed: face up · unseen: anywhere but your hand", inner))}
	cat := func(parts ...[]string) []string {
		var out []string
		for _, p := range parts {
			out = append(out, p...)
		}
		return out
	}
	head := top[:min(1, len(top))]
	var cands [][]string
	if cards, ok := g.trackerCards(a, inner, innerH-len(head)); ok {
		block := strings.Split(cards, "\n")
		cands = append(cands, cat(top, phase, bar, label, block), cat(top, phase, bar, block), cat(top, phase, block), cat(head, block))
	}
	bank := []string{t.Accent.Render(fmt.Sprintf("TREASURY %d%s · DECK %d", v.Treasury, t.Coin(), v.DeckCount))}
	if med, ok := g.trackerMedium(a, inner, innerH-len(head)-1); ok {
		cands = append(cands, cat(bank, top, phase, bar, label, med), cat(bank, top, phase, bar, med), cat(bank, top, phase, med), cat(bank, head, med))
	}
	tiles := g.trackerTiles(a, inner)
	cands = append(cands, cat(bank, top, phase, bar, label, tiles), cat(bank, top, phase, bar, tiles),
		cat(bank, top, phase, tiles), cat(bank, head, tiles), cat(head, tiles))
	lines := cands[len(cands)-1]
	for _, c := range cands {
		if len(c) <= innerH {
			lines = c
			break
		}
	}
	g.locate(lines)
	border := theme.Magenta
	if g.awaitingMe() {
		border = theme.Cyan
		if g.alerting(a) && (a.now.Sub(g.turnAlert)/(250*time.Millisecond))%2 == 1 {
			border = theme.White
		}
	}
	st := lipgloss.NewStyle().Border(t.ThickBorder).BorderForeground(border).Padding(0, 1).Width(w).Height(h)
	return st.Render(fitLines(strings.Join(lines, "\n"), inner, innerH))
}

// wrapStyled wraps plain text to width and styles each line.
func wrapStyled(st lipgloss.Style, text string, width int) []string {
	var out []string
	for _, l := range strings.Split(wrap(text, max(1, width)), "\n") {
		out = append(out, st.Render(l))
	}
	return out
}

func (g *gameScreen) actionLine(a *App, p *engine.PendingView) string {
	s := g.name(p.Actor) + " " + a.t.G("→", "->") + " " + p.Action.Title()
	if p.Target >= 0 {
		s += " " + g.name(p.Target)
	}
	if p.Claim != engine.NoCharacter {
		s += " (claims " + p.Claim.Title() + ")"
	}
	return s
}

func (g *gameScreen) phaseLine(a *App) string {
	t := a.t
	v := g.v
	switch v.Phase {
	case engine.PhaseChallengeWindow:
		return t.Warn.Render("CHALLENGE WINDOW OPEN")
	case engine.PhaseBlockWindow:
		return t.Warn.Render("BLOCK WINDOW OPEN")
	case engine.PhaseBlockChallengeWindow:
		return t.Warn.Render("CHALLENGE THE BLOCK?")
	case engine.PhaseProveClaim:
		return t.Loss.Render("PROVE IT OR LOSE IT")
	case engine.PhaseLoseInfluence:
		return t.Loss.Render(g.name(v.Loser) + " LOSES AN INFLUENCE")
	case engine.PhaseExchange:
		return t.Accent.Render(g.name(v.Current) + " IS SWAPPING CARDS")
	}
	return ""
}

func (g *gameScreen) timerBar(a *App, w int) string {
	if g.deadline.IsZero() || g.timerTotal <= 0 || g.beat != nil {
		return ""
	}
	left := g.deadline.Sub(g.clock(a))
	if left < 0 {
		left = 0
	}
	frac := left.Seconds() / g.timerTotal.Seconds()
	barW := max(10, w-8)
	filled := int(math.Round(frac * float64(barW)))
	col := theme.Cyan
	if frac < 0.35 {
		col = theme.Red
	} else if frac < 0.6 {
		col = theme.Acid
	}
	bar := lipgloss.NewStyle().Foreground(col).Render(strings.Repeat(a.t.G("▓", "#"), filled)) +
		a.t.Faint.Render(strings.Repeat(a.t.G("░", "."), barW-filled))
	return bar + a.t.Text.Render(fmt.Sprintf(" %4.1fs", left.Seconds()))
}

// --- hand ----------------------------------------------------------------------

func (g *gameScreen) handRow(a *App, w, h int, compact bool) string {
	t := a.t
	if g.me < 0 {
		box := t.Box.Width(w).Height(h).Render(t.Accent.Render("SPECTATING") + "\n" + t.Muted.Render("hands stay hidden until they're lost. enjoy the show."))
		return box
	}
	v := g.v
	if v.Phase == engine.PhaseExchange && g.awaitingMe() {
		return g.exchangeRow(a, w, h)
	}
	var cards []string
	choosing := g.awaitingMe() && (v.Phase == engine.PhaseProveClaim || v.Phase == engine.PhaseLoseInfluence)
	for slot, c := range v.Hand {
		o := art.CardOpts{Compact: compact, Lost: c.Revealed, Shimmer: (a.frame / 4) % 3}
		if a.cfg.ReduceMotion {
			o.Shimmer = 0
		}
		if choosing && !c.Revealed {
			o.Label = fmt.Sprint(slot + 1)
			o.Selected = true
			if v.Phase == engine.PhaseProveClaim && v.Challenge != nil {
				if c.Char == v.Challenge.Claim {
					o.Hint = t.G("✓ FLIP: PROVES", "v FLIP: PROVES")
				} else {
					o.Hint = t.G("✖ FLIP: CONCEDE", "x FLIP: CONCEDE")
				}
			}
		}
		full := art.CardW
		if compact {
			full = art.CompactW
		}
		if card, ok := g.beatSlot(a, slot, o, full); ok {
			cards = append(cards, card)
			continue
		}
		if ts, ok := g.flips[slot]; ok && a.now.Sub(ts) < flipDur && !a.cfg.ReduceMotion {
			ph := a.now.Sub(ts).Seconds() / flipDur.Seconds()
			o.Width = max(1, int(math.Abs(math.Cos(ph*math.Pi))*float64(full)))
			if ph < 0.5 {
				cards = append(cards, a.art.Back(o))
				continue
			}
		}
		cards = append(cards, a.art.Card(c.Char, o))
	}
	hand := lipgloss.JoinHorizontal(lipgloss.Top, cards...)
	infoW := max(20, w-lipgloss.Width(hand)-2)
	return lipgloss.JoinHorizontal(lipgloss.Top, hand, "  ", g.infoPanel(a, infoW, h))
}

func (g *gameScreen) infoPanel(a *App, w, h int) string {
	t := a.t
	me := g.v.Players[g.me]
	var lines []string
	lines = append(lines, t.Bold.Render(a.cfg.Name)+t.Faint.Render(fmt.Sprintf("  seat %d", g.me+1)))
	big := lipgloss.NewStyle().Foreground(theme.Acid).Bold(true)
	if ts, ok := g.coinFx[g.me]; ok && a.now.Sub(ts) < flashDur {
		big = map[bool]lipgloss.Style{true: t.Gain, false: t.Loss}[g.coinUp[g.me]]
	}
	lines = append(lines, big.Render(fmt.Sprintf("%s %d", t.Coin(), int(math.Round(g.shown[g.me])))))
	lines = append(lines, t.Muted.Render("influence ")+lipgloss.NewStyle().Foreground(theme.Magenta).Render(
		t.G(strings.Repeat("●", me.Hidden)+strings.Repeat("○", len(me.Revealed)), strings.Repeat("o", me.Hidden)+strings.Repeat(".", len(me.Revealed)))))
	if !me.Alive {
		lines = append(lines, t.Loss.Render("FLATLINED"), t.Muted.Render("you're spectating now"))
	} else {
		if me.Coins >= engine.MustCoupAt {
			lines = append(lines, t.Warn.Render("10+ coins: COUP is forced"))
		} else if me.Coins >= engine.CoupCost {
			lines = append(lines, t.Gain.Render("coup armed (7)"))
		}
		var held []string
		for _, c := range g.v.Hand {
			if !c.Revealed {
				held = append(held, c.Char.Title())
			}
		}
		lines = append(lines, "", t.Faint.Render("you hold"), t.Text.Render(strings.Join(held, " + ")))
	}
	return fitLines(strings.Join(lines, "\n"), w, h)
}

func (g *gameScreen) exchangeRow(a *App, w, h int) string {
	t := a.t
	opts := g.v.ExchangeOptions
	compact := len(opts)*art.CardW > w-20 || h < art.CardH
	var cards []string
	for i, c := range opts {
		o := art.CardOpts{Compact: compact, Label: fmt.Sprint(i + 1)}
		if slices.Contains(g.exSel, i) {
			o.Selected = true
			o.Hint = t.G("✓ KEEP", "v KEEP")
		}
		cards = append(cards, a.art.Card(c, o))
	}
	need := g.v.Players[g.me].Hidden
	info := []string{
		t.Accent.Render("EXCHANGE"),
		t.Text.Render(fmt.Sprintf("keep %d of %d", need, len(opts))),
		t.Muted.Render(fmt.Sprintf("selected %d/%d", len(g.exSel), need)),
		"",
		t.Faint.Render("1-" + fmt.Sprint(len(opts)) + " toggle"),
		t.Faint.Render("enter confirm"),
	}
	hand := lipgloss.JoinHorizontal(lipgloss.Top, cards...)
	return lipgloss.JoinHorizontal(lipgloss.Top, hand, "  ", fitLines(strings.Join(info, "\n"), max(14, w-lipgloss.Width(hand)-2), h))
}

// --- side panel ----------------------------------------------------------------

func (g *gameScreen) sidePanel(a *App, w, h int) string {
	t := a.t
	chatH := chatRows + 2
	logH := h - chatH
	inner := w - 4

	var lines []string
	var logLines []logLine
	var chatLines []logLine
	for _, l := range g.log {
		if l.kind == logChat || l.kind == logSystem {
			chatLines = append(chatLines, l)
		} else {
			logLines = append(logLines, l)
		}
	}
	// Wrap from the newest line backwards until the panel is full.
	end := max(0, len(logLines)-g.scroll)
	visible := logH - 3
	for i := end - 1; i >= 0; i-- {
		l := logLines[i]
		text := l.text
		if l.icon != "" {
			text = l.icon + " " + text
		}
		wrapped := strings.Split(wrap(t.A(text), inner-2), "\n")
		if len(lines)+len(wrapped) > visible {
			break // never show half an entry
		}
		for j := range wrapped {
			if j > 0 {
				wrapped[j] = "  " + wrapped[j]
			}
			wrapped[j] = g.logStyle(a, l).Render(trunc(wrapped[j], inner))
		}
		lines = append(wrapped, lines...)
	}
	title := t.Hot.Render("NETLOG")
	if g.scroll > 0 {
		title += t.Faint.Render(fmt.Sprintf("  (scrolled %d, end to return)", g.scroll))
	}
	logBox := lipgloss.NewStyle().Border(t.RoundBorder).BorderForeground(theme.Faint).Padding(0, 1).Width(w).Height(logH).
		Render(fitLines(title+"\n"+strings.Join(lines, "\n"), inner, logH-2))

	var chat []string
	for _, l := range lastN(chatLines, chatRows-1) {
		chat = append(chat, g.logStyle(a, l).Render(trunc(t.A(l.text), inner)))
	}
	in := t.Faint.Render("enter: chat · t: taunt")
	if g.chatting {
		in = g.input.view(t, inner, true)
	}
	chatBox := lipgloss.NewStyle().Border(t.RoundBorder).BorderForeground(theme.Purple).Padding(0, 1).Width(w).Height(chatH).
		Render(fitLines(strings.Join(chat, "\n"), inner, chatRows-1) + "\n" + in)
	return lipgloss.JoinVertical(lipgloss.Left, logBox, chatBox)
}

func (g *gameScreen) logStyle(a *App, l logLine) lipgloss.Style {
	t := a.t
	switch l.kind {
	case logGain:
		return t.Gain.UnsetBold()
	case logLoss:
		return t.Loss.UnsetBold()
	case logAlert:
		return t.Warn.UnsetBold()
	case logPrivate:
		return t.Accent.Italic(true)
	case logTurn:
		return t.Faint
	case logChat:
		return t.Text
	case logSystem:
		return t.Muted
	case logAction:
		return t.Text
	case logChallenge:
		return t.Loss.UnsetBold()
	case logBlock:
		return t.Warn.UnsetBold()
	case logSummary:
		return t.Accent
	}
	return t.Text
}

// --- action bar ----------------------------------------------------------------

var actionLabels = map[engine.Action]string{
	engine.Income: "INCOME", engine.ForeignAid: "AID", engine.Coup: "COUP", engine.Tax: "TAX",
	engine.Assassinate: "ASSASSIN", engine.Steal: "STEAL", engine.Exchange: "SWAP", engine.Audit: "AUDIT",
}

// actionBar is the context bar (what's happening, your choices and their
// risks, the countdown) over the hotkeys.
func (g *gameScreen) actionBar(a *App, w int) string {
	_, keys := g.prompt(a)
	bg := lipgloss.NewStyle().Background(theme.Panel)
	l1 := bg.Render(padTo(" "+g.contextLine(a, w-2), w))
	l2 := bg.Render(padTo(" "+keys, w))
	return fit(l1, w, 1) + "\n" + fit(l2, w, 1)
}

func (g *gameScreen) key2(a *App, k, label string, on bool, st *lipgloss.Style) string {
	t := a.t
	if !on {
		return t.KeyDisabled.Render(keycap(t, k)) + t.Faint.Render(label) + "  "
	}
	ls := t.Text
	if st != nil {
		ls = *st
	}
	return t.Key.Render(keycap(t, k)) + ls.Render(label) + "  "
}

// keycap draws a key label; brackets keep it readable without color.
func keycap(t *theme.Theme, k string) string {
	return "[" + k + "]"
}

func (g *gameScreen) prompt(a *App) (string, string) {
	t := a.t
	v := g.v
	tail := g.key2(a, "t", "TAUNT", true, nil) + g.key2(a, "?", "HELP", true, nil)
	if g.me < 0 {
		return t.Accent.Render("SPECTATING // the table can't see you"), tail
	}
	if !g.have {
		return t.Muted.Render("syncing..."), tail
	}
	if !v.Players[g.me].Alive {
		return t.Loss.Render("FLATLINED // you're watching the rest of the run"), tail
	}
	if !g.awaitingMe() {
		if g.sent {
			return t.Muted.Render("sent. waiting for the table ") + a.spinner(), tail
		}
		who := []string{}
		for _, s := range v.Awaiting {
			who = append(who, g.name(s))
		}
		return t.Muted.Render("waiting on "+strings.Join(who, ", ")+" ") + a.spinner(), tail
	}
	pend := v.Pending
	switch v.Phase {
	case engine.PhaseTurnStart:
		if g.mode == modeTarget {
			var opts []string
			for i, s := range g.targets(g.pending) {
				p := v.Players[s]
				opts = append(opts, t.Key.Render(keycap(t, fmt.Sprint(i+1)))+t.Text.Render(fmt.Sprintf(" %s %s%d ", g.name(s), t.Coin(), p.Coins)))
			}
			return t.Hot.Render("TARGET FOR " + g.pending.Title() + ":"), strings.Join(opts, " ") + t.Faint.Render(" esc cancel")
		}
		var b strings.Builder
		for i, act := range actionKeys {
			if act == engine.Audit && !v.Magistrate {
				continue
			}
			on := len(g.targets(act)) > 0 || g.legal(engine.ActionIntent(act, -1))
			label := actionLabels[act]
			var st *lipgloss.Style
			if c := act.Claim(); c != engine.NoCharacter && on {
				s := lipgloss.NewStyle().Foreground(theme.Green)
				if !handHas(v.Hand, c) {
					s = lipgloss.NewStyle().Foreground(theme.Magenta)
					label += t.G("~", "?")
				}
				st = &s
			}
			b.WriteString(g.key2(a, fmt.Sprint(i+1), label, on, st))
		}
		msg := t.Hot.Render("YOUR MOVE") + t.Muted.Render(" // green: you hold it · magenta~: bluff")
		if v.Players[g.me].Coins >= engine.MustCoupAt {
			msg = t.Warn.Render("10+ COINS: YOU MUST COUP (3)")
		}
		return msg, b.String() + tail
	case engine.PhaseChallengeWindow:
		return t.Warn.Render(fmt.Sprintf("%s claims %s for %s.", g.name(pend.Actor), pend.Claim.Title(), pend.Action.Title())) + t.Muted.Render(" call the bluff?"),
			g.key2(a, "c", "CHALLENGE", true, nil) + g.key2(a, "p", "PASS", true, nil) + tail
	case engine.PhaseBlockWindow:
		blocks := pend.Action.Blockers(v.Magistrate)
		var names []string
		for _, c := range blocks {
			names = append(names, c.Title())
		}
		what := fmt.Sprintf("%s is going for %s", g.name(pend.Actor), pend.Action.Title())
		if pend.Target == g.me {
			what = fmt.Sprintf("%s's %s is aimed at YOU", g.name(pend.Actor), pend.Action.Title())
		}
		if g.mode == modeBlock {
			var opts []string
			for i, c := range blocks {
				col := theme.Green
				if !handHas(v.Hand, c) {
					col = theme.Magenta
				}
				opts = append(opts, t.Key.Render(keycap(t, fmt.Sprint(i+1)))+lipgloss.NewStyle().Foreground(col).Render(" "+c.Title())+" ")
			}
			return t.Warn.Render("BLOCK WITH:"), strings.Join(opts, " ") + t.Faint.Render(" esc cancel")
		}
		return t.Warn.Render(what+".") + t.Muted.Render(" block with "+strings.Join(names, "/")+"?"),
			g.key2(a, "b", "BLOCK", true, nil) + g.key2(a, "p", "ALLOW", true, nil) + tail
	case engine.PhaseBlockChallengeWindow:
		return t.Warn.Render(fmt.Sprintf("%s blocks with %s.", g.name(pend.Blocker), pend.BlockClaim.Title())) + t.Muted.Render(" challenge the block?"),
			g.key2(a, "c", "CHALLENGE", true, nil) + g.key2(a, "p", "ACCEPT", true, nil) + tail
	case engine.PhaseProveClaim:
		var opts []string
		for slot, c := range v.Hand {
			if c.Revealed {
				continue
			}
			what := "concede"
			col := theme.Red
			if v.Challenge != nil && c.Char == v.Challenge.Claim {
				what, col = "proves it", theme.Green
			}
			opts = append(opts, t.Key.Render(keycap(t, fmt.Sprint(slot+1)))+lipgloss.NewStyle().Foreground(col).Render(fmt.Sprintf(" %s (%s) ", c.Char.Title(), what)))
		}
		return t.Loss.Render("CHALLENGED! flip a card:"), strings.Join(opts, " ")
	case engine.PhaseLoseInfluence:
		var opts []string
		for slot, c := range v.Hand {
			if !c.Revealed {
				opts = append(opts, t.Key.Render(keycap(t, fmt.Sprint(slot+1)))+t.Text.Render(" "+c.Char.Title()+" "))
			}
		}
		return t.Loss.Render("LOSE AN INFLUENCE: pick the card to flip"), strings.Join(opts, " ")
	case engine.PhaseExchange:
		var sel []string
		for _, i := range g.exSel {
			sel = append(sel, v.ExchangeOptions[i].Title())
		}
		return t.Accent.Render(fmt.Sprintf("EXCHANGE: keep %d.", v.Players[g.me].Hidden)) + t.Muted.Render(" selected: "+strings.Join(sel, ", ")),
			t.Faint.Render(fmt.Sprintf("1-%d toggle · enter confirm · esc clear", len(v.ExchangeOptions)))
	}
	return "", tail
}

// --- overlays ------------------------------------------------------------------

func (g *gameScreen) tauntMenu(a *App) string {
	t := a.t
	var b strings.Builder
	b.WriteString(t.Hot.Render("TAUNT") + "\n\n")
	for i, e := range protocol.Emotes {
		b.WriteString(t.Key.Render(keycap(t, fmt.Sprint(i+1))) + " " + t.Text.Render(e) + "\n")
	}
	b.WriteString("\n" + t.Faint.Render("any other key to close"))
	return lipgloss.NewStyle().Border(t.RoundBorder).BorderForeground(theme.Magenta).Background(theme.Panel).Padding(1, 2).Render(t.A(b.String()))
}

func (g *gameScreen) resultsView(a *App) string {
	t := a.t
	over := g.over
	won := g.me >= 0 && over.Winner == g.me
	el := a.now.Sub(g.overAt)
	var title string
	var col color.Color
	switch {
	case won:
		title, col = "SYSTEM SEIZED", theme.Green
	case g.me < 0:
		title, col = "RUN COMPLETE", theme.Cyan
	default:
		title, col = "CONNECTION TERMINATED", theme.Red
	}
	if el < 900*time.Millisecond && !a.cfg.ReduceMotion {
		title = art.Glitch(title, 0.9-el.Seconds(), a.rng)
	}
	big := lipgloss.NewStyle().Foreground(theme.Bg).Background(col).Bold(true).Padding(0, 3).Render(title)
	winner := "?"
	if over.Winner >= 0 && over.Winner < len(over.Names) {
		winner = over.Names[over.Winner]
	}
	lines := []string{
		big, "",
		t.Text.Render("winner: ") + t.Bold.Render(winner) + t.Muted.Render(fmt.Sprintf("   turns: %d", g.v.Turn)),
	}
	if g.me >= 0 {
		s := g.stats
		lines = append(lines,
			t.Muted.Render(fmt.Sprintf("your challenges: %d (%d won)   blocks: %d   coins earned: %d   coups: %d",
				s.Challenges, s.ChallengesWon, s.Blocks, s.CoinsEarned, s.Coups)))
	}
	if pg := g.post; pg != nil {
		lines = append(lines, "", t.Accent.Render("THE CARDS // every hand, face up"))
		for seat, hand := range pg.hands {
			row := padTo(t.Bold.Render(trunc(g.name(seat), 12)), 13)
			for _, c := range hand {
				st := lipgloss.NewStyle().Foreground(theme.CharColor(c.Char)).Bold(true)
				if c.Revealed {
					st = lipgloss.NewStyle().Foreground(theme.Dim).Strikethrough(true)
				}
				row += padTo(st.Render(c.Char.Title()), 12)
			}
			b := pg.bluffs[seat]
			bl := t.Muted.Render(fmt.Sprintf("bluffs: %d got away, %d caught", b.ok, b.caught))
			if b.ok+b.caught == 0 {
				bl = t.Faint.Render("played it straight")
			}
			lines = append(lines, row+bl)
		}
		// Key moments, as many of the latest as fit.
		room := a.h - 6 - len(lines) - 6
		if room >= 2 && len(pg.moments) > 0 {
			lines = append(lines, "", t.Accent.Render("KEY MOMENTS"))
			ms := pg.moments
			if len(ms) > room {
				ms = ms[len(ms)-room:]
			}
			for _, m := range ms {
				lines = append(lines, t.Text.Render(trunc(m, 72)))
			}
		}
	}
	if g.replayPath != "" {
		lines = append(lines, t.Faint.Render("replay saved: "+trunc(g.replayPath, 60)))
	}
	keys := "[v] replay  [enter] menu  [q] quit"
	if a.mode != "solo" && a.mode != "tutorial" {
		keys = "[v] replay  [enter] lobby  [q] leave"
	}
	if (a.welcome.Host || a.mode == "solo") && a.mode != "tutorial" {
		keys = "[r] rematch  " + keys
	}
	lines = append(lines, "", t.Accent.Render(keys))
	return lipgloss.NewStyle().Border(t.ThickBorder).BorderForeground(col).Background(theme.Panel).Padding(1, 4).
		Render(t.A(strings.Join(lines, "\n")))
}

// deathScreen tears the whole screen apart when you are eliminated.
func (g *gameScreen) deathScreen(a *App, body string) string {
	el := a.now.Sub(g.myDeath)
	amount := 1 - el.Seconds()/deathDur.Seconds()
	lines := strings.Split(body, "\n")
	for i := range lines {
		if a.rng.Float64() < amount*0.35 {
			shift := a.rng.IntN(7) - 3
			if shift > 0 {
				lines[i] = ansi.Cut(strings.Repeat(" ", shift)+lines[i], 0, a.w)
			} else if shift < 0 {
				lines[i] = cutLeft(lines[i], -shift) + strings.Repeat(" ", -shift)
			}
		}
	}
	body = strings.Join(lines, "\n")
	msg := lipgloss.NewStyle().Foreground(theme.Bg).Background(theme.Red).Bold(true).Padding(1, 6).Render(
		art.Glitch("F L A T L I N E D", amount*0.5, a.rng))
	return overlay(body, msg, a.w, a.h)
}

// shake offsets the frame horizontally right after a challenge.
func (g *gameScreen) shake(a *App, body string) string {
	if g.jolt.IsZero() {
		return body
	}
	el := max(0, a.now.Sub(g.jolt))
	if el >= joltDur {
		return body
	}
	pattern := []int{2, -2, 1, -1, 1, 0}
	off := pattern[min(len(pattern)-1, int(el/(joltDur/time.Duration(len(pattern)))))]
	if off == 0 {
		return body
	}
	lines := strings.Split(body, "\n")
	for i, l := range lines {
		if off > 0 {
			lines[i] = ansi.Cut(strings.Repeat(" ", off)+l, 0, a.w)
		} else {
			lines[i] = cutLeft(l, -off) + strings.Repeat(" ", -off)
		}
	}
	return strings.Join(lines, "\n")
}

// --- helpers -------------------------------------------------------------------

// padTo pads a styled string with spaces to width w.
func padTo(s string, w int) string {
	if n := cellWidth(s); n < w {
		return s + strings.Repeat(" ", w-n)
	}
	return s
}

// fitLines crops a block to w x h without padding the right edge beyond w.
func fitLines(s string, w, h int) string {
	lines := strings.Split(s, "\n")
	if len(lines) > h {
		lines = lines[:h]
	}
	for i, l := range lines {
		if cellWidth(l) > w {
			lines[i] = ansi.Cut(l, 0, w)
		}
	}
	return strings.Join(lines, "\n")
}
