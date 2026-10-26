package server

import (
	"fmt"
	"log/slog"
	"math/rand/v2"
	"slices"
	"strings"
	"time"

	"github.com/v1rushb/neoncoup/bot"
	"github.com/v1rushb/neoncoup/engine"
	"github.com/v1rushb/neoncoup/protocol"
)

// Inbox messages.
type (
	joinMsg struct {
		cl    *client
		hello protocol.Hello
		reply chan string // "" on success, else the reject code
	}
	frameMsg struct {
		cl  *client
		env protocol.Envelope
	}
	dropMsg struct {
		cl    *client
		leave bool
	}
	timerMsg struct{ gen int }
	botMsg   struct {
		seat int
		key  string
	}
	holdMsg struct {
		member string
		gen    int
	}
	funcMsg  struct{ f func(*Room) }
	idleMsg  struct{ gen int }
	closeMsg struct{}
)

type member struct {
	id, name, token string
	cl              *client
	host            bool
	ready           bool
	// spectate is the member's own choice; seat is -1 when not playing.
	spectate bool
	seat     int
	holdGen  int
}

type seat struct {
	idx      int
	name     string
	memberID string // "" for a bot seat
	bot      bot.Bot
	level    bot.Level
	takeover bool // a bot is covering for a disconnected human
	offline  bool
	botKey   string
}

// Room is one lobby and its games. All fields are owned by the run goroutine.
type Room struct {
	srv      *Server
	code     string
	log      *slog.Logger
	inbox    chan any
	done     chan struct{}
	rng      *rand.Rand
	settings protocol.Settings

	members []*member
	nextID  int

	game       *engine.Game
	seats      []*seat
	gameNo     int
	replay     protocol.Replay
	started    time.Time
	lastWinner string

	timerGen   int
	timerKey   string
	deadline   time.Time
	timerTotal time.Duration
	timer      *time.Timer

	idleGen int
	idle    bool
}

func newRoom(s *Server, code string, settings protocol.Settings, seed uint64) *Room {
	return &Room{
		srv:      s,
		code:     code,
		log:      s.log.With("room", code),
		inbox:    make(chan any, 256),
		done:     make(chan struct{}),
		rng:      rand.New(rand.NewPCG(seed, 0x6a09e667)),
		settings: settings,
	}
}

// Code is the room code.
func (r *Room) Code() string { return r.code }

// post delivers a message to the room loop. It returns false once the room is closed.
func (r *Room) post(m any) bool {
	select {
	case <-r.done:
		return false
	default:
	}
	select {
	case r.inbox <- m:
		return true
	case <-r.done:
		return false
	}
}

// Do runs f on the room goroutine and waits for it. Used by embedding code
// (solo setup, tests) to inspect or adjust a room safely.
func (r *Room) Do(f func(*Room)) bool {
	ch := make(chan struct{})
	if !r.post(funcMsg{f: func(r *Room) { f(r); close(ch) }}) {
		return false
	}
	select {
	case <-ch:
		return true
	case <-r.done:
		return false
	}
}

// Settings returns the room settings (call from Do or the room goroutine).
func (r *Room) Settings() protocol.Settings { return r.settings }

// SetSettings replaces the settings (call from Do).
func (r *Room) SetSettings(s protocol.Settings) {
	r.settings = s.Clamp()
	r.enforceCapacity()
	r.broadcastLobby()
}

// Game exposes the running game for tests (call from Do).
func (r *Room) Game() *engine.Game { return r.game }

// Close shuts the room down, disconnecting everyone in it.
func (r *Room) Close() { r.post(closeMsg{}) }

// Done is closed once the room has shut down.
func (r *Room) Done() <-chan struct{} { return r.done }

// checkIdle arms or disarms the idle-close timer.
func (r *Room) checkIdle() {
	if r.srv.opts.IdleClose <= 0 {
		return
	}
	connected := false
	for _, m := range r.members {
		if m.cl != nil {
			connected = true
			break
		}
	}
	if connected {
		if r.idle {
			r.idle = false
			r.idleGen++
		}
		return
	}
	if r.idle {
		return
	}
	r.idle = true
	r.idleGen++
	gen := r.idleGen
	time.AfterFunc(r.srv.opts.IdleClose, func() { r.post(idleMsg{gen: gen}) })
}

