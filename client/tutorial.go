package client

import (
	"context"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/v1rushb/neoncoup/bot"
	"github.com/v1rushb/neoncoup/client/theme"
	"github.com/v1rushb/neoncoup/engine"
	"github.com/v1rushb/neoncoup/protocol"
	"github.com/v1rushb/neoncoup/transport"
)

// The tutorial is a rigged three-player game. A tiny local host plays two
// scripted opponents (VEX and MOTH) for the first eight turns, then hands
// them to an easy bot for free play, while the tutor overlays coaching.

var tutorialSeats = []string{"YOU", "VEX", "MOTH"}

// tutorialRig: you hold Duke + Captain; VEX holds Assassin + Contessa; MOTH
// holds Ambassador + Duke.
func tutorialRig() *engine.Rig {
	hands := [][]engine.Character{
		{engine.Duke, engine.Captain},
		{engine.Assassin, engine.Contessa},
		{engine.Ambassador, engine.Duke},
	}
	deck := []engine.Character{
		engine.Captain, engine.Contessa, engine.Assassin, engine.Ambassador, engine.Duke,
		engine.Captain, engine.Contessa, engine.Assassin, engine.Ambassador,
	}
	return &engine.Rig{Hands: hands, Deck: deck}
}

// scripted moves for the opponents, keyed by turn.
type scriptMove struct {
	turn  int
	phase engine.Phase
	seat  int
	in    engine.Intent
}

var tutorialScript = []scriptMove{
	{2, engine.PhaseTurnStart, 1, engine.ActionIntent(engine.Income, -1)},
	{3, engine.PhaseTurnStart, 2, engine.ActionIntent(engine.Tax, -1)},
	{5, engine.PhaseTurnStart, 1, engine.ActionIntent(engine.Tax, -1)}, // a bluff
	{5, engine.PhaseProveClaim, 1, engine.RevealIntent(1)},             // flips Contessa: caught
	{6, engine.PhaseTurnStart, 2, engine.ActionIntent(engine.ForeignAid, -1)},
	{8, engine.PhaseTurnStart, 2, engine.ActionIntent(engine.Assassinate, 0)}, // another bluff
	{8, engine.PhaseProveClaim, 2, engine.RevealIntent(0)},
}

const tutorialFreePlay = 9

// tutorialHost speaks the protocol over a pipe like a real host.
type tutorialHost struct {
	conn  transport.Conn
	game  *engine.Game
	bots  map[int]bot.Bot
	start time.Time
	steps []protocol.Step
	speed string
	// presentEnd paces the scripted opponents like a real host: they move
	// only once the player has seen everything in front of them.
	presentEnd time.Time
	paused     bool
}

func startTutorialHost(speed string) (transport.Conn, error) {
	g, err := engine.New(engine.Config{Players: 3, Seed: 2087, First: 0, Rig: tutorialRig()})
	if err != nil {
		return nil, err
	}
	a, b := transport.Pipe()
	h := &tutorialHost{conn: b, game: g, bots: map[int]bot.Bot{}, speed: protocol.ParseSpeed(speed)}
	for _, s := range []int{1, 2} {
		h.bots[s] = bot.New(bot.Easy, s, uint64(s), bot.Personality{Handle: tutorialSeats[s], Bluff: 0.5, Suspicion: 0.6, Aggression: 1})
	}
	go h.run()
	return a, nil
}

func (h *tutorialHost) send(typ string, v any) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = h.conn.Send(ctx, protocol.MustEncode(typ, v))
}

func (h *tutorialHost) seats() []protocol.Seat {
	out := make([]protocol.Seat, 3)
	for i, n := range tutorialSeats {
		out[i] = protocol.Seat{Seat: i, Name: n, Bot: i != 0}
		if i != 0 {
			out[i].Level = "tutorial"
		}
	}
	return out
}

