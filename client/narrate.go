package client

import (
	"fmt"
	"strings"

	"github.com/v1rushb/neoncoup/engine"
)

// logKind colors a log line.
type logKind uint8

const (
	logInfo logKind = iota
	logGain
	logLoss
	logAlert
	logPrivate
	logTurn
	logChat
	logSystem
)

type logLine struct {
	text string
	kind logKind
	turn int
}

// namer resolves seat numbers to display names.
type namer func(seat int) string

func tag(n namer, seat int) string { return "[" + n(seat) + "]" }

var signal = map[engine.Character]string{
	engine.Duke:       "DUKE SIGNAL",
	engine.Assassin:   "ASSASSIN PROTOCOL",
	engine.Captain:    "CAPTAIN BADGE",
	engine.Ambassador: "AMBASSADOR CHANNEL",
	engine.Contessa:   "CONTESSA SHIELD",
	engine.Magistrate: "MAGISTRATE WRIT",
}

// narrate turns an event into a cyberpunk log line. ok is false for events
// that are too noisy to log.
func narrate(e engine.Event, n namer, me int, coin string) (logLine, bool) {
	l := logLine{turn: e.Turn}
	who := tag(n, e.Seat)
	other := ""
	if e.Other >= 0 {
		other = tag(n, e.Other)
	}
	switch e.Kind {
	case engine.EvTurnStart:
		l.text = fmt.Sprintf("── TURN %d · %s ──", e.Turn, n(e.Seat))
		l.kind = logTurn
	case engine.EvActionDeclared:
		switch e.Action {
		case engine.Income:
			l.text = fmt.Sprintf("> %s SKIMS A CREDIT // INCOME +1%s", who, coin)
		case engine.ForeignAid:
			l.text = fmt.Sprintf("> %s REQUESTS FOREIGN AID // +2%s?", who, coin)
		case engine.Coup:
			l.text = fmt.Sprintf("> %s LAUNCHES A COUP ON %s // -7%s", who, other, coin)
			l.kind = logAlert
		case engine.Tax:
			l.text = fmt.Sprintf("> %s BROADCASTS %s // TAX +3%s", who, signal[e.Claim], coin)
		case engine.Assassinate:
			l.text = fmt.Sprintf("> %s RUNS %s ON %s // -3%s", who, signal[e.Claim], other, coin)
			l.kind = logAlert
		case engine.Steal:
			l.text = fmt.Sprintf("> %s FLASHES A %s AT %s // STEAL 2%s", who, signal[e.Claim], other, coin)
		case engine.Exchange:
			l.text = fmt.Sprintf("> %s OPENS AN %s // EXCHANGE", who, signal[e.Claim])
		case engine.Audit:
			l.text = fmt.Sprintf("> %s SERVES A %s ON %s // AUDIT -2%s", who, signal[e.Claim], other, coin)
		}
	case engine.EvChallenge:
		l.text = fmt.Sprintf("! %s CALLS %s'S %s A LIE", who, other, e.Claim.Title())
		l.kind = logAlert
	case engine.EvClaimProven:
		l.text = fmt.Sprintf("  %s SHOWS %s // CLAIM VERIFIED", who, e.Card.Title())
		l.kind = logGain
	case engine.EvClaimFailed:
		l.text = fmt.Sprintf("  %s WAS BLUFFING // FLIPS %s", who, e.Card.Title())
		l.kind = logLoss
	case engine.EvCardReplaced:
		if e.Seat == me && len(e.Secret) == 1 {
			l.text = fmt.Sprintf("  (you shuffle it back and draw %s)", e.Secret[0].Title())
			l.kind = logPrivate
		} else {
			l.text = fmt.Sprintf("  %s RESHUFFLES AND DRAWS A FRESH CARD", who)
		}
	case engine.EvBlock:
		l.text = fmt.Sprintf("> %s RAISES A %s // BLOCKS %s", who, signal[e.Claim], strings.ReplaceAll(e.Action.Title(), "_", " "))
		l.kind = logAlert
	case engine.EvCoins:
		switch e.Reason {
		case engine.ReasonSteal:
			if e.Amount > 0 {
				l.text = fmt.Sprintf("  %s LIFTS %d%s FROM %s", who, e.Amount, coin, other)
				l.kind = logGain
			} else {
				return l, false
			}
		case engine.ReasonRefund:
			l.text = fmt.Sprintf("  %s GETS %d%s BACK", who, e.Amount, coin)
		case engine.ReasonAudit:
			l.text = fmt.Sprintf("  %s PAYS %d%s TO THE TREASURY", who, -e.Amount, coin)
			l.kind = logLoss
		case engine.ReasonForeignAid, engine.ReasonTax:
			l.text = fmt.Sprintf("  +%d%s → %s", e.Amount, coin, who)
			l.kind = logGain
		default:
			return l, false
		}
	case engine.EvInfluenceLost:
		l.text = fmt.Sprintf("✖ %s LOSES INFLUENCE // %s EXPOSED", who, e.Card.Title())
		l.kind = logLoss
	case engine.EvEliminated:
		l.text = fmt.Sprintf("✖✖ %s FLATLINED // OUT OF THE GAME", who)
		if e.Amount > 0 {
			l.text += fmt.Sprintf(" (%d%s to treasury)", e.Amount, coin)
		}
		l.kind = logLoss
	case engine.EvActionBlocked:
		l.text = fmt.Sprintf("  BLOCK HOLDS // %s'S %s FIZZLES", n(e.Seat), e.Action.Title())
	case engine.EvBlockFailed:
		l.text = fmt.Sprintf("  BLOCK BROKEN // %s GOES THROUGH", e.Action.Title())
	case engine.EvActionFailed:
		switch e.Reason {
		case engine.ReasonChallenged:
			l.text = fmt.Sprintf("  %s'S %s IS VOIDED", n(e.Seat), e.Action.Title())
		case engine.ReasonNoTarget:
			l.text = fmt.Sprintf("  %s HAS NO TARGET LEFT", e.Action.Title())
		default:
			return l, false
		}
	case engine.EvExchangeDraw:
		if e.Seat == me && len(e.Secret) > 0 {
			names := make([]string, len(e.Secret))
			for i, c := range e.Secret {
				names[i] = c.Title()
			}
			l.text = "  (you draw " + strings.Join(names, " + ") + ")"
			l.kind = logPrivate
		} else {
			l.text = fmt.Sprintf("  %s PULLS %d CARDS FROM THE COURT", who, e.Amount)
		}
	case engine.EvExchangeDone:
		l.text = fmt.Sprintf("  %s RETURNS %d CARDS", who, e.Amount)
	case engine.EvGameOver:
		l.text = fmt.Sprintf("»» %s SEIZES THE CITY", who)
		l.kind = logGain
	default:
		return l, false
	}
	return l, true
}

// pendingSummary describes the action on the table in one line.
func pendingSummary(v engine.PlayerView, n namer) string {
	p := v.Pending
	if p == nil {
		return ""
	}
	s := n(p.Actor) + " → " + p.Action.Title()
	if p.Target >= 0 {
		s += " " + n(p.Target)
	}
	if p.Claim != engine.NoCharacter {
		s += " (claims " + p.Claim.Title() + ")"
	}
	if p.Blocker >= 0 {
		s += " // BLOCKED by " + n(p.Blocker) + " (" + p.BlockClaim.Title() + ")"
	}
	return s
}
