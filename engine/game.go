package engine

import (
	"errors"
	"fmt"
	"math/rand/v2"
)

// Config sets up a game.
type Config struct {
	Players    int    `json:"players"`
	Magistrate bool   `json:"magistrate,omitempty"`
	Seed       uint64 `json:"seed"`
	// First is the starting seat, or -1 to pick one with the seeded RNG.
	First int `json:"first"`
	// Rig fixes hands, deck order and coins. Used by tests and the tutorial.
	Rig *Rig `json:"rig,omitempty"`
}

// Rig describes an exact starting position. The cards in Hands and Deck
// together must match the court deck composition for the variant, and Coins
// must leave a non-negative Treasury.
type Rig struct {
	Hands [][]Character `json:"hands"`
	// Deck lists the court deck from the top (next draw) down.
	Deck  []Character `json:"deck"`
	Coins []int       `json:"coins,omitempty"`
}

// ErrIllegal wraps every rejected intent.
var ErrIllegal = errors.New("illegal intent")

func illegal(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrIllegal, fmt.Sprintf(format, args...))
}

type player struct {
	coins int
	cards []Card
	alive bool
}

func (p *player) hidden() int {
	n := 0
	for _, c := range p.cards {
		if !c.Revealed {
			n++
		}
	}
	return n
}

func (p *player) hiddenSlots() []int {
	var s []int
	for i, c := range p.cards {
		if !c.Revealed {
			s = append(s, i)
		}
	}
	return s
}

// turnState tracks the action being resolved.
type turnState struct {
	actor            int
	action           Action
	target           int
	claim            Character
	paid             int
	actionChallenged bool
	blocker          int
	blockClaim       Character
	blockChallenged  bool
}

type challengeState struct {
	challenger int
	claimant   int
	claim      Character
	onBlock    bool
}

// continuation says what happens once a pending influence loss is settled.
type continuation uint8

const (
	contEndTurn continuation = iota
	contAfterActionProven
	contBlockStands
)

// Game is the authoritative game state. Only the host holds one; players and
// bots see a PlayerView.
type Game struct {
	cfg      Config
	rng      *rand.Rand
	players  []player
	deck     []Character // top of deck is the last element
	treasury int
	phase    Phase
	turn     int
	current  int
	version  uint64
	winner   int

	act      *turnState
	chal     *challengeState
	awaiting []bool
	loser    int
	lossWhy  Reason
	after    continuation
	drawn    []Character

	events []Event
}

// New creates a game and deals it.
func New(cfg Config) (*Game, error) {
	if cfg.Players < MinPlayers || cfg.Players > MaxPlayers {
		return nil, fmt.Errorf("engine: %d players; need %d-%d", cfg.Players, MinPlayers, MaxPlayers)
	}
	g := &Game{
		cfg:      cfg,
		rng:      rand.New(rand.NewPCG(cfg.Seed, cfg.Seed^0x9e3779b97f4a7c15)),
		players:  make([]player, cfg.Players),
		awaiting: make([]bool, cfg.Players),
		winner:   -1,
		loser:    -1,
	}
	if cfg.Rig != nil {
		if err := g.applyRig(cfg.Rig); err != nil {
			return nil, err
		}
	} else {
		for _, c := range DeckComposition(cfg.Magistrate) {
			for range CopiesPerCharacter {
				g.deck = append(g.deck, c)
			}
		}
		g.shuffle()
		for i := range g.players {
			g.players[i] = player{coins: StartingCoins, alive: true, cards: make([]Card, HandSize)}
			for s := range HandSize {
				g.players[i].cards[s] = Card{Char: g.draw()}
			}
		}
	}

	first := cfg.First
	if first < 0 || first >= cfg.Players {
		first = g.rng.IntN(cfg.Players)
	}
	g.current = first
	// Official rule: in a two-player game the starting player gets only 1 coin.
	if cfg.Players == 2 && (cfg.Rig == nil || cfg.Rig.Coins == nil) {
		g.players[first].coins = StartingCoins - 1
	}
	g.treasury = TotalCoins
	for _, p := range g.players {
		g.treasury -= p.coins
	}
	g.turn = 1
	g.phase = PhaseTurnStart
	g.awaitOnly(first)
	return g, nil
}

