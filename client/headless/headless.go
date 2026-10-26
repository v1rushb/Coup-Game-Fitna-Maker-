// Package headless is a network client with no UI: a bot brain that speaks
// the wire protocol like any other player. It is used for integration tests
// and to fill seats on remote hosts.
package headless

import (
	"context"
	"fmt"
	"time"

	"github.com/v1rushb/neoncoup/bot"
	"github.com/v1rushb/neoncoup/engine"
	"github.com/v1rushb/neoncoup/protocol"
	"github.com/v1rushb/neoncoup/transport"
)

// Options configure a headless player.
type Options struct {
	Name  string
	Room  string
	Level bot.Level
	Seed  uint64
	// StartWhen makes this client (if it is the host) start the game once
	// this many ready players are in the lobby. 0 never starts.
	StartWhen int
	// Settings, when set and this client is the host, are sent before starting.
	Settings *protocol.Settings
}

// Result describes the finished game from this client's seat.
type Result struct {
	Seat   int
	Winner int
	Names  []string
	Replay protocol.Replay
	Frames int
}

// Won reports whether this client won.
func (r Result) Won() bool { return r.Seat >= 0 && r.Seat == r.Winner }

// Play joins, readies up, plays one game and returns when it ends.
func Play(ctx context.Context, conn transport.Conn, opts Options) (Result, error) {
	defer func() { _ = conn.Close() }()
	send := func(typ string, v any) error {
		sctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		return conn.Send(sctx, protocol.MustEncode(typ, v))
	}
	if err := send(protocol.THello, protocol.Hello{Proto: protocol.Version, Client: "neoncoup-headless", Name: opts.Name, Room: opts.Room}); err != nil {
		return Result{}, err
	}

	frames := make(chan protocol.Envelope, 1024)
	readErr := make(chan error, 1)
	go func() {
		for {
			f, err := conn.Recv(ctx)
			if err != nil {
				readErr <- err
				return
			}
			env, err := protocol.Decode(f)
			if err != nil {
				readErr <- err
				return
			}
			frames <- env
		}
	}()

	var (
		res      = Result{Seat: -1, Winner: -1}
		host     bool
		brain    bot.Bot
		latest   *protocol.State
		answered = ^uint64(0)
		retry    <-chan time.Time
		started  bool
	)
	answer := func() error {
		if brain == nil || latest == nil || len(latest.View.Legal) == 0 || latest.View.Version == answered {
			return nil
		}
		answered = latest.View.Version
		in := brain.Decide(latest.View)
		return send(protocol.TIntent, protocol.IntentMsg{Ver: latest.View.Version, Intent: in})
	}
	for {
		var env protocol.Envelope
		select {
		case <-ctx.Done():
			return res, ctx.Err()
		case err := <-readErr:
			return res, fmt.Errorf("connection lost: %w", err)
		case <-retry:
			retry = nil
			answered = ^uint64(0)
			if err := answer(); err != nil {
				return res, err
			}
			continue
		case env = <-frames:
		}
		res.Frames++
		switch env.Type {
		case protocol.TReject:
			var r protocol.Reject
			_ = env.Into(&r)
			return res, fmt.Errorf("rejected (%s): %s", r.Code, r.Reason)
		case protocol.TWelcome:
			var w protocol.Welcome
			_ = env.Into(&w)
			host = w.Host
			if host && opts.Settings != nil {
				if err := send(protocol.TSettings, opts.Settings); err != nil {
					return res, err
				}
			}
			if err := send(protocol.TReady, protocol.Ready{Ready: true}); err != nil {
				return res, err
			}
		case protocol.TLobby:
			var l protocol.Lobby
			_ = env.Into(&l)
			if host && !started && !l.InGame && opts.StartWhen > 0 && readyPlayers(l) >= opts.StartWhen {
				started = true
				if err := send(protocol.TStart, nil); err != nil {
					return res, err
				}
			}
		case protocol.TGameStart:
			var g protocol.GameStart
			_ = env.Into(&g)
			res.Seat = g.YouSeat
			if g.YouSeat >= 0 {
				brain = bot.New(opts.Level, g.YouSeat, opts.Seed, bot.Personality{Handle: opts.Name, Bluff: 1, Suspicion: 1, Aggression: 1})
				// The bot learns the table size from the opening events.
				brain.Observe(engine.Event{Kind: engine.EvGameStart, Seat: -1, Other: -1, Amount: len(g.Seats), Turn: 1})
			}
		case protocol.TEvents:
			var e protocol.Events
			_ = env.Into(&e)
			for _, ev := range e.Events {
				if brain != nil && ev.Kind != engine.EvGameStart {
					brain.Observe(ev)
				}
			}
		case protocol.TState:
			var s protocol.State
			if err := env.Into(&s); err != nil {
				return res, err
			}
			latest = &s
		case protocol.TError:
			var e protocol.Error
			_ = env.Into(&e)
			if e.Code == protocol.ErrRateLimit {
				retry = time.After(250 * time.Millisecond)
			}
		case protocol.TGameOver:
			var g protocol.GameOver
			if err := env.Into(&g); err != nil {
				return res, err
			}
			res.Winner = g.Winner
			res.Names = g.Names
			res.Replay = g.Replay
			_ = send(protocol.TLeave, nil)
			return res, nil
		}
		// Answer only the newest state, once the queue is drained.
		if len(frames) == 0 {
			if err := answer(); err != nil {
				return res, err
			}
		}
	}
}

func readyPlayers(l protocol.Lobby) int {
	n := 0
	for _, m := range l.Members {
		if !m.Spectator && m.Connected && (m.Ready || m.Host) {
			n++
		}
	}
	return n
}
