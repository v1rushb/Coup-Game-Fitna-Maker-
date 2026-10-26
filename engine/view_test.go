package engine

import (
	"encoding/json"
	"math/rand/v2"
	"reflect"
	"strings"
	"testing"
)

func TestViewShowsOnlyOwnHand(t *testing.T) {
	g := rigged(t, [][]Character{{D, T}, {C, B}, {A, A}}, rigOpts{})
	for seat := range 3 {
		v := g.View(seat)
		if len(v.Hand) != 2 || v.Hand[0] != g.players[seat].cards[0] {
			t.Fatalf("seat %d hand %v", seat, v.Hand)
		}
		for _, p := range v.Players {
			if p.Hidden != 2 || len(p.Revealed) != 0 {
				t.Fatalf("public player %+v", p)
			}
		}
	}
	spec := g.View(-1)
	if spec.Hand != nil || spec.Legal != nil {
		t.Fatal("spectator sees a hand or legal moves")
	}
	raw, _ := json.Marshal(spec)
	if strings.Contains(string(raw), `"hand"`) {
		t.Fatalf("spectator JSON has a hand: %s", raw)
	}
}

func TestViewJSONNeverContainsOpponentHiddenCards(t *testing.T) {
	// Seat 1 holds the only face-down Ambassadors; nobody else's JSON may
	// mention "ambassador" until one is revealed.
	g := rigged(t, [][]Character{{D, T}, {B, B}, {A, A}}, rigOpts{deckTop: []Character{B}})
	for _, seat := range []int{0, 2, -1} {
		raw, _ := json.Marshal(g.View(seat))
		if strings.Contains(string(raw), "ambassador") {
			t.Fatalf("seat %d view leaks: %s", seat, raw)
		}
	}
	raw, _ := json.Marshal(g.View(1))
	if !strings.Contains(string(raw), "ambassador") {
		t.Fatal("owner should see their own cards")
	}
}

func TestEventsRedactSecrets(t *testing.T) {
	g := rigged(t, [][]Character{{D, T}, {C, B}}, rigOpts{})
	do(t, g, 0, ActionIntent(Tax, -1))
	do(t, g, 1, ChallengeIntent())
	evs := do(t, g, 0, RevealIntent(0))
	repl, ok := findEvent(evs, EvCardReplaced)
	if !ok || len(repl.Secret) != 1 {
		t.Fatal("unredacted replacement should carry the new card")
	}
	if s := repl.Redact(1).Secret; s != nil {
		t.Fatalf("opponent sees replacement %v", s)
	}
	if s := repl.Redact(-1).Secret; s != nil {
		t.Fatal("spectator sees replacement")
	}
	if len(repl.Redact(0).Secret) != 1 {
		t.Fatal("owner lost their own secret")
	}
	for _, e := range RedactAll(evs, 1) {
		if e.Secret != nil {
			t.Fatalf("RedactAll leaked %+v", e)
		}
	}
}

func TestExchangeDrawIsSecret(t *testing.T) {
	g := rigged(t, [][]Character{{B, D}, {C, C}}, rigOpts{deckTop: []Character{T, A}})
	do(t, g, 0, ActionIntent(Exchange, -1))
	evs := do(t, g, 1, PassIntent())
	draw, _ := findEvent(evs, EvExchangeDraw)
	if draw.Redact(1).Secret != nil || draw.Redact(1).Amount != 2 {
		t.Fatal("others should only learn how many cards were drawn")
	}
}