func (r *Room) run() {
	defer close(r.done)
	r.checkIdle()
	for {
		select {
		case <-r.srv.ctx.Done():
			r.shutdown("host shut down")
			return
		case m := <-r.inbox:
			switch m := m.(type) {
			case closeMsg:
				r.shutdown("room closed")
				return
			case idleMsg:
				if r.idle && m.gen == r.idleGen {
					r.log.Info("room idle; closing")
					r.shutdown("room closed after being empty")
					return
				}
				continue
			}
			r.handle(m)
			r.checkIdle()
		}
	}
}

func (r *Room) shutdown(reason string) {
	for _, m := range r.members {
		if m.cl != nil {
			m.cl.send(protocol.MustEncode(protocol.TReject, protocol.Reject{Code: protocol.RejectClosed, Reason: reason}))
		}
	}
	if r.timer != nil {
		r.timer.Stop()
	}
}

func (r *Room) handle(m any) {
	defer func() {
		if p := recover(); p != nil {
			// A bug must not take the whole host down; end the game instead.
			r.log.Error("room panic", "panic", p)
			r.system(fmt.Sprintf("host error: %v (game aborted)", p))
			r.abortGame()
		}
	}()
	switch m := m.(type) {
	case joinMsg:
		func() {
			// Never leave a joiner waiting, even if join panics.
			defer func() {
				if p := recover(); p != nil {
					m.reply <- protocol.RejectClosed
					panic(p)
				}
			}()
			m.reply <- r.join(m.cl, m.hello)
		}()
	case frameMsg:
		r.onFrame(m.cl, m.env)
	case dropMsg:
		r.onDrop(m.cl, m.leave)
	case timerMsg:
		r.onTimer(m.gen)
	case botMsg:
		r.onBot(m.seat, m.key)
	case holdMsg:
		r.onHoldExpired(m.member, m.gen)
	case funcMsg:
		m.f(r)
	}
}

// --- membership ------------------------------------------------------------

func (r *Room) memberByClient(cl *client) *member {
	for _, m := range r.members {
		if m.cl == cl {
			return m
		}
	}
	return nil
}

func (r *Room) memberByID(id string) *member {
	for _, m := range r.members {
		if m.id == id {
			return m
		}
	}
	return nil
}

func (r *Room) host() *member {
	for _, m := range r.members {
		if m.host {
			return m
		}
	}
	return nil
}

func (r *Room) players() []*member {
	var out []*member
	for _, m := range r.members {
		if !m.spectate {
			out = append(out, m)
		}
	}
	return out
}

func (r *Room) uniqueName(name string) string {
	taken := func(n string) bool {
		for _, m := range r.members {
			if strings.EqualFold(m.name, n) {
				return true
			}
		}
		for _, s := range r.seats {
			if strings.EqualFold(s.name, n) {
				return true
			}
		}
		return false
	}
	if !taken(name) {
		return name
	}
	for i := 2; ; i++ {
		suffix := fmt.Sprintf("-%d", i)
		base := name
		if len([]rune(base))+len(suffix) > protocol.MaxNameLen {
			base = string([]rune(base)[:protocol.MaxNameLen-len(suffix)])
		}
		if !taken(base + suffix) {
			return base + suffix
		}
	}
}

