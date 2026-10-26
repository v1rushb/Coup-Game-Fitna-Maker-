package engine

import (
	"testing"
)

// --- setup -----------------------------------------------------------------

func TestSetupDealsTwoCardsAndTwoCoins(t *testing.T) {
	for n := MinPlayers; n <= MaxPlayers; n++ {
		g, err := New(Config{Players: n, Seed: uint64(n), First: -1})
		if err != nil {
			t.Fatal(err)
		}
		check(t, g)
		if got, want := len(g.deck), 15-2*n; got != want {
			t.Errorf("%dp: deck %d, want %d", n, got, want)
		}
		for s := range n {
			wantHidden(t, g, s, 2)
			want := 2
			if n == 2 && s == g.Current() {
				want = 1
			}
			wantCoins(t, g, s, want)
		}
		wantPhase(t, g, PhaseTurnStart)
	}
}

func TestTwoPlayerStartingPlayerGetsOneCoin(t *testing.T) {
	for first := range 2 {
		g, _ := New(Config{Players: 2, Seed: 7, First: first})
		wantCoins(t, g, first, 1)
		wantCoins(t, g, 1-first, 2)
		if g.treasury != 47 {
			t.Fatalf("treasury %d, want 47", g.treasury)
		}
	}
}

func TestDeckIsFifteenCardsThreeOfEach(t *testing.T) {
	g, _ := New(Config{Players: 3, Seed: 1, First: 0})
	counts := map[Character]int{}
	s := g.Snapshot()
	for _, c := range s.Deck {
		counts[c]++
	}
	for _, h := range s.Hands {
		for _, c := range h {
			counts[c.Char]++
		}
	}
	if len(counts) != 5 {
		t.Fatalf("characters %v", counts)
	}
	for c, n := range counts {
		if n != 3 {
			t.Errorf("%s x%d", c, n)
		}
	}
}

func TestPlayerCountBounds(t *testing.T) {
	for _, n := range []int{0, 1, 7} {
		if _, err := New(Config{Players: n}); err == nil {
			t.Errorf("%d players accepted", n)
		}
	}
}

func TestMagistrateDeckHasEighteenCards(t *testing.T) {
	g, _ := New(Config{Players: 6, Seed: 3, Magistrate: true, First: 0})
	check(t, g)
	if len(g.deck) != 6 {
		t.Fatalf("deck %d, want 6", len(g.deck))
	}
	if DeckSize(true) != 18 || DeckSize(false) != 15 {
		t.Fatal("deck sizes")
	}
}

// --- general actions -------------------------------------------------------

func TestIncome(t *testing.T) {
	g := rigged(t, [][]Character{{D, D}, {A, A}}, rigOpts{})
	evs := do(t, g, 0, ActionIntent(Income, -1))
	wantCoins(t, g, 0, 3)
	wantPhase(t, g, PhaseTurnStart)
	if g.Current() != 1 || g.Turn() != 2 {
		t.Fatalf("turn did not pass: current %d turn %d", g.Current(), g.Turn())
	}
	if !hasEvent(evs, EvActionResolved) || !hasEvent(evs, EvTurnEnd) {
		t.Fatalf("events %v", evs)
	}
}

func TestIncomeCannotBeChallengedOrBlocked(t *testing.T) {
	g := rigged(t, [][]Character{{D, D}, {A, A}, {C, C}}, rigOpts{})
	do(t, g, 0, ActionIntent(Income, -1))
	// Straight to the next turn: no window opened.
	wantPhase(t, g, PhaseTurnStart)
	reject(t, g, 2, ChallengeIntent())
}

func TestForeignAidUnblocked(t *testing.T) {
	g := rigged(t, [][]Character{{A, A}, {C, C}, {B, B}}, rigOpts{})
	do(t, g, 0, ActionIntent(ForeignAid, -1))
	wantPhase(t, g, PhaseBlockWindow)
	// No challenge window: foreign aid makes no claim.
	reject(t, g, 1, ChallengeIntent())
	do(t, g, 1, PassIntent())
	wantPhase(t, g, PhaseBlockWindow) // still waiting on seat 2
	do(t, g, 2, PassIntent())
	wantCoins(t, g, 0, 4)
	wantPhase(t, g, PhaseTurnStart)
}

func TestForeignAidBlockedByAnyDuke(t *testing.T) {
	g := rigged(t, [][]Character{{A, A}, {C, C}, {D, B}}, rigOpts{})
	do(t, g, 0, ActionIntent(ForeignAid, -1))
	// Seat 2 is not targeted but may still block.
	reject(t, g, 2, BlockIntent(Contessa))
	do(t, g, 2, BlockIntent(Duke))
	wantPhase(t, g, PhaseBlockChallengeWindow)
	// Everyone but the blocker may challenge, the actor included.
	if aw := g.Awaiting(); len(aw) != 2 || aw[0] != 0 || aw[1] != 1 {
		t.Fatalf("awaiting %v", aw)
	}
	passAll(t, g)
	wantCoins(t, g, 0, 2)
	wantPhase(t, g, PhaseTurnStart)
}

