package bot

import (
	"math"
	"slices"

	"github.com/v1rushb/neoncoup/engine"
)

// --- view helpers ------------------------------------------------------------

func (b *brain) self(v engine.PlayerView) engine.PublicPlayer { return v.Players[b.seat] }

func (b *brain) opponents(v engine.PlayerView) []engine.PublicPlayer {
	var out []engine.PublicPlayer
	for _, p := range v.Players {
		if p.Seat != b.seat && p.Alive {
			out = append(out, p)
		}
	}
	return out
}

// myCards lists the bot's face-down cards.
func myCards(v engine.PlayerView) []engine.Character {
	var out []engine.Character
	for _, c := range v.Hand {
		if !c.Revealed {
			out = append(out, c.Char)
		}
	}
	return out
}

func holding(v engine.PlayerView, c engine.Character) bool {
	return slices.Contains(myCards(v), c)
}

func slotOf(v engine.PlayerView, c engine.Character) int {
	for i, h := range v.Hand {
		if !h.Revealed && h.Char == c {
			return i
		}
	}
	return -1
}

func actionIntents(legal []engine.Intent) []engine.Intent {
	var out []engine.Intent
	for _, in := range legal {
		if in.Kind == engine.IntentAction {
			out = append(out, in)
		}
	}
	return out
}

func find(legal []engine.Intent, a engine.Action, target int) (engine.Intent, bool) {
	for _, in := range legal {
		if in.Kind == engine.IntentAction && in.Action == a && (!a.Targeted() || in.Target == target) {
			return in, true
		}
	}
	return engine.Intent{}, false
}

// --- shared valuations -----------------------------------------------------

// threat estimates how dangerous an opponent is to everyone else.
func (b *brain) threat(v engine.PlayerView, o engine.PublicPlayer) float64 {
	t := 2.5*float64(o.Hidden) + 0.5*float64(o.Coins)
	if o.Coins >= engine.CoupCost {
		t += 3
	}
	if b.level == Hard {
		t += 1.5 * b.k.holds(o.Seat, engine.Assassin) * math.Min(1, float64(o.Coins)/3)
		t += 1.0 * b.k.holds(o.Seat, engine.Duke)
	} else {
		t += 0.5 * float64(b.k.claims[o.Seat][engine.Duke]+b.k.claims[o.Seat][engine.Assassin])
	}
	return t
}

func (b *brain) leader(v engine.PlayerView, among []engine.PublicPlayer) engine.PublicPlayer {
	best, bestT := among[0], math.Inf(-1)
	for _, o := range among {
		t := b.threat(v, o) + b.rng.Float64()*0.01
		if t > bestT {
			best, bestT = o, t
		}
	}
	return best
}

// cardValue scores keeping a character, for losses and exchanges.
func (b *brain) cardValue(v engine.PlayerView, c engine.Character) float64 {
	me := b.self(v)
	opps := b.opponents(v)
	assassinThreat := 0.0
	richOpp := false
	for _, o := range opps {
		assassinThreat = math.Max(assassinThreat, float64(b.k.claims[o.Seat][engine.Assassin]))
		if b.level == Hard {
			assassinThreat = math.Max(assassinThreat, 2*b.k.holds(o.Seat, engine.Assassin))
		}
		if o.Coins >= 3 {
			richOpp = true
		}
	}
	switch c {
	case engine.Duke:
		return 3.0
	case engine.Assassin:
		if me.Coins >= 3 {
			return 3.2
		}
		return 2.4
	case engine.Captain:
		if richOpp {
			return 2.9
		}
		return 2.4
	case engine.Contessa:
		return 1.8 + math.Min(1.6, 0.8*assassinThreat) + boolf(len(opps) == 1)*0.6
	case engine.Ambassador:
		return 1.5
	case engine.Magistrate:
		return 2.0
	}
	return 0
}

