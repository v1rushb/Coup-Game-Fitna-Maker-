package client

import (
	"time"

	"github.com/v1rushb/neoncoup/engine"
	"github.com/v1rushb/neoncoup/protocol"
)

// The presentation queue. Frames from the host are queued in arrival order
// and played one at a time: every event is a timed beat (announce, animate,
// settle; lengths in protocol.BeatMS) and a state is only applied once the
// beats in front of it are over, so the table never jumps ahead of what the
// player has seen. The host holds the next decision for the same total time,
// so nobody loses reaction time to an animation.

type qItem struct {
	ev    *engine.Event
	state *protocol.State
	over  *protocol.GameOver
	chat  *protocol.Envelope
	recv  time.Time
}

// beat is the event being played.
type beat struct {
	ev         engine.Event
	start, end time.Time
}

// progress is how far through the beat we are, from 0 to 1.
func (b *beat) progress(now time.Time) float64 {
	d := b.end.Sub(b.start)
	if d <= 0 {
		return 1
	}
	return min(1, max(0, float64(now.Sub(b.start))/float64(d)))
}

// presenting reports whether beats are playing or waiting to play.
func (g *gameScreen) presenting() bool { return g.beat != nil || len(g.queue) > 0 }

func (g *gameScreen) enqueue(a *App, it qItem) {
	it.recv = a.now
	g.queue = append(g.queue, it)
}

// statesQueued counts states waiting behind the current beat; more than one
// means this client has fallen behind the table.
func (g *gameScreen) statesQueued() int {
	n := 0
	for _, it := range g.queue {
		if it.state != nil {
			n++
		}
	}
	return n
}

// clock is the presentation time: it stands still while paused.
func (g *gameScreen) clock(a *App) time.Time {
	if g.paused {
		return g.pausedAt
	}
	return a.now
}

// pump plays the queue up to the first beat that is still running.
func (g *gameScreen) pump(a *App) {
	for !g.paused {
		if b := g.beat; b != nil {
			if a.now.Before(b.end) {
				return
			}
			g.beat = nil
			g.project(b.ev, false)
		}
		if len(g.queue) == 0 {
			g.updateHint(a)
			return
		}
		it := g.queue[0]
		g.queue = g.queue[1:]
		switch {
		case it.state != nil:
			g.onState(a, *it.state, it.recv)
		case it.over != nil:
			g.onGameOver(a, *it.over)
		case it.chat != nil:
			g.onChat(a, *it.chat)
		case it.ev != nil:
			g.startBeat(a, *it.ev)
		}
	}
}

// drain plays everything at once (tests, and screens built mid-game).
func (g *gameScreen) drain(a *App) {
	paused := g.paused
	g.paused = false
	for g.presenting() {
		if g.beat != nil {
			g.beat.end = a.now
		}
		g.pump(a)
	}
	g.paused = paused
}

// startBeat announces an event: log line, sound, effects, and the part of
// its change that should be visible while it plays (coins start ticking).
func (g *gameScreen) startBeat(a *App, e engine.Event) {
	g.project(e, true)
	if l, ok := narrate(e, g.name, g.me, a.t.Coin()); ok {
		g.addLog(l)
	}
	g.record(a, e)
	g.track(e)
	g.effects(a, e)
	if g.tutor != nil {
		g.tutor.onEvent(a, g, e)
	}
	d := time.Duration(protocol.BeatMS(e, g.speed)) * time.Millisecond
	if g.statesQueued() > 1 {
		d /= 4 // catch up with the table
	}
	if d <= 0 {
		g.project(e, false)
		return
	}
	g.beat = &beat{ev: e, start: a.now, end: a.now.Add(d)}
}

// skipBeat ends the current beat now and tells the host how much was
// skipped, so its clock keeps step (solo only).
func (g *gameScreen) skipBeat(a *App) {
	b := g.beat
	if b == nil || g.paused {
		return
	}
	left := b.end.Sub(a.now)
	b.end = a.now
	g.pump(a)
	if left > 0 && a.sess != nil {
		a.sess.Send(protocol.TControl, protocol.Control{SkipMS: int(left.Milliseconds())})
	}
}

