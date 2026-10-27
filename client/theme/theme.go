// Package theme holds the NEON//COUP palette, styles and the Unicode/ASCII
// glyph switch. Colors are truecolor; Bubble Tea downsamples them to 256 or
// 16 colors (or none, with NO_COLOR) for the terminal at hand.
package theme

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/v1rushb/neoncoup/engine"
)

// Palette entries.
var (
	Bg      = lipgloss.Color("#0b0814")
	Panel   = lipgloss.Color("#140f22")
	Fg      = lipgloss.Color("#e8e3f0")
	Dim     = lipgloss.Color("#6b6185")
	Faint   = lipgloss.Color("#3a3150")
	Magenta = lipgloss.Color("#ff2bd6")
	Cyan    = lipgloss.Color("#00f0ff")
	Acid    = lipgloss.Color("#e6ff00")
	Green   = lipgloss.Color("#39ff14")
	Red     = lipgloss.Color("#ff3355")
	Purple  = lipgloss.Color("#9d4dff")
	Orange  = lipgloss.Color("#ff8c1a")
	White   = lipgloss.Color("#ffffff")
)

// Palette is a full set of colors.
type Palette struct {
	Bg, Panel, Fg, Dim, Faint                              color.Color
	Magenta, Cyan, Acid, Green, Red, Purple, Orange, White color.Color
}

// Palettes: the neon default, a colorblind-safe set (after Okabe and Ito,
// brightened for a dark background, so red/green pairs become vermillion and
// bluish green) and a high-contrast set with pure black, white and full
// saturation.
var palettes = map[string]Palette{
	"neon": {Bg, Panel, Fg, Dim, Faint, Magenta, Cyan, Acid, Green, Red, Purple, Orange, White},
	"colorblind": {
		Bg: lipgloss.Color("#0b0814"), Panel: lipgloss.Color("#151122"), Fg: lipgloss.Color("#ecebf2"),
		Dim: lipgloss.Color("#8a84a3"), Faint: lipgloss.Color("#4a4462"),
		Magenta: lipgloss.Color("#e48bc0"), Cyan: lipgloss.Color("#6cc4f5"), Acid: lipgloss.Color("#f5ea5a"),
		Green: lipgloss.Color("#1fc495"), Red: lipgloss.Color("#ff7a1f"), Purple: lipgloss.Color("#b49cff"),
		Orange: lipgloss.Color("#f0b000"), White: lipgloss.Color("#ffffff"),
	},
	"contrast": {
		Bg: lipgloss.Color("#000000"), Panel: lipgloss.Color("#000000"), Fg: lipgloss.Color("#ffffff"),
		Dim: lipgloss.Color("#d8d8d8"), Faint: lipgloss.Color("#a8a8a8"),
		Magenta: lipgloss.Color("#ff40ff"), Cyan: lipgloss.Color("#00ffff"), Acid: lipgloss.Color("#ffff00"),
		Green: lipgloss.Color("#00ff00"), Red: lipgloss.Color("#ff5050"), Purple: lipgloss.Color("#d090ff"),
		Orange: lipgloss.Color("#ffa500"), White: lipgloss.Color("#ffffff"),
	},
}

// PaletteNames lists the palettes in menu order.
var PaletteNames = []string{"neon", "colorblind", "contrast"}

// PaletteTitle is a palette's menu label.
func PaletteTitle(name string) string {
	switch name {
	case "colorblind":
		return "COLORBLIND-SAFE"
	case "contrast":
		return "HIGH CONTRAST"
	}
	return "NEON"
}

// Use switches the process to a palette (unknown names mean neon). Styles
// built before the switch keep their old colors, so callers rebuild their
// Theme and art renderer afterwards. It is process-wide: the SSH server,
// which hosts many sessions, never calls it.
func Use(name string) {
	p, ok := palettes[name]
	if !ok {
		p = palettes["neon"]
	}
	Bg, Panel, Fg, Dim, Faint = p.Bg, p.Panel, p.Fg, p.Dim, p.Faint
	Magenta, Cyan, Acid, Green, Red, Purple, Orange, White = p.Magenta, p.Cyan, p.Acid, p.Green, p.Red, p.Purple, p.Orange, p.White
}

// CharColor is a character's signature color.
func CharColor(c engine.Character) color.Color {
	switch c {
	case engine.Duke:
		return Magenta
	case engine.Assassin:
		return Red
	case engine.Captain:
		return Cyan
	case engine.Ambassador:
		return Green
	case engine.Contessa:
		return Acid
	case engine.Magistrate:
		return Orange
	}
	return Dim
}

// Theme bundles styles for one session (each SSH session gets its own).
type Theme struct {
	ASCII bool

	Border      lipgloss.Border
	ThickBorder lipgloss.Border
	RoundBorder lipgloss.Border

	Text, Muted, Faint, Bold     lipgloss.Style
	Title, Accent, Hot, Warn     lipgloss.Style
	Gain, Loss, Key, KeyDisabled lipgloss.Style
	Box, BoxActive, BoxDead      lipgloss.Style
	Terminal                     lipgloss.Style
}

