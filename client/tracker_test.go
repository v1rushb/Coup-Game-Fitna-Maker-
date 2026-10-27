package client

import (
	"strings"
	"testing"

	"github.com/v1rushb/neoncoup/engine"
)

func view(magistrate bool, hand []engine.Card, revealed ...[]engine.Character) engine.PlayerView {
	v := engine.PlayerView{Seat: 0, Magistrate: magistrate, Hand: hand}
	for i, r := range revealed {
		v.Players = append(v.Players, engine.PublicPlayer{Seat: i, Revealed: r, Hidden: 2 - len(r), Alive: len(r) < 2})
	}
	return v
}

func TestTrackerCountsRevealedAndUnseen(t *testing.T) {
	D, A, C, T := engine.Duke, engine.Assassin, engine.Captain, engine.Contessa
	// You hold Duke + a face-up Captain; others show Duke, Assassin, Assassin.
	v := view(false, []engine.Card{{Char: D}, {Char: C, Revealed: true}},
		[]engine.Character{C}, []engine.Character{D, A}, []engine.Character{A})
	got := courtCount(v)
	want := map[engine.Character]tally{
		D:                 {Revealed: 1, Mine: 1, Unseen: 1},
		A:                 {Revealed: 2, Mine: 0, Unseen: 1},
		C:                 {Revealed: 1, Mine: 0, Unseen: 2}, // your face-up Captain counts once, as revealed
		engine.Ambassador: {Unseen: 3},
		T:                 {Unseen: 3},
	}
	if len(got) != 5 {
		t.Fatalf("base game tracks %d characters, want 5", len(got))
	}
	for c, w := range want {
		if got[c] != w {
			t.Errorf("%s: got %+v, want %+v", c, got[c], w)
		}
	}
	if _, ok := got[engine.Magistrate]; ok {
		t.Fatal("Magistrate tracked without the variant")
	}
}

func TestTrackerMagistrateAndNoCopiesLeft(t *testing.T) {
	M, D := engine.Magistrate, engine.Duke
	// Two Magistrates face up and the third in your hand: none unseen.
	v := view(true, []engine.Card{{Char: M}, {Char: D}},
		[]engine.Character{}, []engine.Character{M}, []engine.Character{M})
	got := courtCount(v)
	if len(got) != 6 {
		t.Fatalf("Magistrate game tracks %d characters, want 6", len(got))
	}
	if got[M] != (tally{Revealed: 2, Mine: 1, Unseen: 0}) {
		t.Fatalf("magistrate: %+v", got[M])
	}
	if got[D].Unseen != 2 {
		t.Fatalf("duke: %+v", got[D])
	}
	// All three copies in front of you count too.
	v = view(true, []engine.Card{{Char: D}, {Char: D}}, []engine.Character{D}, []engine.Character{}, []engine.Character{})
	if got := courtCount(v)[D]; got.Unseen != 0 {
		t.Fatalf("every Duke accounted for, but %d unseen", got.Unseen)
	}
}

func TestTrackerCountsExchangeDrawsAsSeen(t *testing.T) {
	A, B := engine.Assassin, engine.Ambassador
	v := view(false, []engine.Card{{Char: B}, {Char: A}}, []engine.Character{}, []engine.Character{})
	v.ExchangeOptions = []engine.Character{B, A, A, A} // your two cards plus two drawn Assassins
	if got := courtCount(v)[A]; got.Mine != 3 || got.Unseen != 0 {
		t.Fatalf("drawn cards are seen by you: %+v", got)
	}
}

func TestTrackerRendersAtEverySize(t *testing.T) {
	sc := newScripted(t, false)
	sc.do(0, engine.ActionIntent(engine.Tax, -1))
	// Fake two face-up Captains: with yours, none are unseen.
	sc.s.v.Players[2].Revealed = []engine.Character{engine.Captain, engine.Captain}
	for _, tc := range []struct {
		w, h int
		want []string
	}{
		{160, 45, []string{"REVEALED 2/3", "UNSEEN 0", "▓NO COPIES", "TREASURY", "DECK"}}, // the card row
		{120, 40, []string{"REVEALED 2/3", "▓ NO COPIES ▓"}},                              // medium tiles
		{100, 30, []string{"CAPT R2/3 U0▓", "DUKE R0/3 U2"}},                              // one-line tiles
	} {
		sc.a.w, sc.a.h = tc.w, tc.h
		out := shot(sc.a)
		for _, w := range tc.want {
			if !strings.Contains(out, w) {
				t.Errorf("%dx%d tracker lacks %q:\n%s", tc.w, tc.h, w, out)
			}
		}
	}
}