func (r *Room) join(cl *client, h protocol.Hello) string {
	// Rejoin with a token.
	if h.Token != "" {
		for _, m := range r.members {
			if m.token == h.Token {
				return r.rejoin(m, cl)
			}
		}
	}
	spectators := 0
	for _, m := range r.members {
		if m.spectate || (r.game != nil && m.seat < 0) {
			spectators++
		}
	}
	wantSeat := !h.Spectate
	seatsTaken := len(r.players())
	if r.game != nil || seatsTaken >= r.settings.MaxPlayers {
		wantSeat = false
	}
	if !wantSeat && spectators >= r.srv.opts.MaxSpectators {
		cl.send(protocol.MustEncode(protocol.TReject, protocol.Reject{Code: protocol.RejectFull, Reason: "room is full"}))
		return protocol.RejectFull
	}
	name := protocol.CleanName(h.Name)
	if name == "" {
		name = "RUNNER"
	}
	r.nextID++
	m := &member{
		id:       fmt.Sprintf("m%d", r.nextID),
		name:     r.uniqueName(name),
		token:    protocol.NewToken(),
		cl:       cl,
		host:     r.liveHost() == nil,
		spectate: !wantSeat && (h.Spectate || r.game == nil),
		seat:     -1,
	}
	r.members = append(r.members, m)
	cl.send(protocol.MustEncode(protocol.TWelcome, protocol.Welcome{
		Proto: protocol.Version, You: m.id, Token: m.token, Room: r.code, Host: m.host,
	}))
	r.log.Info("member joined", "id", m.id, "name", m.name, "spectator", m.spectate || r.game != nil)
	r.broadcastLobby()
	if r.game != nil {
		r.sendGameStart(m)
		r.sendState(m)
		r.system(m.name + " is watching")
	} else {
		r.system(m.name + " jacked in")
	}
	return ""
}

func (r *Room) rejoin(m *member, cl *client) string {
	if m.cl != nil && m.cl != cl {
		m.cl.close()
	}
	m.cl = cl
	m.holdGen++
	if r.liveHost() == nil {
		r.makeHost(m)
	}
	cl.send(protocol.MustEncode(protocol.TWelcome, protocol.Welcome{
		Proto: protocol.Version, You: m.id, Token: m.token, Room: r.code, Host: m.host, Rejoined: r.game != nil && m.seat >= 0,
	}))
	if r.game != nil && m.seat >= 0 {
		s := r.seats[m.seat]
		s.offline = false
		if s.takeover {
			s.bot = nil
			s.takeover = false
			s.botKey = ""
			r.system(m.name + " is back and took their seat from the bot")
		} else {
			r.system(m.name + " reconnected")
		}
	}
	r.broadcastLobby()
	if r.game != nil {
		r.sendGameStart(m)
		r.broadcastState()
		r.schedule()
	}
	return ""
}

func (r *Room) onDrop(cl *client, leave bool) {
	m := r.memberByClient(cl)
	if m == nil {
		return
	}
	m.cl = nil
	m.ready = false
	inGame := r.game != nil && m.seat >= 0 && r.game.Alive(m.seat)
	if leave && !inGame {
		r.removeMember(m)
		r.system(m.name + " left")
		return
	}
	m.holdGen++
	gen := m.holdGen
	hold := time.Duration(r.settings.ReconnectS) * time.Second
	if inGame {
		r.seats[m.seat].offline = true
		r.system(fmt.Sprintf("%s lost the uplink. Seat held for %ds.", m.name, r.settings.ReconnectS))
		r.broadcastState()
	} else {
		r.broadcastLobby()
	}
	if leave {
		hold = 0
	}
	id := m.id
	time.AfterFunc(hold, func() { r.post(holdMsg{member: id, gen: gen}) })
}

func (r *Room) onHoldExpired(id string, gen int) {
	m := r.memberByID(id)
	if m == nil || m.cl != nil || m.holdGen != gen {
		return
	}
	if r.game != nil && m.seat >= 0 {
		s := r.seats[m.seat]
		if r.game.Alive(m.seat) && s.bot == nil {
			s.bot = bot.New(bot.Normal, s.idx, r.rng.Uint64(), bot.Personality{Handle: s.name, Bluff: 1, Suspicion: 1, Aggression: 1})
			s.level = bot.Normal
			s.takeover = true
			r.system(fmt.Sprintf("%s timed out. A %s bot is running their seat (they can still rejoin).", m.name, bot.Normal.Title()))
			r.broadcastState()
			r.schedule()
		}
		return
	}
	r.removeMember(m)
	r.system(m.name + " disconnected")
}