func TestForeignAidBlockChallengedAndProven(t *testing.T) {
	g := rigged(t, [][]Character{{A, C}, {C, C}, {D, B}}, rigOpts{})
	do(t, g, 0, ActionIntent(ForeignAid, -1))
	do(t, g, 2, BlockIntent(Duke))
	evs := do(t, g, 0, ChallengeIntent())
	// Seat 2 holds a Duke and a different card, so they choose what to flip.
	wantPhase(t, g, PhaseProveClaim)
	if len(evs) != 1 || evs[0].Kind != EvChallenge {
		t.Fatalf("events %v", evs)
	}
	evs = do(t, g, 2, RevealIntent(slotOf(g, 2, Duke)))
	if !hasEvent(evs, EvClaimProven) || !hasEvent(evs, EvCardReplaced) {
		t.Fatalf("events %v", evs)
	}
	// Challenger (seat 0) has two different cards and must choose.
	wantPhase(t, g, PhaseLoseInfluence)
	evs = do(t, g, 0, LoseIntent(1))
	if !hasEvent(evs, EvActionBlocked) {
		t.Fatalf("block should stand: %v", evs)
	}
	wantCoins(t, g, 0, 2)
	wantHidden(t, g, 0, 1)
	wantHidden(t, g, 2, 2)
}

func TestForeignAidBlockChallengedAndFailed(t *testing.T) {
	g := rigged(t, [][]Character{{A, C}, {C, C}, {T, B}}, rigOpts{})
	do(t, g, 0, ActionIntent(ForeignAid, -1))
	do(t, g, 2, BlockIntent(Duke)) // bluff
	do(t, g, 1, ChallengeIntent())
	wantPhase(t, g, PhaseProveClaim)
	evs := do(t, g, 2, RevealIntent(0)) // flips Contessa: claim fails, card lost
	if !hasEvent(evs, EvClaimFailed) || !hasEvent(evs, EvBlockFailed) {
		t.Fatalf("events %v", evs)
	}
	wantHidden(t, g, 2, 1)
	wantCoins(t, g, 0, 4) // foreign aid goes through
	wantPhase(t, g, PhaseTurnStart)
}

func TestCoup(t *testing.T) {
	g := rigged(t, [][]Character{{A, C}, {D, B}, {T, T}}, rigOpts{coins: []int{7, 2, 2}})
	do(t, g, 0, ActionIntent(Coup, 1))
	// No challenge or block window: straight to the target's choice.
	wantPhase(t, g, PhaseLoseInfluence)
	wantCoins(t, g, 0, 0)
	if g.treasury != 50-0-2-2 {
		t.Fatalf("treasury %d", g.treasury)
	}
	reject(t, g, 2, ChallengeIntent())
	reject(t, g, 1, BlockIntent(Contessa))
	do(t, g, 1, LoseIntent(0))
	wantHidden(t, g, 1, 1)
	if !g.players[1].cards[0].Revealed {
		t.Fatal("chosen card should be face up")
	}
	wantPhase(t, g, PhaseTurnStart)
}

func TestCoupNeedsSevenCoins(t *testing.T) {
	g := rigged(t, [][]Character{{A, C}, {D, B}}, rigOpts{coins: []int{6, 2}})
	reject(t, g, 0, ActionIntent(Coup, 1))
}

func TestMustCoupAtTenCoins(t *testing.T) {
	for _, n := range []int{10, 12} {
		g := rigged(t, [][]Character{{D, C}, {D, B}}, rigOpts{coins: []int{n, 2}})
		for _, a := range []Action{Income, ForeignAid, Tax, Steal, Exchange, Assassinate} {
			reject(t, g, 0, ActionIntent(a, 1))
		}
		legal := g.Legal(0)
		if len(legal) != 1 || legal[0].Action != Coup {
			t.Fatalf("legal %v", legal)
		}
		do(t, g, 0, ActionIntent(Coup, 1))
	}
	// Nine coins is still free choice.
	g := rigged(t, [][]Character{{D, C}, {D, B}}, rigOpts{coins: []int{9, 2}})
	do(t, g, 0, ActionIntent(Income, -1))
}

func TestCannotTargetSelfOrEliminated(t *testing.T) {
	g := rigged(t, [][]Character{{A, C}, {D, B}, {T, T}}, rigOpts{coins: []int{7, 2, 2}})
	reject(t, g, 0, ActionIntent(Coup, 0))
	reject(t, g, 0, ActionIntent(Coup, 9))
	reject(t, g, 0, Intent{Kind: IntentAction, Action: Income, Target: 1}) // untargeted with a target
	g.players[2].cards[0].Revealed = true
	g.players[2].cards[1].Revealed = true
	g.players[2].alive = false
	g.treasury += g.players[2].coins
	g.players[2].coins = 0
	reject(t, g, 0, ActionIntent(Coup, 2))
}

