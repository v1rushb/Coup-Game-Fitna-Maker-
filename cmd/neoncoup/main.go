// Command neoncoup is NEON//COUP: a multiplayer terminal bluffing game.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/v1rushb/neoncoup/audio"
	"github.com/v1rushb/neoncoup/bot"
	"github.com/v1rushb/neoncoup/client"
	"github.com/v1rushb/neoncoup/config"
	"github.com/v1rushb/neoncoup/protocol"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "1.0.0"

const usage = `NEON//COUP — a bluffing game for the terminal

usage:
  neoncoup                         main menu
  neoncoup solo [flags]            quick game against bots
  neoncoup tutorial                interactive tutorial
  neoncoup host [flags]            open a room and print its code
  neoncoup join CODE|HOST:PORT     join a room (LAN discovery first, then relay)
  neoncoup relay [flags]           run a matchmaking/forwarding relay
  neoncoup serve-ssh [flags]       let players join with ssh, no install
  neoncoup replay [FILE]           step through a saved game
  neoncoup stats                   your record
  neoncoup version                 print the version

global flags (any command):
  --ascii         plain ASCII instead of Unicode box drawing
  --no-audio      disable sound
  --name NAME     your nickname
  --config DIR    config directory (default: OS config dir; env NEONCOUP_HOME)
  --skip-boot     skip the boot sequence
  --crt           CRT scanlines

run "neoncoup COMMAND -h" for command flags.
`

// globals are flags every command accepts.
type globals struct {
	ascii, noAudio, skipBoot, crt bool
	name, configDir               string
}

func (g *globals) register(fs *flag.FlagSet) {
	fs.BoolVar(&g.ascii, "ascii", false, "plain ASCII instead of Unicode box drawing")
	fs.BoolVar(&g.noAudio, "no-audio", false, "disable sound")
	fs.BoolVar(&g.skipBoot, "skip-boot", false, "skip the boot sequence")
	fs.BoolVar(&g.crt, "crt", false, "CRT scanlines")
	fs.StringVar(&g.name, "name", "", "your nickname")
	fs.StringVar(&g.configDir, "config", "", "config directory")
}

func main() {
	if err := run(os.Args[1:]); err != nil && !errors.Is(err, flag.ErrHelp) {
		fmt.Fprintln(os.Stderr, "neoncoup:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	cmd := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		cmd, args = args[0], args[1:]
	}
	switch cmd {
	case "", "menu", "play":
		return cmdMenu(args)
	case "solo":
		return cmdSolo(args)
	case "tutorial":
		return cmdTutorial(args)
	case "host":
		return cmdHost(args)
	case "join":
		return cmdJoin(args)
	case "relay":
		return cmdRelay(args)
	case "serve-ssh", "ssh":
		return cmdServeSSH(args)
	case "replay", "replays":
		return cmdReplay(args)
	case "stats":
		return cmdStats(args)
	case "version", "--version":
		fmt.Printf("neoncoup %s (protocol v%d)\n", version, protocol.Version)
		return nil
	case "help", "-h", "--help":
		fmt.Print(usage)
		return nil
	}
	fmt.Fprint(os.Stderr, usage)
	return fmt.Errorf("unknown command %q", cmd)
}

// parse handles flags that may appear before or after positional arguments.
func parse(fs *flag.FlagSet, args []string) ([]string, error) {
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		args = fs.Args()
		if len(args) == 0 {
			return pos, nil
		}
		pos = append(pos, args[0])
		args = args[1:]
	}
}

func newFlags(name string, g *globals) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	g.register(fs)
	return fs
}

// env is everything a TUI command needs.
type env struct {
	store *config.Store
	cfg   config.Config
	sound audio.Player
}

func setup(g *globals) (*env, error) {
	store, err := config.Open(g.configDir)
	if err != nil {
		return nil, err
	}
	cfg := store.Load()
	if g.name != "" {
		if n := protocol.CleanName(g.name); n != "" {
			cfg.Name = n
		}
	}
	if g.ascii {
		cfg.ASCII = true
	}
	if g.crt {
		cfg.CRT = true
	}
	if g.skipBoot {
		cfg.SkipBoot = true
	}
	var sound audio.Player = audio.Nop{}
	if !g.noAudio {
		sound = openAudio(cfg)
	}
	return &env{store: store, cfg: cfg, sound: sound}, nil
}

