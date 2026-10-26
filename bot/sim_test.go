package bot

import (
	"fmt"
	"math/rand/v2"
	"testing"

	"github.com/v1rushb/neoncoup/engine"
)

type simResult struct {
	winner int
	steps  int
	turns  int
}

// simulate plays one bot-only game, checking invariants after every intent.
func simulate(t testing.TB, cfg engine.Config, levels []Level, seed uint64) simResult {
	g, err := engine.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	n := cfg.Players
	bots := make([]Bot, n)
	pers := Pick(n, seed, nil)
	for i := range n {
		bots[i] = New(levels[i], i, seed+uint64(i)*31, pers[i])
	}
	order := rand.New(rand.NewPCG(seed, 3))
	broadcast := func(evs []engine.Event) {
		for i, b := range bots {
			for _, e := range evs {
				b.Observe(e.Redact(i))
			}
			b.Taunt()
		}
	}
	broadcast(g.StartEvents())
	steps := 0
	for !g.Over() {
		steps++
		if steps > 20000 {
			t.Fatalf("seed %d: game did not terminate (turn %d)", seed, g.Turn())
		}
		aw := g.Awaiting()
		seat := aw[order.IntN(len(aw))]
		v := g.View(seat)
		in := bots[seat].Decide(v)
		evs, err := g.Apply(seat, in)
		if err != nil {
			t.Fatalf("seed %d: %s bot chose illegal %s in %s: %v", seed, levels[seat], in, g.Phase(), err)
		}
		if err := g.CheckInvariants(); err != nil {
			t.Fatalf("seed %d step %d: %v", seed, steps, err)
		}
		broadcast(evs)
	}
	return simResult{winner: g.Winner(), steps: steps, turns: g.Turn()}
}

// TestSimulation plays 10,000 bot-vs-bot games with random seeds and mixed
// difficulties. Each step asserts coin conservation and card accounting
// (engine.CheckInvariants); every game must terminate without a panic.
func TestSimulation(t *testing.T) {
	games := 10000
	if testing.Short() {
		games = 500
	}
	seeds := rand.New(rand.NewPCG(uint64(0xC0FFEE), 1))
	totalTurns, longest, stalled := 0, 0, 0
	for i := range games {
		seed := seeds.Uint64()
		n := engine.MinPlayers + int(seed%5)
		levels := make([]Level, n)
		for s := range levels {
			levels[s] = Levels[(seed>>(8+2*s))%3]
		}
		cfg := engine.Config{Players: n, Seed: seed, First: -1, Magistrate: i%4 == 0}
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("game %d seed %d panicked: %v", i, seed, r)
				}
			}()
			res := simulate(t, cfg, levels, seed)
			totalTurns += res.turns
			longest = max(longest, res.turns)
			if res.turns >= stallTurn {
				stalled++
			}
		}()
	}
	t.Logf("%d games, %.1f turns/game, longest %d, %d reached the anti-stall turn",
		games, float64(totalTurns)/float64(games), longest, stalled)
}

// TestBotStrengthOrdering checks that harder bots actually win more.
func TestBotStrengthOrdering(t *testing.T) {
	games := 1800
	if testing.Short() {
		games = 300
	}
	wins := map[Level]int{}
	for i := range games {
		// Rotate seats so no level keeps the first-player advantage.
		levels := []Level{Hard, Normal, Easy}
		r := i % 3
		levels = append(levels[r:], levels[:r]...)
		res := simulate(t, engine.Config{Players: 3, Seed: uint64(i) + 1, First: -1}, levels, uint64(i)*13+5)
		wins[levels[res.winner]]++
	}
	t.Logf("3-way win rates: hard %.1f%%  normal %.1f%%  easy %.1f%%",
		pct(wins[Hard], games), pct(wins[Normal], games), pct(wins[Easy], games))
	if wins[Hard] <= wins[Normal] || wins[Normal] <= wins[Easy] || pct(wins[Hard], games) < 42 {
		t.Fatalf("difficulty ordering violated: %v", wins)
	}

	duel := func(a, b Level) float64 {
		w := 0
		for i := range games / 2 {
			levels := []Level{a, b}
			flip := i%2 == 1
			if flip {
				levels = []Level{b, a}
			}
			res := simulate(t, engine.Config{Players: 2, Seed: uint64(i) + 99, First: -1}, levels, uint64(i)*17+3)
			if (res.winner == 0) != flip {
				w++
			}
		}
		return pct(w, games/2)
	}
	hn := duel(Hard, Normal)
	ne := duel(Normal, Easy)
	t.Logf("1v1: hard beats normal %.1f%%, normal beats easy %.1f%%", hn, ne)
	if hn < 60 || ne < 60 {
		t.Fatalf("1v1 ordering violated: hard-normal %.1f%%, normal-easy %.1f%%", hn, ne)
	}
}

