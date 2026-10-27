package client

import (
	"image/color"
	"math"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/v1rushb/neoncoup/client/art"
	"github.com/v1rushb/neoncoup/client/theme"
	"github.com/v1rushb/neoncoup/engine"
)

// Sprites: things that move across the table during a beat (coins, cards).
// They are drawn over the finished frame at positions taken from the table
// layout, and driven only by the beat's progress, so they follow the speed
// setting, stop when a beat is skipped and never outlive it. Opponents' cards
// only ever travel face down; a face is drawn only when the event made it
// public.

type point struct{ x, y int }

type rect struct{ x, y, w, h int }

func (r rect) mid() point { return point{r.x + r.w/2, r.y + r.h/2} }

// geom is where things are on screen.
type geom struct {
	seats  map[int]rect // opponent panels
	hand   rect         // your cards
	info   rect         // your coins and influence
	center rect         // the center terminal
	deck   point
	bank   point
}

// geometry mirrors view/tableArea's layout.
func (g *gameScreen) geometry(a *App) geom {
	W, H := a.w, a.h
	logW, sideW := layout(W)
	mainW := W - logW
	bodyH := H - 3
	handH := handHeight(bodyH)
	midH := bodyH - oppH - handH
	gm := geom{seats: map[int]rect{}}

	others := g.seatOrder()
	left, right := -1, -1
	topSeats := others
	if len(others) >= 4 {
		left, right = others[0], others[len(others)-1]
		topSeats = others[1 : len(others)-1]
	}
	if len(topSeats) > 0 {
		bw := min(maxTopW, mainW/len(topSeats))
		x0 := (mainW - bw*len(topSeats)) / 2
		for i, s := range topSeats {
			gm.seats[s] = rect{x0 + i*bw, 1, bw, oppH}
		}
	}
	centerW, cx := mainW, 0
	sideH := min(midH, oppH+2)
	sideY := 1 + oppH + (midH-sideH)/2
	if left >= 0 {
		gm.seats[left] = rect{0, sideY, sideW, sideH}
		centerW -= sideW
		cx = sideW
	}
	if right >= 0 {
		centerW -= sideW
		gm.seats[right] = rect{cx + centerW, sideY, sideW, sideH}
	}
	gm.center = rect{cx, 1 + oppH, centerW, midH}
	cardW := art.CardW
	if handH == art.CompactH {
		cardW = art.CompactW
	}
	handY := 1 + oppH + midH
	gm.hand = rect{0, handY, cardW * max(1, len(g.v.Hand)), handH}
	gm.info = rect{gm.hand.w + 2, handY, 20, handH}
	gm.deck, gm.bank = gm.center.mid(), gm.center.mid()
	if g.trackDeck.x >= 0 {
		gm.deck = point{cx + g.trackDeck.x, 1 + oppH + g.trackDeck.y}
	}
	if g.trackBank.x >= 0 {
		gm.bank = point{cx + g.trackBank.x, 1 + oppH + g.trackBank.y}
	}
	return gm
}

// locate records where the deck and the treasury ended up in the center
// terminal's rendered lines (offsets from its top-left corner).
func (g *gameScreen) locate(lines []string) {
	g.trackDeck, g.trackBank = point{-1, -1}, point{-1, -1}
	for i, l := range lines {
		p := ansi.Strip(l)
		if j := strings.Index(p, "DECK"); j >= 0 && g.trackDeck.x < 0 {
			col := lipgloss.Width(p[:j])
			y := i
			if strings.HasPrefix(strings.TrimSpace(p[j:]), "DECK ") && i >= 3 && strings.Contains(ansi.Strip(lines[i-2]), "│") {
				y = i - 2 // the card row: aim at the middle of the stack graphic
			}
			g.trackDeck = point{2 + col + 3, 1 + y}
		}
		if j := strings.Index(p, "TREASURY"); j >= 0 && g.trackBank.x < 0 {
			g.trackBank = point{2 + lipgloss.Width(p[:j]) + 4, 1 + i}
		}
	}
}

