// Package bot implements computer players. A bot sees exactly what a human
// client sees: redacted events through Observe and its own redacted
// engine.PlayerView in Decide. It never receives an *engine.Game, so hidden
// state is out of reach by construction.
package bot

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"strings"

	"github.com/v1rushb/neoncoup/engine"
)

// Bot is the interface every computer player implements.
type Bot interface {
	// Observe receives every event the bot's seat may see, in order.
	Observe(ev engine.Event)
	// Decide returns one of v.Legal. It is only called when v.Legal is non-empty.
	Decide(v engine.PlayerView) engine.Intent
	// Taunt returns a line to post in the log, if the bot has one queued.
	Taunt() (string, bool)
	// Handle is the bot's display name.
	Handle() string
	// Level is the bot's difficulty.
	Level() Level
}

// Level is a bot difficulty.
type Level uint8

// Difficulty levels.
const (
	Easy Level = iota
	Normal
	Hard
)

var levelNames = []string{"easy", "normal", "hard"}

// Levels lists all difficulties, easiest first.
var Levels = []Level{Easy, Normal, Hard}

func (l Level) String() string {
	if int(l) < len(levelNames) {
		return levelNames[l]
	}
	return fmt.Sprintf("level(%d)", uint8(l))
}

// Title is the in-fiction name of the difficulty.
func (l Level) Title() string {
	switch l {
	case Easy:
		return "Script Kiddie"
	case Normal:
		return "Fixer"
	case Hard:
		return "Black ICE"
	}
	return l.String()
}

func (l Level) MarshalText() ([]byte, error) { return []byte(l.String()), nil }

func (l *Level) UnmarshalText(b []byte) error {
	v, err := ParseLevel(string(b))
	*l = v
	return err
}

// ParseLevel accepts "easy", "normal", "hard" and the in-fiction titles.
func ParseLevel(s string) (Level, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	for _, l := range Levels {
		if s == l.String() || s == strings.ToLower(l.Title()) || s == strings.ReplaceAll(strings.ToLower(l.Title()), " ", "-") {
			return l, nil
		}
	}
	switch s {
	case "kiddie", "script":
		return Easy, nil
	case "ice", "blackice":
		return Hard, nil
	}
	return Normal, fmt.Errorf("unknown bot level %q (easy, normal, hard)", s)
}

// New creates a bot for a seat.
func New(level Level, seat int, seed uint64, p Personality) Bot {
	return &brain{
		level: level,
		seat:  seat,
		rng:   rand.New(rand.NewPCG(seed, uint64(seat)*0x5851f42d4c957f2d+1)),
		pers:  p,
		k:     newKnowledge(seat),
	}
}

// brain is the shared implementation; level changes the policy.
type brain struct {
	level Level
	seat  int
	rng   *rand.Rand
	pers  Personality
	k     *knowledge
	taunt string
	quiet int // turns until the next taunt is allowed
	turn  int
}

func (b *brain) Handle() string { return b.pers.Handle }
func (b *brain) Level() Level   { return b.level }

func (b *brain) Observe(ev engine.Event) {
	b.k.observe(ev)
	if ev.Kind == engine.EvTurnStart && ev.Turn != b.turn {
		b.turn = ev.Turn
		if b.quiet > 0 {
			b.quiet--
		}
	}
	b.considerTaunt(ev)
}

func (b *brain) Taunt() (string, bool) {
	if b.taunt == "" {
		return "", false
	}
	t := b.taunt
	b.taunt = ""
	return t, true
}

func (b *brain) Decide(v engine.PlayerView) engine.Intent {
	if len(v.Legal) == 0 {
		return engine.PassIntent()
	}
	b.k.sync(v)
	var in engine.Intent
	switch v.Phase {
	case engine.PhaseTurnStart:
		in = b.chooseAction(v)
	case engine.PhaseChallengeWindow, engine.PhaseBlockChallengeWindow:
		in = b.chooseChallenge(v)
	case engine.PhaseBlockWindow:
		in = b.chooseBlock(v)
	case engine.PhaseProveClaim:
		in = b.chooseReveal(v)
	case engine.PhaseLoseInfluence:
		in = b.chooseLoss(v)
	case engine.PhaseExchange:
		in = b.chooseExchange(v)
	}
	if isLegal(in, v.Legal) {
		return in
	}
	// Policies only build legal intents; this is a safety net.
	for _, l := range v.Legal {
		if l.Kind == engine.IntentPass {
			return l
		}
	}
	return v.Legal[0]
}

func isLegal(in engine.Intent, legal []engine.Intent) bool {
	return slices.ContainsFunc(legal, in.Equal)
}

func (b *brain) chance(p float64) bool { return b.rng.Float64() < p }