// runTUI starts the Bubble Tea program.
func runTUI(e *env, g *globals, start client.Start, be client.Backend, tweak func(*client.Options)) error {
	defer e.sound.Close()
	opts := client.Options{
		Backend: be,
		Store:   e.store,
		Config:  e.cfg,
		Sound:   e.sound,
		ASCII:   g.ascii,
		Start:   start,
		Version: version,
	}
	if tweak != nil {
		tweak(&opts)
	}
	app := client.New(opts)
	p := tea.NewProgram(app, tea.WithFPS(30))
	_, err := p.Run()
	be.Close()
	return err
}

func localBackend(e *env, relay string) *client.LocalBackend {
	if relay == "" {
		relay = e.cfg.Relay
	}
	if relay == "" {
		relay = os.Getenv("NEONCOUP_RELAY")
	}
	return &client.LocalBackend{Relay: relay, Logger: fileLogger(e.store)}
}

// fileLogger writes server logs next to the config, so they never paint
// over the TUI.
func fileLogger(store *config.Store) *slog.Logger {
	f, err := os.OpenFile(filepath.Join(store.Dir(), "neoncoup.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return slog.New(slog.NewTextHandler(f, &slog.HandlerOptions{Level: slog.LevelInfo}))
}

func cmdMenu(args []string) error {
	var g globals
	fs := newFlags("neoncoup", &g)
	if _, err := parse(fs, args); err != nil {
		return err
	}
	e, err := setup(&g)
	if err != nil {
		return err
	}
	return runTUI(e, &g, client.Start{Kind: client.StartMenu}, localBackend(e, ""), nil)
}

func cmdSolo(args []string) error {
	var g globals
	fs := newFlags("solo", &g)
	bots := fs.Int("bots", 0, "number of bot opponents (1-5)")
	level := fs.String("difficulty", "", "bot difficulty: easy, normal, hard")
	magistrate := fs.Bool("magistrate", false, "play with the Magistrate variant")
	window := fs.Int("window", 0, "reaction window in seconds")
	if _, err := parse(fs, args); err != nil {
		return err
	}
	e, err := setup(&g)
	if err != nil {
		return err
	}
	o := client.SoloOptions{Bots: e.cfg.Bots, Level: e.cfg.BotLevel, Magistrate: e.cfg.Magistrate || *magistrate, WindowMS: e.cfg.WindowMS}
	if *bots != 0 {
		if *bots < 1 || *bots > 5 {
			return errors.New("--bots must be between 1 and 5")
		}
		o.Bots = *bots
	}
	if *level != "" {
		l, err := bot.ParseLevel(*level)
		if err != nil {
			return err
		}
		o.Level = l.String()
	}
	if *window > 0 {
		o.WindowMS = *window * 1000
	}
	return runTUI(e, &g, client.Start{Kind: client.StartSolo, Solo: o}, localBackend(e, ""), nil)
}

func cmdTutorial(args []string) error {
	var g globals
	fs := newFlags("tutorial", &g)
	if _, err := parse(fs, args); err != nil {
		return err
	}
	e, err := setup(&g)
	if err != nil {
		return err
	}
	return runTUI(e, &g, client.Start{Kind: client.StartTutorial}, localBackend(e, ""), nil)
}

func cmdReplay(args []string) error {
	var g globals
	fs := newFlags("replay", &g)
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	e, err := setup(&g)
	if err != nil {
		return err
	}
	g.skipBoot = true
	e.cfg.SkipBoot = true
	if len(pos) == 0 {
		return listReplays(e)
	}
	if _, err := config.LoadReplay(pos[0]); err != nil {
		return err
	}
	return runTUI(e, &g, client.Start{Kind: client.StartReplay, Replay: pos[0]}, localBackend(e, ""), nil)
}

func listReplays(e *env) error {
	items, err := e.store.Replays()
	if err != nil {
		return err
	}
	if len(items) == 0 {
		fmt.Println("no replays yet; finish a game and it is saved automatically")
		return nil
	}
	for _, it := range items {
		fmt.Printf("%s  %d players  %3d moves  winner %-16s %s\n", it.Created.Local().Format("2006-01-02 15:04"), it.Players, it.Turns, it.Winner, it.Path)
	}
	fmt.Println("\nwatch one with: neoncoup replay FILE")
	return nil
}

func cmdStats(args []string) error {
	var g globals
	fs := newFlags("stats", &g)
	if _, err := parse(fs, args); err != nil {
		return err
	}
	e, err := setup(&g)
	if err != nil {
		return err
	}
	e.cfg.SkipBoot = true
	return runTUI(e, &g, client.Start{Kind: client.StartStats}, localBackend(e, ""), nil)
}
