// Package engine implements the rules of Coup as a deterministic state
// machine. It performs no I/O and reads no clock; all randomness comes from a
// seeded PCG generator, so a seed plus a sequence of intents replays a game
// exactly.
package engine

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Character is an influence card.
type Character uint8

// Court characters. Magistrate only exists in the optional variant.
const (
	NoCharacter Character = iota
	Duke
	Assassin
	Captain
	Ambassador
	Contessa
	Magistrate
)

var characterNames = []string{"", "duke", "assassin", "captain", "ambassador", "contessa", "magistrate"}

// AllCharacters lists every real character, including the Magistrate variant.
var AllCharacters = []Character{Duke, Assassin, Captain, Ambassador, Contessa, Magistrate}

func (c Character) String() string {
	if int(c) < len(characterNames) {
		if c == NoCharacter {
			return "none"
		}
		return characterNames[c]
	}
	return fmt.Sprintf("character(%d)", uint8(c))
}

// Title is the display name, e.g. "DUKE".
func (c Character) Title() string { return strings.ToUpper(c.String()) }

// Valid reports whether c is a real character.
func (c Character) Valid() bool { return c >= Duke && c <= Magistrate }

func (c Character) MarshalText() ([]byte, error) { return marshalEnum(characterNames, uint8(c)) }

func (c *Character) UnmarshalText(b []byte) error {
	v, err := unmarshalEnum(characterNames, b, "character")
	*c = Character(v)
	return err
}

// CopiesPerCharacter is how many of each character the court deck holds.
const CopiesPerCharacter = 3

// DeckComposition returns the characters in play for a variant.
func DeckComposition(magistrate bool) []Character {
	if magistrate {
		return []Character{Duke, Assassin, Captain, Ambassador, Contessa, Magistrate}
	}
	return []Character{Duke, Assassin, Captain, Ambassador, Contessa}
}

// DeckSize is the total number of court cards for a variant.
func DeckSize(magistrate bool) int { return len(DeckComposition(magistrate)) * CopiesPerCharacter }

// Action is something a player does on their turn.
type Action uint8

// Actions. Character actions carry an implicit claim (see Action.Claim).
const (
	NoAction Action = iota
	Income
	ForeignAid
	Coup
	Tax
	Assassinate
	Steal
	Exchange
	Audit
)

var actionNames = []string{"", "income", "foreign_aid", "coup", "tax", "assassinate", "steal", "exchange", "audit"}

// AllActions lists every action, including the Magistrate variant's Audit.
var AllActions = []Action{Income, ForeignAid, Coup, Tax, Assassinate, Steal, Exchange, Audit}

func (a Action) String() string {
	if a == NoAction {
		return "none"
	}
	if int(a) < len(actionNames) {
		return actionNames[a]
	}
	return fmt.Sprintf("action(%d)", uint8(a))
}

// Title is the display name, e.g. "FOREIGN AID".
func (a Action) Title() string { return strings.ToUpper(strings.ReplaceAll(a.String(), "_", " ")) }

func (a Action) MarshalText() ([]byte, error) { return marshalEnum(actionNames, uint8(a)) }

func (a *Action) UnmarshalText(b []byte) error {
	v, err := unmarshalEnum(actionNames, b, "action")
	*a = Action(v)
	return err
}

// Claim is the character an action requires, or NoCharacter for general actions.
func (a Action) Claim() Character {
	switch a {
	case Tax:
		return Duke
	case Assassinate:
		return Assassin
	case Steal:
		return Captain
	case Exchange:
		return Ambassador
	case Audit:
		return Magistrate
	}
	return NoCharacter
}

// Cost is the number of coins paid when the action is declared.
func (a Action) Cost() int {
	switch a {
	case Coup:
		return CoupCost
	case Assassinate:
		return AssassinateCost
	}
	return 0
}

// Targeted reports whether the action names another player.
func (a Action) Targeted() bool {
	switch a {
	case Coup, Assassinate, Steal, Audit:
		return true
	}
	return false
}

// Blockers lists the characters that can block the action.
func (a Action) Blockers(magistrate bool) []Character {
	switch a {
	case ForeignAid:
		return []Character{Duke}
	case Assassinate:
		return []Character{Contessa}
	case Steal:
		return []Character{Captain, Ambassador}
	case Tax:
		if magistrate {
			return []Character{Magistrate}
		}
	}
	return nil
}

// BlockedByTargetOnly reports whether only the action's target may block it.
// Foreign Aid (and Tax, under Magistrate) can be blocked by anyone.
func (a Action) BlockedByTargetOnly() bool { return a == Assassinate || a == Steal }

// Rule constants.
const (
	MinPlayers      = 2
	MaxPlayers      = 6
	StartingCoins   = 2
	TotalCoins      = 50
	CoupCost        = 7
	AssassinateCost = 3
	MustCoupAt      = 10
	AuditThreshold  = 3
	AuditAmount     = 2
	StealAmount     = 2
	HandSize        = 2
	ExchangeDraw    = 2
)

// Phase is a step of the turn state machine.
type Phase uint8

// Phases. ActionDeclared, Resolve and TurnEnd are transient: the engine passes
// through them while applying an intent and reports them through events, but
// never waits in them.
const (
	PhaseTurnStart Phase = iota
	PhaseActionDeclared
	PhaseChallengeWindow
	PhaseBlockWindow
	PhaseBlockChallengeWindow
	PhaseProveClaim
	PhaseResolve
	PhaseLoseInfluence
	PhaseExchange
	PhaseTurnEnd
	PhaseGameOver
)

