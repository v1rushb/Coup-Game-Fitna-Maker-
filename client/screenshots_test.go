package client

import (
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/v1rushb/neoncoup/bot"
	"github.com/v1rushb/neoncoup/config"
	"github.com/v1rushb/neoncoup/engine"
	"github.com/v1rushb/neoncoup/protocol"
)

// scripted drives a rigged engine game into a game screen exactly as the
// server would: redacted events, then the redacted state.
type scripted struct {
	t testing.TB
	a *App
	g *engine.Game
	s *gameScreen
}

func newScripted(t testing.TB, ascii bool) *scripted {
	t.Helper()
	cfg := config.Default()
	cfg.Name = "RUNNER"
	a := New(Options{Backend: &LocalBackend{}, Config: cfg, ASCII: ascii, Version: "1.0.0"})
	a.w, a.h = 120, 40
	a.now = time.Date(2087, 1, 1, 0, 0, 0, 0, time.UTC)
	a.mode = "host"
	a.conn = &Connection{Room: "NEON-7K2Q", Via: "you are hosting"}
	a.welcome = protocol.Welcome{You: "m1", Host: true, Room: "NEON-7K2Q"}
	D, A, C, B, T := engine.Duke, engine.Assassin, engine.Captain, engine.Ambassador, engine.Contessa
	rig := &engine.Rig{
		Hands: [][]engine.Character{{A, T}, {D, C}, {B, D}, {C, T}, {A, B}},
		Deck:  []engine.Character{D, A, C, B, T},
	}
	g, err := engine.New(engine.Config{Players: 5, Seed: 2087, First: 0, Rig: rig})
	if err != nil {
		t.Fatal(err)
	}
	seats := []protocol.Seat{
		{Seat: 0, Name: "VEX", Bot: true, Level: "hard"},
		{Seat: 1, Name: "RUNNER", ID: "m1"},
		{Seat: 2, Name: "MOTH", Bot: true, Level: "normal"},
		{Seat: 3, Name: "GHOSTWIRE", Bot: true, Level: "hard"},
		{Seat: 4, Name: "NULLBYTE", Bot: true, Level: "easy"},
	}
	gs := protocol.GameStart{Game: 3, Seats: seats, YouSeat: 1, Settings: protocol.DefaultSettings()}
	s := newGameScreen(a, gs)
	a.game = s
	a.scr = scrGame
	sc := &scripted{t: t, a: a, g: g, s: s}
	sc.feed(g.StartEvents())
	return sc
}

func (sc *scripted) feed(evs []engine.Event) {
	for _, e := range engine.RedactAll(evs, 1) {
		sc.s.enqueue(sc.a, qItem{ev: &e})
	}
	st := protocol.State{View: sc.g.View(1)}
	if sc.g.Phase().Timed() {
		st.DeadlineMS, st.TimerMS = 5200, 8000
	}
	sc.s.enqueue(sc.a, qItem{state: &st})
	sc.s.drain(sc.a)
	// Let counters settle and effects finish, as if time passed.
	for i := range sc.s.shown {
		sc.s.shown[i] = float64(sc.g.View(1).Players[i].Coins)
	}
	sc.s.coinFx, sc.s.flips, sc.s.oppFx, sc.s.glitch = map[int]time.Time{}, map[int]time.Time{}, map[int]time.Time{}, map[int]time.Time{}
	sc.s.jolt = time.Time{}
}

func (sc *scripted) do(seat int, in engine.Intent) {
	sc.t.Helper()
	evs, err := sc.g.Apply(seat, in)
	if err != nil {
		sc.t.Fatalf("seat %d %s: %v", seat, in, err)
	}
	sc.feed(evs)
}

func (sc *scripted) passAll() {
	for _, s := range sc.g.Awaiting() {
		sc.do(s, engine.PassIntent())
	}
}

func (sc *scripted) chat(name, text string, bot bool) {
	env, _ := protocol.Decode(protocol.MustEncode(protocol.TChat, protocol.Chat{Name: name, Text: text, Bot: bot}))
	sc.s.onChat(sc.a, env)
}

// shot renders the app as plain text.
func shot(a *App) string {
	lines := strings.Split(ansi.Strip(a.View().Content), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " ")
	}
	return strings.TrimRight(strings.Join(lines, "\n"), "\n") + "\n"
}

