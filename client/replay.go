package client

import (
	"fmt"
	"image/color"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/v1rushb/neoncoup/client/art"
	"github.com/v1rushb/neoncoup/client/theme"
	"github.com/v1rushb/neoncoup/config"
	"github.com/v1rushb/neoncoup/engine"
	"github.com/v1rushb/neoncoup/protocol"
)

// --- stats -------------------------------------------------------------------

type statsScreen struct {
	st  config.Stats
	ok  bool
	err string
}

func newStatsScreen(a *App) statsScreen {
	if a.opts.Store == nil {
		return statsScreen{err: "stats are kept on your own machine; not available here"}
	}
	return statsScreen{st: a.opts.Store.LoadStats(), ok: true}
}

func (s *statsScreen) view(a *App) string {
	t := a.t
	if !s.ok {
		return a.centerBox("STATS", t.Muted.Render(s.err)+"\n\n"+t.Faint.Render("esc back"))
	}
	st := s.st
	pct := func(f float64) string { return fmt.Sprintf("%5.1f%%", f*100) }
	bar := func(f float64, col color.Color) string {
		n := int(f*30 + 0.5)
		return lipgloss.NewStyle().Foreground(col).Render(strings.Repeat(t.G("█", "#"), n)) + t.Faint.Render(strings.Repeat(t.G("░", "."), 30-n))
	}
	rows := []string{
		t.Text.Render(fmt.Sprintf("%-20s %d", "games played", st.Games)),
		t.Text.Render(fmt.Sprintf("%-20s %d", "wins", st.Wins)) + "   " + bar(st.WinRate(), theme.Green) + " " + pct(st.WinRate()),
		t.Text.Render(fmt.Sprintf("%-20s %d / %d best", "win streak", st.CurrentStreak, st.BestStreak)),
		"",
		t.Text.Render(fmt.Sprintf("%-20s %d (%d caught)", "bluffs", st.Bluffs, st.BluffsCaught)),
		t.Text.Render(fmt.Sprintf("%-20s", "bluff success")) + " " + bar(st.BluffRate(), theme.Magenta) + " " + pct(st.BluffRate()),
		t.Text.Render(fmt.Sprintf("%-20s %d (%d won)", "challenges", st.Challenges, st.ChallengesWon)),
		t.Text.Render(fmt.Sprintf("%-20s", "challenge accuracy")) + " " + bar(st.ChallengeAccuracy(), theme.Cyan) + " " + pct(st.ChallengeAccuracy()),
		"",
		t.Text.Render(fmt.Sprintf("%-20s %d", "blocks", st.Blocks)),
		t.Text.Render(fmt.Sprintf("%-20s %d", "coins earned", st.CoinsEarned)),
		t.Text.Render(fmt.Sprintf("%-20s %d", "coups launched", st.Coups)),
		t.Text.Render(fmt.Sprintf("%-20s %d", "times flatlined", st.Eliminations)),
		t.Text.Render(fmt.Sprintf("%-20s %d turns", "longest game", st.LongestGame)),
	}
	var modes []string
	for _, m := range []string{"solo", "host", "join"} {
		if g := st.GamesByMode[m]; g > 0 {
			modes = append(modes, fmt.Sprintf("%s %d/%d", m, st.WinsByMode[m], g))
		}
	}
	if len(modes) > 0 {
		rows = append(rows, "", t.Muted.Render("wins by mode: "+strings.Join(modes, " · ")))
	}
	if !st.LastPlayed.IsZero() {
		rows = append(rows, t.Faint.Render("last run: "+st.LastPlayed.Format("2006-01-02 15:04")))
	}
	return a.centerBox("RUNNER RECORD // "+a.cfg.Name, t.A(strings.Join(rows, "\n"))+"\n\n"+t.Faint.Render("esc back"))
}

// --- replay list -------------------------------------------------------------

type replayList struct {
	items []config.ReplayInfo
	sel   int
	err   string
}

func newReplayList(a *App) replayList {
	if a.opts.Store == nil {
		return replayList{err: "replays live on your own machine; not available here"}
	}
	items, err := a.opts.Store.Replays()
	l := replayList{items: items}
	if err != nil {
		l.err = err.Error()
	}
	return l
}

func (l *replayList) key(a *App, msg tea.KeyPressMsg) tea.Cmd {
	switch msg.String() {
	case "esc", "q":
		a.goTo(scrMenu)
	case "up", "k":
		l.sel = max(0, l.sel-1)
	case "down", "j":
		l.sel = min(max(0, len(l.items)-1), l.sel+1)
	case "enter":
		if l.sel < len(l.items) {
			return a.openReplay(l.items[l.sel].Path)
		}
	}
	return nil
}