var phaseNames = []string{"turn_start", "action_declared", "challenge_window", "block_window",
	"block_challenge_window", "prove_claim", "resolve", "lose_influence", "exchange", "turn_end", "game_over"}

func (p Phase) String() string {
	if int(p) < len(phaseNames) {
		return phaseNames[p]
	}
	return fmt.Sprintf("phase(%d)", uint8(p))
}

func (p Phase) MarshalText() ([]byte, error) { return marshalEnum(phaseNames, uint8(p)) }

func (p *Phase) UnmarshalText(b []byte) error {
	v, err := unmarshalEnum(phaseNames, b, "phase")
	*p = Phase(v)
	return err
}

// Timed reports whether the phase is a reaction window that the host times.
func (p Phase) Timed() bool {
	return p == PhaseChallengeWindow || p == PhaseBlockWindow || p == PhaseBlockChallengeWindow
}

// IntentKind is the kind of decision a player submits.
type IntentKind uint8

// Intent kinds.
const (
	NoIntent IntentKind = iota
	IntentAction
	IntentChallenge
	IntentBlock
	IntentPass
	IntentReveal
	IntentLose
	IntentExchange
)

var intentNames = []string{"", "action", "challenge", "block", "pass", "reveal", "lose", "exchange"}

func (k IntentKind) String() string {
	if k == NoIntent {
		return "none"
	}
	if int(k) < len(intentNames) {
		return intentNames[k]
	}
	return fmt.Sprintf("intent(%d)", uint8(k))
}

func (k IntentKind) MarshalText() ([]byte, error) { return marshalEnum(intentNames, uint8(k)) }

func (k *IntentKind) UnmarshalText(b []byte) error {
	v, err := unmarshalEnum(intentNames, b, "intent kind")
	*k = IntentKind(v)
	return err
}

// Intent is a player's decision. Which fields matter depends on Kind:
//
//	IntentAction:   Action, Target (seat, or -1 for untargeted actions)
//	IntentBlock:    Claim
//	IntentReveal:   Card (hand slot to flip when challenged)
//	IntentLose:     Card (hand slot to give up)
//	IntentExchange: Keep (indices into PlayerView.ExchangeOptions)
type Intent struct {
	Kind   IntentKind `json:"kind"`
	Action Action     `json:"action,omitempty"`
	Target int        `json:"target"`
	Claim  Character  `json:"claim,omitempty"`
	Card   int        `json:"card"`
	Keep   []int      `json:"keep,omitempty"`
}

// Constructors for intents, mostly for tests and bots.

func ActionIntent(a Action, target int) Intent {
	if !a.Targeted() {
		target = -1
	}
	return Intent{Kind: IntentAction, Action: a, Target: target}
}

func ChallengeIntent() Intent           { return Intent{Kind: IntentChallenge, Target: -1} }
func PassIntent() Intent                { return Intent{Kind: IntentPass, Target: -1} }
func BlockIntent(c Character) Intent    { return Intent{Kind: IntentBlock, Claim: c, Target: -1} }
func RevealIntent(slot int) Intent      { return Intent{Kind: IntentReveal, Card: slot, Target: -1} }
func LoseIntent(slot int) Intent        { return Intent{Kind: IntentLose, Card: slot, Target: -1} }
func ExchangeIntent(keep ...int) Intent { return Intent{Kind: IntentExchange, Keep: keep, Target: -1} }

// UnmarshalJSON defaults a missing target to -1 (no target), so a client
// that omits it does not silently aim at seat 0.
func (in *Intent) UnmarshalJSON(b []byte) error {
	type plain Intent
	aux := struct {
		*plain
		Target *int `json:"target"`
	}{plain: (*plain)(in)}
	if err := json.Unmarshal(b, &aux); err != nil {
		return err
	}
	in.Target = -1
	if aux.Target != nil {
		in.Target = *aux.Target
	}
	return nil
}

func (in Intent) String() string {
	switch in.Kind {
	case IntentAction:
		if in.Action.Targeted() {
			return fmt.Sprintf("%s->%d", in.Action, in.Target)
		}
		return in.Action.String()
	case IntentBlock:
		return "block:" + in.Claim.String()
	case IntentReveal, IntentLose:
		return fmt.Sprintf("%s:%d", in.Kind, in.Card)
	case IntentExchange:
		return fmt.Sprintf("exchange:%v", in.Keep)
	}
	return in.Kind.String()
}

// Equal compares intents field by field (Keep order matters).
func (in Intent) Equal(o Intent) bool {
	if in.Kind != o.Kind || in.Action != o.Action || in.Target != o.Target || in.Claim != o.Claim || in.Card != o.Card {
		return false
	}
	if len(in.Keep) != len(o.Keep) {
		return false
	}
	for i := range in.Keep {
		if in.Keep[i] != o.Keep[i] {
			return false
		}
	}
	return true
}

// Card is one influence slot in a hand.
type Card struct {
	Char     Character `json:"char"`
	Revealed bool      `json:"revealed,omitempty"`
}

func marshalEnum(names []string, v uint8) ([]byte, error) {
	if int(v) >= len(names) {
		return nil, fmt.Errorf("engine: enum value %d out of range", v)
	}
	return []byte(names[v]), nil
}

func unmarshalEnum(names []string, b []byte, what string) (uint8, error) {
	s := string(b)
	for i, n := range names {
		if n == s {
			return uint8(i), nil
		}
	}
	if s == "none" {
		return 0, nil
	}
	return 0, fmt.Errorf("engine: unknown %s %q", what, s)
}
