package bot

import (
	"math"

	"github.com/v1rushb/neoncoup/engine"
)

// Likelihood ratios used to turn observed behavior into evidence about
// hidden cards. A ratio above 1 means "this behavior is more likely if the
// player holds the character".
const (
	lrClaim          = 2.6  // claiming a character's action
	lrRepeatClaim    = 1.8  // claiming it again
	lrBlock          = 2.2  // blocking with a character
	lrForcedBlock    = 1.15 // Contessa block on the last card: bluffing is the only way to live
	lrNoContessa     = 0.08 // assassinated and did not block
	lrNoContessaLast = 0.02 // ...while on the last card
	lrNoStealBlock   = 0.35 // stolen from and did not block
	lrNoAidBlock     = 0.75 // let foreign aid through
	logFailedClaim   = -6.0 // flipped another card when challenged
	logCap           = 3.5
)

// knowledge is what a seat has learned from the public record.
type knowledge struct {
	me int
	n  int
	// evidence[seat][char] is the summed log likelihood ratio for "seat holds char".
	evidence [][engine.Magistrate + 1]float64
	// claims[seat][char] counts claims since the seat last reshuffled its hand.
	claims [][engine.Magistrate + 1]int
	hidden []int
	coins  []int
	alive  []bool
	// challenge chances taken and passed, per seat, to learn how trigger-happy each player is.
	challenged []int
	passed     []int

	// unseen[char] = copies that are neither in my hand nor face up.
	unseen     [engine.Magistrate + 1]int
	unseenTot  int
	revealed   [engine.Magistrate + 1]int
	magistrate bool

	// Current action, reconstructed from events.
	curActor, curTarget int
	curAction           engine.Action
	lastChallengeClaim  engine.Character
	turn                int
	lastLoss            int // turn of the most recent influence loss
}

func newKnowledge(me int) *knowledge {
	return &knowledge{me: me, curActor: -1, curTarget: -1}
}

func (k *knowledge) ensure(n int) {
	if k.n == n {
		return
	}
	k.n = n
	k.evidence = make([][engine.Magistrate + 1]float64, n)
	k.claims = make([][engine.Magistrate + 1]int, n)
	k.hidden = make([]int, n)
	k.coins = make([]int, n)
	k.alive = make([]bool, n)
	k.challenged = make([]int, n)
	k.passed = make([]int, n)
	for i := range n {
		k.hidden[i] = engine.HandSize
		k.alive[i] = true
	}
}

func (k *knowledge) add(seat int, c engine.Character, logLR float64) {
	if seat < 0 || seat >= k.n || seat == k.me || !c.Valid() {
		return
	}
	v := k.evidence[seat][c] + logLR
	k.evidence[seat][c] = math.Max(-logCap*2, math.Min(logCap, v))
}

func (k *knowledge) observe(ev engine.Event) {
	if ev.Kind == engine.EvGameStart {
		k.ensure(ev.Amount)
	}
	if k.n == 0 {
		return
	}
	k.turn = ev.Turn
	switch ev.Kind {
	case engine.EvActionDeclared:
		k.curActor, k.curTarget, k.curAction = ev.Seat, ev.Other, ev.Action
		if ev.Claim != engine.NoCharacter {
			// Repeating a claim is weaker evidence than making it: a bluff
			// that works tends to get repeated.
			lr := lrClaim
			if k.claims[ev.Seat][ev.Claim] > 0 {
				lr = lrRepeatClaim
			}
			k.add(ev.Seat, ev.Claim, math.Log(lr))
			k.claims[ev.Seat][ev.Claim]++
		}
	case engine.EvBlock:
		lr := lrBlock
		if ev.Claim == engine.Contessa && k.hidden[ev.Seat] == 1 {
			lr = lrForcedBlock
		}
		k.add(ev.Seat, ev.Claim, math.Log(lr))
		k.claims[ev.Seat][ev.Claim]++
	case engine.EvPass:
		if ev.Phase == engine.PhaseChallengeWindow || ev.Phase == engine.PhaseBlockChallengeWindow {
			if ev.Seat >= 0 && ev.Seat < k.n {
				k.passed[ev.Seat]++
			}
			return
		}
		if ev.Phase != engine.PhaseBlockWindow {
			return
		}
		switch k.curAction {
		case engine.Assassinate:
			if ev.Seat == k.curTarget {
				lr := lrNoContessa
				if k.hidden[ev.Seat] == 1 {
					lr = lrNoContessaLast
				}
				k.add(ev.Seat, engine.Contessa, math.Log(lr))
			}
		case engine.Steal:
			if ev.Seat == k.curTarget {
				k.add(ev.Seat, engine.Captain, math.Log(lrNoStealBlock))
				k.add(ev.Seat, engine.Ambassador, math.Log(lrNoStealBlock))
			}
		case engine.ForeignAid:
			k.add(ev.Seat, engine.Duke, math.Log(lrNoAidBlock))
		case engine.Tax:
			k.add(ev.Seat, engine.Magistrate, math.Log(lrNoAidBlock))
		}
	case engine.EvChallenge:
		k.lastChallengeClaim = ev.Claim
		if ev.Seat >= 0 && ev.Seat < k.n {
			k.challenged[ev.Seat]++
		}
	case engine.EvClaimFailed:
		if ev.Seat >= 0 && ev.Seat < k.n && ev.Seat != k.me {
			k.evidence[ev.Seat][k.lastChallengeClaim] = logFailedClaim
		}
	case engine.EvCardReplaced:
		// The proven card went back into the deck; the new one is random.
		if ev.Seat >= 0 && ev.Seat < k.n {
			k.evidence[ev.Seat][k.lastChallengeClaim] = 0
			k.claims[ev.Seat][k.lastChallengeClaim] = 0
		}
	case engine.EvExchangeDone:
		if ev.Seat >= 0 && ev.Seat < k.n {
			k.evidence[ev.Seat] = [engine.Magistrate + 1]float64{}
			k.claims[ev.Seat] = [engine.Magistrate + 1]int{}
		}
	case engine.EvInfluenceLost:
		k.lastLoss = ev.Turn
		if ev.Seat >= 0 && ev.Seat < k.n {
			k.hidden[ev.Seat]--
			// A face-up card says nothing about the remaining one.
			if k.evidence[ev.Seat][ev.Card] > 0 {
				k.evidence[ev.Seat][ev.Card] = 0
			}
		}
	case engine.EvEliminated:
		if ev.Seat >= 0 && ev.Seat < k.n {
			k.alive[ev.Seat] = false
			k.hidden[ev.Seat] = 0
		}
	}
}

