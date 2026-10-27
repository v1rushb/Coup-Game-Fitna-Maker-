package client

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/v1rushb/neoncoup/audio"
	"github.com/v1rushb/neoncoup/config"
	"github.com/v1rushb/neoncoup/engine"
	"github.com/v1rushb/neoncoup/protocol"
)

type inputMode uint8

const (
	modeNormal inputMode = iota
	modeTarget
	modeBlock
	modeTaunt
)

// actionKeys maps number keys to actions.
var actionKeys = []engine.Action{
	engine.Income, engine.ForeignAid, engine.Coup, engine.Tax,
	engine.Assassinate, engine.Steal, engine.Exchange, engine.Audit,
}

// Effect durations.
const (
	flipDur    = 600 * time.Millisecond
	flashDur   = 1200 * time.Millisecond
	glitchDur  = 1300 * time.Millisecond
	joltDur    = 320 * time.Millisecond
	deathDur   = 1800 * time.Millisecond
	coinPerSec = 14.0
)

type gameScreen struct {
	gs         protocol.GameStart
	seats      []protocol.Seat
	me         int
	v          engine.PlayerView
	have       bool
	deadline   time.Time
	timerTotal time.Duration
	offline    map[int]bool

	log      []logLine
	scroll   int
	chatting bool
	input    lineInput

	mode    inputMode
	pending engine.Action
	exSel   []int
	sentVer uint64
	sent    bool

	shown    []float64
	coinFx   map[int]time.Time
	coinUp   map[int]bool
	flips    map[int]time.Time
	oppFx    map[int]time.Time
	glitch   map[int]time.Time
	jolt     time.Time
	myDeath  time.Time
	lastTick time.Time

	over       *protocol.GameOver
	overAt     time.Time
	stats      config.GameStats
	statsDone  bool
	replayPath string
	lastHand   []engine.Card
	lastClaim  engine.Character

	tutor *tutor

	// Focus (focus.go): your-turn alert.
	turnAlert time.Time
	alertTurn int
	// tense mirrors what the music was last told about the game's mood.
	tense bool

	// Extras (extras.go, prefs.go).
	chron   chronicle
	inspect int // seat being inspected, -1 for none
	hint    string
	hintVer uint64
	prefs   *prefsOverlay
	post    *postGame

	// Animation (sprites.go): where the tracker drew the deck and treasury,
	// and the card each seat last proved.
	trackDeck, trackBank point
	lastProven           map[int]engine.Character

	// Presentation (present.go).
	queue    []qItem
	beat     *beat
	speed    string
	paused   bool
	pausedAt time.Time
}

func newGameScreen(a *App, gs protocol.GameStart) *gameScreen {
	g := &gameScreen{
		gs: gs, seats: gs.Seats, me: gs.YouSeat,
		input:   newLineInput("say: ", protocol.MaxChatLen),
		offline: map[int]bool{},
		coinFx:  map[int]time.Time{}, coinUp: map[int]bool{},
		flips: map[int]time.Time{}, oppFx: map[int]time.Time{}, glitch: map[int]time.Time{},
		stats: config.GameStats{Mode: a.mode},
		speed: protocol.ParseSpeed(gs.Settings.Speed),

		trackDeck: point{-1, -1}, trackBank: point{-1, -1},
		lastProven: map[int]engine.Character{},
		chron:      newChronicle(),
		inspect:    -1,
	}
	g.log = append(g.log, logLine{text: fmt.Sprintf("// GAME %d // %d RUNNERS // JACKED IN", gs.Game, len(gs.Seats)), kind: logTurn})
	if gs.Settings.Magistrate {
		g.log = append(g.log, logLine{text: "// MAGISTRATE VARIANT ACTIVE", kind: logTurn})
	}
	if a.mode == "tutorial" {
		g.tutor = newTutor()
	}
	return g
}

