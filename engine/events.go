package engine

import "fmt"

// EventKind identifies what happened.
type EventKind uint8

// Event kinds, in roughly the order a turn produces them.
const (
	EvNone EventKind = iota
	EvGameStart
	EvTurnStart
	EvActionDeclared
	EvPass
	EvChallenge
	EvClaimProven
	EvClaimFailed
	EvCardReplaced
	EvBlock
	EvCoins
	EvInfluenceLost
	EvEliminated
	EvActionResolved
	EvActionFailed
	EvActionBlocked
	EvBlockFailed
	EvExchangeDraw
	EvExchangeDone
	EvTurnEnd
	EvGameOver
	EvWindowOpen
)

var eventNames = []string{"", "game_start", "turn_start", "action_declared", "pass", "challenge", "claim_proven",
	"claim_failed", "card_replaced", "block", "coins", "influence_lost", "eliminated", "action_resolved",
	"action_failed", "action_blocked", "block_failed", "exchange_draw", "exchange_done", "turn_end", "game_over", "window_open"}

func (k EventKind) String() string {
	if int(k) < len(eventNames) && k != EvNone {
		return eventNames[k]
	}
	return fmt.Sprintf("event(%d)", uint8(k))
}

func (k EventKind) MarshalText() ([]byte, error) { return marshalEnum(eventNames, uint8(k)) }

func (k *EventKind) UnmarshalText(b []byte) error {
	v, err := unmarshalEnum(eventNames, b, "event kind")
	*k = EventKind(v)
	return err
}

// Reason qualifies coin movements and failures.
type Reason uint8

// Reasons.
const (
	ReasonNone Reason = iota
	ReasonIncome
	ReasonForeignAid
	ReasonTax
	ReasonSteal
	ReasonCoupFee
	ReasonAssassinFee
	ReasonRefund
	ReasonAudit
	ReasonEliminated
	ReasonChallenged
	ReasonNoTarget
	ReasonLostChallenge
	ReasonCoup
	ReasonAssassinated
	ReasonFailedClaim
)

var reasonNames = []string{"", "income", "foreign_aid", "tax", "steal", "coup_fee", "assassin_fee", "refund",
	"audit", "eliminated", "challenged", "no_target", "lost_challenge", "coup", "assassinated", "failed_claim"}

func (r Reason) String() string {
	if int(r) < len(reasonNames) && r != ReasonNone {
		return reasonNames[r]
	}
	return "none"
}

func (r Reason) MarshalText() ([]byte, error) { return marshalEnum(reasonNames, uint8(r)) }

func (r *Reason) UnmarshalText(b []byte) error {
	v, err := unmarshalEnum(reasonNames, b, "reason")
	*r = Reason(v)
	return err
}

// Event records one observable step of the game. Seat is the subject of the
// event and Other its counterpart (-1 when there is none):
//
//	EvActionDeclared  Seat=actor  Other=target  Action, Claim
//	EvPass            Seat=passer
//	EvChallenge       Seat=challenger  Other=claimant  Claim
//	EvClaimProven     Seat=claimant  Other=challenger  Card=shown, Slot
//	EvClaimFailed     Seat=claimant  Other=challenger  Card=flipped (lost), Slot
//	EvCardReplaced    Seat=claimant  Slot, Secret=[new card]
//	EvBlock           Seat=blocker  Other=actor  Claim, Action
//	EvCoins           Seat=player  Other=counterparty (steal)  Amount=delta  Reason
//	EvInfluenceLost   Seat=player  Card, Slot, Reason
//	EvEliminated      Seat=player  Amount=coins returned to the Treasury
//	EvExchangeDraw    Seat=actor  Amount=cards drawn  Secret=drawn cards
//	EvGameOver        Seat=winner
//	EvWindowOpen      Seat=actor  Other=target  Action, Phase=the window, Amount=seats asked
//
// Phase is the phase the game was in when the event happened, so an EvPass
// tells you which window was passed. Secret is private to Seat; Redact
// removes it for everyone else.
type Event struct {
	Kind   EventKind   `json:"k"`
	Turn   int         `json:"turn"`
	Phase  Phase       `json:"phase"`
	Seat   int         `json:"seat"`
	Other  int         `json:"other"`
	Action Action      `json:"action,omitempty"`
	Claim  Character   `json:"claim,omitempty"`
	Card   Character   `json:"card,omitempty"`
	Slot   int         `json:"slot"`
	Amount int         `json:"amount,omitempty"`
	Reason Reason      `json:"reason,omitempty"`
	Secret []Character `json:"secret,omitempty"`
}

// Redact returns the event as the given seat may see it. Spectators use -1.
func (e Event) Redact(seat int) Event {
	if seat != e.Seat {
		e.Secret = nil
	}
	return e
}

// RedactAll redacts a batch of events for one seat.
func RedactAll(evs []Event, seat int) []Event {
	out := make([]Event, len(evs))
	for i, e := range evs {
		out[i] = e.Redact(seat)
	}
	return out
}

func (e Event) String() string {
	s := fmt.Sprintf("t%d %s seat=%d", e.Turn, e.Kind, e.Seat)
	if e.Other >= 0 {
		s += fmt.Sprintf(" other=%d", e.Other)
	}
	if e.Action != NoAction {
		s += " action=" + e.Action.String()
	}
	if e.Claim != NoCharacter {
		s += " claim=" + e.Claim.String()
	}
	if e.Card != NoCharacter {
		s += " card=" + e.Card.String()
	}
	if e.Amount != 0 {
		s += fmt.Sprintf(" amount=%d", e.Amount)
	}
	if e.Reason != ReasonNone {
		s += " reason=" + e.Reason.String()
	}
	if len(e.Secret) > 0 {
		s += fmt.Sprintf(" secret=%v", e.Secret)
	}
	return s
}
