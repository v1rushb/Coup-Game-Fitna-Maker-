// Package art holds NEON//COUP's original ASCII/ANSI artwork: the logo,
// the six character cards and the card back. Everything here was drawn for
// this game; none of it is derived from the published Coup illustrations.
package art

import (
	"image/color"
	"math/rand/v2"
	"strings"
	"sync"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/v1rushb/neoncoup/client/theme"
	"github.com/v1rushb/neoncoup/engine"
)

// Card dimensions.
const (
	CardW        = 18
	CardH        = 12
	CompactW     = 12
	CompactH     = 7
	artW         = 16
	compactInner = 10
)

// Face is one character card's artwork and rules text.
type Face struct {
	Name    string
	Glyph   string
	Art     [6]string
	Mini    [2]string
	Ability [2]string
	Short   [2]string // ability text for compact cards (10 cells)
}

// Faces holds every character card.
var Faces = map[engine.Character]Face{
	// Megacorp CEO: slicked hair, a cabled cyber-monocle, a credit chip on the lapel.
	engine.Duke: {
		Name: "DUKE", Glyph: "◆",
		Art: [6]string{
			"     ▄▄▄▄▄▄     ",
			"   ▄█▀▀▀▀▀▀█▄   ",
			"   █  ▄▄ (◉)█═╗ ",
			"   █   ▄▄   █ ║ ",
			"   ▀█▄═══▄▄█▀ ║ ",
			"  ▄▓▓▓█[¢]█▓▓▓▄ ",
		},
		Mini:    [2]string{" ▄█▀▀(◉)═╗", " ▓▓█[¢]█▓▓"},
		Ability: [2]string{"TAX +3¢", "BLOCKS FOR.AID"},
		Short:   [2]string{"TAX +3¢", "BLK AID"},
	},
	// Hooded netrunner: glowing visor slit, mono-blade drawn over the shoulder.
	engine.Assassin: {
		Name: "ASSASSIN", Glyph: "╳",
		Art: [6]string{
			"      ▄▄▄▄     ╱",
			"    ▄█▓▓▓▓█▄  ╱ ",
			"   █▓▀▀▀▀▀▀▓█╱  ",
			"   █ ══════ ▐▌  ",
			"   ▀█▄    ▄█▀▌  ",
			"  ▄▓▓▓▓▓▓▓▓▓▓▄  ",
		},
		Mini:    [2]string{"  ▄█▓▓█▄ ╱", "  █ ════▐▌"},
		Ability: [2]string{"PAY 3¢: STRIKE", "-1 INFLUENCE"},
		Short:   [2]string{"KILL 3¢", "-1 INFL"},
	},
	// Armored enforcer: riot helmet with a lit visor, cannon on the shoulder.
	engine.Captain: {
		Name: "CAPTAIN", Glyph: "▲",
		Art: [6]string{
			"═══▄▄   ▄▄▄▄▄▄  ",
			"  ▐██▌ █▀▀▀▀▀▀█ ",
			"   ▀█▄▐▓▓▓▓▓▓▓▌ ",
			"     ▀█▄▄▄▄▄▄▄█ ",
			"  ▄▓▓▓█▓▓▓▓▓▓▓█▄",
			"  █▓▓▓█▓▓┃┃▓▓▓██",
		},
		Mini:    [2]string{"══▄ █▀▀▀▀█", " ▐█▌▐▓▓▓▓▌"},
		Ability: [2]string{"STEAL 2¢", "BLOCKS STEAL"},
		Short:   [2]string{"STEAL 2¢", "BLK STEAL"},
	},
	// Hologram diplomat projected upward from a hovering drone.
	engine.Ambassador: {
		Name: "AMBASSADOR", Glyph: "◎",
		Art: [6]string{
			"     ░▒▓▓▒░     ",
			"    ▒▓▀  ▀▓▒    ",
			"    ░▓ ── ▓░    ",
			"   ░ ▀▓▓▓▓▀ ░   ",
			"   ╲  ╲  ╱  ╱   ",
			"   ▐══[▓▓]══▌   ",
		},
		Mini:    [2]string{"  ░▒▓▀▀▓▒░", "  ▐═[▓▓]═▌"},
		Ability: [2]string{"EXCHANGE 2", "BLOCKS STEAL"},
		Short:   [2]string{"SWAP 2", "BLK STEAL"},
	},
	// Chrome socialite half-hidden behind a shimmering energy fan.
	engine.Contessa: {
		Name: "CONTESSA", Glyph: "✧",
		Art: [6]string{
			"     ▄████▄     ",
			"    ▐█▀▀▀▀█▌    ",
			"    ▐ ◆  ◆ ▌    ",
			"  ╱▒╲╱▒╲╱▒╲╱▒╲  ",
			" ▐░▒▓░▒▓░▒▓░▒▓▌ ",
			"  ╲▓░▒▓░▒▓░▒▓╱  ",
		},
		Mini:    [2]string{"   ▐◆  ◆▌ ", " ▐░▒▓░▒▓░▌"},
		Ability: [2]string{"BLOCKS", "ASSASSINATION"},
		Short:   [2]string{"BLK KILL", ""},
	},
	// Optional variant. Chrome arbiter weighing credits on holo-scales.
	engine.Magistrate: {
		Name: "MAGISTRATE", Glyph: "≡",
		Art: [6]string{
			"    ▄▄▄▄▄▄▄▄    ",
			"   █▀▀▀▀▀▀▀▀█   ",
			"   █ ▓▓▓▓▓▓ █   ",
			"    ▀█▄▄▄▄█▀    ",
			"  ┌─┐  ║║  ┌─┐  ",
			"  └¢┘══╩╩══└¢┘  ",
		},
		Mini:    [2]string{"  █▀▓▓▓▓▀█", "└¢┘═╩╩═└¢┘"},
		Ability: [2]string{"AUDIT: -2¢", "BLOCKS TAX"},
		Short:   [2]string{"AUDIT -2¢", "BLK TAX"},
	},
}