func (g *gameScreen) name(seat int) string {
	if seat == g.me && seat >= 0 {
		return "YOU"
	}
	if seat >= 0 && seat < len(g.seats) {
		return g.seats[seat].Name
	}
	return "?"
}

// awaitingMe reports whether you have a decision to make now: the table is
// waiting on you and everything in front of it has finished playing.
func (g *gameScreen) awaitingMe() bool {
	return g.have && g.me >= 0 && !g.presenting() && slices.Contains(g.v.Awaiting, g.me) && len(g.v.Legal) > 0
}

func (g *gameScreen) legal(in engine.Intent) bool {
	return slices.ContainsFunc(g.v.Legal, in.Equal)
}

func handHas(hand []engine.Card, c engine.Character) bool {
	for _, h := range hand {
		if !h.Revealed && h.Char == c {
			return true
		}
	}
	return false
}

// --- incoming ------------------------------------------------------------------

// onFrame queues a frame for presentation.
func (g *gameScreen) onFrame(a *App, env protocol.Envelope) tea.Cmd {
	switch env.Type {
	case protocol.TState:
		var st protocol.State
		if env.Into(&st) != nil {
			return nil
		}
		if st.Speed != "" {
			g.speed = protocol.ParseSpeed(st.Speed)
		}
		g.enqueue(a, qItem{state: &st})
	case protocol.TEvents:
		var evs protocol.Events
		if env.Into(&evs) != nil {
			return nil
		}
		for i := range evs.Events {
			g.enqueue(a, qItem{ev: &evs.Events[i]})
		}
	case protocol.TGameOver:
		var over protocol.GameOver
		if env.Into(&over) != nil {
			return nil
		}
		g.enqueue(a, qItem{over: &over})
	case protocol.TChat, protocol.TEmote:
		g.enqueue(a, qItem{chat: &env})
	}
	g.pump(a)
	return nil
}

// onState applies a state; recv is when it arrived, which its timer counts from.
func (g *gameScreen) onState(a *App, st protocol.State, recv time.Time) {
	first := !g.have
	g.v = st.View
	g.have = true
	if st.DeadlineMS > 0 {
		g.deadline = recv.Add(time.Duration(st.DeadlineMS) * time.Millisecond)
		g.timerTotal = time.Duration(st.TimerMS) * time.Millisecond
	} else {
		g.deadline = time.Time{}
	}
	g.offline = map[int]bool{}
	for _, s := range st.Offline {
		g.offline[s] = true
	}
	if len(st.Seats) == len(g.seats) {
		g.seats = st.Seats
	}
	if len(g.shown) != len(g.v.Players) || first {
		g.shown = make([]float64, len(g.v.Players))
		for i, p := range g.v.Players {
			g.shown[i] = float64(p.Coins)
		}
	}
	if g.v.Version != g.sentVer {
		g.sent = false
	}
	if !g.awaitingMe() && g.mode != modeTaunt {
		g.mode = modeNormal
	}
	if g.v.Phase != engine.PhaseExchange {
		g.exSel = nil
	}
	g.lastHand = g.v.Hand
	v := g.v
	if g.me >= 0 && v.Phase == engine.PhaseTurnStart && v.Current == g.me && slices.Contains(v.Awaiting, g.me) &&
		len(v.Legal) > 0 && g.alertTurn != v.Turn {
		g.alertTurn, g.turnAlert = v.Turn, a.now
		a.sound.SFX(audio.SFXYourTurn)
	}
	if t := tenseTable(v); t != g.tense {
		g.tense = t
		a.sound.SetIntensity(t)
	}
	if g.tutor != nil {
		g.tutor.onState(a, g)
	}
}

func (g *gameScreen) addLog(l logLine) {
	g.log = append(g.log, l)
	if len(g.log) > 500 {
		g.log = g.log[len(g.log)-500:]
	}
	if g.scroll > 0 {
		g.scroll++ // keep the reader's place while scrolled back
	}
}