func TestOutOfTurnIntentsRejected(t *testing.T) {
	g := rigged(t, [][]Character{{A, C}, {D, B}, {T, T}}, rigOpts{})
	reject(t, g, 1, ActionIntent(Income, -1))
	reject(t, g, 0, PassIntent())
	do(t, g, 0, ActionIntent(Tax, -1))
	reject(t, g, 0, ChallengeIntent()) // actor cannot challenge themself
	reject(t, g, 1, ActionIntent(Income, -1))
	reject(t, g, 1, BlockIntent(Duke)) // no blocking in the challenge window
}

// --- character actions -----------------------------------------------------

func TestTaxUnchallenged(t *testing.T) {
	g := rigged(t, [][]Character{{T, T}, {C, B}, {A, A}}, rigOpts{})
	do(t, g, 0, ActionIntent(Tax, -1)) // a bluff nobody calls
	wantPhase(t, g, PhaseChallengeWindow)
	do(t, g, 1, PassIntent())
	wantPhase(t, g, PhaseChallengeWindow)
	do(t, g, 2, PassIntent())
	wantCoins(t, g, 0, 5)
}

func TestTaxIsNotBlockableWithoutMagistrate(t *testing.T) {
	g := rigged(t, [][]Character{{D, T}, {C, B}}, rigOpts{})
	do(t, g, 0, ActionIntent(Tax, -1))
	do(t, g, 1, PassIntent())
	wantCoins(t, g, 0, 5)
	wantPhase(t, g, PhaseTurnStart)
}

func TestChallengeProvenReplacesCard(t *testing.T) {
	g := rigged(t, [][]Character{{D, T}, {C, B}, {A, A}}, rigOpts{})
	do(t, g, 0, ActionIntent(Tax, -1))
	deckBefore := len(g.deck)
	do(t, g, 2, ChallengeIntent())
	evs := do(t, g, 0, RevealIntent(0))
	proven, ok := findEvent(evs, EvClaimProven)
	if !ok || proven.Card != Duke || proven.Seat != 0 || proven.Other != 2 {
		t.Fatalf("events %v", evs)
	}
	repl, _ := findEvent(evs, EvCardReplaced)
	if len(repl.Secret) != 1 || repl.Secret[0] != g.players[0].cards[0].Char {
		t.Fatalf("replacement event %+v vs hand %v", repl, g.players[0].cards)
	}
	if len(g.deck) != deckBefore {
		t.Fatal("deck size changed after replacement")
	}
	wantHidden(t, g, 0, 2)
	// Seat 2 holds two Assassins but still chooses: skipping the choice
	// would reveal the other card.
	wantPhase(t, g, PhaseLoseInfluence)
	do(t, g, 2, LoseIntent(0))
	wantHidden(t, g, 2, 1)
	wantCoins(t, g, 0, 5)
	wantPhase(t, g, PhaseTurnStart)
}

func TestChallengeFailedLosesCardAndAction(t *testing.T) {
	g := rigged(t, [][]Character{{C, T}, {C, B}}, rigOpts{})
	do(t, g, 0, ActionIntent(Tax, -1))
	do(t, g, 1, ChallengeIntent())
	evs := do(t, g, 0, RevealIntent(1))
	if !hasEvent(evs, EvClaimFailed) || !hasEvent(evs, EvActionFailed) {
		t.Fatalf("events %v", evs)
	}
	if !g.players[0].cards[1].Revealed || g.players[0].cards[1].Char != Contessa {
		t.Fatal("the flipped card is the lost influence")
	}
	wantCoins(t, g, 0, 2)
	wantHidden(t, g, 1, 2)
	wantPhase(t, g, PhaseTurnStart)
	if g.Current() != 1 {
		t.Fatal("turn should pass")
	}
}

func TestClaimantMayDeclineToProve(t *testing.T) {
	// The rulebook lets a challenged player lose on purpose: flipping a card
	// other than the claimed one forfeits the challenge.
	g := rigged(t, [][]Character{{D, T}, {C, B}}, rigOpts{})
	do(t, g, 0, ActionIntent(Tax, -1))
	do(t, g, 1, ChallengeIntent())
	evs := do(t, g, 0, RevealIntent(1)) // flip Contessa despite holding Duke
	if !hasEvent(evs, EvClaimFailed) {
		t.Fatalf("events %v", evs)
	}
	wantCoins(t, g, 0, 2)
	wantHidden(t, g, 0, 1)
	wantHidden(t, g, 1, 2)
}

func TestProveIsAutomaticWithOneCard(t *testing.T) {
	g := rigged(t, [][]Character{{D, T}, {C, B}}, rigOpts{})
	g.players[0].cards[1].Revealed = true
	do(t, g, 0, ActionIntent(Tax, -1))
	evs := do(t, g, 1, ChallengeIntent())
	// No ProveClaim pause: seat 0 had one card and it was a Duke.
	if !hasEvent(evs, EvClaimProven) {
		t.Fatalf("events %v", evs)
	}
	wantPhase(t, g, PhaseLoseInfluence) // the challenger has two different cards
}