// back is the circuit-board pattern on the card back; row 4 carries the logo.
var back = [10]string{
	"┌─┐ ┌──●  ●──┐ ┌",
	"┘ └─┘ ┌┘  └┐ └─┘",
	"●─┐   │ ▄▄ │   ┌",
	"  └───┤▐██▌├───┘",
	"   NEON//COUP   ",
	"  ┌───┤▐██▌├───┐",
	"●─┘   │ ▀▀ │   └",
	"┐ ┌─┐ └┐  ┌┘ ┌─┐",
	"└─┘ └──●  ●──┘ └",
	"  ●──┐    ┌──●  ",
}

var miniBack = [5]string{
	"┌─●  ●─┐ ┌",
	"┘ ▐██▌ └─┘",
	"NEON//COUP",
	"┐ ▐██▌ ┌─┐",
	"└─●  ●─┘ └",
}

// Logo is the NEON//COUP wordmark, four rows tall.
var Logo = [4]string{
	"█▄  █ █▀▀▀ ▄▀▀▄ █▄  █    ▄▀    ▄▀ ▄▀▀▀ ▄▀▀▄ █  █ █▀▀▄",
	"█▀▄ █ █▄▄  █  █ █▀▄ █   ▄▀    ▄▀  █    █  █ █  █ █▄▄▀",
	"█ ▀▄█ █    █  █ █ ▀▄█  ▄▀    ▄▀   █    █  █ █  █ █   ",
	"▀   ▀ ▀▀▀▀  ▀▀  ▀   ▀ ▀     ▀      ▀▀▀  ▀▀   ▀▀  ▀   ",
}

// LogoASCII is the --ascii wordmark.
var LogoASCII = [4]string{
	" _  _  ___  ___  _  _      __  __  ___  ___  _  _  ___ ",
	"| \\| || __|/ _ \\| \\| |    / / / / / __|/ _ \\| || || _ \\",
	"| .` || _|| (_) | .` |   / / / / | (__| (_) | || ||  _/",
	"|_|\\_||___|\\___/|_|\\_|  /_/ /_/   \\___|\\___/ \\__/ |_|  ",
}

// CardOpts tweak how a card is drawn.
type CardOpts struct {
	Lost     bool   // face up and out of play
	Selected bool   // highlighted for a choice
	Compact  bool   // small variant for short terminals
	Label    string // shown in the top border, e.g. "1"
	Width    int    // squash for flip animations (0 = full width)
	Shimmer  int    // animation phase for shimmering details
	Hint     string // replaces the second ability line (e.g. "✓ PROVES")
}

// Renderer draws art with a theme and caches the results.
type Renderer struct {
	T     *theme.Theme
	mu    sync.Mutex
	cache map[cardKey]string
}

type cardKey struct {
	c    engine.Character
	o    CardOpts
	back bool
}

// NewRenderer creates a renderer for a theme.
func NewRenderer(t *theme.Theme) *Renderer {
	return &Renderer{T: t, cache: map[cardKey]string{}}
}

// Card draws a face-up card.
func (r *Renderer) Card(c engine.Character, o CardOpts) string {
	return r.cached(cardKey{c: c, o: o}, func() string { return r.drawCard(c, o) })
}

// Back draws a face-down card.
func (r *Renderer) Back(o CardOpts) string {
	return r.cached(cardKey{o: o, back: true}, func() string { return r.drawBack(o) })
}

func (r *Renderer) cached(k cardKey, draw func() string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if s, ok := r.cache[k]; ok {
		return s
	}
	if len(r.cache) > 512 {
		r.cache = map[cardKey]string{}
	}
	s := draw()
	r.cache[k] = s
	return s
}