// track updates this game's personal stats.
func (g *gameScreen) track(e engine.Event) {
	if g.me < 0 {
		return
	}
	s := &g.stats
	switch e.Kind {
	case engine.EvActionDeclared:
		if e.Seat == g.me {
			if e.Claim != engine.NoCharacter && !handHas(g.lastHand, e.Claim) {
				s.Bluffs++
			}
			if e.Action == engine.Coup {
				s.Coups++
			}
		}
	case engine.EvBlock:
		if e.Seat == g.me {
			s.Blocks++
			if !handHas(g.lastHand, e.Claim) {
				s.Bluffs++
			}
		}
	case engine.EvChallenge:
		g.lastClaim = e.Claim
		if e.Seat == g.me {
			s.Challenges++
		}
	case engine.EvClaimFailed:
		if e.Other == g.me {
			s.ChallengesWon++
		}
		if e.Seat == g.me && !handHas(g.lastHand, g.lastClaim) {
			s.BluffsCaught++
		}
	case engine.EvCoins:
		if e.Seat == g.me && e.Amount > 0 {
			switch e.Reason {
			case engine.ReasonIncome, engine.ReasonForeignAid, engine.ReasonTax, engine.ReasonSteal:
				s.CoinsEarned += e.Amount
			}
		}
	case engine.EvEliminated:
		if e.Seat == g.me {
			s.Eliminated = true
		}
	}
}

func (g *gameScreen) effects(a *App, e engine.Event) {
	now := a.now
	switch e.Kind {
	case engine.EvCoins:
		if e.Amount != 0 && e.Reason != engine.ReasonEliminated {
			g.coinFx[e.Seat] = now
			g.coinUp[e.Seat] = e.Amount > 0
			if e.Amount > 0 {
				a.sound.SFX(audio.SFXCoin)
			}
		}
	case engine.EvChallenge:
		g.jolt = now
		a.sound.SFX(audio.SFXChallenge)
	case engine.EvBlock:
		a.sound.SFX(audio.SFXBlock)
	case engine.EvInfluenceLost:
		if e.Seat == g.me {
			g.flips[e.Slot] = now
		} else {
			g.oppFx[e.Seat] = now
		}
		a.sound.SFX(audio.SFXFlip)
	case engine.EvClaimProven:
		if e.Seat == g.me {
			g.flips[e.Slot] = now
		} else {
			g.oppFx[e.Seat] = now
		}
		a.sound.SFX(audio.SFXFlip)
	case engine.EvEliminated:
		g.glitch[e.Seat] = now
		a.sound.SFX(audio.SFXGlitch)
		if e.Seat == g.me {
			g.myDeath = now
		}
	case engine.EvWindowOpen:
		// The tension sting plays when the claim on the table is aimed at you.
		aimed := e.Other == g.me && g.me >= 0
		myBlocked := e.Phase == engine.PhaseBlockChallengeWindow && e.Seat == g.me
		if (aimed && e.Phase == engine.PhaseChallengeWindow) || myBlocked || (e.Action == engine.Assassinate && e.Phase == engine.PhaseChallengeWindow) {
			a.sound.Stinger()
		}
	}
}

func (g *gameScreen) onGameOver(a *App, over protocol.GameOver) {
	g.over = &over
	g.inspect, g.prefs = -1, nil
	if pg, err := analyze(over.Replay, g.name); err == nil {
		g.post = pg
	}
	g.tense = false
	a.sound.SetIntensity(false)
	g.overAt = a.now
	g.mode = modeNormal
	won := g.me >= 0 && over.Winner == g.me
	if won {
		a.sound.Music(audio.SongVictory)
	} else {
		a.sound.Music(audio.SongDefeat)
	}
	if a.opts.Store == nil || g.tutor != nil {
		return
	}
	if p, err := a.opts.Store.SaveReplay(over.Replay); err == nil {
		g.replayPath = p
	}
	if g.me >= 0 && !g.statsDone {
		g.statsDone = true
		g.stats.Won = won
		g.stats.Turns = g.v.Turn
		_, _ = a.opts.Store.AddGame(g.stats)
	}
}