func TestOnlyFirstChallengeResolves(t *testing.T) {
	g := rigged(t, [][]Character{{D, T}, {C, B}, {A, A}, {C, B}}, rigOpts{})
	do(t, g, 0, ActionIntent(Tax, -1))
	do(t, g, 3, ChallengeIntent())
	// The window is closed: other challenges and passes are refused.
	reject(t, g, 1, ChallengeIntent())
	reject(t, g, 2, PassIntent())
	do(t, g, 0, RevealIntent(0))
	wantPhase(t, g, PhaseLoseInfluence)
	if g.loser != 3 {
		t.Fatalf("loser %d, want first challenger 3", g.loser)
	}
	do(t, g, 3, LoseIntent(0))
	wantHidden(t, g, 1, 2)
	wantHidden(t, g, 2, 2)
}

func TestAnyPlayerMayChallengeEvenIfNotTargeted(t *testing.T) {
	g := rigged(t, [][]Character{{C, T}, {D, B}, {A, A}}, rigOpts{})
	do(t, g, 0, ActionIntent(Steal, 1))
	do(t, g, 2, ChallengeIntent()) // seat 2 is not the target
	evs := do(t, g, 0, RevealIntent(0))
	if !hasEvent(evs, EvClaimProven) {
		t.Fatal("captain should be proven")
	}
	do(t, g, 2, LoseIntent(1))
	wantHidden(t, g, 2, 1)
	// The steal continues: the target gets their block window.
	wantPhase(t, g, PhaseBlockWindow)
}

// --- assassination ---------------------------------------------------------

func TestAssassinateResolves(t *testing.T) {
	g := rigged(t, [][]Character{{A, D}, {C, B}}, rigOpts{coins: []int{3, 2}})
	do(t, g, 0, ActionIntent(Assassinate, 1))
	wantCoins(t, g, 0, 0) // paid up front
	do(t, g, 1, PassIntent())
	wantPhase(t, g, PhaseBlockWindow)
	do(t, g, 1, PassIntent())
	wantPhase(t, g, PhaseLoseInfluence)
	do(t, g, 1, LoseIntent(1))
	wantHidden(t, g, 1, 1)
	wantCoins(t, g, 0, 0)
}

func TestAssassinateNeedsThreeCoins(t *testing.T) {
	g := rigged(t, [][]Character{{A, D}, {C, B}}, rigOpts{coins: []int{2, 2}})
	reject(t, g, 0, ActionIntent(Assassinate, 1))
}

func TestAssassinCoinsSpentWhenContessaBlocks(t *testing.T) {
	g := rigged(t, [][]Character{{A, D}, {T, B}}, rigOpts{coins: []int{5, 2}})
	do(t, g, 0, ActionIntent(Assassinate, 1))
	do(t, g, 1, PassIntent())
	do(t, g, 1, BlockIntent(Contessa))
	do(t, g, 0, PassIntent())
	wantCoins(t, g, 0, 2) // 3 coins stay spent
	wantHidden(t, g, 1, 2)
	wantPhase(t, g, PhaseTurnStart)
}

func TestAssassinCoinsReturnedWhenClaimChallenged(t *testing.T) {
	g := rigged(t, [][]Character{{C, D}, {T, B}}, rigOpts{coins: []int{4, 2}})
	do(t, g, 0, ActionIntent(Assassinate, 1))
	wantCoins(t, g, 0, 1)
	do(t, g, 1, ChallengeIntent())
	evs := do(t, g, 0, RevealIntent(0))
	refund := false
	for _, e := range evs {
		if e.Kind == EvCoins && e.Reason == ReasonRefund && e.Amount == 3 {
			refund = true
		}
	}
	if !refund {
		t.Fatalf("no refund event: %v", evs)
	}
	wantCoins(t, g, 0, 4)
	wantHidden(t, g, 0, 1)
	wantHidden(t, g, 1, 2)
}

func TestAssassinRefundGoesToTreasuryWhenActorEliminated(t *testing.T) {
	g := rigged(t, [][]Character{{C, D}, {T, B}, {C, B}}, rigOpts{coins: []int{4, 2, 2}})
	g.players[0].cards[1].Revealed = true
	do(t, g, 0, ActionIntent(Assassinate, 1))
	do(t, g, 2, ChallengeIntent()) // single Captain flips automatically
	if g.Alive(0) {
		t.Fatal("seat 0 should be eliminated")
	}
	wantCoins(t, g, 0, 0)
}

func TestTargetWronglyChallengesAssassinLosesTwo(t *testing.T) {
	g := rigged(t, [][]Character{{A, D}, {C, B}}, rigOpts{coins: []int{3, 2}})
	do(t, g, 0, ActionIntent(Assassinate, 1))
	do(t, g, 1, ChallengeIntent())
	do(t, g, 0, RevealIntent(0)) // proven
	// Target loses one for the challenge...
	wantPhase(t, g, PhaseLoseInfluence)
	do(t, g, 1, LoseIntent(0))
	wantHidden(t, g, 1, 1)
	// ...may still try to block, declines, and the assassination lands.
	wantPhase(t, g, PhaseBlockWindow)
	evs := do(t, g, 1, PassIntent())
	if !hasEvent(evs, EvEliminated) || !hasEvent(evs, EvGameOver) {
		t.Fatalf("events %v", evs)
	}
	if g.Winner() != 0 {
		t.Fatal("seat 0 should win")
	}
}