func boolf(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

// handValue scores a set of cards, penalizing duplicates.
func (b *brain) handValue(v engine.PlayerView, cards []engine.Character) float64 {
	sum := 0.0
	seen := map[engine.Character]bool{}
	for _, c := range cards {
		val := b.cardValue(v, c)
		if seen[c] {
			val -= 1.2
		}
		seen[c] = true
		sum += val
	}
	return sum
}

// --- reveal, loss, exchange ------------------------------------------------

func (b *brain) chooseReveal(v engine.PlayerView) engine.Intent {
	if v.Challenge != nil {
		if s := slotOf(v, v.Challenge.Claim); s >= 0 {
			return engine.RevealIntent(s)
		}
	}
	return engine.RevealIntent(b.worstSlot(v))
}

func (b *brain) chooseLoss(v engine.PlayerView) engine.Intent {
	return engine.LoseIntent(b.worstSlot(v))
}

func (b *brain) worstSlot(v engine.PlayerView) int {
	var slots []int
	for i, h := range v.Hand {
		if !h.Revealed {
			slots = append(slots, i)
		}
	}
	if b.level == Easy && b.chance(0.5) {
		return slots[b.rng.IntN(len(slots))]
	}
	worst, worstV := slots[0], math.Inf(1)
	for _, s := range slots {
		val := b.cardValue(v, v.Hand[s].Char) + b.rng.Float64()*0.05
		// Keep cards we have been claiming so our story stays consistent.
		val += 0.4 * math.Min(2, float64(b.k.claims[b.seat][v.Hand[s].Char]))
		if val < worstV {
			worst, worstV = s, val
		}
	}
	return worst
}

func (b *brain) chooseExchange(v engine.PlayerView) engine.Intent {
	opts := v.ExchangeOptions
	if b.level == Easy && b.chance(0.4) {
		return v.Legal[b.rng.IntN(len(v.Legal))]
	}
	best, bestV := v.Legal[0], math.Inf(-1)
	for _, in := range v.Legal {
		var cards []engine.Character
		for _, k := range in.Keep {
			cards = append(cards, opts[k])
		}
		val := b.handValue(v, cards) + b.rng.Float64()*0.05
		if val > bestV {
			best, bestV = in, val
		}
	}
	return best
}

// --- actions ---------------------------------------------------------------

// stallTurn is the turn after which bots stop playing coin-neutral moves.
const stallTurn = 200

// staleTurns without an influence loss make the hard bot press harder.
const staleTurns = 24

func (b *brain) chooseAction(v engine.PlayerView) engine.Intent {
	legal := actionIntents(v.Legal)
	opps := b.opponents(v)
	if len(legal) > 0 && legal[0].Action == engine.Coup && len(legal) == len(opps) {
		// Forced coup at 10+ coins.
		return b.coupTarget(v, legal)
	}
	if v.Turn >= stallTurn {
		// Anti-stall: coin-neutral loops (steal ping-pong, repeated audits)
		// are possible, so very long games fall back to Income and Coup,
		// which always terminates.
		if b.self(v).Coins >= engine.CoupCost {
			return b.coupTarget(v, filterAction(legal, engine.Coup))
		}
		in, _ := find(legal, engine.Income, -1)
		return in
	}
	switch b.level {
	case Easy:
		return b.easyAction(v, legal)
	case Normal:
		return b.normalAction(v, legal)
	}
	return b.hardAction(v, legal)
}

func (b *brain) coupTarget(v engine.PlayerView, legal []engine.Intent) engine.Intent {
	opps := b.opponents(v)
	if b.level == Easy {
		o := opps[b.rng.IntN(len(opps))]
		in, _ := find(legal, engine.Coup, o.Seat)
		return in
	}
	best, bestV := legal[0], math.Inf(-1)
	for _, o := range opps {
		in, ok := find(legal, engine.Coup, o.Seat)
		if !ok {
			continue
		}
		val := b.threat(v, o)
		if o.Hidden == 1 {
			val += 2 // a coup that eliminates removes a whole opponent
		}
		if val > bestV {
			best, bestV = in, val
		}
	}
	return best
}

func (b *brain) easyAction(v engine.PlayerView, legal []engine.Intent) engine.Intent {
	me := b.self(v)
	opps := b.opponents(v)
	if me.Coins >= engine.CoupCost && b.chance(0.6) {
		return b.coupTarget(v, filterAction(legal, engine.Coup))
	}
	if b.chance(0.07 * b.pers.Bluff) {
		return legal[b.rng.IntN(len(legal))]
	}
	type weighted struct {
		in engine.Intent
		w  float64
	}
	var opts []weighted
	add := func(a engine.Action, target int, w float64) {
		if in, ok := find(legal, a, target); ok {
			opts = append(opts, weighted{in, w})
		}
	}
	add(engine.Income, -1, 1)
	add(engine.ForeignAid, -1, 1.5)
	randomOpp := opps[b.rng.IntN(len(opps))]
	for _, c := range myCards(v) {
		switch c {
		case engine.Duke:
			add(engine.Tax, -1, 3)
		case engine.Captain:
			if randomOpp.Coins > 0 {
				add(engine.Steal, randomOpp.Seat, 2)
			}
		case engine.Assassin:
			add(engine.Assassinate, randomOpp.Seat, 2)
		case engine.Ambassador:
			add(engine.Exchange, -1, 0.7)
		case engine.Magistrate:
			add(engine.Audit, randomOpp.Seat, 1)
		}
	}
	total := 0.0
	for _, o := range opts {
		total += o.w
	}
	r := b.rng.Float64() * total
	for _, o := range opts {
		if r -= o.w; r <= 0 {
			return o.in
		}
	}
	return opts[len(opts)-1].in
}

func filterAction(legal []engine.Intent, a engine.Action) []engine.Intent {
	var out []engine.Intent
	for _, in := range legal {
		if in.Action == a {
			out = append(out, in)
		}
	}
	return out
}

func (b *brain) normalAction(v engine.PlayerView, legal []engine.Intent) engine.Intent {
	me := b.self(v)
	opps := b.opponents(v)
	lead := b.leader(v, opps)
	if me.Coins >= engine.CoupCost {
		return b.coupTarget(v, filterAction(legal, engine.Coup))
	}
	if holding(v, engine.Assassin) && me.Coins >= engine.AssassinateCost {
		// Prefer a target that has not been claiming Contessa.
		target := lead
		if b.k.claims[target.Seat][engine.Contessa] > 0 {
			for _, o := range opps {
				if b.k.claims[o.Seat][engine.Contessa] == 0 {
					target = o
					break
				}
			}
		}
		if b.k.claims[target.Seat][engine.Contessa] == 0 || b.chance(0.3) {
			if in, ok := find(legal, engine.Assassinate, target.Seat); ok {
				return in
			}
		}
	}
	if holding(v, engine.Duke) {
		if in, ok := find(legal, engine.Tax, -1); ok {
			return in
		}
	}
	if holding(v, engine.Captain) {
		var best *engine.PublicPlayer
		for i, o := range opps {
			if o.Coins >= 2 && b.k.claims[o.Seat][engine.Captain]+b.k.claims[o.Seat][engine.Ambassador] == 0 {
				if best == nil || o.Coins > best.Coins {
					best = &opps[i]
				}
			}
		}
		if best != nil {
			if in, ok := find(legal, engine.Steal, best.Seat); ok {
				return in
			}
		}
	}
	// Occasional bluffs, avoided when the cards are visibly scarce.
	if b.chance(0.2*b.pers.Bluff) && b.k.visible(engine.Duke) <= 1 {
		if in, ok := find(legal, engine.Tax, -1); ok {
			return in
		}
	}
	if me.Coins >= 3 && b.chance(0.08*b.pers.Bluff*b.pers.Aggression) && b.k.visible(engine.Assassin) <= 1 {
		if in, ok := find(legal, engine.Assassinate, lead.Seat); ok {
			return in
		}
	}
	if holding(v, engine.Ambassador) && b.handValue(v, myCards(v)) < 2.2*float64(len(myCards(v))) {
		if in, ok := find(legal, engine.Exchange, -1); ok {
			return in
		}
	}
	dukeClaimed := false
	for _, o := range opps {
		if b.k.claims[o.Seat][engine.Duke] > 0 {
			dukeClaimed = true
		}
	}
	if !dukeClaimed {
		if in, ok := find(legal, engine.ForeignAid, -1); ok {
			return in
		}
	}
	in, _ := find(legal, engine.Income, -1)
	return in
}

// --- hard: expected-value play ---------------------------------------------

// loseCost is the value of one of our influences.
func (b *brain) loseCost(v engine.PlayerView) float64 {
	if b.self(v).Hidden <= 1 {
		return 40
	}
	return 9
}

// killValue is what removing one influence from o is worth to us.
func (b *brain) killValue(v engine.PlayerView, o engine.PublicPlayer) float64 {
	val := 6 + b.threat(v, o)
	if o.Hidden == 1 {
		val += 6
	}
	return val
}

func (b *brain) coinValue(v engine.PlayerView, n int) float64 {
	me := b.self(v)
	val := float64(n)
	after := me.Coins + n
	if me.Coins < engine.CoupCost && after >= engine.CoupCost {
		val += 2
	}
	if me.Coins < engine.AssassinateCost && after >= engine.AssassinateCost && holding(v, engine.Assassin) {
		val++
	}
	// Sitting on a pile makes us the obvious target for everyone else.
	if n > 0 && after > engine.CoupCost && after < engine.MustCoupAt {
		val -= 0.3 * float64(after-engine.CoupCost)
	}
	return val
}

// pChallenged estimates how likely the table is to challenge our claim.
func (b *brain) pChallenged(v engine.PlayerView, claim engine.Character, a engine.Action, target int) float64 {
	if b.k.revealed[claim] >= engine.CopiesPerCharacter {
		return 0.97
	}
	miss := 1.0
	for _, o := range b.opponents(v) {
		// Start from how often this player actually challenges, then adjust
		// for how suspicious this particular claim looks.
		p := b.k.challengeRate(o.Seat) * (1 + 1.2*float64(b.k.revealed[claim]))
		if o.Seat == target {
			p += 0.05
			if a == engine.Assassinate && o.Hidden == 1 {
				p += 0.25
			}
		}
		p *= math.Pow(0.85, math.Min(3, float64(b.k.claims[b.seat][claim])))
		miss *= 1 - math.Max(0.01, math.Min(0.95, p))
	}
	return 1 - miss
}

// pBlocked estimates how likely an action is to be blocked.
func (b *brain) pBlocked(v engine.PlayerView, a engine.Action, target int) float64 {
	switch a {
	case engine.Assassinate:
		o := v.Players[target]
		h := b.k.holds(target, engine.Contessa)
		bluff := 0.2
		if o.Hidden == 1 {
			bluff = 0.8
		}
		return h + (1-h)*bluff
	case engine.Steal:
		h := b.k.holdsAny(target, []engine.Character{engine.Captain, engine.Ambassador})
		return 0.9*h + (1-h)*0.12
	case engine.ForeignAid, engine.Tax:
		blocker := engine.Duke
		if a == engine.Tax {
			if !v.Magistrate {
				return 0
			}
			blocker = engine.Magistrate
		}
		miss := 1.0
		for _, o := range b.opponents(v) {
			miss *= 1 - (0.8*b.k.holds(o.Seat, blocker) + 0.04)
		}
		return 1 - miss
	}
	return 0
}

func (b *brain) hardAction(v engine.PlayerView, legal []engine.Intent) engine.Intent {
	me := b.self(v)
	opps := b.opponents(v)
	cost := b.loseCost(v)

	// Endgame: a coup that ends the game always wins.
	if len(opps) == 1 && opps[0].Hidden == 1 {
		if in, ok := find(legal, engine.Coup, opps[0].Seat); ok {
			return in
		}
	}

	best, bestU := legal[0], math.Inf(-1)
	for _, in := range legal {
		u := b.hardUtility(v, in, me, opps, cost)
		u += b.rng.NormFloat64() * 0.35 // keep it human and unreadable
		if u > bestU {
			best, bestU = in, u
		}
	}
	return best
}

func (b *brain) hardUtility(v engine.PlayerView, in engine.Intent, me engine.PublicPlayer, opps []engine.PublicPlayer, cost float64) float64 {
	a := in.Action
	claim := a.Claim()
	honest := claim == engine.NoCharacter || holding(v, claim)
	pCh := 0.0
	if claim != engine.NoCharacter {
		pCh = b.pChallenged(v, claim, a, in.Target)
	}
	// Being challenged on a true claim costs the challenger an influence.
	challengeUpside := 0.0
	if claim != engine.NoCharacter && honest {
		challengeUpside = pCh * 4
	}
	success := 1.0
	if !honest {
		success = 1 - pCh
	}
	bluffRisk := 0.0
	if !honest {
		bluffRisk = pCh * cost / math.Max(0.3, b.pers.Bluff)
		if b.k.stale() > staleTurns {
			bluffRisk *= 0.4 // a standoff only ends if someone takes a risk
		}
	}

	var gain float64
	switch a {
	case engine.Income:
		gain = b.coinValue(v, 1)
	case engine.ForeignAid:
		gain = (1 - b.pBlocked(v, a, -1)) * b.coinValue(v, 2)
	case engine.Tax:
		gain = (1 - b.pBlocked(v, a, -1)) * b.coinValue(v, 3)
	case engine.Steal:
		t := v.Players[in.Target]
		n := min(engine.StealAmount, t.Coins)
		gain = b.coinValue(v, n) + 0.15*float64(n)*b.threat(v, t)/5
		if t.Coins >= engine.CoupCost && t.Coins-n < engine.CoupCost {
			// Knocking someone below Coup range buys us a turn.
			gain += 4
			if me.Hidden == 1 {
				gain += 12
			}
		}
		gain *= 1 - b.pBlocked(v, a, in.Target)
	case engine.Assassinate:
		t := v.Players[in.Target]
		gain = -engine.AssassinateCost + (1-b.pBlocked(v, a, in.Target))*b.killValue(v, t)*b.pers.Aggression
	case engine.Coup:
		t := v.Players[in.Target]
		gain = -engine.CoupCost + 2 + b.killValue(v, t)*b.pers.Aggression
		if t.Coins >= engine.CoupCost {
			gain += 3 // they were about to coup someone, possibly us
		}
	case engine.Exchange:
		cards := myCards(v)
		gain = math.Max(0, 2.6*float64(len(cards))-b.handValue(v, cards))*0.8 + 0.4
	case engine.Audit:
		// Audit gains us nothing directly, so it only pays when it denies a
		// real threat; otherwise two players can trade Audits forever.
		t := v.Players[in.Target]
		gain = 0.15 * b.threat(v, t)
		if t.Coins >= engine.CoupCost && t.Coins-engine.AuditAmount < engine.CoupCost {
			gain += 4
			if me.Hidden == 1 {
				gain += 12
			}
		}
		if b.k.stale() > staleTurns {
			gain *= 0.3 // a standoff; stop propping it up
		}
	}

	// Danger: an opponent with Coup money next turn while we are on one card.
	if me.Hidden == 1 {
		for _, o := range opps {
			if o.Coins >= engine.CoupCost && a != engine.Coup && (a != engine.Steal || in.Target != o.Seat) {
				gain--
			}
		}
	}
	return success*gain + challengeUpside - bluffRisk
}

// --- reactions -------------------------------------------------------------

// claimInFlight returns the claimant and character being challenged.
func claimInFlight(v engine.PlayerView) (int, engine.Character, bool) {
	p := v.Pending
	if p == nil {
		return -1, engine.NoCharacter, false
	}
	if v.Phase == engine.PhaseBlockChallengeWindow {
		return p.Blocker, p.BlockClaim, true
	}
	return p.Actor, p.Claim, true
}

func (b *brain) chooseChallenge(v engine.PlayerView) engine.Intent {
	claimant, claim, ok := claimInFlight(v)
	if !ok {
		return engine.PassIntent()
	}
	impossible := b.k.unseen[claim] <= 0
	switch b.level {
	case Easy:
		if (impossible && b.chance(0.85)) || b.chance(0.04*b.pers.Suspicion) {
			return engine.ChallengeIntent()
		}
		return engine.PassIntent()
	case Normal:
		if impossible {
			return engine.ChallengeIntent()
		}
		return b.normalChallenge(v, claimant, claim)
	}
	if impossible {
		return engine.ChallengeIntent()
	}
	return b.hardChallenge(v, claimant, claim)
}

func (b *brain) normalChallenge(v engine.PlayerView, claimant int, claim engine.Character) engine.Intent {
	me := b.self(v)
	p := v.Pending
	pLack := 1 - b.k.prior(claimant, claim)
	// Repeated claims make a claim more believable.
	pLack *= math.Pow(0.8, float64(b.k.claims[claimant][claim]))
	targetedMe := p.Target == b.seat && v.Phase == engine.PhaseChallengeWindow
	if targetedMe && p.Action == engine.Assassinate && me.Hidden == 1 && !holding(v, engine.Contessa) {
		if pLack > 0.45 {
			return engine.ChallengeIntent()
		}
		return engine.PassIntent() // try a Contessa block instead
	}
	threshold := 0.82
	if me.Hidden == 1 {
		threshold = 0.9
	}
	if targetedMe || (v.Phase == engine.PhaseBlockChallengeWindow && p.Actor == b.seat) {
		threshold -= 0.1
	}
	if pLack*b.pers.Suspicion > threshold && b.chance(0.75) {
		return engine.ChallengeIntent()
	}
	return engine.PassIntent()
}

func (b *brain) hardChallenge(v engine.PlayerView, claimant int, claim engine.Character) engine.Intent {
	me := b.self(v)
	p := v.Pending
	pHas := b.k.holds(claimant, claim)
	cost := b.loseCost(v)
	cl := v.Players[claimant]
	win := 3 + 0.5*b.threat(v, cl)
	if cl.Hidden == 1 {
		win += 4
	}

	onBlock := v.Phase == engine.PhaseBlockChallengeWindow
	if !onBlock {
		switch p.Action {
		case engine.Tax:
			win += 1.5
			if cl.Coins+3 >= engine.CoupCost {
				win += 2
			}
		case engine.Steal:
			if p.Target == b.seat {
				win += 2 + float64(min(2, me.Coins))
			} else {
				win += 0.5
			}
		case engine.Assassinate:
			if p.Target == b.seat {
				if holding(v, engine.Contessa) {
					return engine.PassIntent() // block for free instead
				}
				// Losing this challenge also lets the assassination land.
				cost = 40
				if me.Hidden == 1 {
					// Compare with a Contessa bluff in the block window.
					survBlock := 1 - b.pBlockChallenged(v, engine.Contessa, p.Actor)
					if 1-pHas > survBlock {
						return engine.ChallengeIntent()
					}
					return engine.PassIntent()
				}
				win += 9
			} else {
				win++
			}
		case engine.Exchange:
			win += 0.5
		case engine.Audit:
			if p.Target == b.seat {
				win += 2
			}
		}
	} else if p.Actor == b.seat {
		// Our own action was blocked: winning lets it through.
		switch p.Action {
		case engine.Assassinate:
			win += b.killValue(v, v.Players[p.Target])
		case engine.Steal:
			win += 3
		case engine.ForeignAid:
			win += 2
		case engine.Tax:
			win += 3
		}
	}
	ev := (1-pHas)*win - pHas*cost
	if st := b.k.stale(); st > staleTurns {
		// Nobody has lost influence in a while: call claims to break the standoff.
		ev += float64(st-staleTurns) * 1.5
	}
	if ev*b.pers.Suspicion > b.rng.Float64()*1.5 {
		return engine.ChallengeIntent()
	}
	return engine.PassIntent()
}

// pBlockChallenged estimates the chance our block claim gets challenged.
func (b *brain) pBlockChallenged(v engine.PlayerView, claim engine.Character, actor int) float64 {
	if b.k.revealed[claim] >= engine.CopiesPerCharacter {
		return 0.97
	}
	p := 0.18 + 0.2*float64(b.k.revealed[claim])
	if v.Pending != nil && v.Pending.Action == engine.Assassinate && v.Players[b.seat].Hidden == 1 {
		p += 0.25 // everyone knows a dying player bluffs Contessa
	}
	_ = actor
	return math.Min(0.95, p)
}

func (b *brain) chooseBlock(v engine.PlayerView) engine.Intent {
	p := v.Pending
	if p == nil {
		return engine.PassIntent()
	}
	blockers := p.Action.Blockers(v.Magistrate)
	for _, c := range blockers {
		if holding(v, c) && (b.level != Easy || b.chance(0.9)) {
			return engine.BlockIntent(c)
		}
	}
	me := b.self(v)
	targeted := p.Target == b.seat
	// Assassinated on our last card: a bluff is the only way to survive.
	if p.Action == engine.Assassinate && targeted && me.Hidden == 1 {
		prob := map[Level]float64{Easy: 0.4, Normal: 0.85, Hard: 1}[b.level]
		if b.chance(prob) {
			return engine.BlockIntent(engine.Contessa)
		}
		return engine.PassIntent()
	}
	bluff := b.bestBluffBlock(v, blockers)
	switch b.level {
	case Easy:
		if b.chance(0.04 * b.pers.Bluff) {
			return engine.BlockIntent(bluff)
		}
	case Normal:
		var prob float64
		switch p.Action {
		case engine.Steal:
			prob = 0.12
		case engine.Assassinate:
			if b.k.visible(engine.Contessa) == 0 {
				prob = 0.15
			}
		case engine.ForeignAid, engine.Tax:
			if b.leader(v, b.opponents(v)).Seat == p.Actor {
				prob = 0.08
			}
		}
		if b.chance(prob * b.pers.Bluff) {
			return engine.BlockIntent(bluff)
		}
	case Hard:
		if b.hardBluffBlock(v, bluff) {
			return engine.BlockIntent(bluff)
		}
	}
	return engine.PassIntent()
}

// bestBluffBlock picks the most believable blocker: the one with most unseen copies.
func (b *brain) bestBluffBlock(v engine.PlayerView, blockers []engine.Character) engine.Character {
	best := blockers[0]
	for _, c := range blockers[1:] {
		if b.k.unseen[c] > b.k.unseen[best] {
			best = c
		}
	}
	return best
}

func (b *brain) hardBluffBlock(v engine.PlayerView, claim engine.Character) bool {
	p := v.Pending
	if b.k.unseen[claim] <= 0 {
		return false // provably false
	}
	pCB := b.pBlockChallenged(v, claim, p.Actor)
	cost := b.loseCost(v)
	var gain float64
	switch p.Action {
	case engine.Assassinate:
		// Caught bluffing Contessa costs both cards; not blocking costs one.
		return (1-pCB)*9 > pCB*40/math.Max(0.3, b.pers.Bluff)+0.5
	case engine.Steal:
		gain = 2*float64(min(2, b.self(v).Coins)) + 0.5
	case engine.ForeignAid:
		actor := v.Players[p.Actor]
		gain = 0.5 + 0.2*b.threat(v, actor)
		if actor.Coins+2 >= engine.CoupCost {
			gain += 2
		}
	case engine.Tax:
		actor := v.Players[p.Actor]
		gain = 1 + 0.2*b.threat(v, actor)
	}
	return (1-pCB)*gain*b.pers.Bluff > pCB*cost+0.5
}