// TestScreenshots writes text screenshots of every screen for the README
// when NEONCOUP_SCREENSHOTS names a directory.
func TestScreenshots(t *testing.T) {
	dir := os.Getenv("NEONCOUP_SCREENSHOTS")
	if dir == "" {
		t.Skip("set NEONCOUP_SCREENSHOTS=dir to write screenshots")
	}
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name+".txt"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Static screens at the minimum size.
	cfg := config.Default()
	cfg.Name = "RUNNER"
	store, _ := config.Open(t.TempDir())
	_, _ = store.AddGame(config.GameStats{Mode: "solo", Won: true, Bluffs: 9, BluffsCaught: 2, Challenges: 6, ChallengesWon: 4, Blocks: 5, CoinsEarned: 61, Coups: 3, Turns: 41})
	_, _ = store.AddGame(config.GameStats{Mode: "host", Won: false, Eliminated: true, Bluffs: 4, BluffsCaught: 2, Challenges: 3, ChallengesWon: 1, Blocks: 1, CoinsEarned: 22, Coups: 1, Turns: 28})
	_, _ = store.AddGame(config.GameStats{Mode: "solo", Won: true, Bluffs: 6, BluffsCaught: 1, Challenges: 2, ChallengesWon: 2, Blocks: 2, CoinsEarned: 40, Coups: 2, Turns: 33})
	a := New(Options{Backend: &LocalBackend{}, Store: store, Config: cfg, Version: "1.0.0"})
	a.rng = rand.New(rand.NewPCG(7, 7))
	a.w, a.h = 100, 30
	a.goTo(scrBoot)
	a.now = a.boot.start.Add(2700 * time.Millisecond)
	a.frame = 10 // between the logo's idle flickers
	write("01-boot", shot(a))
	a.goTo(scrMenu)
	write("02-menu", shot(a))
	a.goTo(scrSolo)
	write("03-solo-setup", shot(a))
	a.goTo(scrHost)
	write("04-host-setup", shot(a))
	a.goTo(scrJoin)
	a.join.input.Set("NEON-7K2Q")
	write("05-join", shot(a))
	a.welcome = protocol.Welcome{You: "m1", Host: true, Room: "NEON-7K2Q"}
	a.lobbyMsg = protocol.Lobby{Room: "NEON-7K2Q", HostID: "m1", Settings: protocol.DefaultSettings(), Members: []protocol.Member{
		{ID: "m1", Name: "RUNNER", Host: true, Connected: true},
		{ID: "m2", Name: "TRINITY", Ready: true, Connected: true},
		{ID: "m3", Name: "CASE", Connected: true},
		{ID: "m4", Name: "MOLLY", Spectator: true, Connected: true},
	}}
	a.conn = &Connection{Room: "NEON-7K2Q", Host: &HostInfo{Room: "NEON-7K2Q", Lines: []string{
		"same network: neoncoup join NEON-7K2Q",
		"direct:       neoncoup join 192.168.1.20:7777",
		"internet:     neoncoup join NEON-7K2Q --relay wss://relay.example.com",
	}}}
	a.lobby = newLobbyScreen()
	for _, c := range []protocol.Chat{{Seat: -1, Text: "TRINITY jacked in", System: true}, {Name: "TRINITY", Text: "ready when you are"}, {Seat: -1, Text: "MOLLY is watching", System: true}} {
		env, _ := protocol.Decode(protocol.MustEncode(protocol.TChat, c))
		a.lobby.onChat(a, env)
	}
	a.goTo(scrLobby)
	write("06-lobby", shot(a))
	a.goTo(scrStats)
	write("15-stats", shot(a))
	a.goTo(scrMenu)
	a.help = true
	write("13-help", shot(a))
	a.help = false
	a.w, a.h = 80, 24
	write("18-too-small", shot(a))

	// The table, from a scripted 5-player game.
	sc := newScripted(t, false)
	sc.do(0, engine.ActionIntent(engine.Income, -1))
	sc.do(1, engine.ActionIntent(engine.Tax, -1))
	sc.passAll()
	sc.do(2, engine.ActionIntent(engine.ForeignAid, -1))
	sc.do(1, engine.BlockIntent(engine.Duke))
	sc.passAll()
	sc.do(3, engine.ActionIntent(engine.Steal, 0))
	sc.passAll()
	sc.do(0, engine.BlockIntent(engine.Ambassador))
	sc.do(3, engine.ChallengeIntent())
	sc.do(0, engine.RevealIntent(1)) // VEX flips Contessa: caught
	sc.do(4, engine.ActionIntent(engine.Tax, -1))
	sc.do(2, engine.ChallengeIntent())
	sc.do(4, engine.RevealIntent(0)) // NULLBYTE flips Assassin: caught
	sc.chat("VEX", "your firewall is made of paper", true)
	sys, _ := protocol.Decode(protocol.MustEncode(protocol.TChat, protocol.Chat{Seat: -1, Text: "TRINITY lost the uplink. Seat held for 60s.", System: true}))
	sc.s.onChat(sc.a, sys)
	sc.chat("CASE", "nobody believes that Duke", false)
	sc.do(0, engine.ActionIntent(engine.Income, -1))
	sc.do(1, engine.ActionIntent(engine.Steal, 2))
	sc.passAll()
	sc.passAll()
	sc.do(2, engine.ActionIntent(engine.Tax, -1))
	sc.passAll()
	sc.do(3, engine.ActionIntent(engine.Assassinate, 1)) // a bluff aimed at you
	sc.a.frame = 3
	write("07-table-challenge-window", shot(sc.a))
	sc.a.w, sc.a.h = 100, 30
	write("08-table-100x30", shot(sc.a))
	sc.a.w, sc.a.h = 120, 40

	// Let it through, then decline to block: you must give up a card.
	sc.passAll()
	sc.do(1, engine.PassIntent())
	write("10-lose-influence", shot(sc.a))
	sc.do(1, engine.LoseIntent(1))

	sc.do(4, engine.ActionIntent(engine.Income, -1))
	sc.do(0, engine.ActionIntent(engine.Income, -1))
	sc.do(1, engine.ActionIntent(engine.Exchange, -1))
	sc.passAll()
	sc.s.exSel = []int{0}
	write("11-exchange", shot(sc.a))
	sc.do(1, engine.ExchangeIntent(0))
	sc.do(2, engine.ActionIntent(engine.Tax, -1))
	sc.do(1, engine.ChallengeIntent())
	write("09-prove-claim-opponent", shot(sc.a))

	// ASCII mode, same moment.
	asc := newScripted(t, true)
	asc.do(0, engine.ActionIntent(engine.Income, -1))
	asc.do(1, engine.ActionIntent(engine.Tax, -1))
	asc.passAll()
	asc.do(2, engine.ActionIntent(engine.Steal, 1))
	write("17-ascii-mode", shot(asc.a))

	// Elimination and results.
	fl := newScripted(t, false)
	for range 12 {
		seat := fl.g.Awaiting()[0]
		fl.do(seat, engine.ActionIntent(engine.Income, -1))
	}
	fl.s.myDeath = fl.a.now.Add(-1300 * time.Millisecond)
	fl.a.rng = rand.New(rand.NewPCG(1, 2))
	write("12-flatlined", shot(fl.a))
	fl.s.myDeath = time.Time{}
	_, won := recordedGame(t)
	fl.s.onGameOver(fl.a, protocol.GameOver{Winner: 1, Names: []string{"VEX", "RUNNER", "MOTH", "GHOSTWIRE", "NULLBYTE"}, Replay: won})
	fl.s.overAt = fl.a.now.Add(-5 * time.Second)
	fl.s.stats = config.GameStats{Bluffs: 3, BluffsCaught: 1, Challenges: 2, ChallengesWon: 2, Blocks: 1, CoinsEarned: 17, Coups: 2}
	fl.s.replayPath = "~/.config/neoncoup/replays/20870101-000000-7K2Q.json"
	write("14a-victory", shot(fl.a))
	fl.s.over.Winner = 3
	write("14b-defeat", shot(fl.a))
	fl.s.over = nil
	fl.s.mode = modeTaunt
	write("16-taunt-menu", shot(fl.a))

	// Replay viewer over a real recorded game.
	rp, rep := recordedGame(t)
	steps := rep.Steps
	rp.a.viewReplay(rep)
	if rp.a.replay == nil {
		t.Fatal("replay did not open")
	}
	rp.a.replay.pos = len(steps) * 2 / 3
	rp.a.w, rp.a.h = 120, 40
	write("19-replay", shot(rp.a))

	// The UX update: a move being announced, the full tracker, inspect and
	// the settings overlay.
	an := newScripted(t, false)
	an.a.rng = rand.New(rand.NewPCG(1, 2))
	an.s.speed = protocol.SpeedRelaxed
	an.queueFrames(an.apply(0, engine.ActionIntent(engine.Steal, 2)))
	an.s.pump(an.a)
	an.a.now = an.a.now.Add(600 * time.Millisecond)
	write("21-announcement", shot(an.a))
	wide := newScripted(t, false)
	wide.do(0, engine.ActionIntent(engine.Tax, -1))
	wide.do(1, engine.ChallengeIntent())
	wide.do(0, engine.RevealIntent(0))
	wide.a.w, wide.a.h = 160, 45
	write("22-tracker-160x45", shot(wide.a))
	wide.a.w, wide.a.h = 120, 40
	wide.s.inspect = 0
	write("23-inspect", shot(wide.a))
	wide.s.inspect = -1
	wide.s.prefs = &prefsOverlay{}
	write("24-settings-overlay", shot(wide.a))

	// Tutorial coach.
	tu := newScripted(t, false)
	tu.s.tutor = newTutor()
	tu.s.tutor.intro = false
	tu.s.tutor.title = "SMELL THE BLUFF"
	tu.s.tutor.text = "VEX claims DUKE. MOTH just showed confidence in a Duke and you hold one.\nVEX never acted like a Duke before. Call it: press c to CHALLENGE."
	tu.do(0, engine.ActionIntent(engine.Tax, -1))
	write("20-tutorial", shot(tu.a))
	tu.s.tutor.intro = true
	write("20b-tutorial-intro", shot(tu.a))
	if readme := os.Getenv("NEONCOUP_README"); readme != "" {
		embedScreens(t, readme, dir)
	}
	fmt.Println("screenshots written to", dir)
}

