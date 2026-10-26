package art

import (
	"math/rand/v2"
	"os"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/v1rushb/neoncoup/client/theme"
	"github.com/v1rushb/neoncoup/engine"
)

func checkBlock(t *testing.T, what, s string, w, h int) {
	t.Helper()
	lines := strings.Split(s, "\n")
	if len(lines) != h {
		t.Fatalf("%s: %d lines, want %d\n%s", what, len(lines), h, ansi.Strip(s))
	}
	for i, l := range lines {
		if got := lipgloss.Width(l); got != w {
			t.Fatalf("%s line %d: width %d, want %d: %q", what, i, got, w, ansi.Strip(l))
		}
	}
}

func TestArtLinesHaveExactWidths(t *testing.T) {
	for c, f := range Faces {
		for i, l := range f.Art {
			if w := lipgloss.Width(l); w != artW {
				t.Errorf("%s art line %d is %d wide: %q", c, i, w, l)
			}
		}
		for i, l := range f.Mini {
			if w := lipgloss.Width(l); w != compactInner {
				t.Errorf("%s mini line %d is %d wide: %q", c, i, w, l)
			}
		}
		for i, l := range f.Ability {
			if w := lipgloss.Width(l); w > artW-1 {
				t.Errorf("%s ability %d too long: %q", c, i, l)
			}
		}
		for i, l := range f.Short {
			if w := lipgloss.Width(l); w > compactInner {
				t.Errorf("%s short ability %d too long: %q", c, i, l)
			}
		}
	}
	for i, l := range back {
		if w := lipgloss.Width(l); w != artW {
			t.Errorf("back line %d is %d wide", i, w)
		}
	}
	for i, l := range miniBack {
		if w := lipgloss.Width(l); w != compactInner {
			t.Errorf("mini back line %d is %d wide", i, w)
		}
	}
	for i := range Logo {
		if lipgloss.Width(Logo[i]) != lipgloss.Width(Logo[0]) || len(LogoASCII[i]) != len(LogoASCII[0]) {
			t.Errorf("logo row %d width mismatch", i)
		}
	}
}

func TestCardsRenderAtFixedSize(t *testing.T) {
	for _, ascii := range []bool{false, true} {
		r := NewRenderer(theme.New(ascii))
		for _, c := range engine.AllCharacters {
			for _, o := range []CardOpts{{}, {Lost: true}, {Selected: true, Label: "1"}, {Shimmer: 2}, {Width: 6}, {Hint: "PROVES IT"}} {
				checkBlock(t, c.String(), r.Card(c, o), CardW, CardH)
				o.Compact = true
				checkBlock(t, c.String()+" compact", r.Card(c, o), CompactW, CompactH)
			}
		}
		checkBlock(t, "back", r.Back(CardOpts{}), CardW, CardH)
		checkBlock(t, "back compact", r.Back(CardOpts{Compact: true}), CompactW, CompactH)
		if ascii {
			for _, c := range engine.AllCharacters {
				plain := ansi.Strip(r.Card(c, CardOpts{}))
				for _, ru := range plain {
					if ru > 0x7f {
						t.Fatalf("ASCII mode card %s contains %q", c, ru)
					}
				}
			}
		}
	}
}

func TestGlitchKeepsShape(t *testing.T) {
	in := "hello world\nsecond line"
	out := Glitch(in, 0.8, rand.New(rand.NewPCG(1, 2)))
	if len(strings.Split(out, "\n")) != 2 {
		t.Fatal("line count changed")
	}
	for i, l := range strings.Split(out, "\n") {
		if len([]rune(l)) != len([]rune(strings.Split(in, "\n")[i])) {
			t.Fatal("line length changed")
		}
	}
}

// TestPreview writes the cards as plain text when NEONCOUP_PREVIEW is set.
func TestPreview(t *testing.T) {
	path := os.Getenv("NEONCOUP_PREVIEW")
	if path == "" {
		t.Skip("set NEONCOUP_PREVIEW=file to dump the art")
	}
	var b strings.Builder
	for _, ascii := range []bool{false, true} {
		r := NewRenderer(theme.New(ascii))
		var cards []string
		for _, c := range engine.AllCharacters {
			cards = append(cards, r.Card(c, CardOpts{}))
		}
		cards = append(cards, r.Back(CardOpts{}))
		b.WriteString(ansi.Strip(lipgloss.JoinHorizontal(lipgloss.Top, cards...)) + "\n\n")
		var small []string
		for _, c := range engine.AllCharacters {
			small = append(small, r.Card(c, CardOpts{Compact: true}))
		}
		small = append(small, r.Back(CardOpts{Compact: true}), r.Card(engine.Duke, CardOpts{Compact: true, Lost: true}))
		b.WriteString(ansi.Strip(lipgloss.JoinHorizontal(lipgloss.Top, small...)) + "\n\n")
		b.WriteString(ansi.Strip(r.Card(engine.Contessa, CardOpts{Lost: true})) + "\n\n")
	}
	for _, l := range Logo {
		b.WriteString(l + "\n")
	}
	for _, l := range LogoASCII {
		b.WriteString(l + "\n")
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}
