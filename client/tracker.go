package client

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/v1rushb/neoncoup/client/art"
	"github.com/v1rushb/neoncoup/client/theme"
	"github.com/v1rushb/neoncoup/engine"
)

// tally is the card tracker's count for one character, from one seat's view.
type tally struct {
	// Revealed copies are face up for everyone.
	Revealed int
	// Mine are copies you can see that nobody else can: your face-down cards
	// (and, mid-exchange, the cards you drew).
	Mine int
	// Unseen copies could be in an opponent's hand or the deck. Zero means
	// any claim of the character is a bluff.
	Unseen int
}

// courtCount tallies every character in play from v's point of view.
func courtCount(v engine.PlayerView) map[engine.Character]tally {
	out := map[engine.Character]tally{}
	for _, c := range engine.DeckComposition(v.Magistrate) {
		out[c] = tally{}
	}
	for _, p := range v.Players {
		for _, c := range p.Revealed {
			t := out[c]
			t.Revealed++
			out[c] = t
		}
	}
	if len(v.ExchangeOptions) > 0 {
		// Your face-down cards plus the two you drew.
		for _, c := range v.ExchangeOptions {
			t := out[c]
			t.Mine++
			out[c] = t
		}
	} else {
		for _, c := range v.Hand {
			if !c.Revealed {
				t := out[c.Char]
				t.Mine++
				out[c.Char] = t
			}
		}
	}
	for c, t := range out {
		t.Unseen = max(0, engine.CopiesPerCharacter-t.Revealed-t.Mine)
		out[c] = t
	}
	return out
}

// Tracker layout.
const (
	trackCardW = art.CompactW
	trackCardH = art.CompactH + 2 // card art plus the two count lines
	stackW     = 8                // deck and treasury column
)

// trackerCards renders the card row: each character as a mini card with its
// counts underneath, the deck and treasury beside it. ok is false when the
// row does not fit in w x h.
func (g *gameScreen) trackerCards(a *App, w, h int) (string, bool) {
	t := a.t
	chars := engine.DeckComposition(g.v.Magistrate)
	counts := courtCount(g.v)
	// One space between cards keeps their count lines apart.
	if len(chars)*(trackCardW+1)+stackW > w || h < trackCardH {
		return "", false
	}
	var cols []string
	for _, c := range chars {
		n := counts[c]
		card := a.art.Card(c, art.CardOpts{Compact: true, Stamp: n.Unseen == 0})
		revSt, unSt := t.Faint, t.Accent
		if n.Revealed > 0 {
			revSt = t.Text
		}
		if n.Unseen == 0 {
			unSt = t.Loss
		}
		rev := revSt.Render(fmt.Sprintf("REVEALED %d/%d", n.Revealed, engine.CopiesPerCharacter))
		un := unSt.Render(fmt.Sprintf("UNSEEN %d", n.Unseen))
		col := lipgloss.JoinVertical(lipgloss.Left, card, padTo(rev, trackCardW), padTo(un, trackCardW))
		cols = append(cols, col, " ")
	}
	cols = append(cols, g.stacks(a))
	return lipgloss.JoinHorizontal(lipgloss.Top, cols...), true
}

// stacks draws the court deck as a stack of card backs (more layers for a
// fuller deck) with the treasury below it.
func (g *gameScreen) stacks(a *App) string {
	t := a.t
	deck := g.v.DeckCount
	layers := 0
	switch {
	case deck >= 9:
		layers = 3
	case deck >= 4:
		layers = 2
	case deck >= 1:
		layers = 1
	}
	rows := []string{"  ╭────╮", " ╭│▒░▒░│", "╭││░▒░▒│", "╰╰╰────╯"}
	switch layers {
	case 2:
		rows = []string{"  ╭────╮", " ╭│▒░▒░│", " ││░▒░▒│", " ╰╰────╯"}
	case 1:
		rows = []string{"  ╭────╮", "  │▒░▒░│", "  │░▒░▒│", "  ╰────╯"}
	case 0:
		rows = []string{"  ┌╌╌╌╌┐", "  ╎    ╎", "  ╎    ╎", "  └╌╌╌╌┘"}
	}
	deckSt := lipgloss.NewStyle().Foreground(theme.Purple)
	if g.shuffling(a) {
		// Riffle: the top cards jog sideways and the backs flicker.
		deckSt = lipgloss.NewStyle().Foreground(theme.Magenta)
		if a.frame%2 == 0 {
			for i := 0; i < 2 && i < len(rows); i++ {
				if strings.HasPrefix(rows[i], " ") {
					rows[i] = rows[i][1:] + " "
				}
			}
		}
		for i := range rows {
			rows[i] = strings.NewReplacer("▒", "░", "░", "▒").Replace(rows[i])
		}
	}
	var out []string
	for _, r := range rows {
		out = append(out, deckSt.Render(t.A(r)))
	}
	out = append(out,
		t.Muted.Render(fmt.Sprintf("DECK %3d", deck)),
		"",
		t.Faint.Render("TREASURY"),
		lipgloss.NewStyle().Foreground(theme.Acid).Bold(true).Render(fmt.Sprintf("%7d%s", g.v.Treasury, t.Coin())))
	return strings.Join(out, "\n")
}

