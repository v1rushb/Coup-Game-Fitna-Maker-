package client

import (
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/v1rushb/neoncoup/config"
	"github.com/v1rushb/neoncoup/engine"
	"github.com/v1rushb/neoncoup/protocol"
)

var sizes = [][2]int{{100, 30}, {120, 40}, {160, 50}, {100, 45}, {200, 30}}

// fataler is the part of testing.T that checkFrame needs; the harness
// implements it too, because t.Fatal must not run on the program goroutine.
type fataler interface {
	Fatalf(format string, args ...any)
}

// recorder captures the first failure from the program goroutine.
type recorder struct {
	mu  sync.Mutex
	err string
}

func (r *recorder) Fatalf(format string, args ...any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.err == "" {
		r.err = fmt.Sprintf(format, args...)
	}
	panic(errStop)
}

func (r *recorder) failed() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.err
}

var errStop = errors.New("harness stop")

func isStop(r any) bool {
	err, ok := r.(error)
	return ok && errors.Is(err, errStop)
}

// checkFrame asserts the rendered frame is exactly w x h cells.
func checkFrame(t fataler, a *App, where string) {
	ow, oh := a.w, a.h
	defer func() { a.w, a.h = ow, oh }()
	for _, sz := range sizes {
		a.w, a.h = sz[0], sz[1]
		out := a.View().Content
		lines := strings.Split(out, "\n")
		if len(lines) != sz[1] {
			t.Fatalf("%s at %dx%d: %d lines", where, sz[0], sz[1], len(lines))
		}
		for i, l := range lines {
			if w := lipgloss.Width(l); w != sz[0] {
				t.Fatalf("%s at %dx%d: line %d is %d wide:\n%s", where, sz[0], sz[1], i, w, ansi.Strip(out))
			}
		}
		if a.t.ASCII {
			for _, r := range ansi.Strip(out) {
				if r > 0x7f {
					t.Fatalf("%s at %dx%d: non-ASCII %q in ASCII mode:\n%s", where, sz[0], sz[1], r, ansi.Strip(out))
				}
			}
		}
	}
}

// harness wraps the app so tests can poke it from the program goroutine.
type harness struct {
	*App
	t      *recorder
	rng    *rand.Rand
	mu     sync.Mutex
	over   bool
	frames int
	log    []string
	pick   func(g *gameScreen) engine.Intent
}

type probeMsg struct{}

func (h *harness) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if _, ok := msg.(probeMsg); ok {
		func() {
			defer func() {
				if r := recover(); r != nil && !isStop(r) {
					h.t.mu.Lock()
					if h.t.err == "" {
						h.t.err = fmt.Sprint("panic: ", r)
					}
					h.t.mu.Unlock()
				}
			}()
			h.probe()
		}()
		return h, nil
	}
	_, cmd := h.App.Update(msg)
	return h, cmd
}

func (h *harness) press(keys ...string) {
	for _, k := range keys {
		var msg tea.KeyPressMsg
		switch k {
		case "enter":
			msg = tea.KeyPressMsg{Code: tea.KeyEnter}
		case "esc":
			msg = tea.KeyPressMsg{Code: tea.KeyEscape}
		default:
			r := []rune(k)[0]
			msg = tea.KeyPressMsg{Code: r, Text: k}
		}
		h.App.Update(msg)
	}
}

// keysFor translates an intent into the keystrokes a player would type.
func keysFor(g *gameScreen, in engine.Intent) []string {
	switch in.Kind {
	case engine.IntentAction:
		for i, a := range actionKeys {
			if a == in.Action {
				keys := []string{fmt.Sprint(i + 1)}
				if a.Targeted() {
					ts := g.targets(a)
					if len(ts) > 1 {
						for j, s := range ts {
							if s == in.Target {
								keys = append(keys, fmt.Sprint(j+1))
							}
						}
					}
				}
				return keys
			}
		}
	case engine.IntentChallenge:
		return []string{"c"}
	case engine.IntentPass:
		return []string{"p"}
	case engine.IntentBlock:
		blocks := g.v.Pending.Action.Blockers(g.v.Magistrate)
		if len(blocks) == 1 {
			return []string{"b"}
		}
		for i, c := range blocks {
			if c == in.Claim {
				return []string{"b", fmt.Sprint(i + 1)}
			}
		}
	case engine.IntentReveal, engine.IntentLose:
		return []string{fmt.Sprint(in.Card + 1)}
	case engine.IntentExchange:
		var keys []string
		for _, k := range in.Keep {
			keys = append(keys, fmt.Sprint(k+1))
		}
		if len(in.Keep) > 1 {
			keys = append(keys, "enter")
		}
		return keys
	}
	return nil
}

