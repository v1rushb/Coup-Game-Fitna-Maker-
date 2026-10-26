package client

import (
	"fmt"
	"math"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/v1rushb/neoncoup/client/art"
	"github.com/v1rushb/neoncoup/client/theme"
	"github.com/v1rushb/neoncoup/engine"
)

// bootScreen is the fake BIOS POST that plays on launch.
type bootScreen struct {
	start time.Time
	lines []bootLine
	skip  bool
}

type bootLine struct {
	text   string
	result string
	atMS   int // when the line appears, in milliseconds
}

func newBoot(a *App) bootScreen {
	audioStatus := "OK"
	if !a.sound.Available() {
		audioStatus = "NO DEVICE (muted)"
	}
	lines := []bootLine{
		{"NEONTEK BIOS v4.20  (C) 2087 NEONTEK SYSTEMS", "", 0},
		{"CPU: QUANTUM-8 @ 6.66 THz", "OK", 150},
		{"MEMORY TEST: 655360K", "OK", 320},
		{"VECTORCORE GPU", "OK", 470},
		{"NEURAL LINK", "CALIBRATED", 650},
		{fmt.Sprintf("MOUNTING /dev/court (%d cards)", engine.DeckSize(false)), "OK", 820},
		{"SEEDING ENTROPY POOL", "OK", 960},
		{"CHIPTUNE DRIVER (2xPULSE TRI NOISE)", audioStatus, 1120},
		{"FIREWALL", "DISABLED (it's that kind of night)", 1300},
		{"JACKING IN", "", 1500},
	}
	return bootScreen{start: a.now, lines: lines}
}

// tick reports whether the boot sequence is finished.
func (b *bootScreen) tick(a *App) bool {
	if b.start.IsZero() {
		b.start = a.now
	}
	return b.skip || a.now.Sub(b.start) > 3400*time.Millisecond
}

func (b *bootScreen) view(a *App) string {
	el := a.now.Sub(b.start)
	var out []string
	for _, l := range b.lines {
		if el < time.Duration(l.atMS)*time.Millisecond {
			break
		}
		dots := 44 - lipgloss.Width(l.text)
		line := a.t.Muted.Render(l.text)
		if l.result != "" {
			line += a.t.Faint.Render(" " + strings.Repeat(".", max(2, dots)) + " ")
			st := a.t.Gain
			if strings.HasPrefix(l.result, "NO") || strings.HasPrefix(l.result, "DIS") {
				st = a.t.Warn
			}
			line += st.Render(l.result)
		}
		out = append(out, line)
	}
	if el > 1500*time.Millisecond {
		n := int((el - 1500*time.Millisecond) / (120 * time.Millisecond))
		out[len(out)-1] += a.t.Accent.Render(" " + strings.Repeat(".", min(n, 12)))
	}
	text := strings.Join(out, "\n")
	if el > 1900*time.Millisecond {
		glitch := 0.0
		if el < 2300*time.Millisecond {
			glitch = 0.6
		}
		text += "\n\n\n" + a.logo(glitch)
	}
	if el > 2600*time.Millisecond && (a.frame/6)%2 == 0 {
		text += "\n\n" + a.t.Faint.Render("press any key")
	}
	return lipgloss.NewStyle().Padding(1, 3).Render(text)
}

// logo renders the wordmark with a magenta-to-cyan gradient. glitch in 0..1
// tears rows sideways and sprinkles noise.
func (a *App) logo(glitch float64) string {
	rows := art.Logo[:]
	if a.t.ASCII {
		rows = art.LogoASCII[:]
	}
	// Occasional idle flicker keeps the menu alive.
	if glitch == 0 && a.frame%73 < 3 {
		glitch = 0.25
	}
	var out []string
	for i, row := range rows {
		runes := []rune(row)
		shift := 0
		if glitch > 0 && a.rng.Float64() < glitch {
			shift = a.rng.IntN(5) - 2
		}
		var b strings.Builder
		if shift > 0 {
			b.WriteString(strings.Repeat(" ", shift))
		}
		n := len(runes)
		for j, ru := range runes {
			if glitch > 0 && ru != ' ' && a.rng.Float64() < glitch*0.15 {
				ru = []rune("▓▒░█#%")[a.rng.IntN(6)]
				if a.t.ASCII {
					ru = []rune("#%&@")[a.rng.IntN(4)]
				}
			}
			tpos := float64(j)/float64(max(1, n-1)) + 0.15*math.Sin(float64(a.frame)/10+float64(i))
			c := theme.Lerp(theme.Magenta, theme.Cyan, math.Max(0, math.Min(1, tpos)))
			b.WriteString(lipgloss.NewStyle().Foreground(c).Render(string(ru)))
		}
		line := b.String()
		if shift < 0 {
			line = cutLeft(line, -shift)
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

// cutLeft drops the first n cells of a styled line.
func cutLeft(s string, n int) string {
	return ansi.Cut(s, n, lipgloss.Width(s))
}