// glyphClass decides how an art rune is colored.
func glyphColor(ru rune, base color.Color) color.Color {
	switch ru {
	case '▓':
		return lipgloss.Darken(base, 0.35)
	case '▒':
		return lipgloss.Darken(base, 0.55)
	case '░':
		return lipgloss.Darken(base, 0.7)
	case '◉', '◎', '◆', '═', '●', '¢', '$', '━':
		return theme.Cyan
	case '╱', '╲', '─', '│', '┌', '┐', '└', '┘', '║', '╩', '╗', '┃', '┤', '├', '[', ']', '(', ')', '<', '>':
		return theme.Fg
	}
	return base
}

// paint colors an art line by glyph class, grouping runs of one color.
func (r *Renderer) paint(line string, base color.Color, dim bool) string {
	if r.T.ASCII {
		line = theme.Asciify(line)
	}
	var b strings.Builder
	var run []rune
	var runColor color.Color
	flush := func() {
		if len(run) == 0 {
			return
		}
		st := lipgloss.NewStyle().Foreground(runColor)
		b.WriteString(st.Render(string(run)))
		run = run[:0]
	}
	for _, ru := range line {
		c := glyphColor(ru, base)
		if dim {
			c = theme.Faint
		}
		if ru == ' ' {
			c = runColor // spaces join the current run
		}
		if c != runColor && len(run) > 0 {
			flush()
		}
		runColor = c
		run = append(run, ru)
	}
	flush()
	return b.String()
}

// shimmers reports whether a card has an animated energy effect: the
// Contessa's fan shield and the Ambassador's hologram.
func shimmers(c engine.Character) bool { return c == engine.Contessa || c == engine.Ambassador }

// shimmer rotates ░▒▓ so energy fields appear to move.
func shimmer(line string, phase int) string {
	if phase == 0 {
		return line
	}
	cycle := []rune{'░', '▒', '▓'}
	idx := map[rune]int{'░': 0, '▒': 1, '▓': 2}
	out := []rune(line)
	for i, ru := range out {
		if k, ok := idx[ru]; ok {
			out[i] = cycle[(k+phase)%3]
		}
	}
	return string(out)
}

func padRight(s string, w int) string {
	n := lipgloss.Width(s)
	if n >= w {
		return s
	}
	return s + strings.Repeat(" ", w-n)
}

func truncate(s string, w int) string {
	r := []rune(s)
	if len(r) > w {
		return string(r[:w])
	}
	return s
}

type borderSet struct{ tl, tr, bl, br, h, v, ml, mr string }

func (r *Renderer) borders(thick bool) borderSet {
	if r.T.ASCII {
		return borderSet{"+", "+", "+", "+", "-", "|", "+", "+"}
	}
	if thick {
		return borderSet{"╔", "╗", "╚", "╝", "═", "║", "╟", "╢"}
	}
	return borderSet{"┌", "┐", "└", "┘", "─", "│", "├", "┤"}
}

// frame wraps inner lines (already colored, exactly w cells wide) in a border.
func (r *Renderer) frame(inner []string, w int, label string, thick bool, border color.Color) []string {
	bs := r.borders(thick)
	bst := lipgloss.NewStyle().Foreground(border)
	var top string
	if label != "" {
		lab := "[" + label + "]"
		top = lab + strings.Repeat(bs.h, max(0, w-lipgloss.Width(lab)))
	} else {
		top = strings.Repeat(bs.h, w)
	}
	out := []string{bst.Render(bs.tl + top + bs.tr)}
	for _, l := range inner {
		out = append(out, bst.Render(bs.v)+l+bst.Render(bs.v))
	}
	out = append(out, bst.Render(bs.bl+strings.Repeat(bs.h, w)+bs.br))
	return out
}