func (r *Room) removeMember(m *member) {
	r.members = slices.DeleteFunc(r.members, func(x *member) bool { return x == m })
	if m.host {
		r.passHost()
	}
	r.broadcastLobby()
}

// liveHost is the host if they are connected.
func (r *Room) liveHost() *member {
	if h := r.host(); h != nil && h.cl != nil {
		return h
	}
	return nil
}

func (r *Room) makeHost(m *member) {
	for _, x := range r.members {
		x.host = false
	}
	m.host = true
	r.system(m.name + " is now the host")
}

// passHost hands the host role to a connected member, or failing that to
// anyone left, so a room is never stuck without someone who can start it.
func (r *Room) passHost() {
	for _, x := range r.members {
		if x.cl != nil {
			r.makeHost(x)
			return
		}
	}
	if len(r.members) > 0 {
		r.members[0].host = true
	}
}

// enforceCapacity moves the most recent joiners to the spectator rail when
// there are more players than seats (late joiners, or the host lowered the
// seat count).
func (r *Room) enforceCapacity() {
	n := 0
	for _, m := range r.members {
		if m.spectate {
			continue
		}
		n++
		if n > r.settings.MaxPlayers {
			m.spectate = true
			m.ready = false
			r.system(m.name + " is spectating: the table is full")
		}
	}
}

// --- frames ----------------------------------------------------------------

func (r *Room) sendErr(cl *client, code, msg string) {
	cl.send(protocol.MustEncode(protocol.TError, protocol.Error{Code: code, Msg: msg}))
}

func (r *Room) onFrame(cl *client, env protocol.Envelope) {
	m := r.memberByClient(cl)
	if m == nil {
		return
	}
	switch env.Type {
	case protocol.TPing:
		var p protocol.Ping
		_ = env.Into(&p)
		cl.send(protocol.MustEncode(protocol.TPong, p))
	case protocol.TReady:
		var p protocol.Ready
		if env.Into(&p) != nil || r.game != nil {
			return
		}
		m.ready = p.Ready
		r.broadcastLobby()
	case protocol.TSettings:
		var p protocol.Settings
		if err := env.Into(&p); err != nil {
			r.sendErr(cl, protocol.ErrBadMsg, err.Error())
			return
		}
		if !m.host {
			r.sendErr(cl, protocol.ErrNotHost, "only the host can change settings")
			return
		}
		if r.game != nil {
			r.sendErr(cl, protocol.ErrIllegal, "settings are locked during a game")
			return
		}
		r.settings = p.Clamp()
		r.enforceCapacity()
		r.broadcastLobby()
	case protocol.TStart:
		if !m.host {
			r.sendErr(cl, protocol.ErrNotHost, "only the host can start")
			return
		}
		if err := r.startGame(); err != nil {
			r.sendErr(cl, protocol.ErrNotReady, err.Error())
		}
	case protocol.TIntent:
		var p protocol.IntentMsg
		if err := env.Into(&p); err != nil {
			r.sendErr(cl, protocol.ErrBadMsg, err.Error())
			return
		}
		r.onIntent(m, p)
	case protocol.TChat:
		var p protocol.Chat
		if env.Into(&p) != nil {
			return
		}
		text := protocol.CleanText(p.Text, protocol.MaxChatLen)
		if text == "" {
			return
		}
		r.broadcast(protocol.MustEncode(protocol.TChat, protocol.Chat{From: m.id, Name: m.name, Seat: m.seat, Text: text}))
	case protocol.TEmote:
		var p protocol.Emote
		if env.Into(&p) != nil || p.ID < 0 || p.ID >= len(protocol.Emotes) {
			return
		}
		r.broadcast(protocol.MustEncode(protocol.TEmote, protocol.Emote{From: m.id, Name: m.name, Seat: m.seat, ID: p.ID}))
	default:
		r.sendErr(cl, protocol.ErrBadMsg, "unknown message type "+protocol.CleanText(env.Type, 32))
	}
}