// abbr is a character's four-letter tag, e.g. CAPT.
func abbr(c engine.Character) string {
	switch c {
	case engine.Assassin:
		return "ASSN"
	case engine.Captain:
		return "CAPT"
	case engine.Ambassador:
		return "AMBS"
	case engine.Contessa:
		return "CONT"
	case engine.Magistrate:
		return "MAGI"
	}
	return "DUKE"
}

const medTileW = 15

// trackerMedium is the middle size: a three-line tile per character (name
// bar, revealed, unseen). ok is false when it does not fit.
func (g *gameScreen) trackerMedium(a *App, w, h int) ([]string, bool) {
	t := a.t
	chars := engine.DeckComposition(g.v.Magistrate)
	perRow := w / medTileW
	if perRow < 2 {
		return nil, false
	}
	rows := (len(chars) + perRow - 1) / perRow
	if rows*4-1 > h {
		return nil, false
	}
	counts := courtCount(g.v)
	var out []string
	for r := range rows {
		var name, rev, un []string
		for _, c := range chars[r*perRow : min(len(chars), (r+1)*perRow)] {
			n := counts[c]
			bar := lipgloss.NewStyle().Foreground(theme.Bg).Background(theme.CharColor(c)).Bold(true)
			tag := padTo(" "+c.Title(), medTileW-2)
			revSt, unSt := t.Faint, t.Accent
			if n.Revealed > 0 {
				revSt = t.Text
			}
			unTxt := fmt.Sprintf("UNSEEN %d", n.Unseen)
			if n.Unseen == 0 {
				bar = lipgloss.NewStyle().Foreground(theme.White).Background(theme.Red).Bold(true)
				tag = padTo(" "+c.Title(), medTileW-2)
				unSt = t.Loss
				unTxt = t.G("▓ NO COPIES ▓", "# NO COPIES #")
			}
			name = append(name, bar.Render(tag)+"  ")
			rev = append(rev, padTo(revSt.Render(fmt.Sprintf("REVEALED %d/%d", n.Revealed, engine.CopiesPerCharacter)), medTileW))
			un = append(un, padTo(unSt.Render(unTxt), medTileW))
		}
		if r > 0 {
			out = append(out, "")
		}
		out = append(out, strings.Join(name, ""), strings.Join(rev, ""), strings.Join(un, ""))
	}
	return out, true
}

const tileW = 14

// trackerTiles is the narrow fallback: a one-line tile per character with
// its counts, packed into rows.
func (g *gameScreen) trackerTiles(a *App, w int) []string {
	t := a.t
	counts := courtCount(g.v)
	var tiles []string
	for _, c := range engine.DeckComposition(g.v.Magistrate) {
		n := counts[c]
		txt := fmt.Sprintf("%s R%d/%d U%d", abbr(c), n.Revealed, engine.CopiesPerCharacter, n.Unseen)
		st := lipgloss.NewStyle().Foreground(theme.CharColor(c))
		if n.Unseen == 0 {
			st = t.Loss
			txt += t.G("▓", "#")
		}
		tiles = append(tiles, st.Render(txt))
	}
	perRow := max(1, w/tileW)
	var rows []string
	for i := 0; i < len(tiles); i += perRow {
		var row []string
		for _, c := range tiles[i:min(len(tiles), i+perRow)] {
			row = append(row, padTo(c, tileW))
		}
		rows = append(rows, strings.Join(row, ""))
	}
	return rows
}
