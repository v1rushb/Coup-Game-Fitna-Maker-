package engine

import "slices"

// validate checks an intent against the current phase without changing state.
func (g *Game) validate(seat int, in Intent) error {
	if seat < 0 || seat >= len(g.players) {
		return illegal("no such seat %d", seat)
	}
	if g.phase == PhaseGameOver {
		return illegal("the game is over")
	}
	if !g.awaiting[seat] {
		return illegal("not waiting on seat %d", seat)
	}
	p := &g.players[seat]
	switch g.phase {
	case PhaseTurnStart:
		if in.Kind != IntentAction {
			return illegal("choose an action")
		}
		return g.validateAction(seat, p, in)
	case PhaseChallengeWindow, PhaseBlockChallengeWindow:
		if in.Kind != IntentChallenge && in.Kind != IntentPass {
			return illegal("challenge or pass")
		}
	case PhaseBlockWindow:
		switch in.Kind {
		case IntentPass:
		case IntentBlock:
			if !slices.Contains(g.act.action.Blockers(g.cfg.Magistrate), in.Claim) {
				return illegal("%s cannot block %s", in.Claim, g.act.action)
			}
		default:
			return illegal("block or pass")
		}
	case PhaseProveClaim:
		if in.Kind != IntentReveal {
			return illegal("reveal a card")
		}
		return validSlot(p, in.Card)
	case PhaseLoseInfluence:
		if in.Kind != IntentLose {
			return illegal("choose a card to lose")
		}
		return validSlot(p, in.Card)
	case PhaseExchange:
		if in.Kind != IntentExchange {
			return illegal("choose cards to keep")
		}
		opts := len(g.exchangeOptions())
		if len(in.Keep) != p.hidden() {
			return illegal("keep exactly %d card(s)", p.hidden())
		}
		// Keep is a set, written in increasing order, so that Legal can list
		// every acceptable intent exactly once.
		for i, k := range in.Keep {
			if k < 0 || k >= opts {
				return illegal("no exchange option %d", k)
			}
			if i > 0 && k <= in.Keep[i-1] {
				return illegal("list kept options once each, in increasing order")
			}
		}
	default:
		return illegal("nothing to do in %s", g.phase)
	}
	return nil
}

func validSlot(p *player, slot int) error {
	if slot < 0 || slot >= len(p.cards) {
		return illegal("no card slot %d", slot)
	}
	if p.cards[slot].Revealed {
		return illegal("card %d is already face up", slot)
	}
	return nil
}

func (g *Game) validateAction(seat int, p *player, in Intent) error {
	a := in.Action
	if a < Income || a > Audit {
		return illegal("unknown action")
	}
	if a == Audit && !g.cfg.Magistrate {
		return illegal("audit needs the Magistrate variant")
	}
	if p.coins >= MustCoupAt && a != Coup {
		return illegal("with %d or more coins you must Coup", MustCoupAt)
	}
	if p.coins < a.Cost() {
		return illegal("%s costs %d coins", a, a.Cost())
	}
	if a.Targeted() {
		if in.Target < 0 || in.Target >= len(g.players) || in.Target == seat {
			return illegal("choose another player as the target")
		}
		if !g.players[in.Target].alive {
			return illegal("seat %d is out of the game", in.Target)
		}
		if a == Audit && g.players[in.Target].coins < AuditThreshold {
			return illegal("audit targets a player with %d or more coins", AuditThreshold)
		}
	} else if in.Target != -1 {
		return illegal("%s takes no target", a)
	}
	return nil
}

// Legal lists every legal intent for a seat (empty when the game is not
// waiting on it).
func (g *Game) Legal(seat int) []Intent {
	if seat < 0 || seat >= len(g.players) || !g.awaiting[seat] {
		return nil
	}
	var cands []Intent
	switch g.phase {
	case PhaseTurnStart:
		for _, a := range AllActions {
			if a.Targeted() {
				for t := range g.players {
					cands = append(cands, ActionIntent(a, t))
				}
			} else {
				cands = append(cands, ActionIntent(a, -1))
			}
		}
	case PhaseChallengeWindow, PhaseBlockChallengeWindow:
		cands = []Intent{PassIntent(), ChallengeIntent()}
	case PhaseBlockWindow:
		cands = append(cands, PassIntent())
		for _, c := range g.act.action.Blockers(g.cfg.Magistrate) {
			cands = append(cands, BlockIntent(c))
		}
	case PhaseProveClaim:
		for _, s := range g.players[seat].hiddenSlots() {
			cands = append(cands, RevealIntent(s))
		}
	case PhaseLoseInfluence:
		for _, s := range g.players[seat].hiddenSlots() {
			cands = append(cands, LoseIntent(s))
		}
	case PhaseExchange:
		n := len(g.exchangeOptions())
		for _, combo := range combinations(n, g.players[seat].hidden()) {
			cands = append(cands, ExchangeIntent(combo...))
		}
	}
	out := cands[:0]
	for _, c := range cands {
		if g.validate(seat, c) == nil {
			out = append(out, c)
		}
	}
	return out
}

// DefaultIntent is what the host submits when a seat runs out of time: pass
// in windows, Income (or a forced Coup) on a turn, prove a claim if possible,
// give up the first card, keep the current hand.
func (g *Game) DefaultIntent(seat int) (Intent, bool) {
	if !g.IsAwaiting(seat) {
		return Intent{}, false
	}
	p := &g.players[seat]
	switch g.phase {
	case PhaseTurnStart:
		if p.coins >= MustCoupAt {
			for i := 1; i < len(g.players); i++ {
				t := (seat + i) % len(g.players)
				if g.players[t].alive {
					return ActionIntent(Coup, t), true
				}
			}
		}
		return ActionIntent(Income, -1), true
	case PhaseChallengeWindow, PhaseBlockWindow, PhaseBlockChallengeWindow:
		return PassIntent(), true
	case PhaseProveClaim:
		slots := p.hiddenSlots()
		for _, s := range slots {
			if p.cards[s].Char == g.chal.claim {
				return RevealIntent(s), true
			}
		}
		return RevealIntent(slots[0]), true
	case PhaseLoseInfluence:
		return LoseIntent(p.hiddenSlots()[0]), true
	case PhaseExchange:
		keep := make([]int, p.hidden())
		for i := range keep {
			keep[i] = i
		}
		return ExchangeIntent(keep...), true
	}
	return Intent{}, false
}

// combinations returns every k-subset of 0..n-1 in lexicographic order.
func combinations(n, k int) [][]int {
	var out [][]int
	cur := make([]int, 0, k)
	var rec func(start int)
	rec = func(start int) {
		if len(cur) == k {
			out = append(out, append([]int(nil), cur...))
			return
		}
		for i := start; i < n; i++ {
			cur = append(cur, i)
			rec(i + 1)
			cur = cur[:len(cur)-1]
		}
	}
	rec(0)
	return out
}
