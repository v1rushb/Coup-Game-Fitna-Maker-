// Package protocol defines the versioned wire format between NEON//COUP
// clients and hosts. Every frame is a JSON Envelope whose Type selects the
// payload struct below.
package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/v1rushb/neoncoup/engine"
)

// Version is bumped on any incompatible change. Hosts reject clients whose
// Hello carries a different version. v2 added presentation pacing (speed,
// deferred decisions) and the solo control message.
const Version = 2

// MaxFrame is the largest frame either side accepts.
const MaxFrame = 64 << 10

// Message types, client to server.
const (
	THello    = "hello"
	TReady    = "ready"
	TSettings = "settings"
	TStart    = "start"
	TIntent   = "intent"
	TChat     = "chat"
	TEmote    = "emote"
	TLeave    = "leave"
	TPing     = "ping"
	TControl  = "control"
)

// Message types, server to client. TChat and TEmote are used both ways.
const (
	TWelcome   = "welcome"
	TReject    = "reject"
	TLobby     = "lobby"
	TGameStart = "game_start"
	TState     = "state"
	TEvents    = "events"
	TError     = "error"
	TGameOver  = "game_over"
	TPong      = "pong"
)

// Envelope wraps every frame.
type Envelope struct {
	Type string          `json:"t"`
	Data json.RawMessage `json:"d,omitempty"`
}

// Encode marshals a payload into an envelope frame.
func Encode(typ string, payload any) ([]byte, error) {
	env := Envelope{Type: typ}
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		env.Data = raw
	}
	return json.Marshal(env)
}

// MustEncode is Encode for payloads that cannot fail to marshal.
func MustEncode(typ string, payload any) []byte {
	b, err := Encode(typ, payload)
	if err != nil {
		panic(fmt.Sprintf("protocol: encode %s: %v", typ, err))
	}
	return b
}

// Decode parses a frame's envelope.
func Decode(frame []byte) (Envelope, error) {
	if len(frame) > MaxFrame {
		return Envelope{}, errors.New("protocol: frame too large")
	}
	var env Envelope
	if err := json.Unmarshal(frame, &env); err != nil {
		return Envelope{}, fmt.Errorf("protocol: bad frame: %w", err)
	}
	if env.Type == "" {
		return Envelope{}, errors.New("protocol: frame has no type")
	}
	return env, nil
}

// Into unmarshals the envelope payload.
func (e Envelope) Into(v any) error {
	if len(e.Data) == 0 {
		return nil
	}
	if err := json.Unmarshal(e.Data, v); err != nil {
		return fmt.Errorf("protocol: bad %s payload: %w", e.Type, err)
	}
	return nil
}

// --- client to server ------------------------------------------------------

// Hello is the first frame a client sends.
type Hello struct {
	Proto    int    `json:"proto"`
	Client   string `json:"client"`
	Name     string `json:"name"`
	Token    string `json:"token,omitempty"`
	Room     string `json:"room,omitempty"`
	Spectate bool   `json:"spectate,omitempty"`
}

// Ready toggles lobby readiness.
type Ready struct {
	Ready bool `json:"ready"`
}

// Settings are the host's room options.
type Settings struct {
	MaxPlayers int    `json:"max_players"`
	FillBots   bool   `json:"fill_bots"`
	BotLevel   string `json:"bot_level"`
	// WindowMS is the reaction window; 0 picks it from the speed.
	WindowMS   int  `json:"window_ms"`
	TurnMS     int  `json:"turn_ms"`
	Magistrate bool `json:"magistrate,omitempty"`
	ReconnectS int  `json:"reconnect_s"`
	// Speed paces the presentation of events: relaxed, normal, fast, or
	// instant (no presentation; tests and simulations).
	Speed string `json:"speed"`
}

// DefaultSettings are used for new rooms.
func DefaultSettings() Settings {
	return Settings{MaxPlayers: 4, FillBots: true, BotLevel: "normal", TurnMS: 60000, ReconnectS: 60, Speed: SpeedRelaxed}
}

// Window is the reaction window length.
func (s Settings) Window() time.Duration {
	if s.WindowMS > 0 {
		return time.Duration(s.WindowMS) * time.Millisecond
	}
	return AutoWindow(s.Speed)
}

// Clamp forces settings into their legal ranges.
func (s Settings) Clamp() Settings {
	s.MaxPlayers = min(max(s.MaxPlayers, engine.MinPlayers), engine.MaxPlayers)
	if s.WindowMS != 0 {
		s.WindowMS = min(max(s.WindowMS, 2000), 60000)
	}
	s.Speed = ParseSpeed(s.Speed)
	if s.TurnMS != 0 {
		s.TurnMS = min(max(s.TurnMS, 10000), 600000)
	}
	s.ReconnectS = min(max(s.ReconnectS, 5), 600)
	switch s.BotLevel {
	case "easy", "normal", "hard":
	default:
		s.BotLevel = "normal"
	}
	return s
}

// IntentMsg submits a game decision. Ver must equal the version of the
// latest state the client saw.
type IntentMsg struct {
	Ver    uint64        `json:"ver"`
	Intent engine.Intent `json:"intent"`
}

// Control steers a game's presentation. The host honors it only from the one
// human seated at an all-bot table (solo play), since it changes everyone's
// clock.
type Control struct {
	// Pause freezes bots and timers (true) or resumes them (false).
	Pause *bool `json:"pause,omitempty"`
	// SkipMS is how much presentation the client just skipped.
	SkipMS int `json:"skip_ms,omitempty"`
	// Speed changes the room speed mid-game.
	Speed string `json:"speed,omitempty"`
}