func TestTargetCanStillBlockAfterLosingChallenge(t *testing.T) {
	g := rigged(t, [][]Character{{A, D}, {C, T}}, rigOpts{coins: []int{3, 2}})
	do(t, g, 0, ActionIntent(Assassinate, 1))
	do(t, g, 1, ChallengeIntent())
	do(t, g, 0, RevealIntent(0))
	do(t, g, 1, LoseIntent(0)) // gives up the Captain, keeps Contessa
	wantPhase(t, g, PhaseBlockWindow)
	do(t, g, 1, BlockIntent(Contessa))
	do(t, g, 0, PassIntent())
	wantHidden(t, g, 1, 1)
	if !g.Alive(1) {
		t.Fatal("block should save the target")
	}
}

func TestContessaBluffChallengedLosesTwo(t *testing.T) {
	g := rigged(t, [][]Character{{A, D}, {C, B}}, rigOpts{coins: []int{3, 2}})
	do(t, g, 0, ActionIntent(Assassinate, 1))
	do(t, g, 1, PassIntent())
	do(t, g, 1, BlockIntent(Contessa)) // bluff
	do(t, g, 0, ChallengeIntent())
	evs := do(t, g, 1, RevealIntent(0)) // flips Captain: block fails
	if !hasEvent(evs, EvBlockFailed) {
		t.Fatalf("events %v", evs)
	}
	// The last card goes to the assassination itself.
	if !hasEvent(evs, EvEliminated) || g.Winner() != 0 {
		t.Fatalf("target should lose both influence: %v", evs)
	}
}

func TestContessaProvenBlocksAndChallengerLoses(t *testing.T) {
	g := rigged(t, [][]Character{{A, D}, {T, B}}, rigOpts{coins: []int{3, 2}})
	do(t, g, 0, ActionIntent(Assassinate, 1))
	do(t, g, 1, PassIntent())
	do(t, g, 1, BlockIntent(Contessa))
	do(t, g, 0, ChallengeIntent())
	do(t, g, 1, RevealIntent(0))
	wantPhase(t, g, PhaseLoseInfluence)
	evs := do(t, g, 0, LoseIntent(1))
	if !hasEvent(evs, EvActionBlocked) {
		t.Fatalf("events %v", evs)
	}
	wantHidden(t, g, 0, 1)
	wantHidden(t, g, 1, 2)
	wantCoins(t, g, 0, 0) // fee stays spent
}

func TestAssassinationTargetEliminatedByChallengeFizzles(t *testing.T) {
	g := rigged(t, [][]Character{{A, D}, {C, B}, {T, T}}, rigOpts{coins: []int{3, 2, 2}})
	g.players[1].cards[1].Revealed = true
	do(t, g, 0, ActionIntent(Assassinate, 1))
	do(t, g, 1, ChallengeIntent())
	// Seat 0 holds Assassin and Duke, so they choose which card to flip.
	wantPhase(t, g, PhaseProveClaim)
	evs := do(t, g, 0, RevealIntent(0))
	if !hasEvent(evs, EvEliminated) {
		t.Fatalf("target should be out: %v", evs)
	}
	fail, ok := findEvent(evs, EvActionFailed)
	if !ok || fail.Reason != ReasonNoTarget {
		t.Fatalf("assassination should fizzle: %v", evs)
	}
	wantCoins(t, g, 0, 0)
	wantPhase(t, g, PhaseTurnStart)
	if g.Current() != 2 {
		t.Fatalf("next living player should be up, got %d", g.Current())
	}
}

// --- steal -----------------------------------------------------------------

func TestStealAmounts(t *testing.T) {
	for _, tc := range []struct{ victim, want int }{{5, 2}, {2, 2}, {1, 1}, {0, 0}} {
		g := rigged(t, [][]Character{{C, D}, {A, A}}, rigOpts{coins: []int{2, tc.victim}})
		do(t, g, 0, ActionIntent(Steal, 1))
		do(t, g, 1, PassIntent())
		do(t, g, 1, PassIntent())
		wantCoins(t, g, 0, 2+tc.want)
		wantCoins(t, g, 1, tc.victim-tc.want)
	}
}

func TestStealBlockedByCaptainOrAmbassador(t *testing.T) {
	for _, blocker := range []Character{Captain, Ambassador} {
		g := rigged(t, [][]Character{{C, D}, {blocker, T}, {A, A}}, rigOpts{})
		do(t, g, 0, ActionIntent(Steal, 1))
		passAll(t, g)
		wantPhase(t, g, PhaseBlockWindow)
		// Only the target may block a steal.
		if aw := g.Awaiting(); len(aw) != 1 || aw[0] != 1 {
			t.Fatalf("awaiting %v", aw)
		}
		reject(t, g, 2, BlockIntent(Captain))
		reject(t, g, 1, BlockIntent(Duke))
		reject(t, g, 1, BlockIntent(Contessa))
		do(t, g, 1, BlockIntent(blocker))
		passAll(t, g)
		wantCoins(t, g, 0, 2)
		wantCoins(t, g, 1, 2)
	}
}