func (h *tutorialHost) broadcast(evs []engine.Event) {
	now := time.Now()
	if h.presentEnd.Before(now) {
		h.presentEnd = now
	}
	h.presentEnd = h.presentEnd.Add(time.Duration(protocol.PresentMS(evs, h.speed)) * time.Millisecond)
	h.send(protocol.TEvents, protocol.Events{Events: engine.RedactAll(evs, 0)})
	for s, b := range h.bots {
		for _, e := range evs {
			b.Observe(e.Redact(s))
		}
	}
	h.send(protocol.TState, protocol.State{View: h.game.View(0), Seats: h.seats(), Speed: h.speed,
		OpensInMS: max(0, time.Until(h.presentEnd).Milliseconds())})
}

func (h *tutorialHost) run() {
	defer func() { _ = h.conn.Close() }()
	frames := make(chan protocol.Envelope, 64)
	go func() {
		defer close(frames)
		for {
			f, err := h.conn.Recv(context.Background())
			if err != nil {
				return
			}
			if env, err := protocol.Decode(f); err == nil {
				frames <- env
			}
		}
	}()
	started := false
	botTimer := time.NewTimer(time.Hour)
	defer botTimer.Stop()
	armBots := func() {
		aw := h.game.Awaiting()
		if !h.paused && (len(aw) > 0 && aw[0] != 0 || (len(aw) > 1)) {
			think := time.Duration(900*max(0.3, protocol.SpeedFactor(h.speed))) * time.Millisecond
			botTimer.Reset(max(0, time.Until(h.presentEnd)) + think)
		}
	}
	for {
		select {
		case env, ok := <-frames:
			if !ok {
				return
			}
			switch env.Type {
			case protocol.THello:
				h.send(protocol.TWelcome, protocol.Welcome{Proto: protocol.Version, You: "m1", Room: "TUTORIAL", Host: true})
			case protocol.TStart:
				if started {
					continue
				}
				started = true
				h.start = time.Now()
				st := protocol.DefaultSettings()
				st.Speed = h.speed
				h.send(protocol.TGameStart, protocol.GameStart{Game: 1, Seats: h.seats(), YouSeat: 0, Settings: st})
				h.broadcast(h.game.StartEvents())
				armBots()
			case protocol.TIntent:
				var m protocol.IntentMsg
				if env.Into(&m) != nil || m.Ver != h.game.Version() {
					continue
				}
				evs, err := h.game.Apply(0, m.Intent)
				if err != nil {
					h.send(protocol.TError, protocol.Error{Code: protocol.ErrIllegal, Msg: err.Error()})
					continue
				}
				h.steps = append(h.steps, protocol.Step{Seat: 0, Intent: m.Intent, AtMS: time.Since(h.start).Milliseconds()})
				h.after(evs)
				armBots()
			case protocol.TControl:
				var c protocol.Control
				if env.Into(&c) != nil {
					continue
				}
				if c.SkipMS > 0 {
					h.presentEnd = h.presentEnd.Add(-time.Duration(c.SkipMS) * time.Millisecond)
				}
				if c.Pause != nil {
					h.paused = *c.Pause
					if h.paused {
						botTimer.Stop()
					}
				}
				armBots()
			case protocol.TChat:
				var c protocol.Chat
				if env.Into(&c) == nil {
					h.send(protocol.TChat, protocol.Chat{Name: "YOU", Seat: 0, Text: protocol.CleanText(c.Text, protocol.MaxChatLen)})
				}
			}
		case <-botTimer.C:
			if h.game.Over() || h.paused {
				continue
			}
			for _, s := range h.game.Awaiting() {
				if s == 0 {
					continue
				}
				in := h.botMove(s)
				evs, err := h.game.Apply(s, in)
				if err != nil {
					continue
				}
				h.steps = append(h.steps, protocol.Step{Seat: s, Intent: in, AtMS: time.Since(h.start).Milliseconds()})
				h.after(evs)
				break
			}
			armBots()
		}
	}
}

func (h *tutorialHost) botMove(seat int) engine.Intent {
	g := h.game
	if g.Turn() < tutorialFreePlay {
		for _, m := range tutorialScript {
			if m.turn == g.Turn() && m.phase == g.Phase() && m.seat == seat {
				return m.in
			}
		}
		// Everything else in the scripted turns: stay quiet.
		if in, ok := g.DefaultIntent(seat); ok {
			return in
		}
	}
	return h.bots[seat].Decide(g.View(seat))
}

