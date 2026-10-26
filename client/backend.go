package client

import (
	"context"
	"time"

	"github.com/v1rushb/neoncoup/protocol"
	"github.com/v1rushb/neoncoup/server"
	"github.com/v1rushb/neoncoup/transport"
)

// SoloOptions configure a game against bots.
type SoloOptions struct {
	Bots       int
	Level      string
	Magistrate bool
	WindowMS   int
	// Instant skips the bots' think delay (used by the tutorial and tests).
	Instant bool
}

// HostOptions configure a hosted room.
type HostOptions struct {
	Settings protocol.Settings
	Port     int
	Relay    string
}

// HostInfo tells the host how others can join.
type HostInfo struct {
	Room  string
	Lines []string
}

// Connection is a live link to a room, however it was made.
type Connection struct {
	Conn transport.Conn
	// Dial reopens the link for token rejoins; nil when not possible.
	Dial Dialer
	Room string
	// Via describes the route, e.g. "LAN 192.168.1.5:7777".
	Via  string
	Host *HostInfo
	// Target is what the player asked to join, used to key rejoin tokens.
	Target string
}

// Backend makes connections for the menus. The local backend runs servers in
// this process; the SSH backend shares one hub among sessions.
type Backend interface {
	Solo(SoloOptions) (*Connection, error)
	Host(HostOptions) (*Connection, error)
	Join(ctx context.Context, target string) (*Connection, error)
	// Close stops anything the backend started.
	Close()
}

// soloServer starts a private in-process server with bots for one player.
func soloServer(o SoloOptions) (*server.Server, *Connection, error) {
	think := server.Options{ThinkMin: 800 * time.Millisecond, ThinkMax: 2500 * time.Millisecond}
	if o.Instant {
		think = server.Options{ThinkMin: 150 * time.Millisecond, ThinkMax: 400 * time.Millisecond}
	}
	srv := server.New(think)
	st := protocol.DefaultSettings()
	st.MaxPlayers = 1 + o.Bots
	st.FillBots = true
	st.BotLevel = o.Level
	st.Magistrate = o.Magistrate
	st.TurnMS = 0 // nobody is waiting on you in solo
	if o.WindowMS > 0 {
		st.WindowMS = o.WindowMS
	}
	room, err := srv.CreateRoom("", st)
	if err != nil {
		srv.Close()
		return nil, nil, err
	}
	a, b := transport.Pipe()
	go srv.ServeConn(b)
	return srv, &Connection{Conn: a, Room: room.Code(), Via: "local"}, nil
}