// --- exchange --------------------------------------------------------------

func TestExchangeKeepTwoOfFour(t *testing.T) {
	g := rigged(t, [][]Character{{B, D}, {C, C}}, rigOpts{deckTop: []Character{T, A}})
	do(t, g, 0, ActionIntent(Exchange, -1))
	evs := do(t, g, 1, PassIntent())
	wantPhase(t, g, PhaseExchange)
	draw, ok := findEvent(evs, EvExchangeDraw)
	if !ok || len(draw.Secret) != 2 || draw.Secret[0] != T || draw.Secret[1] != A {
		t.Fatalf("draw %+v", draw)
	}
	v := g.View(0)
	want := []Character{B, D, T, A}
	if len(v.ExchangeOptions) != 4 {
		t.Fatalf("options %v", v.ExchangeOptions)
	}
	for i := range want {
		if v.ExchangeOptions[i] != want[i] {
			t.Fatalf("options %v, want %v", v.ExchangeOptions, want)
		}
	}
	if len(g.View(1).ExchangeOptions) != 0 {
		t.Fatal("exchange options leaked to another seat")
	}
	if len(v.Legal) != 6 {
		t.Fatalf("C(4,2) = 6 choices, got %d", len(v.Legal))
	}
	reject(t, g, 0, ExchangeIntent(0))
	reject(t, g, 0, ExchangeIntent(1, 1))
	reject(t, g, 0, ExchangeIntent(0, 4))
	do(t, g, 0, ExchangeIntent(2, 3)) // keep Contessa + Assassin
	h := g.players[0].cards
	if h[0].Char != T || h[1].Char != A {
		t.Fatalf("hand %v", h)
	}
	if len(g.deck) != 11 {
		t.Fatalf("deck %d", len(g.deck))
	}
	wantPhase(t, g, PhaseTurnStart)
}

func TestExchangeWithOneInfluenceKeepsOne(t *testing.T) {
	g := rigged(t, [][]Character{{B, D}, {C, C}}, rigOpts{deckTop: []Character{T, A}})
	g.players[0].cards[1].Revealed = true
	do(t, g, 0, ActionIntent(Exchange, -1))
	do(t, g, 1, PassIntent())
	v := g.View(0)
	if len(v.ExchangeOptions) != 3 || len(v.Legal) != 3 {
		t.Fatalf("options %v legal %v", v.ExchangeOptions, v.Legal)
	}
	reject(t, g, 0, ExchangeIntent(0, 1))
	do(t, g, 0, ExchangeIntent(2))
	if g.players[0].cards[0].Char != A || !g.players[0].cards[1].Revealed || g.players[0].cards[1].Char != D {
		t.Fatalf("hand %v", g.players[0].cards)
	}
}

func TestExchangeProvenThenExchanges(t *testing.T) {
	g := rigged(t, [][]Character{{B, D}, {C, T}}, rigOpts{})
	do(t, g, 0, ActionIntent(Exchange, -1))
	do(t, g, 1, ChallengeIntent())
	do(t, g, 0, RevealIntent(0))
	do(t, g, 1, LoseIntent(0))
	wantPhase(t, g, PhaseExchange)
	if len(g.View(0).ExchangeOptions) != 4 {
		t.Fatal("exchange should continue with the replaced hand")
	}
	do(t, g, 0, ExchangeIntent(0, 1))
}

// --- influence loss and elimination ----------------------------------------

func TestLoseInfluenceChoiceAndFaceUp(t *testing.T) {
	g := rigged(t, [][]Character{{A, D}, {C, B}, {T, T}}, rigOpts{coins: []int{7, 2, 2}})
	do(t, g, 0, ActionIntent(Coup, 1))
	reject(t, g, 0, LoseIntent(0)) // wrong seat
	reject(t, g, 1, LoseIntent(5))
	evs := do(t, g, 1, LoseIntent(1))
	lost, _ := findEvent(evs, EvInfluenceLost)
	if lost.Card != Ambassador || lost.Slot != 1 {
		t.Fatalf("lost %+v", lost)
	}
	v := g.View(2)
	if len(v.Players[1].Revealed) != 1 || v.Players[1].Revealed[0] != Ambassador || v.Players[1].Hidden != 1 {
		t.Fatalf("public view %+v", v.Players[1])
	}
	// Revealed cards stay face up for the rest of the game.
	do(t, g, 1, ActionIntent(Income, -1))
	do(t, g, 2, ActionIntent(Income, -1))
	if !g.players[1].cards[1].Revealed {
		t.Fatal("card flipped back")
	}
}