func (g *Game) applyRig(r *Rig) error {
	if len(r.Hands) != g.cfg.Players {
		return fmt.Errorf("engine: rig has %d hands for %d players", len(r.Hands), g.cfg.Players)
	}
	want := map[Character]int{}
	for _, c := range DeckComposition(g.cfg.Magistrate) {
		want[c] = CopiesPerCharacter
	}
	got := map[Character]int{}
	for i, h := range r.Hands {
		if len(h) != HandSize {
			return fmt.Errorf("engine: rig hand %d has %d cards", i, len(h))
		}
		coins := StartingCoins
		if r.Coins != nil {
			if len(r.Coins) != g.cfg.Players {
				return fmt.Errorf("engine: rig coins has %d entries", len(r.Coins))
			}
			coins = r.Coins[i]
		}
		g.players[i] = player{coins: coins, alive: true, cards: make([]Card, HandSize)}
		for s, c := range h {
			g.players[i].cards[s] = Card{Char: c}
			got[c]++
		}
	}
	// The rig lists the deck top-first; internally the top is the last element.
	for i := len(r.Deck) - 1; i >= 0; i-- {
		g.deck = append(g.deck, r.Deck[i])
		got[r.Deck[i]]++
	}
	for c, n := range want {
		if got[c] != n {
			return fmt.Errorf("engine: rig has %d %s, want %d", got[c], c, n)
		}
	}
	for c := range got {
		if want[c] == 0 {
			return fmt.Errorf("engine: rig contains %s, not in this variant", c)
		}
	}
	sum := 0
	for _, p := range g.players {
		if p.coins < 0 {
			return errors.New("engine: rig has negative coins")
		}
		sum += p.coins
	}
	if sum > TotalCoins {
		return errors.New("engine: rig coins exceed the treasury")
	}
	return nil
}

// Config returns the configuration the game was created with.
func (g *Game) Config() Config { return g.cfg }

// Phase is the current phase. It is always a waiting phase or PhaseGameOver.
func (g *Game) Phase() Phase { return g.phase }

// Version increases with every applied intent.
func (g *Game) Version() uint64 { return g.version }

// Turn is the 1-based turn counter.
func (g *Game) Turn() int { return g.turn }

// Current is the seat whose turn it is.
func (g *Game) Current() int { return g.current }

// Winner is the winning seat, or -1 while the game runs.
func (g *Game) Winner() int { return g.winner }

// Over reports whether the game has finished.
func (g *Game) Over() bool { return g.phase == PhaseGameOver }

// NumPlayers is the seat count.
func (g *Game) NumPlayers() int { return len(g.players) }

// Alive reports whether a seat still has influence.
func (g *Game) Alive(seat int) bool {
	return seat >= 0 && seat < len(g.players) && g.players[seat].alive
}

// AliveCount is the number of seats still in the game.
func (g *Game) AliveCount() int {
	n := 0
	for _, p := range g.players {
		if p.alive {
			n++
		}
	}
	return n
}

// Awaiting lists the seats whose input the game is waiting for.
func (g *Game) Awaiting() []int {
	var s []int
	for i, w := range g.awaiting {
		if w {
			s = append(s, i)
		}
	}
	return s
}

// IsAwaiting reports whether the game waits on this seat.
func (g *Game) IsAwaiting(seat int) bool {
	return seat >= 0 && seat < len(g.awaiting) && g.awaiting[seat]
}

// Apply validates and applies one intent from a seat, returning the
// unredacted events it produced.
func (g *Game) Apply(seat int, in Intent) ([]Event, error) {
	if err := g.validate(seat, in); err != nil {
		return nil, err
	}
	g.events = nil
	switch g.phase {
	case PhaseTurnStart:
		g.declare(seat, in)
	case PhaseChallengeWindow:
		g.onActionChallengeWindow(seat, in)
	case PhaseBlockWindow:
		g.onBlockWindow(seat, in)
	case PhaseBlockChallengeWindow:
		g.onBlockChallengeWindow(seat, in)
	case PhaseProveClaim:
		g.reveal(in.Card)
	case PhaseLoseInfluence:
		g.awaiting[seat] = false
		g.loseCard(seat, in.Card, g.lossWhy)
		if !g.Over() {
			g.continueAfter(g.after)
		}
	case PhaseExchange:
		g.finishExchange(in.Keep)
	}
	g.version++
	evs := g.events
	g.events = nil
	return evs, nil
}

// StartEvents returns the events describing the opening position.
func (g *Game) StartEvents() []Event {
	return []Event{
		{Kind: EvGameStart, Turn: g.turn, Seat: g.current, Other: -1, Amount: len(g.players)},
		{Kind: EvTurnStart, Turn: g.turn, Seat: g.current, Other: -1},
	}
}

func (g *Game) emit(e Event) {
	e.Turn = g.turn
	e.Phase = g.phase
	g.events = append(g.events, e)
}