func (h *tutorialHost) after(evs []engine.Event) {
	h.broadcast(evs)
	if h.game.Over() {
		cfg := h.game.Config()
		h.send(protocol.TGameOver, protocol.GameOver{
			Winner: h.game.Winner(), Names: tutorialSeats,
			Replay: protocol.Replay{Format: protocol.ReplayFormat, Created: h.start.UTC(), Room: "TUTORIAL", Config: cfg, Seats: h.seats(), Steps: h.steps, Winner: h.game.Winner()},
		})
	}
}

func (a *App) startTutorial() tea.Cmd {
	a.connText = "loading the training sim..."
	a.goTo(scrConnecting)
	return func() tea.Msg {
		conn, err := startTutorialHost(a.cfg.Speed)
		if err != nil {
			return connectedMsg{err: err}
		}
		return connectedMsg{conn: &Connection{Conn: conn, Room: "TUTORIAL", Via: "training sim"}, mode: "tutorial"}
	}
}

// --- the coach -------------------------------------------------------------

type tutor struct {
	intro bool
	title string
	text  string
}

func newTutor() *tutor { return &tutor{intro: true} }

// lesson says what to do when the game is waiting on you; want restricts
// your input during the scripted part.
type lesson struct {
	title, text string
	want        func(engine.Intent) bool
}

func isAction(a engine.Action, target int) func(engine.Intent) bool {
	return func(in engine.Intent) bool {
		return in.Kind == engine.IntentAction && in.Action == a && (target < 0 || in.Target == target)
	}
}

func isKind(k engine.IntentKind) func(engine.Intent) bool {
	return func(in engine.Intent) bool { return in.Kind == k }
}

func (t *tutor) lesson(v engine.PlayerView) (lesson, bool) {
	if v.Turn >= tutorialFreePlay {
		return lesson{}, false
	}
	switch {
	case v.Turn == 1 && v.Phase == engine.PhaseTurnStart:
		return lesson{"YOUR FIRST MOVE", "You hold a DUKE. The Duke's action is TAX: take 3 coins.\nPress 4 for TAX.", isAction(engine.Tax, -1)}, true
	case v.Turn == 3 && v.Phase == engine.PhaseChallengeWindow:
		return lesson{"A CLAIM ON THE TABLE", "MOTH claims DUKE for TAX too. Anyone can CHALLENGE a claim:\nif MOTH can't show a Duke, MOTH loses a card; if MOTH can, YOU lose one.\nYou hold a Duke yourself and there are only 3. Risky. Press p to let it go.", isKind(engine.IntentPass)}, true
	case v.Turn == 4 && v.Phase == engine.PhaseTurnStart:
		return lesson{"STEAL", "Your other card is a CAPTAIN: STEAL takes 2 coins from a runner.\nPress 6, then 1 to pick VEX.", isAction(engine.Steal, 1)}, true
	case v.Turn == 5 && v.Phase == engine.PhaseChallengeWindow:
		return lesson{"SMELL THE BLUFF", "VEX claims DUKE. MOTH just showed confidence in a Duke and you hold one.\nVEX never acted like a Duke before. Call it: press c to CHALLENGE.", isKind(engine.IntentChallenge)}, true
	case v.Turn == 6 && v.Phase == engine.PhaseBlockWindow:
		return lesson{"BLOCK", "MOTH wants FOREIGN AID (+2). Any runner claiming DUKE can block it.\nYou really hold a Duke. Press b to BLOCK.", isKind(engine.IntentBlock)}, true
	case v.Turn == 6 && v.Phase == engine.PhaseBlockChallengeWindow:
		return lesson{"", "", isKind(engine.IntentPass)}, true
	case v.Turn == 7 && v.Phase == engine.PhaseTurnStart:
		return lesson{"COUP", "Seven coins buys a COUP: the target loses a card, no blocks, no challenges.\nVEX is down to one card. Press 3, then 1 for VEX.", isAction(engine.Coup, 1)}, true
	case v.Turn == 8 && v.Phase == engine.PhaseChallengeWindow:
		return lesson{"ASSASSINATION", "MOTH sends an ASSASSIN at you (3 coins). You could block with a CONTESSA,\neven a bluffed one. But MOTH has shown Duke energy all game. Press c to CHALLENGE.", isKind(engine.IntentChallenge)}, true
	}
	return lesson{}, false
}