func (r *Room) onIntent(m *member, p protocol.IntentMsg) {
	if r.game == nil {
		r.sendErr(m.cl, protocol.ErrIllegal, "no game is running")
		return
	}
	if m.seat < 0 {
		r.sendErr(m.cl, protocol.ErrNoSeat, "spectators cannot act")
		return
	}
	if p.Ver != r.game.Version() {
		r.sendErr(m.cl, protocol.ErrStale, "too late: the table moved on")
		return
	}
	if err := r.apply(m.seat, p.Intent); err != nil {
		r.sendErr(m.cl, protocol.ErrIllegal, err.Error())
	}
}

// --- game lifecycle --------------------------------------------------------

func (r *Room) startGame() error {
	if r.game != nil {
		return fmt.Errorf("a game is already running")
	}
	var humans []*member
	for _, m := range r.players() {
		if m.cl == nil {
			continue
		}
		if !m.ready && !m.host {
			return fmt.Errorf("%s is not ready", m.name)
		}
		humans = append(humans, m)
	}
	n := len(humans)
	bots := 0
	if r.settings.FillBots {
		bots = max(0, r.settings.MaxPlayers-n)
	}
	if n+bots < engine.MinPlayers {
		return fmt.Errorf("need at least %d players (turn on bot fill or wait for runners)", engine.MinPlayers)
	}
	if n+bots > engine.MaxPlayers {
		return fmt.Errorf("at most %d players", engine.MaxPlayers)
	}
	level, _ := bot.ParseLevel(r.settings.BotLevel)
	taken := map[string]bool{}
	for _, m := range r.members {
		taken[strings.ToUpper(m.name)] = true
	}
	pers := bot.Pick(bots, r.rng.Uint64(), taken)

	r.seats = nil
	for _, m := range humans {
		r.seats = append(r.seats, &seat{name: m.name, memberID: m.id})
	}
	for _, p := range pers {
		r.seats = append(r.seats, &seat{name: p.Handle, level: level, bot: nil})
	}
	r.rng.Shuffle(len(r.seats), func(i, j int) { r.seats[i], r.seats[j] = r.seats[j], r.seats[i] })
	first := -1
	persIdx := 0
	for i, s := range r.seats {
		s.idx = i
		if s.memberID == "" {
			s.bot = bot.New(level, i, r.rng.Uint64(), pers[persIdx])
			persIdx++
		}
		if s.name == r.lastWinner {
			first = i
		}
	}
	for _, m := range r.members {
		m.seat = -1
	}
	for _, s := range r.seats {
		if m := r.memberByID(s.memberID); m != nil {
			m.seat = s.idx
		}
	}

	cfg := engine.Config{Players: len(r.seats), Magistrate: r.settings.Magistrate, Seed: r.rng.Uint64(), First: first}
	g, err := engine.New(cfg)
	if err != nil {
		return err
	}
	r.game = g
	r.gameNo++
	r.started = time.Now()
	r.replay = protocol.Replay{Format: protocol.ReplayFormat, Created: r.started.UTC(), Room: r.code, Config: cfg, Seats: r.seatInfo(), Winner: -1}
	r.timerKey = ""
	r.log.Info("game started", "game", r.gameNo, "seats", len(r.seats), "bots", bots)
	for _, m := range r.members {
		r.sendGameStart(m)
	}
	r.broadcastLobby()
	r.afterChange(g.StartEvents())
	return nil
}

func (r *Room) seatInfo() []protocol.Seat {
	out := make([]protocol.Seat, len(r.seats))
	for i, s := range r.seats {
		out[i] = protocol.Seat{Seat: i, Name: s.name, ID: s.memberID, Bot: s.bot != nil}
		if s.bot != nil {
			out[i].Level = s.level.String()
		}
	}
	return out
}

func (r *Room) sendGameStart(m *member) {
	if m.cl == nil || r.game == nil {
		return
	}
	m.cl.send(protocol.MustEncode(protocol.TGameStart, protocol.GameStart{
		Game: r.gameNo, Seats: r.seatInfo(), YouSeat: m.seat, Settings: r.settings,
	}))
}