func (l *replayList) view(a *App) string {
	t := a.t
	if l.err != "" {
		return a.centerBox("REPLAYS", t.Muted.Render(l.err)+"\n\n"+t.Faint.Render("esc back"))
	}
	if len(l.items) == 0 {
		return a.centerBox("REPLAYS", t.Muted.Render("no saved games yet. finish a game and it lands here.")+"\n\n"+t.Faint.Render("esc back"))
	}
	var rows []string
	start := max(0, min(l.sel-8, len(l.items)-16))
	for i := start; i < min(len(l.items), start+16); i++ {
		it := l.items[i]
		line := fmt.Sprintf("%s  %d runners  %3d moves  winner %s", it.Created.Local().Format("2006-01-02 15:04"), it.Players, it.Turns, it.Winner)
		if i == l.sel {
			rows = append(rows, lipgloss.NewStyle().Foreground(theme.Bg).Background(theme.Cyan).Render(" "+line+" "))
		} else {
			rows = append(rows, t.Text.Render(" "+line))
		}
	}
	return a.centerBox("REPLAYS", strings.Join(rows, "\n")+"\n\n"+t.Faint.Render("↑↓ select · enter watch · esc back"))
}

func (a *App) openReplay(path string) tea.Cmd {
	r, err := config.LoadReplay(path)
	if err != nil {
		a.showError("REPLAY CORRUPTED", err.Error())
		return nil
	}
	return a.viewReplay(r)
}

func (a *App) viewReplay(r protocol.Replay) tea.Cmd {
	v, err := newReplayViewer(r)
	if err != nil {
		a.showError("REPLAY DESYNC", err.Error())
		return nil
	}
	a.replay = v
	a.goTo(scrReplay)
	return nil
}

// --- replay viewer -------------------------------------------------------------

type replayFrame struct {
	snap engine.Snapshot
	view engine.PlayerView
	evs  []engine.Event
	step protocol.Step
}

type replayViewer struct {
	r       protocol.Replay
	frames  []replayFrame // frames[0] is the opening position
	pos     int
	playing bool
	last    time.Time
	speed   time.Duration
}

func newReplayViewer(r protocol.Replay) (*replayViewer, error) {
	g, err := engine.New(r.Config)
	if err != nil {
		return nil, err
	}
	v := &replayViewer{r: r, speed: 900 * time.Millisecond}
	v.frames = append(v.frames, replayFrame{snap: g.Snapshot(), view: g.View(-1), evs: g.StartEvents()})
	for i, st := range r.Steps {
		evs, err := g.Apply(st.Seat, st.Intent)
		if err != nil {
			return nil, fmt.Errorf("step %d (%s by seat %d) does not replay: %w", i+1, st.Intent, st.Seat, err)
		}
		v.frames = append(v.frames, replayFrame{snap: g.Snapshot(), view: g.View(-1), evs: evs, step: st})
	}
	return v, nil
}

func (v *replayViewer) name(seat int) string {
	if seat >= 0 && seat < len(v.r.Seats) {
		return v.r.Seats[seat].Name
	}
	return "?"
}

func (v *replayViewer) key(a *App, msg tea.KeyPressMsg) tea.Cmd {
	switch msg.String() {
	case "right", "l", "n":
		v.pos = min(len(v.frames)-1, v.pos+1)
	case "left", "h", "b":
		v.pos = max(0, v.pos-1)
	case "up", "pgup":
		v.pos = max(0, v.pos-10)
	case "down", "pgdown":
		v.pos = min(len(v.frames)-1, v.pos+10)
	case "home", "g":
		v.pos = 0
	case "end", "G":
		v.pos = len(v.frames) - 1
	case "space", "p":
		v.playing = !v.playing
		v.last = a.now
		if v.playing && v.pos == len(v.frames)-1 {
			v.pos = 0
		}
	case "+", "=":
		v.speed = max(150*time.Millisecond, v.speed-150*time.Millisecond)
	case "-":
		v.speed = min(3*time.Second, v.speed+150*time.Millisecond)
	case "esc", "q":
		v.playing = false
		if a.prev == scrReplays {
			a.goTo(scrReplays)
		} else if a.game != nil {
			a.goTo(scrGame)
		} else {
			a.goTo(scrMenu)
		}
	}
	return nil
}