func TestJSONRoundTrip(t *testing.T) {
	in := Intent{Kind: IntentExchange, Keep: []int{0, 3}, Target: -1}
	raw, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"kind":"exchange"`) {
		t.Fatalf("enums should marshal as names: %s", raw)
	}
	var out Intent
	if err := json.Unmarshal(raw, &out); err != nil || !in.Equal(out) {
		t.Fatalf("round trip %s -> %+v (%v)", raw, out, err)
	}

	ev := Event{Kind: EvBlock, Seat: 2, Other: 0, Claim: Contessa, Action: Assassinate, Reason: ReasonNone}
	raw, _ = json.Marshal(ev)
	var ev2 Event
	if err := json.Unmarshal(raw, &ev2); err != nil || !reflect.DeepEqual(ev, ev2) {
		t.Fatalf("event round trip %s -> %+v (%v)", raw, ev2, err)
	}

	g := rigged(t, [][]Character{{D, T}, {C, B}}, rigOpts{})
	do(t, g, 0, ActionIntent(Tax, -1))
	v := g.View(1)
	raw, _ = json.Marshal(v)
	var v2 PlayerView
	if err := json.Unmarshal(raw, &v2); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(v, v2) {
		t.Fatalf("view round trip:\n%+v\n%+v", v, v2)
	}

	var bad Intent
	if err := json.Unmarshal([]byte(`{"kind":"teleport"}`), &bad); err == nil {
		t.Fatal("unknown enum accepted")
	}
	// A missing target means no target, not seat 0.
	var noTarget Intent
	if err := json.Unmarshal([]byte(`{"kind":"action","action":"income"}`), &noTarget); err != nil || noTarget.Target != -1 {
		t.Fatalf("missing target decoded as %d (%v)", noTarget.Target, err)
	}
}

// playRandom plays a full game choosing uniformly among legal intents.
func playRandom(t testing.TB, cfg Config, choose *rand.Rand) (*Game, []Event, [][2]any) {
	g, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	evs := g.StartEvents()
	var log [][2]any
	for steps := 0; !g.Over(); steps++ {
		if steps > 5000 {
			t.Fatalf("seed %d: no end after %d steps", cfg.Seed, steps)
		}
		aw := g.Awaiting()
		seat := aw[choose.IntN(len(aw))]
		legal := g.Legal(seat)
		if len(legal) == 0 {
			t.Fatalf("seed %d: seat %d awaited with no legal intents in %s", cfg.Seed, seat, g.Phase())
		}
		in := legal[choose.IntN(len(legal))]
		e, err := g.Apply(seat, in)
		if err != nil {
			t.Fatalf("seed %d: legal intent %s refused: %v", cfg.Seed, in, err)
		}
		if err := g.CheckInvariants(); err != nil {
			t.Fatalf("seed %d step %d after %s: %v", cfg.Seed, steps, in, err)
		}
		evs = append(evs, e...)
		log = append(log, [2]any{seat, in})
	}
	return g, evs, log
}

func TestRandomPlayKeepsInvariants(t *testing.T) {
	n := 3000
	if testing.Short() {
		n = 300
	}
	for i := range n {
		cfg := Config{Players: 2 + i%5, Seed: uint64(i) * 7919, First: -1, Magistrate: i%3 == 0}
		g, _, _ := playRandom(t, cfg, rand.New(rand.NewPCG(uint64(i), 1)))
		if g.Winner() < 0 || !g.Alive(g.Winner()) {
			t.Fatalf("seed %d: bad winner %d", cfg.Seed, g.Winner())
		}
	}
}

func TestSameSeedSameGame(t *testing.T) {
	for i := range 50 {
		cfg := Config{Players: 2 + i%5, Seed: uint64(1000 + i), First: -1}
		g1, ev1, log1 := playRandom(t, cfg, rand.New(rand.NewPCG(uint64(i), 9)))
		g2, ev2, log2 := playRandom(t, cfg, rand.New(rand.NewPCG(uint64(i), 9)))
		if !reflect.DeepEqual(ev1, ev2) || !reflect.DeepEqual(log1, log2) || !snapEqual(g1.Snapshot(), g2.Snapshot()) {
			t.Fatalf("seed %d diverged", cfg.Seed)
		}
		// Replaying the intent log against a fresh game reproduces it.
		g3, _ := New(cfg)
		for _, step := range log1 {
			if _, err := g3.Apply(step[0].(int), step[1].(Intent)); err != nil {
				t.Fatalf("replay refused: %v", err)
			}
		}
		if !snapEqual(g1.Snapshot(), g3.Snapshot()) {
			t.Fatalf("seed %d replay diverged", cfg.Seed)
		}
	}
}

func TestEveryEventHasTurnAndValidSeats(t *testing.T) {
	_, evs, _ := playRandom(t, Config{Players: 5, Seed: 42, First: -1}, rand.New(rand.NewPCG(4, 2)))
	for _, e := range evs {
		if e.Turn < 1 || e.Seat < -1 || e.Seat >= 5 || e.Other < -1 || e.Other >= 5 {
			t.Fatalf("bad event %v", e)
		}
	}
}

func FuzzApply(f *testing.F) {
	f.Add(uint64(1), uint8(3), []byte{1, 2, 3, 4})
	f.Fuzz(func(t *testing.T, seed uint64, players uint8, moves []byte) {
		n := 2 + int(players)%5
		g, err := New(Config{Players: n, Seed: seed, First: -1, Magistrate: players&8 != 0})
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; i+3 < len(moves) && !g.Over(); i += 4 {
			// Arbitrary, mostly illegal intents must never panic or corrupt state.
			in := Intent{
				Kind:   IntentKind(moves[i] % 9),
				Action: Action(moves[i+1] % 10),
				Target: int(moves[i+2]%8) - 1,
				Claim:  Character(moves[i+2] % 8),
				Card:   int(moves[i+3]%4) - 1,
				Keep:   []int{int(moves[i+3] % 5), int(moves[i+1] % 5)},
			}
			seat := int(moves[i+3]) % n
			_, _ = g.Apply(seat, in)
			if err := g.CheckInvariants(); err != nil {
				t.Fatal(err)
			}
			if len(moves)%2 == 0 {
				aw := g.Awaiting()
				if len(aw) > 0 {
					l := g.Legal(aw[0])
					if len(l) > 0 {
						if _, err := g.Apply(aw[0], l[int(moves[i])%len(l)]); err != nil {
							t.Fatal(err)
						}
					}
				}
			}
		}
	})
}