// asciiBorder replaces box drawing with + - |.
var asciiBorder = lipgloss.Border{
	Top: "-", Bottom: "-", Left: "|", Right: "|",
	TopLeft: "+", TopRight: "+", BottomLeft: "+", BottomRight: "+",
	MiddleLeft: "+", MiddleRight: "+", Middle: "+", MiddleTop: "+", MiddleBottom: "+",
}

// New builds a theme. ascii swaps every Unicode glyph for plain ASCII.
func New(ascii bool) *Theme {
	t := &Theme{ASCII: ascii}
	if ascii {
		t.Border, t.ThickBorder, t.RoundBorder = asciiBorder, asciiBorder, asciiBorder
	} else {
		t.Border = lipgloss.NormalBorder()
		t.ThickBorder = lipgloss.DoubleBorder()
		t.RoundBorder = lipgloss.RoundedBorder()
	}
	s := lipgloss.NewStyle
	t.Text = s().Foreground(Fg)
	t.Muted = s().Foreground(Dim)
	t.Faint = s().Foreground(Faint)
	t.Bold = s().Foreground(White).Bold(true)
	t.Title = s().Foreground(Magenta).Bold(true)
	t.Accent = s().Foreground(Cyan)
	t.Hot = s().Foreground(Magenta)
	t.Warn = s().Foreground(Acid).Bold(true)
	t.Gain = s().Foreground(Green).Bold(true)
	t.Loss = s().Foreground(Red).Bold(true)
	t.Key = s().Foreground(Cyan).Bold(true)
	t.KeyDisabled = s().Foreground(Faint).Background(Panel)
	t.Box = s().Border(t.RoundBorder).BorderForeground(Faint).Padding(0, 1)
	t.BoxActive = s().Border(t.RoundBorder).BorderForeground(Cyan).Padding(0, 1)
	t.BoxDead = s().Border(t.RoundBorder).BorderForeground(Faint).Foreground(Faint).Padding(0, 1)
	t.Terminal = s().Border(t.ThickBorder).BorderForeground(Magenta).Padding(0, 1)
	return t
}

// G returns the Unicode glyph, or its ASCII stand-in in ASCII mode.
func (t *Theme) G(unicode, ascii string) string {
	if t.ASCII {
		return ascii
	}
	return unicode
}

// Coin is the currency mark.
func (t *Theme) Coin() string { return t.G("¢", "c") }

// asciiMap replaces Unicode art glyphs with ASCII lookalikes.
var asciiMap = map[rune]string{
	'█': "#", '▓': "%", '▒': ":", '░': ".", '▀': "\"", '▄': "_", '▌': "[", '▐': "]",
	'◉': "@", '◎': "@", '◆': "*", '◇': "*", '●': "o", '○': ".", '■': "#", '□': "o",
	'═': "=", '║': "|", '╔': "+", '╗': "+", '╚': "+", '╝': "+", '╦': "+", '╩': "+", '╠': "+", '╣': "+", '╬': "+",
	'┌': "+", '┐': "+", '└': "+", '┘': "+", '├': "+", '┤': "+", '┬': "+", '┴': "+", '┼': "+",
	'╭': "+", '╮': "+", '╰': "+", '╯': "+",
	'─': "-", '━': "-", '│': "|", '┃': "|", '╱': "/", '╲': "\\", '╳': "X",
	'¢': "c", '✖': "x", '✓': "v", '▶': ">", '◀': "<", '►': ">", '◄': "<", '▲': "^", '▼': "v",
	'✧': "*", '≡': "=", '»': ">", '«': "<", '▮': "|", '▯': ".", '╟': "+", '╢': "+", '⇄': "<>", '⟳': "@",
	'♪': "~", '♫': "~", '·': ".", '…': "~", '•': "*", '→': ">", '←': "<", '⚡': "!",
	'▁': "_", '▂': "_", '▃': "_", '▅': "=", '▆': "=", '▇': "#", '▔': "-", '¦': "|", '╌': "-",
	'▸': ">", '↻': "@", '◂': "<",
	'⠋': "|", '⠙': "/", '⠹': "-", '⠸': "\\", '⠼': "|", '⠴': "/", '⠦': "-", '⠧': "\\", '⠇': "|", '⠏': "/",
}

// Asciify maps known Unicode glyphs in s to ASCII; other non-ASCII runes become '?'.
func Asciify(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r < 0x80 {
			b.WriteRune(r)
			continue
		}
		if a, ok := asciiMap[r]; ok {
			b.WriteString(a)
			continue
		}
		b.WriteByte('?')
	}
	return b.String()
}

// A applies Asciify in ASCII mode.
func (t *Theme) A(s string) string {
	if t.ASCII {
		return Asciify(s)
	}
	return s
}

// Lerp blends two colors in RGB; t=0 is a, t=1 is b.
func Lerp(a, b color.Color, t float64) color.Color {
	t = max(0, min(1, t))
	ar, ag, ab, _ := a.RGBA()
	br, bg, bb, _ := b.RGBA()
	mix := func(x, y uint32) uint8 {
		return uint8((float64(x>>8)*(1-t) + float64(y>>8)*t) + 0.5)
	}
	return color.RGBA{R: mix(ar, br), G: mix(ag, bg), B: mix(ab, bb), A: 0xff}
}