func (h *harness) probe() {
	h.frames++
	// Checking every size on every probe is slow under -race; every fifth
	// frame still covers each screen and phase many times over.
	if h.frames%5 == 1 {
		checkFrame(h.t, h.App, fmt.Sprintf("screen %d", h.scr))
	}
	a := h.App
	if a.scr == scrBoot {
		h.press("x")
	}
	g := a.game
	if g == nil {
		return
	}
	if g.tutor != nil && g.tutor.intro {
		h.press("enter")
		return
	}
	if g.over != nil {
		h.mu.Lock()
		h.over = true
		h.mu.Unlock()
		return
	}
	if g.awaitingMe() && !g.sent {
		in := h.pick(g)
		keys := keysFor(g, in)
		h.log = append(h.log, fmt.Sprintf("%s via %v", in, keys))
		h.press(keys...)
		if !g.sent && g.over == nil {
			h.t.Fatalf("keys %v for %s did not send an intent (phase %s, mode %d)", keys, in, g.v.Phase, g.mode)
		}
	}
}

func runHarness(t *testing.T, opts Options, pick func(*rand.Rand, *gameScreen) engine.Intent, timeout time.Duration) *harness {
	t.Helper()
	h := &harness{t: &recorder{}, rng: rand.New(rand.NewPCG(7, 7))}
	h.App = New(opts)
	h.pick = func(g *gameScreen) engine.Intent { return pick(h.rng, g) }
	p := tea.NewProgram(h, tea.WithInput(nil), tea.WithOutput(io.Discard), tea.WithWindowSize(110, 34), tea.WithoutSignals(), tea.WithoutRenderer())
	done := make(chan error, 1)
	go func() {
		_, err := p.Run()
		done <- err
	}()
	deadline := time.After(timeout)
	tick := time.NewTicker(40 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(err)
			}
			return h
		case <-tick.C:
			if e := h.t.failed(); e != "" {
				p.Quit()
				<-done
				t.Fatal(e)
			}
			p.Send(probeMsg{})
			h.mu.Lock()
			over := h.over
			h.mu.Unlock()
			if over {
				p.Send(probeMsg{}) // render the results overlay once more
				time.Sleep(50 * time.Millisecond)
				p.Quit()
				<-done
				if e := h.t.failed(); e != "" {
					t.Fatal(e)
				}
				return h
			}
		case <-deadline:
			p.Quit()
			t.Fatalf("game did not finish in %v (frames %d)\nlast moves: %v", timeout, h.frames, lastN(h.log, 10))
		}
	}
}

func randomPick(r *rand.Rand, g *gameScreen) engine.Intent {
	// Prefer passing in windows so games stay short, but exercise everything.
	l := g.v.Legal
	if g.v.Phase.Timed() && r.Float64() < 0.7 {
		return engine.PassIntent()
	}
	return l[r.IntN(len(l))]
}