// sync refreshes counts from the bot's own view, which is authoritative.
func (k *knowledge) sync(v engine.PlayerView) {
	k.ensure(len(v.Players))
	k.magistrate = v.Magistrate
	k.revealed = [engine.Magistrate + 1]int{}
	for i, p := range v.Players {
		k.hidden[i] = p.Hidden
		k.coins[i] = p.Coins
		k.alive[i] = p.Alive
		for _, c := range p.Revealed {
			k.revealed[c]++
		}
	}
	k.unseen = [engine.Magistrate + 1]int{}
	k.unseenTot = 0
	for _, c := range engine.DeckComposition(v.Magistrate) {
		n := engine.CopiesPerCharacter - k.revealed[c]
		for _, h := range v.Hand {
			if h.Char == c && !h.Revealed {
				n--
			}
		}
		k.unseen[c] = n
		k.unseenTot += n
	}
}

// visible is how many copies of c the bot can account for: its own face-down
// cards plus every face-up card.
func (k *knowledge) visible(c engine.Character) int {
	return engine.CopiesPerCharacter - k.unseen[c]
}

// prior is the chance a seat holds at least one c, from card counting alone:
// its face-down cards are a uniform draw from the unseen pool.
func (k *knowledge) prior(seat int, c engine.Character) float64 {
	h := k.hidden[seat]
	u := k.unseen[c]
	if h <= 0 || u <= 0 {
		return 0
	}
	if u >= k.unseenTot {
		return 1
	}
	// P(none) = C(U-u, h) / C(U, h)
	none := 1.0
	for i := range h {
		none *= float64(k.unseenTot-u-i) / float64(k.unseenTot-i)
	}
	return 1 - math.Max(0, none)
}

// holds is the posterior chance that a seat holds at least one c.
func (k *knowledge) holds(seat int, c engine.Character) float64 {
	p := k.prior(seat, c)
	if p <= 0 || p >= 1 {
		return p
	}
	odds := p / (1 - p) * math.Exp(k.evidence[seat][c])
	return odds / (1 + odds)
}

// stale reports how many turns have passed without anyone losing influence.
func (k *knowledge) stale() int { return k.turn - k.lastLoss }

// challengeRate estimates how often a seat challenges when it gets the
// chance, starting from a prior of about one in sixteen.
func (k *knowledge) challengeRate(seat int) float64 {
	if seat < 0 || seat >= k.n {
		return 1.0 / 16
	}
	return (float64(k.challenged[seat]) + 0.5) / (float64(k.challenged[seat]+k.passed[seat]) + 8)
}

// holdsAny is the chance a seat holds at least one of cs.
func (k *knowledge) holdsAny(seat int, cs []engine.Character) float64 {
	miss := 1.0
	for _, c := range cs {
		miss *= 1 - k.holds(seat, c)
	}
	return 1 - miss
}