func (g *gameScreen) onChat(a *App, env protocol.Envelope) {
	c := decodeChat(env)
	text := c.text
	switch {
	case c.system:
		g.addLog(logLine{text: "// " + text, kind: logSystem})
	case env.Type == protocol.TEmote:
		g.addLog(logLine{text: fmt.Sprintf("» [%s] %s", c.name, strings.ToUpper(text)), kind: logChat})
	default:
		g.addLog(logLine{text: fmt.Sprintf("[%s] %s", c.name, text), kind: logChat})
	}
}

// --- animation -----------------------------------------------------------------

func (g *gameScreen) tick(a *App) tea.Cmd {
	g.pump(a)
	dt := a.now.Sub(g.lastTick).Seconds()
	if g.lastTick.IsZero() || dt > 0.5 {
		dt = 1.0 / fps
	}
	g.lastTick = a.now
	for i := range g.shown {
		if i >= len(g.v.Players) {
			break
		}
		target := float64(g.v.Players[i].Coins)
		d := target - g.shown[i]
		step := coinPerSec * dt
		switch {
		case d > step:
			g.shown[i] += step
		case d < -step:
			g.shown[i] -= step
		default:
			g.shown[i] = target
		}
	}
	if g.tutor != nil {
		return g.tutor.tick(a, g)
	}
	return nil
}

func (g *gameScreen) animating(a *App) bool {
	now := a.now
	if g.smooth(a) {
		return true
	}
	if g.over != nil {
		return now.Sub(g.overAt) < 4*time.Second
	}
	if !g.deadline.IsZero() && now.Before(g.deadline) {
		return true
	}
	if len(g.v.Awaiting) > 0 && !g.awaitingMe() {
		return true // spinners on whoever we're waiting for
	}
	return g.tutor != nil && g.tutor.animating(a)
}

// smooth reports whether something on the table moves and needs the full
// frame rate: a beat, ticking coins, flashes and flips. Countdowns and
// spinners alone run at half rate.
func (g *gameScreen) smooth(a *App) bool {
	now := a.now
	if g.presenting() && !g.paused {
		return true
	}
	for i := range g.shown {
		if i < len(g.v.Players) && g.shown[i] != float64(g.v.Players[i].Coins) {
			return true
		}
	}
	recent := func(t time.Time, d time.Duration) bool { return !t.IsZero() && now.Sub(t) < d }
	if recent(g.jolt, joltDur) || recent(g.myDeath, deathDur) || g.alerting(a) {
		return true
	}
	if g.over != nil && now.Sub(g.overAt) < time.Second {
		return true // the results title glitches in
	}
	for _, m := range []map[int]time.Time{g.flips, g.oppFx, g.glitch, g.coinFx} {
		for _, t := range m {
			if recent(t, flashDur) || recent(t, glitchDur) {
				return true
			}
		}
	}
	return false
}

// --- input ---------------------------------------------------------------------

func (g *gameScreen) send(a *App, in engine.Intent) {
	if a.sess == nil {
		return
	}
	if !g.legal(in) {
		a.flash("not allowed right now")
		a.sound.SFX(audio.SFXError)
		return
	}
	if g.tutor != nil && !g.tutor.allow(a, g, in) {
		return
	}
	a.sess.Send(protocol.TIntent, protocol.IntentMsg{Ver: g.v.Version, Intent: in})
	g.sent = true
	g.sentVer = g.v.Version
	g.mode = modeNormal
	g.exSel = nil
	a.sound.SFX(audio.SFXSelect)
}

