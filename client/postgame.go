package client

import (
	"fmt"

	"github.com/v1rushb/neoncoup/engine"
	"github.com/v1rushb/neoncoup/protocol"
)

// After the game nothing is secret: the replay holds the seed and every
// intent, so re-running it shows every hand and which claims were bluffs.

type bluffCount struct {
	ok, caught int // bluffs that stood, and bluffs that were called
}

type postGame struct {
	hands   [][]engine.Card
	bluffs  []bluffCount
	moments []string
}

func holds(hand []engine.Card, c engine.Character) bool {
	for _, h := range hand {
		if !h.Revealed && h.Char == c {
			return true
		}
	}
	return false
}

// analyze re-runs a finished game.
func analyze(r protocol.Replay, name func(int) string) (*postGame, error) {
	g, err := engine.New(r.Config)
	if err != nil {
		return nil, err
	}
	pg := &postGame{bluffs: make([]bluffCount, r.Config.Players)}
	moment := func(turn int, s string) { pg.moments = append(pg.moments, fmt.Sprintf("T%-2d %s", turn, s)) }
	poss := func(seat int) string {
		if n := name(seat); n != "YOU" {
			return n + "'s"
		}
		return "your"
	}
	open := map[int]bool{} // seats bluffing in the turn being played
	settle := func() {
		for s := range open {
			pg.bluffs[s].ok++
		}
		clear(open)
	}
	var challenge, action engine.Event
	for i, st := range r.Steps {
		before := g.Snapshot()
		evs, err := g.Apply(st.Seat, st.Intent)
		if err != nil {
			return nil, fmt.Errorf("step %d does not replay: %w", i+1, err)
		}
		for _, e := range evs {
			switch e.Kind {
			case engine.EvActionDeclared:
				action = e
				if e.Claim != engine.NoCharacter && !holds(before.Hands[e.Seat], e.Claim) {
					open[e.Seat] = true
				}
				if e.Action == engine.Coup {
					moment(e.Turn, fmt.Sprintf("%s couped %s", name(e.Seat), name(e.Other)))
				}
			case engine.EvBlock:
				if !holds(before.Hands[e.Seat], e.Claim) {
					open[e.Seat] = true
				}
			case engine.EvChallenge:
				challenge = e
			case engine.EvClaimProven:
				moment(e.Turn, fmt.Sprintf("%s called %s %s: it was real", name(challenge.Seat), poss(e.Seat), challenge.Claim.Title()))
			case engine.EvClaimFailed:
				if open[e.Seat] {
					pg.bluffs[e.Seat].caught++
					delete(open, e.Seat)
				}
				moment(e.Turn, fmt.Sprintf("%s caught %s bluffing %s", name(challenge.Seat), name(e.Seat), challenge.Claim.Title()))
			case engine.EvInfluenceLost:
				if e.Reason == engine.ReasonAssassinated {
					moment(e.Turn, fmt.Sprintf("%s assassinated %s %s", name(action.Seat), poss(e.Seat), e.Card.Title()))
				}
			case engine.EvEliminated:
				moment(e.Turn, name(e.Seat)+" flatlined")
			case engine.EvTurnEnd:
				settle()
			case engine.EvGameOver:
				settle()
				moment(e.Turn, name(e.Seat)+" seized the city")
			}
		}
	}
	pg.hands = g.Snapshot().Hands
	return pg, nil
}
