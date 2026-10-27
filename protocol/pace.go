package protocol

import (
	"time"

	"github.com/v1rushb/neoncoup/engine"
)

// Presentation speeds. Clients play every event as a timed "beat" and the
// host holds the next decision until the beats are over, so both sides must
// agree on how long each beat lasts. That table lives here.
const (
	SpeedRelaxed = "relaxed"
	SpeedNormal  = "normal"
	SpeedFast    = "fast"
	// SpeedInstant turns presentation off (tests, simulations, headless play).
	SpeedInstant = "instant"
)

// Speeds lists the speeds a player can pick, slowest first.
var Speeds = []string{SpeedRelaxed, SpeedNormal, SpeedFast}

// ParseSpeed returns a known speed, defaulting to relaxed.
func ParseSpeed(s string) string {
	switch s {
	case SpeedRelaxed, SpeedNormal, SpeedFast, SpeedInstant:
		return s
	}
	return SpeedRelaxed
}

// SpeedFactor scales beat lengths and bot think time.
func SpeedFactor(speed string) float64 {
	switch ParseSpeed(speed) {
	case SpeedNormal:
		return 0.6
	case SpeedFast:
		return 0.3
	case SpeedInstant:
		return 0
	}
	return 1
}

// AutoWindow is the reaction window used when the host leaves it on auto.
func AutoWindow(speed string) time.Duration {
	switch ParseSpeed(speed) {
	case SpeedNormal:
		return 8 * time.Second
	case SpeedFast:
		return 5 * time.Second
	case SpeedInstant:
		return 8 * time.Second
	}
	return 12 * time.Second
}

// relaxedBeatMS is each event's beat at Relaxed speed.
func relaxedBeatMS(e engine.Event) int {
	switch e.Kind {
	case engine.EvActionDeclared:
		return 1500
	case engine.EvChallenge, engine.EvBlock:
		return 2000
	case engine.EvClaimProven, engine.EvClaimFailed:
		return 1000
	case engine.EvCardReplaced:
		return 2500
	case engine.EvInfluenceLost:
		return 2000
	case engine.EvEliminated:
		return 3000
	case engine.EvCoins:
		if e.Amount == 0 || e.Reason == engine.ReasonEliminated {
			return 0 // folded into the elimination beat
		}
		if e.Reason == engine.ReasonSteal && e.Amount < 0 {
			return 0 // the thief's matching event carries the animation
		}
		return 1000
	case engine.EvExchangeDraw, engine.EvExchangeDone:
		return 1500
	case engine.EvActionBlocked, engine.EvActionFailed:
		return 1200
	case engine.EvTurnStart:
		return 600
	}
	return 0
}

// BeatMS is how long an event plays at the given speed. It never depends on
// an event's secret, so every seat computes the same schedule.
func BeatMS(e engine.Event, speed string) int {
	return int(float64(relaxedBeatMS(e))*SpeedFactor(speed) + 0.5)
}

// PresentMS is how long a batch of events takes to play.
func PresentMS(evs []engine.Event, speed string) int {
	total := 0
	for _, e := range evs {
		total += BeatMS(e, speed)
	}
	return total
}
