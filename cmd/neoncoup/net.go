package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/v1rushb/neoncoup/bot"
	"github.com/v1rushb/neoncoup/client"
	"github.com/v1rushb/neoncoup/protocol"
	"github.com/v1rushb/neoncoup/relay"
)

// hostFlags are the room settings a host can pass on the command line.
type hostFlags struct {
	port                 int
	relay                string
	seats                int
	noBots               bool
	level, speed         string
	window, turn, hold   int
	magistrate, headless bool
}

func (h *hostFlags) register(fs interface {
	IntVar(*int, string, int, string)
	StringVar(*string, string, string, string)
	BoolVar(*bool, string, bool, string)
}) {
	fs.IntVar(&h.port, "port", 0, "TCP port to listen on (default from config, 7777)")
	fs.StringVar(&h.relay, "relay", "", "relay URL for players behind NAT (wss://...)")
	fs.IntVar(&h.seats, "seats", 0, "seats at the table, 2-6 (default 4)")
	fs.BoolVar(&h.noBots, "no-bots", false, "leave empty seats empty instead of filling them with bots")
	fs.StringVar(&h.level, "difficulty", "", "bot difficulty: easy, normal, hard")
	fs.IntVar(&h.window, "window", 0, "challenge/block window in seconds (default: by speed, 12 at relaxed)")
	fs.StringVar(&h.speed, "speed", "", "presentation speed: relaxed, normal, fast (default from config, relaxed)")
	fs.IntVar(&h.turn, "turn", -1, "turn timer in seconds; 0 turns it off, -1 uses the config (60)")
	fs.IntVar(&h.hold, "hold", 0, "seconds a dropped player's seat is held before a bot takes over (default 60)")
	fs.BoolVar(&h.magistrate, "magistrate", false, "play with the Magistrate variant")
	fs.BoolVar(&h.headless, "headless", false, "run a dedicated room with no local player or UI")
}

func (h *hostFlags) settings(e *env) (protocol.Settings, error) {
	st := protocol.DefaultSettings()
	st.BotLevel = e.cfg.BotLevel
	st.WindowMS = e.cfg.WindowMS
	st.TurnMS = e.cfg.TurnMS
	st.Magistrate = e.cfg.Magistrate || h.magistrate
	st.Speed = e.cfg.Speed
	if h.speed != "" {
		if protocol.ParseSpeed(h.speed) != h.speed {
			return st, fmt.Errorf("unknown speed %q (relaxed, normal or fast)", h.speed)
		}
		st.Speed = h.speed
	}
	if h.seats != 0 {
		st.MaxPlayers = h.seats
	}
	st.FillBots = !h.noBots
	if h.level != "" {
		l, err := bot.ParseLevel(h.level)
		if err != nil {
			return st, err
		}
		st.BotLevel = l.String()
	}
	if h.window > 0 {
		st.WindowMS = h.window * 1000
	}
	if h.turn >= 0 {
		st.TurnMS = h.turn * 1000
	}
	if h.hold > 0 {
		st.ReconnectS = h.hold
	}
	return st.Clamp(), nil
}

func cmdHost(args []string) error {
	var g globals
	var hf hostFlags
	fs := newFlags("host", &g)
	hf.register(fs)
	if _, err := parse(fs, args); err != nil {
		return err
	}
	e, err := setup(&g)
	if err != nil {
		return err
	}
	st, err := hf.settings(e)
	if err != nil {
		return err
	}
	port := e.cfg.Port
	if hf.port != 0 {
		port = hf.port
	}
	relayURL := hf.relay
	if relayURL == "" {
		relayURL = firstNonEmpty(e.cfg.Relay, os.Getenv("NEONCOUP_RELAY"))
	}
	opts := client.HostOptions{Settings: st, Port: port, Relay: relayURL}

	if hf.headless {
		e.sound.Close()
		log := slog.New(slog.NewTextHandler(os.Stderr, nil))
		h, err := client.OpenRoom(opts, log)
		if err != nil {
			return err
		}
		defer h.Close()
		printHostInfo(h.Info)
		fmt.Println("dedicated room: the first player to join is the table host. ctrl+c to stop.")
		waitForSignal()
		return nil
	}

	be := localBackend(e, relayURL)
	conn, err := be.Host(opts)
	if err != nil {
		e.sound.Close()
		return err
	}
	printHostInfo(conn.Host)
	return runTUI(e, &g, client.Start{Kind: client.StartHost, Conn: conn}, be, nil)
}

func printHostInfo(info *client.HostInfo) {
	fmt.Printf("\n  NEON//COUP room open: %s\n\n", info.Room)
	for _, l := range info.Lines {
		fmt.Println("  " + l)
	}
	fmt.Println()
}

func cmdJoin(args []string) error {
	var g globals
	fs := newFlags("join", &g)
	relayURL := fs.String("relay", "", "relay URL to use when the room is not on the LAN")
	spectate := fs.Bool("spectate", false, "watch instead of taking a seat")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return errors.New("usage: neoncoup join NEON-XXXX | HOST:PORT [--relay URL] [--spectate]")
	}
	e, err := setup(&g)
	if err != nil {
		return err
	}
	be := localBackend(e, *relayURL)
	return runTUI(e, &g, client.Start{Kind: client.StartJoin, Join: pos[0]}, be, func(o *client.Options) { o.Spectate = *spectate })
}

func cmdRelay(args []string) error {
	fs := newFlags("relay", &globals{})
	addr := fs.String("addr", ":8080", "listen address")
	maxRooms := fs.Int("max-rooms", 1000, "maximum registered rooms")
	maxPlayers := fs.Int("max-players", 16, "maximum relayed players per room")
	if _, err := parse(fs, args); err != nil {
		return err
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	r := relay.New(relay.Options{Logger: log, MaxRooms: *maxRooms, MaxPlayers: *maxPlayers})
	srv := &http.Server{Addr: *addr, Handler: r.Handler(), ReadHeaderTimeout: 10 * time.Second}
	errc := make(chan error, 1)
	go func() { errc <- srv.ListenAndServe() }()
	log.Info("relay listening", "addr", *addr, "protocol", protocol.Version)
	fmt.Fprintf(os.Stderr, "hosts: neoncoup host --relay ws://THIS-HOST%s (use wss:// behind TLS)\n", *addr)
	select {
	case err := <-errc:
		return err
	case <-signalChan():
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return srv.Shutdown(ctx)
}

func signalChan() <-chan os.Signal {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	return c
}

func waitForSignal() { <-signalChan() }

func firstNonEmpty(s ...string) string {
	for _, v := range s {
		if v != "" {
			return v
		}
	}
	return ""
}