// anchor is where coins and cards land for a seat.
func (gm geom) anchor(g *gameScreen, seat int) point {
	if seat == g.me {
		return point{gm.info.x + 2, gm.info.y + 1}
	}
	if r, ok := gm.seats[seat]; ok {
		return r.mid()
	}
	return gm.center.mid()
}

// handSlot is the top-left of your card in a hand slot.
func (gm geom) handSlot(slot int) point {
	n := max(1, gm.hand.w/max(1, art.CompactW))
	w := gm.hand.w / max(1, n)
	if gm.hand.h == art.CardH {
		w = art.CardW
	}
	return point{gm.hand.x + slot*w, gm.hand.y}
}

func ease(t float64) float64 { return t * t * (3 - 2*t) }

func lerp(a, b point, t float64) point {
	t = ease(min(1, max(0, t)))
	return point{a.x + int(math.Round(float64(b.x-a.x)*t)), a.y + int(math.Round(float64(b.y-a.y)*t))}
}

// span maps progress p onto the sub-interval [from, to], 0..1 inside it.
func span(p, from, to float64) float64 { return min(1, max(0, (p-from)/(to-from))) }

// sprite is one thing to draw: a block centered at a point.
type sprite struct {
	at   point
	body string
}

// sprites overlays the current beat's moving parts on a finished frame.
func (g *gameScreen) sprites(a *App, frame string) string {
	b := g.beat
	if b == nil || a.cfg.ReduceMotion {
		return frame
	}
	gm := g.geometry(a)
	p := b.progress(g.clock(a))
	e := b.ev
	var list []sprite
	back := a.art.Back(art.CardOpts{Compact: true})
	face := func(c engine.Character) string { return a.art.Card(c, art.CardOpts{Compact: true}) }
	from := func(seat int) point { return gm.anchor(g, seat) }
	switch e.Kind {
	case engine.EvCoins:
		src, dst, ok := g.coinPath(gm, e)
		if !ok {
			break
		}
		n := min(3, abs(e.Amount))
		coin := lipgloss.NewStyle().Foreground(theme.Acid).Bold(true).Render(a.t.Coin())
		for i := range n {
			q := span(p, float64(i)*0.12, 0.7+float64(i)*0.12)
			if q <= 0 || q >= 1 {
				continue
			}
			list = append(list, sprite{lerp(src, dst, q), coin})
		}
	case engine.EvClaimProven:
		// The proven card pops up for everyone to see.
		if e.Seat != g.me {
			at := from(e.Seat)
			at.y -= int(2 * ease(span(p, 0, 0.3)))
			list = append(list, sprite{at, face(e.Card)})
		}
	case engine.EvCardReplaced:
		// The shown card slides into the deck, the deck shuffles, and a
		// fresh card slides back face down.
		owner := from(e.Seat)
		if e.Seat == g.me {
			owner = gm.handSlot(e.Slot)
			owner.x += art.CompactW / 2
			owner.y += art.CompactH / 2
		}
		switch {
		case p < 0.35:
			if c := g.provenCard(e); c != engine.NoCharacter {
				list = append(list, sprite{lerp(owner, gm.deck, span(p, 0, 0.35)), face(c)})
			}
		case p >= 0.55 && p < 0.92:
			list = append(list, sprite{lerp(gm.deck, owner, span(p, 0.55, 0.92)), back})
		}
	case engine.EvExchangeDraw:
		for i := range e.Amount {
			q := span(p, float64(i)*0.2, 0.75+float64(i)*0.2)
			if q > 0 && q < 1 {
				list = append(list, sprite{lerp(gm.deck, from(e.Seat), q), back})
			}
		}
	case engine.EvExchangeDone:
		for i := range e.Amount {
			q := span(p, float64(i)*0.15, 0.6+float64(i)*0.15)
			if q > 0 && q < 1 {
				list = append(list, sprite{lerp(from(e.Seat), gm.deck, q), back})
			}
		}
	case engine.EvInfluenceLost:
		// Opponents: the lost card flips face up over their panel, glitches,
		// then settles into their revealed row. Your own card does this in
		// your hand.
		if e.Seat == g.me || p >= 0.85 {
			break
		}
		at := from(e.Seat)
		switch {
		case p < 0.25:
			w := int(math.Abs(math.Cos(span(p, 0, 0.25)*math.Pi)) * float64(art.CompactW))
			card := back
			if p >= 0.125 {
				card = face(e.Card)
			}
			list = append(list, sprite{at, a.art.Squash(card, art.CompactW, max(1, w))})
		case p < 0.7:
			list = append(list, sprite{at, glitched(a, face(e.Card), 0.5*(1-span(p, 0.25, 0.7)), theme.Red)})
		default:
			list = append(list, sprite{at, face(e.Card)})
		}
	}
	for _, s := range list {
		w, h := lipgloss.Size(s.body)
		frame = overlayAt(frame, s.body, s.at.x-w/2, s.at.y-h/2)
	}
	return frame
}