func TestLoseInfluenceAutomaticWithOneCard(t *testing.T) {
	g := rigged(t, [][]Character{{A, D}, {C, B}, {T, T}}, rigOpts{coins: []int{7, 2, 2}})
	g.players[1].cards[0].Revealed = true
	evs := do(t, g, 0, ActionIntent(Coup, 1))
	if !hasEvent(evs, EvEliminated) {
		t.Fatalf("events %v", evs)
	}
	wantPhase(t, g, PhaseTurnStart)
}

func TestEliminatedCoinsReturnToTreasuryAndSeatIsSkipped(t *testing.T) {
	g := rigged(t, [][]Character{{A, D}, {C, B}, {T, T}}, rigOpts{coins: []int{7, 5, 2}})
	g.players[1].cards[0].Revealed = true
	treasury := g.treasury
	evs := do(t, g, 0, ActionIntent(Coup, 1))
	elim, _ := findEvent(evs, EvEliminated)
	if elim.Amount != 5 {
		t.Fatalf("returned %d", elim.Amount)
	}
	if g.treasury != treasury+7+5 {
		t.Fatalf("treasury %d", g.treasury)
	}
	if g.Current() != 2 {
		t.Fatalf("current %d, want 2", g.Current())
	}
	do(t, g, 2, ActionIntent(Income, -1))
	if g.Current() != 0 {
		t.Fatalf("eliminated seat not skipped: current %d", g.Current())
	}
	reject(t, g, 1, PassIntent())
}

func TestLastPlayerStandingWins(t *testing.T) {
	g := rigged(t, [][]Character{{A, D}, {C, B}}, rigOpts{coins: []int{7, 2}})
	g.players[1].cards[0].Revealed = true
	evs := do(t, g, 0, ActionIntent(Coup, 1))
	over, ok := findEvent(evs, EvGameOver)
	if !ok || over.Seat != 0 || g.Winner() != 0 || !g.Over() {
		t.Fatalf("events %v", evs)
	}
	reject(t, g, 0, ActionIntent(Income, -1))
	if len(g.Legal(0)) != 0 {
		t.Fatal("no legal intents after game over")
	}
}

func TestChallengerEliminatedEndsGameMidTurn(t *testing.T) {
	g := rigged(t, [][]Character{{D, D}, {C, B}}, rigOpts{})
	g.players[1].cards[0].Revealed = true
	do(t, g, 0, ActionIntent(Tax, -1))
	do(t, g, 1, ChallengeIntent())
	evs := do(t, g, 0, RevealIntent(1)) // two Dukes: either proves it
	if !g.Over() || g.Winner() != 0 || !hasEvent(evs, EvGameOver) {
		t.Fatalf("events %v", evs)
	}
	if hasEvent(evs, EvTurnStart) {
		t.Fatal("no new turn after game over")
	}
}

// --- windows ---------------------------------------------------------------

func TestWindowClosesOnlyWhenEveryonePassed(t *testing.T) {
	g := rigged(t, [][]Character{{D, T}, {C, B}, {A, A}, {C, B}}, rigOpts{})
	do(t, g, 0, ActionIntent(Tax, -1))
	if len(g.Awaiting()) != 3 {
		t.Fatal("all three others should be asked")
	}
	do(t, g, 1, PassIntent())
	reject(t, g, 1, PassIntent()) // already passed
	do(t, g, 3, PassIntent())
	wantPhase(t, g, PhaseChallengeWindow)
	do(t, g, 2, PassIntent())
	wantCoins(t, g, 0, 5)
}

func TestEliminatedPlayersAreNotAsked(t *testing.T) {
	g := rigged(t, [][]Character{{D, T}, {C, B}, {A, A}}, rigOpts{})
	g.players[2].cards[0].Revealed = true
	g.players[2].cards[1].Revealed = true
	g.players[2].alive = false
	g.treasury += g.players[2].coins
	g.players[2].coins = 0
	do(t, g, 0, ActionIntent(Tax, -1))
	if aw := g.Awaiting(); len(aw) != 1 || aw[0] != 1 {
		t.Fatalf("awaiting %v", aw)
	}
}

func TestDefaultIntentIsAlwaysLegal(t *testing.T) {
	g := rigged(t, [][]Character{{D, T}, {C, B}, {A, A}}, rigOpts{coins: []int{10, 2, 2}})
	for !g.Over() {
		seat := g.Awaiting()[0]
		in, ok := g.DefaultIntent(seat)
		if !ok {
			t.Fatal("no default")
		}
		do(t, g, seat, in)
		if g.Turn() > 200 {
			t.Fatal("default play does not terminate")
		}
	}
}

// --- magistrate variant ----------------------------------------------------

func TestAuditRequiresVariantAndThreeCoins(t *testing.T) {
	g := rigged(t, [][]Character{{D, T}, {C, B}}, rigOpts{coins: []int{2, 3}})
	reject(t, g, 0, ActionIntent(Audit, 1))

	g = rigged(t, [][]Character{{M, T}, {C, B}}, rigOpts{coins: []int{2, 2}, magistrate: true})
	reject(t, g, 0, ActionIntent(Audit, 1))
}