func (g *gameScreen) key(a *App, msg tea.KeyPressMsg) tea.Cmd {
	k := msg.String()
	if g.over != nil {
		return g.overKey(a, k)
	}
	if g.chatting {
		switch g.input.handle(msg) {
		case "enter":
			if t := strings.TrimSpace(g.input.Value()); t != "" && a.sess != nil {
				a.sess.Send(protocol.TChat, protocol.Chat{Text: t})
			}
			g.input.Set("")
			g.chatting = false
		case "esc":
			g.chatting = false
		}
		return nil
	}
	if g.prefs != nil {
		g.prefs.key(a, g, msg)
		return nil
	}
	if g.inspect >= 0 {
		switch k {
		case "tab":
			g.nextInspect()
		case "esc", "q":
			g.inspect = -1
		}
		return nil
	}
	if g.tutor != nil && g.tutor.key(a, g, k) {
		return nil
	}
	switch k {
	case "tab":
		g.nextInspect()
		return nil
	case "n", "N":
		if !a.sound.Available() {
			a.flash("no audio device: sound is off")
		} else {
			a.sound.NextTrack()
		}
		return nil
	case "space", " ":
		if g.soloControls(a) {
			g.skipBeat(a)
		} else if g.presenting() {
			a.flash("skipping works in solo games")
		}
		return nil
	case "P":
		if g.soloControls(a) {
			g.setPaused(a, !g.paused)
			a.flash(map[bool]string{true: "paused // P to resume", false: "resumed"}[g.paused])
		} else {
			a.flash("pause works in solo games")
		}
		return nil
	}
	if g.paused {
		switch k {
		case "q", "esc", "enter", "t":
		default:
			return nil
		}
	}
	if g.mode == modeTaunt {
		if n, err := strconv.Atoi(k); err == nil && n >= 1 && n <= len(protocol.Emotes) && a.sess != nil {
			a.sess.Send(protocol.TEmote, protocol.Emote{ID: n - 1})
		}
		g.mode = modeNormal
		return nil
	}
	switch k {
	case "enter":
		if g.v.Phase == engine.PhaseExchange && g.awaitingMe() {
			g.confirmExchange(a)
			return nil
		}
		g.chatting = true
		return nil
	case "t":
		g.mode = modeTaunt
		return nil
	case "esc":
		if g.mode != modeNormal || len(g.exSel) > 0 {
			g.mode = modeNormal
			g.exSel = nil
			return nil
		}
		g.openPrefs(a)
		return nil
	case "q":
		a.quitConfirm = true
		return nil
	case "pgup", "ctrl+u", "[":
		g.scroll = min(g.scroll+5, max(0, len(g.log)-3))
		return nil
	case "pgdown", "ctrl+d", "]":
		g.scroll = max(0, g.scroll-5)
		return nil
	case "end":
		g.scroll = 0
		return nil
	}
	if !g.have {
		return nil
	}
	if !g.awaitingMe() {
		if strings.ContainsAny(k, "12345678cbp") && len(k) == 1 {
			if g.me < 0 {
				a.flash("you are spectating")
			} else if g.have && !g.v.Players[g.me].Alive {
				a.flash("you're flatlined; watching the rest")
			} else {
				a.flash("not your move")
			}
		}
		return nil
	}
	switch k {
	case "c":
		g.send(a, engine.ChallengeIntent())
		return nil
	case "p":
		g.send(a, engine.PassIntent())
		return nil
	case "b":
		var opts []engine.Intent
		for _, in := range g.v.Legal {
			if in.Kind == engine.IntentBlock {
				opts = append(opts, in)
			}
		}
		switch len(opts) {
		case 0:
			a.flash("nothing to block here")
		case 1:
			g.send(a, opts[0])
		default:
			g.mode = modeBlock
		}
		return nil
	}
	n, err := strconv.Atoi(k)
	if err != nil || n < 1 {
		return nil
	}
	g.number(a, n)
	return nil
}