// coinPath is where a coin event's coins travel from and to.
func (g *gameScreen) coinPath(gm geom, e engine.Event) (point, point, bool) {
	seat := gm.anchor(g, e.Seat)
	switch {
	case e.Reason == engine.ReasonSteal && e.Amount > 0:
		return gm.anchor(g, e.Other), seat, true
	case e.Reason == engine.ReasonEliminated || e.Amount == 0:
		return point{}, point{}, false
	case e.Amount > 0:
		return gm.bank, seat, true
	}
	return seat, gm.bank, true
}

// provenCard is the card a claimant just showed: the character of the
// claim proven right before this replacement.
func (g *gameScreen) provenCard(e engine.Event) engine.Character {
	return g.lastProven[e.Seat]
}

// glitched scrambles a rendered block and recolors it.
func glitched(a *App, block string, amount float64, col color.Color) string {
	return lipgloss.NewStyle().Foreground(col).Render(art.Glitch(ansi.Strip(block), amount, a.rng))
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// beatSlot draws your own hand slot while a beat moves its card: empty while
// a proven card is away being reshuffled, then the replacement flipping face
// up (only you see it); a lost card glitching before it settles.
func (g *gameScreen) beatSlot(a *App, slot int, o art.CardOpts, full int) (string, bool) {
	b := g.beat
	if b == nil || b.ev.Seat != g.me || b.ev.Slot != slot || a.cfg.ReduceMotion {
		return "", false
	}
	p := b.progress(g.clock(a))
	h := art.CardH
	if o.Compact {
		h = art.CompactH
	}
	switch b.ev.Kind {
	case engine.EvCardReplaced:
		if len(b.ev.Secret) != 1 {
			return "", false
		}
		if p < 0.92 {
			return strings.TrimRight(strings.Repeat(strings.Repeat(" ", full)+"\n", h), "\n"), true
		}
		q := span(p, 0.92, 1)
		o.Width = max(1, int(math.Abs(math.Cos(q*math.Pi))*float64(full)))
		if q < 0.5 {
			return a.art.Back(o), true
		}
		return a.art.Card(b.ev.Secret[0], o), true
	case engine.EvInfluenceLost:
		if p >= 0.3 && p < 0.75 {
			return glitched(a, a.art.Card(b.ev.Card, o), 0.45*(1-span(p, 0.3, 0.75)), theme.Red), true
		}
	}
	return "", false
}

// shuffling reports whether the deck is being shuffled right now.
func (g *gameScreen) shuffling(a *App) bool {
	b := g.beat
	if b == nil || a.cfg.ReduceMotion {
		return false
	}
	p := b.progress(g.clock(a))
	switch b.ev.Kind {
	case engine.EvCardReplaced:
		return p >= 0.35 && p < 0.55
	case engine.EvExchangeDone:
		return p >= 0.7
	}
	return false
}

// eliminating reports whether seat's elimination beat is past its glitch,
// so the panel already shows the runner as gone.
func (g *gameScreen) eliminating(a *App, seat int) bool {
	b := g.beat
	return b != nil && b.ev.Kind == engine.EvEliminated && b.ev.Seat == seat &&
		(a.cfg.ReduceMotion || b.progress(g.clock(a)) >= 0.75)
}