func (v *replayViewer) tick(a *App) {
	if !v.playing || a.now.Sub(v.last) < v.speed {
		return
	}
	v.last = a.now
	if v.pos < len(v.frames)-1 {
		v.pos++
	} else {
		v.playing = false
	}
}

func (v *replayViewer) view(a *App) string {
	t := a.t
	f := v.frames[v.pos]
	W := a.w
	logW := max(36, min(52, W*35/100))
	mainW := W - logW

	head := t.Title.Render(" REPLAY ") + t.Muted.Render(fmt.Sprintf(" %s  step %d/%d  turn %d", v.r.Created.Local().Format("2006-01-02 15:04"), v.pos, len(v.frames)-1, f.snap.Turn))
	state := "paused"
	if v.playing {
		state = "playing"
	}
	head = padTo(head, W-20) + t.Accent.Render(state)

	// Every hand, face up: it's a replay.
	var seats []string
	for i := range v.r.Seats {
		seats = append(seats, v.seatBlock(a, f, i, mainW))
	}
	table := strings.Join(seats, "\n")
	center := t.Accent.Render(fmt.Sprintf("TREASURY %d%s · DECK %d", f.snap.Treasury, t.Coin(), len(f.snap.Deck)))
	if v.pos > 0 {
		center += "\n" + t.Bold.Render(fmt.Sprintf("> %s: %s", v.name(f.step.Seat), describeIntent(f.step.Intent, v.name)))
	}
	if p := pendingSummary(f.view, v.name); p != "" {
		center += "\n" + t.Muted.Render("  on the table: "+p)
	}
	mainCol := lipgloss.JoinVertical(lipgloss.Left, center, "", table)

	// Events up to here.
	var lines []string
	for i := 0; i <= v.pos; i++ {
		for _, e := range v.frames[i].evs {
			if l, ok := narrate(e, v.name, -2, t.Coin()); ok {
				st := t.Text
				if i == v.pos {
					st = t.Hot
				}
				lines = append(lines, st.Render(trunc(t.A(l.text), logW-4)))
			}
		}
	}
	logH := a.h - 4
	logBox := lipgloss.NewStyle().Border(t.RoundBorder).BorderForeground(theme.Faint).Padding(0, 1).Width(logW).Height(logH).
		Render(fitLines(t.Hot.Render("NETLOG")+"\n"+strings.Join(lastN(lines, logH-3), "\n"), logW-4, logH-2))
	keys := t.Faint.Render(t.A("←/→ step · ↑/↓ ±10 · space play/pause · +/- speed · home/end · esc back"))
	body := lipgloss.JoinVertical(lipgloss.Left, head, lipgloss.JoinHorizontal(lipgloss.Top, fit(mainCol, mainW, logH), logBox), keys)
	return body
}

func (v *replayViewer) seatBlock(a *App, f replayFrame, seat, w int) string {
	t := a.t
	var cards []string
	for _, c := range f.snap.Hands[seat] {
		cards = append(cards, a.art.Card(c.Char, art.CardOpts{Compact: true, Lost: c.Revealed}))
	}
	status := t.Text.Render(fmt.Sprintf("%s%d", t.Coin(), f.snap.Coins[seat]))
	if !f.snap.Alive[seat] {
		status = t.Loss.Render("FLATLINED")
	}
	if f.snap.Winner == seat {
		status = t.Gain.Render("WINNER")
	}
	name := t.Bold.Render(v.name(seat))
	if f.snap.Current == seat && f.snap.Phase != engine.PhaseGameOver {
		name = t.Hot.Render(t.G("▶ ", "> ") + v.name(seat))
	}
	info := lipgloss.JoinVertical(lipgloss.Left, name, status)
	return lipgloss.JoinHorizontal(lipgloss.Top, padTo(info, 18), lipgloss.JoinHorizontal(lipgloss.Top, cards...))
}

func describeIntent(in engine.Intent, n namer) string {
	switch in.Kind {
	case engine.IntentAction:
		s := in.Action.Title()
		if in.Target >= 0 {
			s += " → " + n(in.Target)
		}
		return s
	case engine.IntentBlock:
		return "BLOCKS with " + in.Claim.Title()
	case engine.IntentChallenge:
		return "CHALLENGES"
	case engine.IntentPass:
		return "passes"
	case engine.IntentReveal:
		return fmt.Sprintf("flips card %d", in.Card+1)
	case engine.IntentLose:
		return fmt.Sprintf("gives up card %d", in.Card+1)
	case engine.IntentExchange:
		return fmt.Sprintf("keeps %v", in.Keep)
	}
	return in.String()
}