func pct(n, d int) float64 { return 100 * float64(n) / float64(d) }

func TestBotsOnlySeeRedactedState(t *testing.T) {
	// The Bot interface is the whole contract: a redacted view in, an intent
	// out. This test documents that a view for one seat carries no other
	// seat's face-down cards, which is all a bot ever gets.
	g, _ := engine.New(engine.Config{Players: 4, Seed: 5, First: 0})
	snap := g.Snapshot()
	v := g.View(2)
	for i, p := range v.Players {
		if len(p.Revealed) != 0 {
			t.Fatalf("seat %d shows revealed cards at start", i)
		}
	}
	for i, c := range v.Hand {
		if c != snap.Hands[2][i] {
			t.Fatal("own hand mismatch")
		}
	}
}

func TestParseLevel(t *testing.T) {
	for in, want := range map[string]Level{"easy": Easy, "Script Kiddie": Easy, "fixer": Normal, "hard": Hard, "black-ice": Hard, "BLACK ICE": Hard} {
		got, err := ParseLevel(in)
		if err != nil || got != want {
			t.Errorf("ParseLevel(%q) = %v, %v", in, got, err)
		}
	}
	if _, err := ParseLevel("godlike"); err == nil {
		t.Error("unknown level accepted")
	}
}

func TestPickUniqueHandles(t *testing.T) {
	ps := Pick(5, 1, map[string]bool{"VEX": true})
	seen := map[string]bool{}
	for _, p := range ps {
		if seen[p.Handle] || p.Handle == "VEX" {
			t.Fatalf("duplicate or taken handle %q", p.Handle)
		}
		seen[p.Handle] = true
	}
	if got := len(Pick(20, 1, nil)); got != 20 {
		t.Fatalf("Pick(20) returned %d", got)
	}
}

func TestTauntsAppear(t *testing.T) {
	seen := 0
	for seed := range uint64(40) {
		g, _ := engine.New(engine.Config{Players: 3, Seed: seed, First: -1})
		bots := []Bot{New(Hard, 0, seed, Roster[0]), New(Normal, 1, seed, Roster[1]), New(Easy, 2, seed, Roster[2])}
		for _, b := range bots {
			for _, e := range g.StartEvents() {
				b.Observe(e)
			}
		}
		for !g.Over() {
			s := g.Awaiting()[0]
			evs, err := g.Apply(s, bots[s].Decide(g.View(s)))
			if err != nil {
				t.Fatal(err)
			}
			for i, b := range bots {
				for _, e := range evs {
					b.Observe(e.Redact(i))
				}
				if line, ok := b.Taunt(); ok {
					if line == "" {
						t.Fatal("empty taunt")
					}
					seen++
				}
			}
		}
	}
	if seen == 0 {
		t.Fatal("bots never taunted")
	}
}

func BenchmarkHardGame(b *testing.B) {
	for i := range b.N {
		simulate(b, engine.Config{Players: 4, Seed: uint64(i), First: -1}, []Level{Hard, Hard, Hard, Hard}, uint64(i))
	}
}

func ExampleLevel_Title() {
	fmt.Println(Easy.Title(), "/", Normal.Title(), "/", Hard.Title())
	// Output: Script Kiddie / Fixer / Black ICE
}