// apply runs one intent through the engine and fans out the result.
func (r *Room) apply(seatIdx int, in engine.Intent) error {
	evs, err := r.game.Apply(seatIdx, in)
	if err != nil {
		return err
	}
	r.replay.Steps = append(r.replay.Steps, protocol.Step{Seat: seatIdx, Intent: in, AtMS: time.Since(r.started).Milliseconds()})
	r.afterChange(evs)
	return nil
}

func (r *Room) afterChange(evs []engine.Event) {
	// Everyone sees the events first, each redacted for their own seat.
	for _, m := range r.members {
		if m.cl == nil {
			continue
		}
		m.cl.send(protocol.MustEncode(protocol.TEvents, protocol.Events{Events: engine.RedactAll(evs, m.seat)}))
	}
	for _, s := range r.seats {
		if s.bot == nil {
			continue
		}
		for _, e := range evs {
			s.bot.Observe(e.Redact(s.idx))
		}
		if line, ok := s.bot.Taunt(); ok {
			r.broadcast(protocol.MustEncode(protocol.TChat, protocol.Chat{Name: s.name, Seat: s.idx, Text: line, Bot: true}))
		}
	}
	if r.game.Over() {
		r.finishGame()
		return
	}
	r.updateTimer()
	r.broadcastState()
	r.schedule()
}

func (r *Room) finishGame() {
	g := r.game
	w := g.Winner()
	r.replay.Winner = w
	names := make([]string, len(r.seats))
	for i, s := range r.seats {
		names[i] = s.name
	}
	r.lastWinner = names[w]
	r.broadcastState()
	r.broadcast(protocol.MustEncode(protocol.TGameOver, protocol.GameOver{Winner: w, Names: names, Replay: r.replay}))
	r.log.Info("game over", "game", r.gameNo, "winner", names[w], "turns", g.Turn())
	r.resetToLobby()
}

func (r *Room) abortGame() {
	if r.game == nil {
		return
	}
	r.resetToLobby()
}

func (r *Room) resetToLobby() {
	r.game = nil
	r.seats = nil
	r.stopTimer()
	var gone []*member
	for _, m := range r.members {
		m.seat = -1
		m.ready = false
		if m.cl == nil {
			gone = append(gone, m)
		}
	}
	for _, m := range gone {
		r.removeMember(m)
	}
	r.enforceCapacity()
	r.broadcastLobby()
}

// --- timers and bots -------------------------------------------------------

// decisionKey identifies the decision the game is waiting on. Passes inside
// a window don't change it; a new window, turn or choice does.
func (r *Room) decisionKey() string {
	v := r.game.View(-1)
	key := fmt.Sprintf("%d/%s/%d", v.Turn, v.Phase, v.Loser)
	if p := v.Pending; p != nil {
		key += fmt.Sprintf("/%d/%t/%t", p.Blocker, p.ActionChallenged, p.BlockChallenged)
	}
	if c := v.Challenge; c != nil {
		key += fmt.Sprintf("/c%d", c.Challenger)
	}
	return key
}

func (r *Room) stopTimer() {
	if r.timer != nil {
		r.timer.Stop()
		r.timer = nil
	}
	r.timerGen++
	r.timerKey = ""
	r.deadline = time.Time{}
	r.timerTotal = 0
}

func (r *Room) updateTimer() {
	key := r.decisionKey()
	if key == r.timerKey {
		return
	}
	r.stopTimer()
	r.timerKey = key
	var d time.Duration
	if r.game.Phase().Timed() {
		d = time.Duration(r.settings.WindowMS) * time.Millisecond
	} else if r.settings.TurnMS > 0 {
		d = time.Duration(r.settings.TurnMS) * time.Millisecond
	}
	if d <= 0 {
		return
	}
	r.deadline = time.Now().Add(d)
	r.timerTotal = d
	gen := r.timerGen
	r.timer = time.AfterFunc(d, func() { r.post(timerMsg{gen: gen}) })
}

