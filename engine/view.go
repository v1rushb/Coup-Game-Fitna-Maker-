package engine

// PublicPlayer is what everyone can see about a seat. It has no field for
// face-down card identities, so it cannot leak them.
type PublicPlayer struct {
	Seat     int         `json:"seat"`
	Coins    int         `json:"coins"`
	Alive    bool        `json:"alive"`
	Hidden   int         `json:"hidden"`
	Revealed []Character `json:"revealed,omitempty"`
}

// PendingView describes the action being resolved.
type PendingView struct {
	Actor            int       `json:"actor"`
	Action           Action    `json:"action"`
	Target           int       `json:"target"`
	Claim            Character `json:"claim,omitempty"`
	Paid             int       `json:"paid,omitempty"`
	ActionChallenged bool      `json:"action_challenged,omitempty"`
	Blocker          int       `json:"blocker"`
	BlockClaim       Character `json:"block_claim,omitempty"`
	BlockChallenged  bool      `json:"block_challenged,omitempty"`
}

// ChallengeView describes a challenge waiting for the claimant to flip a card.
type ChallengeView struct {
	Challenger int       `json:"challenger"`
	Claimant   int       `json:"claimant"`
	Claim      Character `json:"claim"`
	OnBlock    bool      `json:"on_block,omitempty"`
}

// PlayerView is the game as one seat may see it. Seat -1 is a spectator.
// Hand and ExchangeOptions are only ever filled with the viewer's own cards.
type PlayerView struct {
	Seat            int            `json:"seat"`
	Phase           Phase          `json:"phase"`
	Version         uint64         `json:"ver"`
	Turn            int            `json:"turn"`
	Current         int            `json:"current"`
	Players         []PublicPlayer `json:"players"`
	Hand            []Card         `json:"hand,omitempty"`
	DeckCount       int            `json:"deck"`
	Treasury        int            `json:"treasury"`
	Magistrate      bool           `json:"magistrate,omitempty"`
	Pending         *PendingView   `json:"pending,omitempty"`
	Challenge       *ChallengeView `json:"challenge,omitempty"`
	Awaiting        []int          `json:"awaiting,omitempty"`
	Loser           int            `json:"loser"`
	ExchangeOptions []Character    `json:"exchange,omitempty"`
	Legal           []Intent       `json:"legal,omitempty"`
	Winner          int            `json:"winner"`
}

// View returns the redacted view for a seat (-1 for spectators).
func (g *Game) View(seat int) PlayerView {
	v := PlayerView{
		Seat:       seat,
		Phase:      g.phase,
		Version:    g.version,
		Turn:       g.turn,
		Current:    g.current,
		DeckCount:  len(g.deck),
		Treasury:   g.treasury,
		Magistrate: g.cfg.Magistrate,
		Awaiting:   g.Awaiting(),
		Loser:      -1,
		Winner:     g.winner,
	}
	if g.phase == PhaseLoseInfluence {
		v.Loser = g.loser
	}
	for i, p := range g.players {
		pp := PublicPlayer{Seat: i, Coins: p.coins, Alive: p.alive, Hidden: p.hidden()}
		for _, c := range p.cards {
			if c.Revealed {
				pp.Revealed = append(pp.Revealed, c.Char)
			}
		}
		v.Players = append(v.Players, pp)
	}
	if seat >= 0 && seat < len(g.players) {
		v.Hand = append([]Card(nil), g.players[seat].cards...)
		v.Legal = g.Legal(seat)
	}
	if t := g.act; t != nil {
		v.Pending = &PendingView{
			Actor: t.actor, Action: t.action, Target: t.target, Claim: t.claim, Paid: t.paid,
			ActionChallenged: t.actionChallenged, Blocker: t.blocker, BlockClaim: t.blockClaim,
			BlockChallenged: t.blockChallenged,
		}
	}
	if c := g.chal; c != nil {
		v.Challenge = &ChallengeView{Challenger: c.challenger, Claimant: c.claimant, Claim: c.claim, OnBlock: c.onBlock}
	}
	if g.phase == PhaseExchange && g.act != nil && seat == g.act.actor {
		v.ExchangeOptions = g.exchangeOptions()
	}
	return v
}

// Snapshot is the full, unredacted state. Only the host, tests and the
// post-game replay viewer use it; bots and clients never receive one.
type Snapshot struct {
	Phase    Phase       `json:"phase"`
	Turn     int         `json:"turn"`
	Current  int         `json:"current"`
	Treasury int         `json:"treasury"`
	Deck     []Character `json:"deck"`
	Hands    [][]Card    `json:"hands"`
	Coins    []int       `json:"coins"`
	Alive    []bool      `json:"alive"`
	Drawn    []Character `json:"drawn,omitempty"`
	Winner   int         `json:"winner"`
}

// Snapshot copies the full state. The deck is listed top-first.
func (g *Game) Snapshot() Snapshot {
	s := Snapshot{Phase: g.phase, Turn: g.turn, Current: g.current, Treasury: g.treasury, Winner: g.winner}
	for i := len(g.deck) - 1; i >= 0; i-- {
		s.Deck = append(s.Deck, g.deck[i])
	}
	for _, p := range g.players {
		s.Hands = append(s.Hands, append([]Card(nil), p.cards...))
		s.Coins = append(s.Coins, p.coins)
		s.Alive = append(s.Alive, p.alive)
	}
	s.Drawn = append(s.Drawn, g.drawn...)
	return s
}

// CheckInvariants verifies coin conservation and card accounting. It returns
// nil when the state is consistent.
func (g *Game) CheckInvariants() error {
	coins := g.treasury
	if g.treasury < 0 {
		return illegal("treasury is negative (%d)", g.treasury)
	}
	for i, p := range g.players {
		if p.coins < 0 {
			return illegal("seat %d has negative coins", i)
		}
		if !p.alive && p.coins != 0 {
			return illegal("eliminated seat %d still holds %d coins", i, p.coins)
		}
		if p.alive != (p.hidden() > 0) {
			return illegal("seat %d alive=%v with %d hidden cards", i, p.alive, p.hidden())
		}
		coins += p.coins
	}
	if coins != TotalCoins {
		return illegal("coins not conserved: %d in play, want %d", coins, TotalCoins)
	}
	counts := map[Character]int{}
	total := 0
	for _, c := range g.deck {
		counts[c]++
		total++
	}
	for _, c := range g.drawn {
		counts[c]++
		total++
	}
	for _, p := range g.players {
		if len(p.cards) != HandSize {
			return illegal("hand size %d", len(p.cards))
		}
		for _, c := range p.cards {
			counts[c.Char]++
			total++
		}
	}
	if total != DeckSize(g.cfg.Magistrate) {
		return illegal("%d cards accounted for, want %d", total, DeckSize(g.cfg.Magistrate))
	}
	for _, c := range DeckComposition(g.cfg.Magistrate) {
		if counts[c] != CopiesPerCharacter {
			return illegal("%d copies of %s, want %d", counts[c], c, CopiesPerCharacter)
		}
	}
	if g.phase != PhaseGameOver && g.noneAwaiting() {
		return illegal("phase %s is waiting on nobody", g.phase)
	}
	if g.phase == PhaseGameOver && g.AliveCount() != 1 {
		return illegal("game over with %d players alive", g.AliveCount())
	}
	return nil
}
