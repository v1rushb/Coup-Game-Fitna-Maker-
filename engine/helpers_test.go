package engine

import (
	"errors"
	"testing"
)

// D, A, C, B, T are short names for writing rigged hands.
const (
	D = Duke
	A = Assassin
	C = Captain
	B = Ambassador
	T = Contessa
	M = Magistrate
)

type rigOpts struct {
	coins      []int
	first      int
	magistrate bool
	deckTop    []Character // drawn first, in order
	seed       uint64
}

// rigged builds a game with exact hands. The remaining court cards form the
// deck, with opts.deckTop on top.
func rigged(t *testing.T, hands [][]Character, o rigOpts) *Game {
	t.Helper()
	left := map[Character]int{}
	for _, c := range DeckComposition(o.magistrate) {
		left[c] = CopiesPerCharacter
	}
	for _, h := range hands {
		for _, c := range h {
			left[c]--
		}
	}
	var deck []Character
	for _, c := range o.deckTop {
		left[c]--
		deck = append(deck, c)
	}
	for _, c := range DeckComposition(o.magistrate) {
		for range left[c] {
			deck = append(deck, c)
		}
	}
	for c, n := range left {
		if n < 0 {
			t.Fatalf("rig uses too many %s", c)
		}
	}
	coins := o.coins
	if coins == nil {
		// Rule tests start everyone on 2 coins unless they say otherwise;
		// the 2-player 1-coin rule has its own test.
		coins = make([]int, len(hands))
		for i := range coins {
			coins[i] = StartingCoins
		}
	}
	g, err := New(Config{Players: len(hands), Magistrate: o.magistrate, Seed: o.seed, First: o.first,
		Rig: &Rig{Hands: hands, Deck: deck, Coins: coins}})
	if err != nil {
		t.Fatal(err)
	}
	check(t, g)
	return g
}

func check(t *testing.T, g *Game) {
	t.Helper()
	if err := g.CheckInvariants(); err != nil {
		t.Fatalf("invariant broken: %v\n%+v", err, g.Snapshot())
	}
}

// do applies an intent that must be legal and returns its events.
func do(t *testing.T, g *Game, seat int, in Intent) []Event {
	t.Helper()
	evs, err := g.Apply(seat, in)
	if err != nil {
		t.Fatalf("seat %d %s rejected in %s: %v", seat, in, g.Phase(), err)
	}
	check(t, g)
	return evs
}

// reject asserts that an intent is refused and changes nothing.
func reject(t *testing.T, g *Game, seat int, in Intent) {
	t.Helper()
	before := g.Snapshot()
	ver := g.Version()
	if _, err := g.Apply(seat, in); err == nil {
		t.Fatalf("seat %d %s accepted in %s, want rejection", seat, in, g.Phase())
	} else if !errors.Is(err, ErrIllegal) {
		t.Fatalf("error %v does not wrap ErrIllegal", err)
	}
	if g.Version() != ver {
		t.Fatal("rejected intent changed the version")
	}
	if !snapEqual(before, g.Snapshot()) {
		t.Fatal("rejected intent changed the state")
	}
}

func snapEqual(a, b Snapshot) bool {
	if a.Phase != b.Phase || a.Turn != b.Turn || a.Current != b.Current || a.Treasury != b.Treasury ||
		len(a.Deck) != len(b.Deck) || len(a.Hands) != len(b.Hands) || a.Winner != b.Winner {
		return false
	}
	for i := range a.Deck {
		if a.Deck[i] != b.Deck[i] {
			return false
		}
	}
	for i := range a.Hands {
		if a.Coins[i] != b.Coins[i] || a.Alive[i] != b.Alive[i] {
			return false
		}
		for j := range a.Hands[i] {
			if a.Hands[i][j] != b.Hands[i][j] {
				return false
			}
		}
	}
	return true
}

// passAll makes every awaited seat pass.
func passAll(t *testing.T, g *Game) {
	t.Helper()
	for _, s := range g.Awaiting() {
		do(t, g, s, PassIntent())
	}
}

func coins(g *Game, seat int) int { return g.players[seat].coins }

func hidden(g *Game, seat int) int { return g.players[seat].hidden() }

func wantPhase(t *testing.T, g *Game, p Phase) {
	t.Helper()
	if g.Phase() != p {
		t.Fatalf("phase = %s, want %s", g.Phase(), p)
	}
}

func wantCoins(t *testing.T, g *Game, seat, n int) {
	t.Helper()
	if got := coins(g, seat); got != n {
		t.Fatalf("seat %d has %d coins, want %d", seat, got, n)
	}
}

func wantHidden(t *testing.T, g *Game, seat, n int) {
	t.Helper()
	if got := hidden(g, seat); got != n {
		t.Fatalf("seat %d has %d face-down cards, want %d", seat, got, n)
	}
}

func hasEvent(evs []Event, k EventKind) bool {
	for _, e := range evs {
		if e.Kind == k {
			return true
		}
	}
	return false
}

func findEvent(evs []Event, k EventKind) (Event, bool) {
	for _, e := range evs {
		if e.Kind == k {
			return e, true
		}
	}
	return Event{}, false
}

func slotOf(g *Game, seat int, c Character) int {
	for i, card := range g.players[seat].cards {
		if card.Char == c && !card.Revealed {
			return i
		}
	}
	return -1
}