func testStore(t *testing.T) *config.Store {
	t.Helper()
	s, err := config.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestSoloGameThroughTheUI(t *testing.T) {
	for _, ascii := range []bool{false, true} {
		t.Run(fmt.Sprintf("ascii=%v", ascii), func(t *testing.T) {
			store := testStore(t)
			cfg := config.Default()
			cfg.SkipBoot = true
			opts := Options{
				Backend: &LocalBackend{}, Store: store, Config: cfg, ASCII: ascii, Version: "test",
				Start: Start{Kind: StartSolo, Solo: SoloOptions{Bots: 3, Level: "normal", Instant: true, WindowMS: 4000, Magistrate: ascii}},
			}
			h := runHarness(t, opts, randomPick, 180*time.Second)
			if h.game == nil || h.game.over == nil {
				t.Fatal("no game over")
			}
			if h.game.replayPath == "" {
				t.Fatal("replay was not saved")
			}
			if _, err := config.LoadReplay(h.game.replayPath); err != nil {
				t.Fatal(err)
			}
			if st := store.LoadStats(); st.Games != 1 {
				t.Fatalf("stats not recorded: %+v", st)
			}
		})
	}
}

func TestTutorialThroughTheUI(t *testing.T) {
	cfg := config.Default()
	cfg.SkipBoot = true
	opts := Options{Backend: &LocalBackend{}, Store: testStore(t), Config: cfg, Version: "test", Start: Start{Kind: StartTutorial}}
	followCoach := func(r *rand.Rand, g *gameScreen) engine.Intent {
		if l, ok := g.tutor.lesson(g.v); ok && l.want != nil {
			for _, in := range g.v.Legal {
				if l.want(in) {
					return in
				}
			}
			panic(fmt.Sprintf("lesson %q has no legal intent", l.title))
		}
		return randomPick(r, g)
	}
	h := runHarness(t, opts, followCoach, 120*time.Second)
	if h.game == nil || h.game.over == nil {
		t.Fatal("tutorial did not finish")
	}
	// The scripted part must have happened: VEX caught bluffing and couped.
	var sawCaught, sawCoup bool
	for _, l := range h.game.log {
		sawCaught = sawCaught || strings.Contains(l.text, "[VEX] WAS BLUFFING")
		sawCoup = sawCoup || strings.Contains(l.text, "LAUNCHES A COUP ON [VEX]")
	}
	if !sawCaught || !sawCoup {
		t.Fatalf("scripted beats missing (caught=%v coup=%v)", sawCaught, sawCoup)
	}
}

func TestStaticScreensRender(t *testing.T) {
	for _, ascii := range []bool{false, true} {
		cfg := config.Default()
		a := New(Options{Backend: &LocalBackend{}, Store: testStore(t), Config: cfg, ASCII: ascii, Version: "test"})
		a.w, a.h = 110, 34
		for _, s := range []screen{scrBoot, scrMenu, scrSolo, scrHost, scrJoin, scrConnecting, scrStats, scrReplays, scrSettings} {
			a.goTo(s)
			if s == scrBoot {
				a.now = a.boot.start.Add(3 * time.Second)
			}
			checkFrame(t, a, fmt.Sprintf("screen %d ascii=%v", s, ascii))
		}
		a.help = true
		checkFrame(t, a, "help overlay")
		a.help = false
		a.showError("TEST", "something broke")
		checkFrame(t, a, "error")
		// Lobby with a host view.
		a.welcome = protocol.Welcome{You: "m1", Host: true, Room: "NEON-AB23"}
		a.lobbyMsg = protocol.Lobby{Room: "NEON-AB23", HostID: "m1", Settings: protocol.DefaultSettings(),
			Members: []protocol.Member{{ID: "m1", Name: "HOST", Host: true, Connected: true}, {ID: "m2", Name: "GUEST", Connected: true}}}
		a.conn = &Connection{Room: "NEON-AB23", Host: &HostInfo{Lines: []string{"neoncoup join NEON-AB23"}}}
		a.goTo(scrLobby)
		checkFrame(t, a, "lobby")
	}
}

func TestTooSmallTerminal(t *testing.T) {
	a := New(Options{Backend: &LocalBackend{}, Config: config.Default(), Version: "test"})
	a.w, a.h = 80, 24
	a.goTo(scrMenu)
	out := ansi.Strip(a.View().Content)
	if !strings.Contains(out, "needs at least 100x30") || !strings.Contains(out, "80x24") {
		t.Fatalf("no friendly size message:\n%s", out)
	}
	a.w, a.h = 40, 10
	if out := a.View().Content; out == "" {
		t.Fatal("tiny terminal rendered nothing")
	}
}

func TestMain(m *testing.M) {
	// Keep test runs out of the real config dir.
	dir, _ := os.MkdirTemp("", "neoncoup-test")
	os.Setenv("NEONCOUP_HOME", dir)
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}