func TestAuditTakesTwoToTreasury(t *testing.T) {
	g := rigged(t, [][]Character{{M, T}, {C, B}}, rigOpts{coins: []int{2, 5}, magistrate: true})
	treasury := g.treasury
	do(t, g, 0, ActionIntent(Audit, 1))
	wantPhase(t, g, PhaseChallengeWindow)
	do(t, g, 1, PassIntent())
	wantCoins(t, g, 1, 3)
	wantCoins(t, g, 0, 2)
	if g.treasury != treasury+2 {
		t.Fatal("audit coins should go to the treasury")
	}
}

func TestAuditCanBeChallenged(t *testing.T) {
	g := rigged(t, [][]Character{{D, T}, {C, B}}, rigOpts{coins: []int{2, 5}, magistrate: true})
	do(t, g, 0, ActionIntent(Audit, 1))
	do(t, g, 1, ChallengeIntent())
	do(t, g, 0, RevealIntent(0))
	wantCoins(t, g, 1, 5)
	wantHidden(t, g, 0, 1)
}

func TestMagistrateBlocksTax(t *testing.T) {
	g := rigged(t, [][]Character{{D, T}, {C, B}, {M, A}}, rigOpts{magistrate: true})
	do(t, g, 0, ActionIntent(Tax, -1))
	passAll(t, g)
	wantPhase(t, g, PhaseBlockWindow)
	if len(g.Awaiting()) != 2 {
		t.Fatal("anyone may block tax with a Magistrate")
	}
	reject(t, g, 2, BlockIntent(Duke))
	do(t, g, 2, BlockIntent(Magistrate))
	passAll(t, g)
	wantCoins(t, g, 0, 2)
}

// --- treasury --------------------------------------------------------------

func TestGainsAreCappedByTreasury(t *testing.T) {
	g := rigged(t, [][]Character{{D, T}, {C, B}}, rigOpts{coins: []int{9, 40}})
	do(t, g, 0, ActionIntent(Tax, -1))
	do(t, g, 1, PassIntent())
	wantCoins(t, g, 0, 10)
	if g.treasury != 0 {
		t.Fatalf("treasury %d", g.treasury)
	}
}

func TestIdenticalCardsStillAskSoNothingLeaks(t *testing.T) {
	// Deciding automatically for two identical cards would show the table
	// what the other card is, so the player is always asked.
	g := rigged(t, [][]Character{{A, C}, {D, D}, {T, B}}, rigOpts{coins: []int{7, 2, 2}})
	do(t, g, 0, ActionIntent(Coup, 1))
	wantPhase(t, g, PhaseLoseInfluence)
	if aw := g.Awaiting(); len(aw) != 1 || aw[0] != 1 {
		t.Fatalf("awaiting %v", aw)
	}

	g = rigged(t, [][]Character{{D, D}, {C, B}}, rigOpts{})
	do(t, g, 0, ActionIntent(Tax, -1))
	do(t, g, 1, ChallengeIntent())
	wantPhase(t, g, PhaseProveClaim)

	// One card left is public knowledge, so that case stays automatic.
	g = rigged(t, [][]Character{{D, T}, {C, B}}, rigOpts{})
	g.players[0].cards[1].Revealed = true
	do(t, g, 0, ActionIntent(Tax, -1))
	do(t, g, 1, ChallengeIntent())
	wantPhase(t, g, PhaseLoseInfluence) // proven at once; the challenger chooses
}

func TestRiggedTwoPlayerGameKeepsTheOneCoinRule(t *testing.T) {
	g, err := New(Config{Players: 2, First: 1, Rig: &Rig{
		Hands: [][]Character{{D, T}, {C, B}},
		Deck:  []Character{D, D, A, A, A, C, C, B, B, T, T},
	}})
	if err != nil {
		t.Fatal(err)
	}
	wantCoins(t, g, 1, 1)
	wantCoins(t, g, 0, 2)
	check(t, g)
}

func TestExchangeKeepMustBeIncreasing(t *testing.T) {
	g := rigged(t, [][]Character{{B, D}, {C, C}}, rigOpts{deckTop: []Character{T, A}})
	do(t, g, 0, ActionIntent(Exchange, -1))
	do(t, g, 1, PassIntent())
	reject(t, g, 0, ExchangeIntent(3, 2))
	// Every intent validate accepts is listed by Legal.
	for a := 0; a < 4; a++ {
		for b := 0; b < 4; b++ {
			in := ExchangeIntent(a, b)
			if (g.validate(0, in) == nil) != containsIntent(g.Legal(0), in) {
				t.Fatalf("validate and Legal disagree on %s", in)
			}
		}
	}
	do(t, g, 0, ExchangeIntent(2, 3))
}

func containsIntent(l []Intent, in Intent) bool {
	for _, x := range l {
		if x.Equal(in) {
			return true
		}
	}
	return false
}