func (r *Renderer) drawCard(c engine.Character, o CardOpts) string {
	f, ok := Faces[c]
	if !ok {
		return r.drawBack(o)
	}
	base := theme.CharColor(c)
	border := lipgloss.Darken(base, 0.4)
	if o.Selected {
		border = theme.White
	}
	if o.Lost {
		border = theme.Faint
	}
	nameSt := lipgloss.NewStyle().Foreground(theme.Bg).Background(base).Bold(true)
	if o.Lost {
		nameSt = lipgloss.NewStyle().Foreground(theme.Red).Bold(true).Strikethrough(true)
	}
	textSt := lipgloss.NewStyle().Foreground(theme.Fg)
	if o.Lost {
		textSt = lipgloss.NewStyle().Foreground(theme.Faint)
	}
	glyph := r.T.A(f.Glyph)
	if o.Compact {
		w := compactInner
		name := truncate(f.Name, w-2)
		banner := nameSt.Render(padRight(" "+name, w-1)) + lipgloss.NewStyle().Foreground(base).Render(glyph)
		if r.T.ASCII {
			banner = nameSt.Render(padRight(" "+name, w-1)) + lipgloss.NewStyle().Foreground(base).Render(theme.Asciify(glyph))
		}
		lines := []string{banner}
		phase := o.Shimmer
		if !shimmers(c) || o.Lost {
			phase = 0
		}
		for _, m := range f.Mini {
			lines = append(lines, r.paint(padRight(truncate(shimmer(m, phase), w), w), base, o.Lost))
		}
		ab0, ab1 := f.Short[0], f.Short[1]
		if o.Hint != "" {
			ab1 = o.Hint
		}
		if o.Lost {
			ab0, ab1 = r.T.G("✖ LOST", "x LOST"), ""
		}
		lines = append(lines, textSt.Render(padRight(truncate(r.T.A(ab0), w), w)), textSt.Render(padRight(truncate(r.T.A(ab1), w), w)))
		return r.squash(strings.Join(r.frame(lines, w, o.Label, o.Selected, border), "\n"), CompactW, o.Width)
	}

	w := artW
	banner := nameSt.Render(padRight(" "+f.Name, w-2)) + " " + lipgloss.NewStyle().Foreground(base).Render(glyph)
	lines := []string{banner}
	phase := o.Shimmer
	if !shimmers(c) || o.Lost {
		phase = 0
	}
	for _, a := range f.Art {
		lines = append(lines, r.paint(shimmer(a, phase), base, o.Lost))
	}
	sepCh := r.T.G("─", "-")
	lines = append(lines, lipgloss.NewStyle().Foreground(border).Render(strings.Repeat(sepCh, w)))
	ab0, ab1 := f.Ability[0], f.Ability[1]
	if o.Hint != "" {
		ab1 = o.Hint
	}
	if o.Lost {
		ab0, ab1 = r.T.G("✖ FLATLINED", "x FLATLINED"), ""
		textSt = lipgloss.NewStyle().Foreground(theme.Red)
	}
	lines = append(lines,
		textSt.Render(padRight(" "+truncate(r.T.A(ab0), w-1), w)),
		textSt.Render(padRight(" "+truncate(r.T.A(ab1), w-1), w)))
	return r.squash(strings.Join(r.frame(lines, w, o.Label, o.Selected || !o.Lost, border), "\n"), CardW, o.Width)
}

func (r *Renderer) drawBack(o CardOpts) string {
	border := theme.Purple
	if o.Selected {
		border = theme.White
	}
	if o.Compact {
		var lines []string
		for i, l := range miniBack {
			if i == 2 {
				lines = append(lines, lipgloss.NewStyle().Foreground(theme.Magenta).Bold(true).Render(l))
				continue
			}
			lines = append(lines, r.paint(l, theme.Purple, false))
		}
		return r.squash(strings.Join(r.frame(lines, compactInner, o.Label, false, border), "\n"), CompactW, o.Width)
	}
	var lines []string
	for i, l := range back {
		if i == 4 {
			lines = append(lines, lipgloss.NewStyle().Foreground(theme.Magenta).Bold(true).Render(l))
			continue
		}
		lines = append(lines, r.paint(l, lipgloss.Darken(theme.Purple, 0.2), false))
	}
	return r.squash(strings.Join(r.frame(lines, artW, o.Label, true, border), "\n"), CardW, o.Width)
}

// squash narrows a rendered card for flip animations by keeping a centered
// band of columns; the rest becomes blank so the layout keeps its size.
func (r *Renderer) squash(card string, full, w int) string {
	if w <= 0 || w >= full {
		return card
	}
	lines := strings.Split(card, "\n")
	left := (full - w) / 2
	for i, l := range lines {
		cut := ansi.Cut(l, left, left+w)
		lines[i] = strings.Repeat(" ", left) + cut + strings.Repeat(" ", full-left-w)
	}
	return strings.Join(lines, "\n")
}

// Glitch scrambles a fraction of a rendered block's visible characters. It
// works on the plain text, so the result is recolored by the caller.
func Glitch(plain string, amount float64, rng *rand.Rand) string {
	noise := []rune("▓▒░█▀▄#%&@$*/\\|<>")
	var b strings.Builder
	for _, l := range strings.Split(plain, "\n") {
		runes := []rune(l)
		if rng.Float64() < amount {
			// Horizontal tear: shift the line.
			k := rng.IntN(3) + 1
			if len(runes) > k {
				runes = append(runes[k:], runes[:k]...)
			}
		}
		for i, ru := range runes {
			if ru != ' ' && rng.Float64() < amount*0.6 {
				runes[i] = noise[rng.IntN(len(noise))]
			}
		}
		b.WriteString(string(runes))
		b.WriteByte('\n')
	}
	return strings.TrimSuffix(b.String(), "\n")
}