func (g *gameScreen) number(a *App, n int) {
	switch g.mode {
	case modeTarget:
		targets := g.targets(g.pending)
		if n <= len(targets) {
			g.send(a, engine.ActionIntent(g.pending, targets[n-1]))
		}
		return
	case modeBlock:
		blocks := g.v.Pending.Action.Blockers(g.v.Magistrate)
		if n <= len(blocks) {
			g.send(a, engine.BlockIntent(blocks[n-1]))
		}
		return
	}
	switch g.v.Phase {
	case engine.PhaseTurnStart:
		if n > len(actionKeys) {
			return
		}
		act := actionKeys[n-1]
		if act == engine.Audit && !g.v.Magistrate {
			return
		}
		if !act.Targeted() {
			g.send(a, engine.ActionIntent(act, -1))
			return
		}
		targets := g.targets(act)
		switch len(targets) {
		case 0:
			if g.v.Players[g.me].Coins < act.Cost() {
				a.flash(fmt.Sprintf("%s needs %d%s", act.Title(), act.Cost(), a.t.Coin()))
			} else if g.v.Players[g.me].Coins >= engine.MustCoupAt {
				a.flash("10+ coins: you must COUP (3)")
			} else {
				a.flash("no valid target")
			}
			a.sound.SFX(audio.SFXError)
		case 1:
			g.send(a, engine.ActionIntent(act, targets[0]))
		default:
			g.mode = modeTarget
			g.pending = act
		}
	case engine.PhaseProveClaim:
		g.send(a, engine.RevealIntent(n-1))
	case engine.PhaseLoseInfluence:
		g.send(a, engine.LoseIntent(n-1))
	case engine.PhaseExchange:
		idx := n - 1
		if idx >= len(g.v.ExchangeOptions) {
			return
		}
		if i := slices.Index(g.exSel, idx); i >= 0 {
			g.exSel = slices.Delete(g.exSel, i, i+1)
		} else if len(g.exSel) < g.v.Players[g.me].Hidden {
			g.exSel = append(g.exSel, idx)
		}
		a.sound.SFX(audio.SFXSelect)
		if len(g.exSel) == g.v.Players[g.me].Hidden && g.v.Players[g.me].Hidden == 1 {
			g.confirmExchange(a)
		}
	}
}

func (g *gameScreen) confirmExchange(a *App) {
	need := g.v.Players[g.me].Hidden
	if len(g.exSel) != need {
		a.flash(fmt.Sprintf("pick %d card(s) to keep", need))
		return
	}
	keep := append([]int(nil), g.exSel...)
	slices.Sort(keep)
	g.send(a, engine.ExchangeIntent(keep...))
}

// targets lists legal targets for an action, in seat order.
func (g *gameScreen) targets(act engine.Action) []int {
	var out []int
	for _, in := range g.v.Legal {
		if in.Kind == engine.IntentAction && in.Action == act && in.Target >= 0 {
			out = append(out, in.Target)
		}
	}
	return out
}

func (g *gameScreen) overKey(a *App, k string) tea.Cmd {
	switch k {
	case "r":
		if a.sess != nil && (a.welcome.Host || a.mode == "solo") && a.mode != "tutorial" {
			a.sess.Send(protocol.TStart, nil)
			a.flash("rematch requested")
		} else {
			a.flash("only the host can start a rematch")
		}
	case "v":
		if g.over != nil {
			return a.viewReplay(g.over.Replay)
		}
	case "enter", "l":
		if a.mode == "solo" || a.mode == "tutorial" || a.sess == nil {
			a.leaveRoom()
		} else {
			a.game = nil
			a.goTo(scrLobby)
		}
	case "q", "esc":
		a.leaveRoom()
	}
	return nil
}

// tenseTable reports whether the game is on a knife's edge: someone is down
// to one influence or sitting on a coup (7+ coins), or it's down to two.
func tenseTable(v engine.PlayerView) bool {
	alive := 0
	tense := false
	for _, p := range v.Players {
		if !p.Alive {
			continue
		}
		alive++
		if p.Hidden == 1 || p.Coins >= engine.CoupCost {
			tense = true
		}
	}
	return tense || (alive == 2 && len(v.Players) > 0)
}