func (r *Room) onTimer(gen int) {
	if gen != r.timerGen || r.game == nil {
		return
	}
	key := r.timerKey
	// Time's up: everyone still pending gets the default (pass, or the
	// safest choice for a decision).
	for r.game != nil && !r.game.Over() && r.decisionKey() == key {
		aw := r.game.Awaiting()
		if len(aw) == 0 {
			return
		}
		in, ok := r.game.DefaultIntent(aw[0])
		if !ok {
			return
		}
		if r.seats[aw[0]].bot == nil && !r.game.Phase().Timed() {
			r.system(r.seats[aw[0]].name + " ran out of time")
		}
		if err := r.apply(aw[0], in); err != nil {
			r.log.Error("default intent refused", "err", err)
			return
		}
	}
}

func (r *Room) think() time.Duration {
	lo, hi := r.srv.opts.ThinkMin, r.srv.opts.ThinkMax
	if hi <= lo {
		return lo
	}
	return lo + time.Duration(r.rng.Int64N(int64(hi-lo)))
}

// schedule gives every bot that is being waited on a decision tick.
func (r *Room) schedule() {
	if r.game == nil || r.game.Over() {
		return
	}
	key := r.decisionKey()
	for _, s := range r.seats {
		if s.bot == nil || !r.game.IsAwaiting(s.idx) || s.botKey == key {
			continue
		}
		s.botKey = key
		idx := s.idx
		time.AfterFunc(r.think(), func() { r.post(botMsg{seat: idx, key: key}) })
	}
}

func (r *Room) onBot(idx int, key string) {
	if r.game == nil || r.game.Over() || idx >= len(r.seats) {
		return
	}
	s := r.seats[idx]
	if s.bot == nil || !r.game.IsAwaiting(idx) || r.decisionKey() != key {
		return
	}
	in := s.bot.Decide(r.game.View(idx))
	if err := r.apply(idx, in); err != nil {
		r.log.Error("bot intent refused", "seat", idx, "intent", in.String(), "err", err)
		if d, ok := r.game.DefaultIntent(idx); ok {
			_ = r.apply(idx, d)
		}
	}
}

// --- broadcasting ----------------------------------------------------------

func (r *Room) broadcast(frame []byte) {
	for _, m := range r.members {
		if m.cl != nil {
			m.cl.send(frame)
		}
	}
}

func (r *Room) system(text string) {
	r.broadcast(protocol.MustEncode(protocol.TChat, protocol.Chat{Seat: -1, Text: text, System: true}))
}

func (r *Room) lobbyMsg() protocol.Lobby {
	l := protocol.Lobby{Room: r.code, Settings: r.settings, InGame: r.game != nil}
	for _, m := range r.members {
		if m.host {
			l.HostID = m.id
		}
		l.Members = append(l.Members, protocol.Member{
			ID: m.id, Name: m.name, Ready: m.ready, Spectator: m.spectate || (r.game != nil && m.seat < 0),
			Connected: m.cl != nil, Host: m.host,
		})
	}
	return l
}

func (r *Room) broadcastLobby() {
	r.broadcast(protocol.MustEncode(protocol.TLobby, r.lobbyMsg()))
}

func (r *Room) stateFor(seatIdx int) protocol.State {
	st := protocol.State{View: r.game.View(seatIdx), Seats: r.seatInfo()}
	if !r.deadline.IsZero() {
		st.DeadlineMS = max(0, time.Until(r.deadline).Milliseconds())
		st.TimerMS = r.timerTotal.Milliseconds()
	}
	for _, s := range r.seats {
		if s.offline {
			st.Offline = append(st.Offline, s.idx)
		}
	}
	return st
}

func (r *Room) sendState(m *member) {
	if m.cl == nil || r.game == nil {
		return
	}
	m.cl.send(protocol.MustEncode(protocol.TState, r.stateFor(m.seat)))
}

func (r *Room) broadcastState() {
	if r.game == nil {
		return
	}
	for _, m := range r.members {
		r.sendState(m)
	}
}