// screenTitles lists the README's screenshots in order.
var screenTitles = []struct{ file, title string }{
	{"01-boot", "Boot: fake BIOS POST with a glitching logo"},
	{"02-menu", "Main menu"},
	{"03-solo-setup", "Solo setup"},
	{"04-host-setup", "Host setup"},
	{"05-join", "Join"},
	{"06-lobby", "Lobby (host view)"},
	{"07-table-challenge-window", "Game table, 120x40: an assassination aimed at you, challenge window open"},
	{"08-table-100x30", "Game table at the minimum size, 100x30"},
	{"09-prove-claim-opponent", "You called a bluff: waiting for the claimant to flip a card"},
	{"10-lose-influence", "Losing an influence: pick the card to flip"},
	{"11-exchange", "Ambassador exchange: pick what to keep"},
	{"12-flatlined", "Your elimination (the screen tears apart, in color)"},
	{"13-help", "Help overlay (?)"},
	{"14a-victory", "Victory"},
	{"14b-defeat", "Defeat"},
	{"15-stats", "Stats"},
	{"16-taunt-menu", "Taunt menu (t)"},
	{"17-ascii-mode", "--ascii mode"},
	{"18-too-small", "Terminal too small"},
	{"19-replay", "Replay viewer: every hand face up"},
	{"20b-tutorial-intro", "Tutorial intro"},
	{"20-tutorial", "Tutorial coach"},
	{"21-announcement", "A move being announced: banner, target marked, the rest dimmed"},
	{"22-tracker-160x45", "The card tracker at 160x45, after a bluff was caught"},
	{"23-inspect", "Inspect (tab): one player's claims and history"},
	{"24-settings-overlay", "Settings overlay (esc)"},
}