// setPaused freezes or resumes the presentation and asks the host to do the
// same with its timers and bots (solo only).
func (g *gameScreen) setPaused(a *App, p bool) {
	if p == g.paused {
		return
	}
	if p {
		g.paused, g.pausedAt = true, a.now
	} else {
		d := a.now.Sub(g.pausedAt)
		g.paused = false
		if g.beat != nil {
			g.beat.start, g.beat.end = g.beat.start.Add(d), g.beat.end.Add(d)
		}
		if !g.deadline.IsZero() {
			g.deadline = g.deadline.Add(d)
		}
		g.pump(a)
	}
	if a.sess != nil {
		a.sess.Send(protocol.TControl, protocol.Control{Pause: &p})
	}
}

// soloControls reports whether pause and skip are available: you are the
// only human at the table.
func (g *gameScreen) soloControls(a *App) bool {
	if g.me < 0 {
		return false
	}
	for i, s := range g.seats {
		if i != g.me && !s.Bot {
			return false
		}
	}
	return a.mode == "solo" || a.mode == "tutorial"
}

// project applies an event to the displayed view, so the table stays
// coherent while beats play: at the start of a beat (start) the parts that
// animate during it, at the end the parts it settles into. The next state
// from the host replaces the view wholesale and agrees with it.
func (g *gameScreen) project(e engine.Event, start bool) {
	v := &g.v
	seatOK := func(s int) bool { return s >= 0 && s < len(v.Players) }
	if start {
		if protocol.BeatMS(e, protocol.SpeedRelaxed) > 0 && e.Kind != engine.EvTurnStart {
			// Whatever window or choice was open is over while this plays;
			// nothing is open until the host says what comes next.
			v.Phase, v.Awaiting, v.Legal = engine.PhaseResolve, nil, nil
		}
		switch e.Kind {
		case engine.EvTurnStart:
			if seatOK(e.Seat) {
				v.Turn, v.Current = e.Turn, e.Seat
				v.Phase, v.Pending, v.Challenge, v.Loser = engine.PhaseTurnStart, nil, nil, -1
				v.Awaiting, v.Legal = []int{e.Seat}, nil
			}
		case engine.EvActionDeclared:
			v.Pending = &engine.PendingView{Actor: e.Seat, Action: e.Action, Target: e.Other, Claim: e.Claim, Blocker: -1}
		case engine.EvBlock:
			if v.Pending != nil {
				v.Pending.Blocker, v.Pending.BlockClaim = e.Seat, e.Claim
			}
		case engine.EvChallenge:
			v.Challenge = &engine.ChallengeView{Challenger: e.Seat, Claimant: e.Other, Claim: e.Claim}
		case engine.EvClaimProven, engine.EvClaimFailed:
			v.Challenge = nil
			if e.Kind == engine.EvClaimProven {
				g.lastProven[e.Seat] = e.Card
			}
		case engine.EvCoins:
			if seatOK(e.Seat) {
				v.Players[e.Seat].Coins = max(0, v.Players[e.Seat].Coins+e.Amount)
				if e.Reason != engine.ReasonSteal {
					v.Treasury -= e.Amount
				}
			}
		}
		return
	}
	switch e.Kind {
	case engine.EvInfluenceLost:
		if seatOK(e.Seat) {
			p := &v.Players[e.Seat]
			if p.Hidden > 0 {
				p.Hidden--
				p.Revealed = append(p.Revealed, e.Card)
			}
			if e.Seat == g.me && e.Slot >= 0 && e.Slot < len(v.Hand) {
				v.Hand[e.Slot] = engine.Card{Char: e.Card, Revealed: true}
			}
		}
	case engine.EvEliminated:
		if seatOK(e.Seat) {
			v.Players[e.Seat].Alive = false
		}
	case engine.EvCardReplaced:
		if e.Seat == g.me && len(e.Secret) == 1 && e.Slot >= 0 && e.Slot < len(v.Hand) {
			v.Hand[e.Slot] = engine.Card{Char: e.Secret[0]}
		}
	case engine.EvExchangeDraw:
		v.DeckCount = max(0, v.DeckCount-e.Amount)
	case engine.EvExchangeDone:
		v.DeckCount += e.Amount
	}
}

// isBotChat reports whether a chat frame is a bot's taunt.
func isBotChat(env protocol.Envelope) bool {
	if env.Type != protocol.TChat {
		return false
	}
	var c protocol.Chat
	return env.Into(&c) == nil && c.Bot
}