// openWindow enters a reaction window and announces it.
func (g *Game) openWindow(p Phase) {
	g.phase = p
	t := g.act
	g.emit(Event{Kind: EvWindowOpen, Seat: t.actor, Other: t.target, Action: t.action, Amount: len(g.Awaiting())})
}

func (g *Game) awaitOnly(seats ...int) {
	for i := range g.awaiting {
		g.awaiting[i] = false
	}
	for _, s := range seats {
		g.awaiting[s] = true
	}
}

func (g *Game) awaitAllExcept(except int) {
	for i := range g.awaiting {
		g.awaiting[i] = i != except && g.players[i].alive
	}
}

func (g *Game) noneAwaiting() bool {
	for _, w := range g.awaiting {
		if w {
			return false
		}
	}
	return true
}

// --- turn flow -------------------------------------------------------------

func (g *Game) declare(seat int, in Intent) {
	t := &turnState{actor: seat, action: in.Action, target: -1, claim: in.Action.Claim(), blocker: -1}
	if in.Action.Targeted() {
		t.target = in.Target
	}
	g.act = t
	g.phase = PhaseActionDeclared
	g.emit(Event{Kind: EvActionDeclared, Seat: seat, Other: t.target, Action: t.action, Claim: t.claim})
	if cost := in.Action.Cost(); cost > 0 {
		reason := ReasonCoupFee
		if in.Action == Assassinate {
			reason = ReasonAssassinFee
		}
		g.players[seat].coins -= cost
		g.treasury += cost
		t.paid = cost
		g.emit(Event{Kind: EvCoins, Seat: seat, Other: -1, Amount: -cost, Reason: reason})
	}
	switch {
	case t.claim != NoCharacter:
		g.awaitAllExcept(seat)
		g.openWindow(PhaseChallengeWindow)
	case len(t.action.Blockers(g.cfg.Magistrate)) > 0:
		g.openBlockWindow()
	default:
		g.resolve()
	}
}

func (g *Game) onActionChallengeWindow(seat int, in Intent) {
	switch in.Kind {
	case IntentPass:
		g.awaiting[seat] = false
		g.emit(Event{Kind: EvPass, Seat: seat, Other: -1})
		if g.noneAwaiting() {
			g.continueAfter(contAfterActionProven)
		}
	case IntentChallenge:
		g.act.actionChallenged = true
		g.startChallenge(seat, g.act.actor, g.act.claim, false)
	}
}

// blockers lists the seats that may block the current action.
func (g *Game) blockerSeats() []int {
	t := g.act
	if len(t.action.Blockers(g.cfg.Magistrate)) == 0 {
		return nil
	}
	if t.action.BlockedByTargetOnly() {
		if g.Alive(t.target) {
			return []int{t.target}
		}
		return nil
	}
	var s []int
	for i, p := range g.players {
		if i != t.actor && p.alive {
			s = append(s, i)
		}
	}
	return s
}

func (g *Game) openBlockWindow() {
	seats := g.blockerSeats()
	if len(seats) == 0 {
		g.resolve()
		return
	}
	g.awaitOnly(seats...)
	g.openWindow(PhaseBlockWindow)
}

func (g *Game) onBlockWindow(seat int, in Intent) {
	switch in.Kind {
	case IntentPass:
		g.awaiting[seat] = false
		g.emit(Event{Kind: EvPass, Seat: seat, Other: -1})
		if g.noneAwaiting() {
			g.resolve()
		}
	case IntentBlock:
		t := g.act
		t.blocker = seat
		t.blockClaim = in.Claim
		g.emit(Event{Kind: EvBlock, Seat: seat, Other: t.actor, Claim: in.Claim, Action: t.action})
		g.awaitAllExcept(seat)
		g.openWindow(PhaseBlockChallengeWindow)
	}
}

func (g *Game) onBlockChallengeWindow(seat int, in Intent) {
	switch in.Kind {
	case IntentPass:
		g.awaiting[seat] = false
		g.emit(Event{Kind: EvPass, Seat: seat, Other: -1})
		if g.noneAwaiting() {
			g.continueAfter(contBlockStands)
		}
	case IntentChallenge:
		g.act.blockChallenged = true
		g.startChallenge(seat, g.act.blocker, g.act.blockClaim, true)
	}
}

func (g *Game) startChallenge(challenger, claimant int, claim Character, onBlock bool) {
	g.emit(Event{Kind: EvChallenge, Seat: challenger, Other: claimant, Claim: claim})
	g.chal = &challengeState{challenger: challenger, claimant: claimant, claim: claim, onBlock: onBlock}
	g.phase = PhaseProveClaim
	g.awaitOnly(claimant)
	if slot, ok := g.forcedReveal(); ok {
		g.reveal(slot)
	}
}