// Ping and Pong measure latency and keep connections alive.
type Ping struct {
	T int64 `json:"t"`
}

// --- server to client ------------------------------------------------------

// Welcome accepts a Hello.
type Welcome struct {
	Proto int    `json:"proto"`
	You   string `json:"you"`
	Token string `json:"token"`
	Room  string `json:"room"`
	Host  bool   `json:"host"`
	// Rejoined is set when the token reclaimed a seat in a running game.
	Rejoined bool `json:"rejoined,omitempty"`
}

// Reject codes.
const (
	RejectVersion  = "version"
	RejectFull     = "full"
	RejectNotFound = "not_found"
	RejectBadHello = "bad_hello"
	RejectClosed   = "closed"
)

// Reject refuses a connection. The server closes it afterwards.
type Reject struct {
	Code   string `json:"code"`
	Reason string `json:"reason"`
}

// Member is one lobby entry.
type Member struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Ready     bool   `json:"ready,omitempty"`
	Bot       bool   `json:"bot,omitempty"`
	Level     string `json:"level,omitempty"`
	Spectator bool   `json:"spectator,omitempty"`
	Connected bool   `json:"connected"`
	Host      bool   `json:"host,omitempty"`
}

// Lobby is the room roster and settings.
type Lobby struct {
	Room     string   `json:"room"`
	Members  []Member `json:"members"`
	Settings Settings `json:"settings"`
	HostID   string   `json:"host_id"`
	InGame   bool     `json:"in_game,omitempty"`
}

// Seat describes who sits where in a game.
type Seat struct {
	Seat  int    `json:"seat"`
	Name  string `json:"name"`
	ID    string `json:"id,omitempty"`
	Bot   bool   `json:"bot,omitempty"`
	Level string `json:"level,omitempty"`
}

// GameStart announces a new game.
type GameStart struct {
	Game     int      `json:"game"`
	Seats    []Seat   `json:"seats"`
	YouSeat  int      `json:"you_seat"`
	Settings Settings `json:"settings"`
}

// State is a redacted view plus the active timer, if any.
type State struct {
	View engine.PlayerView `json:"view"`
	// DeadlineMS is the time left on the current timer, 0 for none.
	DeadlineMS int64 `json:"deadline_ms,omitempty"`
	// TimerMS is the timer's full length, for drawing a progress bar.
	TimerMS int64 `json:"timer_ms,omitempty"`
	// Seats is who controls each seat now (bots take over dropped players).
	Seats []Seat `json:"seats,omitempty"`
	// Offline lists seats whose human is disconnected and being held.
	Offline []int `json:"offline,omitempty"`
	// OpensInMS is how long until the decision being waited on opens: the
	// host holds timers, bots and early intents until every client has had
	// time to play the events in front of it.
	OpensInMS int64 `json:"opens_in_ms,omitempty"`
	// Speed is the room's presentation speed.
	Speed string `json:"speed,omitempty"`
	// Paused is set while solo play is paused.
	Paused bool `json:"paused,omitempty"`
}

// Events carries engine events redacted for the receiver.
type Events struct {
	Events []engine.Event `json:"events"`
}

// Chat is a chat line. Seat is -1 for system lines and spectators.
type Chat struct {
	From   string `json:"from,omitempty"`
	Name   string `json:"name,omitempty"`
	Seat   int    `json:"seat"`
	Text   string `json:"text"`
	System bool   `json:"system,omitempty"`
	Bot    bool   `json:"bot,omitempty"`
}

// Emote is a quick taunt. ID indexes Emotes.
type Emote struct {
	From string `json:"from,omitempty"`
	Name string `json:"name,omitempty"`
	Seat int    `json:"seat"`
	ID   int    `json:"id"`
}

// Emotes are the quick taunts players can fire with the taunt menu.
var Emotes = []string{
	"gg, choom",
	"nice bluff... or was it?",
	"i have the Duke. obviously.",
	"your firewall is showing",
	"say goodbye to your credits",
	"challenge me. i dare you.",
	"pure static",
	"see you in the next run",
}

// Error codes.
const (
	ErrIllegal   = "illegal"
	ErrStale     = "stale"
	ErrRateLimit = "rate_limit"
	ErrNotHost   = "not_host"
	ErrNotReady  = "not_ready"
	ErrBadMsg    = "bad_message"
	ErrNoSeat    = "no_seat"
)

// Error reports a refused request. The connection stays open.
type Error struct {
	Code string `json:"code"`
	Msg  string `json:"msg"`
}

// GameOver ends a game. The replay is sent now that nothing is secret.
type GameOver struct {
	Winner int      `json:"winner"`
	Names  []string `json:"names"`
	Replay Replay   `json:"replay"`
}

// Replay is enough to re-run a game exactly: the engine is deterministic
// given its Config (which holds the seed) and the sequence of intents.
type Replay struct {
	Format  int           `json:"format"`
	Created time.Time     `json:"created"`
	Room    string        `json:"room,omitempty"`
	Config  engine.Config `json:"config"`
	Seats   []Seat        `json:"seats"`
	Steps   []Step        `json:"steps"`
	Winner  int           `json:"winner"`
}

// ReplayFormat is the current replay file format.
const ReplayFormat = 1

// Step is one applied intent. AtMS is milliseconds since the game started.
type Step struct {
	Seat   int           `json:"seat"`
	Intent engine.Intent `json:"intent"`
	AtMS   int64         `json:"at_ms"`
}