func (t *tutor) onState(a *App, g *gameScreen) {
	if l, ok := t.lesson(g.v); ok && g.awaitingMe() && l.title != "" {
		t.title, t.text = l.title, l.text
	}
	if g.v.Turn >= tutorialFreePlay && t.title != "FREE PLAY" {
		t.title = "FREE PLAY"
		t.text = "That's the con. MOTH is on one card. From here it's a real game.\nWatch the timer, read the claims, and finish the run. (? for help)"
	}
}

func (t *tutor) onEvent(a *App, g *gameScreen, e engine.Event) {
	switch {
	case e.Kind == engine.EvActionDeclared && e.Seat == 1 && e.Action == engine.Income && e.Turn == 2:
		t.title, t.text = "INCOME", "VEX takes INCOME: +1 coin. Income can't be challenged or blocked."
	case e.Kind == engine.EvClaimFailed && e.Seat == 1:
		t.title, t.text = "CAUGHT", "VEX couldn't show a Duke and flipped a CONTESSA instead.\nA caught bluff costs a card, and the action fails."
	case e.Kind == engine.EvActionBlocked && e.Other == 0:
		t.title, t.text = "BLOCKED", "Your Duke block held. MOTH gets nothing."
	case e.Kind == engine.EvEliminated && e.Seat == 1:
		t.title, t.text = "FLATLINED", "VEX lost both cards and is out. Coins of eliminated runners go back to the treasury."
	case e.Kind == engine.EvClaimFailed && e.Seat == 2:
		t.title, t.text = "CAUGHT AGAIN", "MOTH had no Assassin. The 3 coins come back to MOTH because the claim failed,\nbut MOTH loses a card. (A Contessa block would have cost MOTH the coins.)"
	}
}

// allow enforces the scripted move; it flashes a hint instead of sending.
func (t *tutor) allow(a *App, g *gameScreen, in engine.Intent) bool {
	l, ok := t.lesson(g.v)
	if !ok || l.want == nil || l.want(in) {
		return true
	}
	lines := strings.Split(l.text, "\n")
	a.flash("follow the coach for this one: " + lines[len(lines)-1])
	return false
}

func (t *tutor) key(a *App, g *gameScreen, k string) bool {
	if t.intro {
		if k == "enter" || k == "space" {
			t.intro = false
		}
		return true
	}
	return false
}

func (t *tutor) tick(a *App, g *gameScreen) tea.Cmd { return nil }

func (t *tutor) animating(a *App) bool { return false }

func (t *tutor) overlay(a *App, g *gameScreen, body string) string {
	th := a.t
	if t.intro {
		text := strings.Join([]string{
			th.Title.Render("WELCOME TO THE CON"),
			"",
			th.Text.Render("Every runner holds 2 face-down cards: their INFLUENCE."),
			th.Text.Render("Lose both and you're out. Last runner standing takes the city."),
			"",
			th.Text.Render("On your turn you take one action. Some actions belong to a character:"),
			th.Text.Render("you may claim ANY character, whether you hold it or not."),
			th.Text.Render("Others can CHALLENGE the claim, or BLOCK it with a character of their own."),
			"",
			th.Muted.Render("Your cards are at the bottom. The log is on the right."),
			"",
			th.Accent.Render("press enter to start"),
		}, "\n")
		box := lipgloss.NewStyle().Border(th.ThickBorder).BorderForeground(theme.Acid).Background(theme.Panel).Padding(1, 3).Render(th.A(text))
		return overlay(body, box, a.w, a.h)
	}
	if t.title == "" {
		return body
	}
	box := lipgloss.NewStyle().Border(th.RoundBorder).BorderForeground(theme.Acid).Background(theme.Panel).Padding(0, 2).Width(min(78, a.w-4)).
		Render(th.Warn.Render("COACH // "+t.title) + "\n" + th.Text.Render(th.A(t.text)))
	return overlayAt(body, box, 2, 1+oppH)
}