// embedScreens rewrites the README section between the screens markers.
func embedScreens(t *testing.T, readme, dir string) {
	const begin, end = "<!-- screens:begin -->", "<!-- screens:end -->"
	b, err := os.ReadFile(readme)
	if err != nil {
		t.Fatal(err)
	}
	doc := string(b)
	i, j := strings.Index(doc, begin), strings.Index(doc, end)
	if i < 0 || j < i {
		t.Fatalf("%s has no %s ... %s markers", readme, begin, end)
	}
	var sb strings.Builder
	sb.WriteString(begin + "\n")
	for _, s := range screenTitles {
		body, err := os.ReadFile(filepath.Join(dir, s.file+".txt"))
		if err != nil {
			t.Fatal(err)
		}
		open := ""
		if s.file == "07-table-challenge-window" {
			open = " open"
		}
		fmt.Fprintf(&sb, "<details%s>\n<summary><b>%s</b></summary>\n\n```text\n%s\n```\n\n</details>\n\n", open, s.title, strings.TrimRight(string(body), "\n"))
	}
	doc = doc[:i] + sb.String() + doc[j:]
	if err := os.WriteFile(readme, []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
}

// recordedGame plays the scripted table to the end with Black ICE bots in
// every seat, picking the first bot seeds with which seat 1 (you) wins, and
// returns it with its replay.
func recordedGame(t *testing.T) (*scripted, protocol.Replay) {
	t.Helper()
	for seed := uint64(7); seed < 200; seed++ {
		rp := newScripted(t, false)
		var steps []protocol.Step
		brains := make([]bot.Bot, 5)
		for i := range brains {
			brains[i] = bot.New(bot.Hard, i, uint64(i)+seed, bot.Roster[i])
			for _, e := range rp.g.StartEvents() {
				brains[i].Observe(e.Redact(i))
			}
		}
		for !rp.g.Over() && len(steps) < 400 {
			seat := rp.g.Awaiting()[0]
			in := brains[seat].Decide(rp.g.View(seat))
			steps = append(steps, protocol.Step{Seat: seat, Intent: in})
			evs, err := rp.g.Apply(seat, in)
			if err != nil {
				t.Fatal(err)
			}
			for i, b := range brains {
				for _, e := range evs {
					b.Observe(e.Redact(i))
				}
			}
		}
		if rp.g.Over() && rp.g.Winner() == 1 {
			return rp, protocol.Replay{Format: protocol.ReplayFormat, Created: rp.a.now, Room: "NEON-7K2Q",
				Config: rp.g.Config(), Seats: rp.s.seats, Steps: steps, Winner: 1}
		}
	}
	t.Fatal("no seed lets seat 1 win")
	return nil, protocol.Replay{}
}