// forcedReveal returns the slot to flip when the claimant has a single
// face-down card. Two identical cards still go to the player: deciding for
// them would show the whole table what the other card is.
func (g *Game) forcedReveal() (int, bool) {
	slots := g.players[g.chal.claimant].hiddenSlots()
	if len(slots) == 1 {
		return slots[0], true
	}
	return 0, false
}

// reveal settles the challenge in progress by flipping the claimant's slot.
func (g *Game) reveal(slot int) {
	c := g.chal
	g.chal = nil
	g.awaitOnly()
	claimant := &g.players[c.claimant]
	shown := claimant.cards[slot].Char
	if shown == c.claim {
		g.emit(Event{Kind: EvClaimProven, Seat: c.claimant, Other: c.challenger, Card: shown, Slot: slot})
		// Return the proven card, reshuffle, and draw a replacement.
		g.deck = append(g.deck, shown)
		g.shuffle()
		claimant.cards[slot] = Card{Char: g.draw()}
		g.emit(Event{Kind: EvCardReplaced, Seat: c.claimant, Other: -1, Slot: slot,
			Secret: []Character{claimant.cards[slot].Char}})
		cont := contAfterActionProven
		if c.onBlock {
			cont = contBlockStands
		}
		g.startLoss(c.challenger, ReasonLostChallenge, cont)
		return
	}

	g.emit(Event{Kind: EvClaimFailed, Seat: c.claimant, Other: c.challenger, Card: shown, Slot: slot})
	if c.onBlock {
		g.loseCard(c.claimant, slot, ReasonFailedClaim)
		if g.Over() {
			return
		}
		g.emit(Event{Kind: EvBlockFailed, Seat: c.claimant, Other: g.act.actor, Claim: c.claim, Action: g.act.action})
		g.resolve()
		return
	}
	// A successfully challenged action fails entirely and its cost is
	// refunded. Refund before the influence loss so an eliminated actor's
	// coins all flow back to the Treasury.
	t := g.act
	if t.paid > 0 {
		g.players[t.actor].coins += t.paid
		g.treasury -= t.paid
		g.emit(Event{Kind: EvCoins, Seat: t.actor, Other: -1, Amount: t.paid, Reason: ReasonRefund})
		t.paid = 0
	}
	g.loseCard(c.claimant, slot, ReasonFailedClaim)
	if g.Over() {
		return
	}
	g.emit(Event{Kind: EvActionFailed, Seat: t.actor, Other: t.target, Action: t.action, Reason: ReasonChallenged})
	g.endTurn()
}

// startLoss makes a seat lose one influence, asking them which card when
// there is a real choice.
func (g *Game) startLoss(seat int, why Reason, cont continuation) {
	p := &g.players[seat]
	if !p.alive {
		g.continueAfter(cont)
		return
	}
	// Only a single card is decided automatically; skipping the choice for
	// two identical cards would leak them (see forcedReveal).
	slots := p.hiddenSlots()
	if len(slots) == 1 {
		g.loseCard(seat, slots[0], why)
		if !g.Over() {
			g.continueAfter(cont)
		}
		return
	}
	g.phase = PhaseLoseInfluence
	g.loser = seat
	g.lossWhy = why
	g.after = cont
	g.awaitOnly(seat)
}

// loseCard flips a card face up and eliminates the seat if it was the last.
func (g *Game) loseCard(seat, slot int, why Reason) {
	p := &g.players[seat]
	p.cards[slot].Revealed = true
	g.loser = -1
	g.emit(Event{Kind: EvInfluenceLost, Seat: seat, Other: -1, Card: p.cards[slot].Char, Slot: slot, Reason: why})
	if p.hidden() > 0 {
		return
	}
	p.alive = false
	returned := p.coins
	g.treasury += p.coins
	p.coins = 0
	g.emit(Event{Kind: EvEliminated, Seat: seat, Other: -1, Amount: returned})
	if returned > 0 {
		g.emit(Event{Kind: EvCoins, Seat: seat, Other: -1, Amount: -returned, Reason: ReasonEliminated})
	}
	if g.AliveCount() == 1 {
		for i, q := range g.players {
			if q.alive {
				g.winner = i
			}
		}
		g.phase = PhaseGameOver
		g.awaitOnly()
		g.act = nil
		g.chal = nil
		g.drawn = nil
		g.emit(Event{Kind: EvGameOver, Seat: g.winner, Other: -1})
	}
}

func (g *Game) continueAfter(c continuation) {
	switch c {
	case contAfterActionProven:
		g.openBlockWindow()
	case contBlockStands:
		t := g.act
		g.emit(Event{Kind: EvActionBlocked, Seat: t.actor, Other: t.blocker, Action: t.action, Claim: t.blockClaim})
		g.endTurn()
	default:
		g.endTurn()
	}
}

func (g *Game) gain(seat, n int, why Reason) {
	n = min(n, g.treasury)
	g.players[seat].coins += n
	g.treasury -= n
	g.emit(Event{Kind: EvCoins, Seat: seat, Other: -1, Amount: n, Reason: why})
}

func (g *Game) resolve() {
	t := g.act
	g.phase = PhaseResolve
	g.awaitOnly()
	targetGone := t.action.Targeted() && !g.Alive(t.target)
	if targetGone {
		// The target was eliminated during a challenge; nothing left to do.
		g.emit(Event{Kind: EvActionFailed, Seat: t.actor, Other: t.target, Action: t.action, Reason: ReasonNoTarget})
		g.endTurn()
		return
	}
	g.emit(Event{Kind: EvActionResolved, Seat: t.actor, Other: t.target, Action: t.action})
	switch t.action {
	case Income:
		g.gain(t.actor, 1, ReasonIncome)
	case ForeignAid:
		g.gain(t.actor, 2, ReasonForeignAid)
	case Tax:
		g.gain(t.actor, 3, ReasonTax)
	case Steal:
		victim := &g.players[t.target]
		n := min(StealAmount, victim.coins)
		victim.coins -= n
		g.players[t.actor].coins += n
		g.emit(Event{Kind: EvCoins, Seat: t.target, Other: t.actor, Amount: -n, Reason: ReasonSteal})
		g.emit(Event{Kind: EvCoins, Seat: t.actor, Other: t.target, Amount: n, Reason: ReasonSteal})
	case Audit:
		victim := &g.players[t.target]
		n := min(AuditAmount, victim.coins)
		victim.coins -= n
		g.treasury += n
		g.emit(Event{Kind: EvCoins, Seat: t.target, Other: t.actor, Amount: -n, Reason: ReasonAudit})
	case Coup:
		g.startLoss(t.target, ReasonCoup, contEndTurn)
		return
	case Assassinate:
		g.startLoss(t.target, ReasonAssassinated, contEndTurn)
		return
	case Exchange:
		n := min(ExchangeDraw, len(g.deck))
		g.drawn = g.drawn[:0]
		for range n {
			g.drawn = append(g.drawn, g.draw())
		}
		g.phase = PhaseExchange
		g.awaitOnly(t.actor)
		g.emit(Event{Kind: EvExchangeDraw, Seat: t.actor, Other: -1, Amount: n, Secret: append([]Character(nil), g.drawn...)})
		return
	}
	g.endTurn()
}

// exchangeOptions lists the actor's face-down cards (slot order) followed by
// the drawn cards.
func (g *Game) exchangeOptions() []Character {
	p := &g.players[g.act.actor]
	var opts []Character
	for _, s := range p.hiddenSlots() {
		opts = append(opts, p.cards[s].Char)
	}
	return append(opts, g.drawn...)
}

func (g *Game) finishExchange(keep []int) {
	p := &g.players[g.act.actor]
	opts := g.exchangeOptions()
	kept := make([]bool, len(opts))
	for _, k := range keep {
		kept[k] = true
	}
	for i, s := range p.hiddenSlots() {
		p.cards[s].Char = opts[keep[i]]
	}
	for i, c := range opts {
		if !kept[i] {
			g.deck = append(g.deck, c)
		}
	}
	g.drawn = nil
	g.shuffle()
	g.emit(Event{Kind: EvExchangeDone, Seat: g.act.actor, Other: -1, Amount: len(opts) - len(keep)})
	g.endTurn()
}

func (g *Game) endTurn() {
	g.phase = PhaseTurnEnd
	g.emit(Event{Kind: EvTurnEnd, Seat: g.current, Other: -1})
	g.act = nil
	g.chal = nil
	next := g.current
	for range len(g.players) {
		next = (next + 1) % len(g.players)
		if g.players[next].alive {
			break
		}
	}
	g.current = next
	g.turn++
	g.phase = PhaseTurnStart
	g.awaitOnly(next)
	g.emit(Event{Kind: EvTurnStart, Seat: next, Other: -1})
}

// --- deck ------------------------------------------------------------------

func (g *Game) shuffle() {
	g.rng.Shuffle(len(g.deck), func(i, j int) { g.deck[i], g.deck[j] = g.deck[j], g.deck[i] })
}

func (g *Game) draw() Character {
	c := g.deck[len(g.deck)-1]
	g.deck = g.deck[:len(g.deck)-1]
	return c
}
