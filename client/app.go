// Package client is the NEON//COUP terminal UI.
package client

import (
	"context"
	"fmt"
	"math/rand/v2"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/v1rushb/neoncoup/audio"
	"github.com/v1rushb/neoncoup/client/art"
	"github.com/v1rushb/neoncoup/client/theme"
	"github.com/v1rushb/neoncoup/config"
	"github.com/v1rushb/neoncoup/protocol"
)

// Minimum terminal size.
const (
	MinWidth  = 100
	MinHeight = 30
	fps       = 20
)

// StartKind picks the first screen after boot.
type StartKind uint8

// Start screens.
const (
	StartMenu StartKind = iota
	StartSolo
	StartHost
	StartJoin
	StartTutorial
	StartReplay
	StartStats
)

// Start says what to do once the boot sequence ends.
type Start struct {
	Kind   StartKind
	Solo   SoloOptions
	Host   HostOptions
	Join   string
	Replay string
	// Conn is an already-open connection (the CLI host opens its room
	// before the TUI starts so it can print the join command).
	Conn *Connection
}

// Options configure the app.
type Options struct {
	Backend Backend
	// Store persists settings, stats and replays; nil keeps everything in memory.
	Store   *config.Store
	Config  config.Config
	Sound   audio.Player
	ASCII   bool
	Start   Start
	Version string
	// Remote is set for SSH sessions: no audio, no local files.
	Remote bool
	// Spectate joins as a spectator.
	Spectate bool
}

type screen uint8

const (
	scrBoot screen = iota
	scrMenu
	scrSolo
	scrHost
	scrJoin
	scrConnecting
	scrLobby
	scrGame
	scrStats
	scrReplays
	scrReplay
	scrSettings
	scrError
)

type tickMsg time.Time

type connectedMsg struct {
	conn *Connection
	err  error
	mode string
}

// App is the root Bubble Tea model.
type App struct {
	opts  Options
	t     *theme.Theme
	art   *art.Renderer
	cfg   config.Config
	sound audio.Player

	w, h    int
	now     time.Time
	ticking bool
	frame   int
	rng     *rand.Rand

	scr      screen
	prev     screen
	boot     bootScreen
	menu     menuScreen
	solo     soloSetup
	host     hostSetup
	join     joinScreen
	lobby    lobbyScreen
	game     *gameScreen
	stats    statsScreen
	replays  replayList
	replay   *replayViewer
	settings settingsScreen
	errTitle string
	errText  string
	connText string

	sess     *Session
	conn     *Connection
	mode     string
	welcome  protocol.Welcome
	lobbyMsg protocol.Lobby

	help        bool
	quitConfirm bool
	toast       string
	toastUntil  time.Time
	retrying    int

	// nowPlaying is the corner toast for a new in-game track.
	nowPlaying string
	npUntil    time.Time
}

// audioNoticeMsg carries a track title from the audio engine.
type audioNoticeMsg string

// New builds the app.
func New(opts Options) *App {
	if opts.Sound == nil {
		opts.Sound = audio.Nop{}
	}
	if !opts.Remote {
		theme.Use(opts.Config.Palette)
	}
	t := theme.New(opts.ASCII || opts.Config.ASCII)
	a := &App{
		opts:  opts,
		t:     t,
		art:   art.NewRenderer(t),
		cfg:   opts.Config,
		sound: opts.Sound,
		now:   time.Now(),
		rng:   rand.New(rand.NewPCG(uint64(time.Now().UnixNano()), 1)),
		w:     MinWidth,
		h:     MinHeight,
	}
	a.sound.SetVolume(a.cfg.Volume)
	a.sound.SetMusicVolume(a.cfg.MusicVolume)
	a.sound.SetSFXVolume(a.cfg.SFXVolume)
	a.sound.SetMuted(a.cfg.Muted)
	a.boot = newBoot(a)
	a.menu = newMenu()
	a.solo = newSoloSetup(a.cfg)
	a.host = newHostSetup(a.cfg)
	a.join = newJoinScreen(a.cfg)
	a.settings = newSettingsScreen(a.cfg)
	return a
}

// Init starts the boot sequence.
func (a *App) Init() tea.Cmd {
	if a.cfg.SkipBoot {
		return tea.Batch(a.afterBoot(), a.kick(), a.listenAudio())
	}
	a.sound.Music(audio.SongMenu)
	return tea.Batch(a.kick(), a.listenAudio())
}

// listenAudio waits for the next "now playing" notice, if there is audio.
func (a *App) listenAudio() tea.Cmd {
	ch := a.sound.Notices()
	if ch == nil {
		return nil
	}
	return func() tea.Msg {
		title, ok := <-ch
		if !ok {
			return nil
		}
		return audioNoticeMsg(title)
	}
}

// kick starts the frame ticker if it is not running.
func (a *App) kick() tea.Cmd {
	if a.ticking {
		return nil
	}
	a.ticking = true
	return tea.Tick(a.tickInterval(), func(t time.Time) tea.Msg { return tickMsg(t) })
}

// tickInterval is the frame time for the current screen. Ambient screens
// (menu, lobby) animate at half rate to keep CPU low; the table gets the
// full 20 fps while something on it moves, half rate for countdowns.
func (a *App) tickInterval() time.Duration {
	switch a.scr {
	case scrMenu, scrLobby, scrConnecting:
		return 2 * time.Second / fps
	case scrGame:
		if a.game != nil && !a.game.smooth(a) && a.toast == "" {
			return 2 * time.Second / fps
		}
	}
	return time.Second / fps
}

// animating reports whether anything on screen moves.
func (a *App) animating() bool {
	if a.toast != "" || a.retrying > 0 || a.nowPlaying != "" {
		return true
	}
	switch a.scr {
	case scrBoot, scrMenu, scrConnecting:
		return true
	case scrGame:
		return a.game != nil && a.game.animating(a)
	case scrLobby:
		return true // spinner and logo shimmer at a low rate
	case scrReplay:
		return a.replay != nil && a.replay.playing
	}
	return false
}

// Update routes messages.
func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.w, a.h = msg.Width, msg.Height
		return a, nil
	case tickMsg:
		a.setNow(time.Time(msg))
		a.frame++
		cmd := a.onTick()
		if a.animating() {
			return a, tea.Batch(cmd, tea.Tick(a.tickInterval(), func(t time.Time) tea.Msg { return tickMsg(t) }))
		}
		a.ticking = false
		return a, cmd
	case tea.KeyPressMsg:
		a.setNow(time.Now())
		return a, tea.Batch(a.onKey(msg), a.kick())
	case connectedMsg:
		return a, tea.Batch(a.onConnected(msg), a.kick())
	case audioNoticeMsg:
		a.setNow(time.Now())
		if !a.cfg.Muted && a.scr == scrGame {
			a.nowPlaying, a.npUntil = string(msg), a.now.Add(4*time.Second)
		}
		return a, tea.Batch(a.listenAudio(), a.kick())
	case frameMsg:
		if msg.s != a.sess {
			return a, nil
		}
		a.setNow(time.Now())
		cmd := a.onFrame(msg.env)
		if a.sess == msg.s {
			return a, tea.Batch(cmd, a.sess.Wait(), a.kick())
		}
		return a, tea.Batch(cmd, a.kick())
	case sessionRetryMsg:
		if msg.s != a.sess {
			return a, nil
		}
		a.retrying = msg.attempt
		return a, tea.Batch(a.sess.Wait(), a.kick())
	case sessionBackMsg:
		if msg.s != a.sess {
			return a, nil
		}
		a.retrying = 0
		a.flash("uplink restored")
		return a, tea.Batch(a.sess.Wait(), a.kick())
	case sessionDownMsg:
		if msg.s != a.sess {
			return a, nil
		}
		a.retrying = 0
		a.dropSession()
		if a.scr == scrGame || a.scr == scrLobby || a.scr == scrConnecting {
			a.showError("CONNECTION LOST", fmt.Sprintf("The link to the host dropped: %v", msg.err))
		}
		return a, nil
	}
	return a, nil
}

// setNow advances the UI clock. Tick messages can arrive after a frame has
// already moved the clock on, so it never goes backwards; effects compute
// elapsed times from it.
func (a *App) setNow(t time.Time) {
	if t.After(a.now) {
		a.now = t
	}
}

func (a *App) onTick() tea.Cmd {
	if a.toast != "" && a.now.After(a.toastUntil) {
		a.toast = ""
	}
	if a.nowPlaying != "" && a.now.After(a.npUntil) {
		a.nowPlaying = ""
	}
	switch a.scr {
	case scrBoot:
		if a.boot.tick(a) {
			return a.afterBoot()
		}
	case scrGame:
		if a.game != nil {
			return a.game.tick(a)
		}
	case scrReplay:
		if a.replay != nil {
			a.replay.tick(a)
		}
	}
	return nil
}

func (a *App) afterBoot() tea.Cmd {
	s := a.opts.Start
	a.opts.Start = Start{}
	switch s.Kind {
	case StartSolo:
		return a.startSolo(s.Solo)
	case StartHost:
		if s.Conn != nil {
			conn := s.Conn
			a.connText = "opening the room..."
			a.goTo(scrConnecting)
			return func() tea.Msg { return connectedMsg{conn: conn, mode: "host"} }
		}
		return a.startHost(s.Host)
	case StartJoin:
		return a.startJoin(s.Join)
	case StartTutorial:
		return a.startTutorial()
	case StartReplay:
		return a.openReplay(s.Replay)
	case StartStats:
		a.goTo(scrStats)
		return nil
	}
	a.goTo(scrMenu)
	return nil
}

func (a *App) goTo(s screen) {
	if s != a.scr {
		a.prev = a.scr
	}
	a.scr = s
	switch s {
	case scrMenu:
		a.sound.Music(audio.SongMenu)
	case scrStats:
		a.stats = newStatsScreen(a)
	case scrReplays:
		a.replays = newReplayList(a)
	}
}

func (a *App) flash(s string) {
	a.toast = s
	a.toastUntil = a.now.Add(3 * time.Second)
}

func (a *App) showError(title, text string) {
	a.errTitle, a.errText = title, text
	a.goTo(scrError)
	a.sound.SFX(audio.SFXError)
}

// saveConfig persists settings when a store is available.
func (a *App) saveConfig() {
	if a.opts.Store != nil {
		_ = a.opts.Store.Save(a.cfg)
	}
}

func (a *App) quit() tea.Cmd {
	a.dropSession()
	if a.opts.Backend != nil {
		a.opts.Backend.Close()
	}
	return tea.Quit
}

func (a *App) dropSession() {
	if a.sess != nil {
		a.sess.Close()
		a.sess = nil
	}
}

func (a *App) onKey(msg tea.KeyPressMsg) tea.Cmd {
	k := msg.String()
	if k == "ctrl+c" {
		return a.quit()
	}
	if a.help {
		a.help = false
		return nil
	}
	if a.quitConfirm {
		a.quitConfirm = false
		if k == "y" || k == "enter" {
			if a.scr == scrGame || a.scr == scrLobby {
				a.leaveRoom()
				return nil
			}
			return a.quit()
		}
		return nil
	}
	typing := a.typing()
	if !typing {
		switch k {
		case "?":
			a.help = true
			return nil
		case "m":
			a.toggleMute()
			return nil
		case "s":
			if a.scr == scrGame || a.scr == scrLobby || a.scr == scrReplay {
				a.cfg.CRT = !a.cfg.CRT
				a.saveConfig()
				a.flash(map[bool]string{true: "CRT scanlines on", false: "CRT scanlines off"}[a.cfg.CRT])
				return nil
			}
		}
	}
	switch a.scr {
	case scrBoot:
		a.boot.skip = true
		return nil
	case scrMenu:
		return a.menu.key(a, msg)
	case scrSolo:
		return a.solo.key(a, msg)
	case scrHost:
		return a.host.key(a, msg)
	case scrJoin:
		return a.join.key(a, msg)
	case scrConnecting:
		if k == "esc" || k == "q" {
			a.leaveRoom()
		}
	case scrLobby:
		return a.lobby.key(a, msg)
	case scrGame:
		if a.game != nil {
			return a.game.key(a, msg)
		}
	case scrStats:
		if k == "esc" || k == "q" || k == "enter" {
			a.goTo(scrMenu)
		}
	case scrReplays:
		return a.replays.key(a, msg)
	case scrReplay:
		if a.replay != nil {
			return a.replay.key(a, msg)
		}
	case scrSettings:
		return a.settings.key(a, msg)
	case scrError:
		if k == "enter" || k == "esc" || k == "q" {
			a.goTo(scrMenu)
		}
	}
	return nil
}

// typing reports whether keystrokes go to a text field.
func (a *App) typing() bool {
	switch a.scr {
	case scrJoin:
		return true
	case scrSettings:
		return a.settings.editing
	case scrLobby:
		return a.lobby.chatting
	case scrGame:
		return a.game != nil && a.game.chatting
	}
	return false
}

func (a *App) toggleMute() {
	if !a.sound.Available() {
		a.flash("no audio device: sound is off")
		return
	}
	a.cfg.Muted = !a.cfg.Muted
	a.sound.SetMuted(a.cfg.Muted)
	a.saveConfig()
	a.flash(map[bool]string{true: "audio muted", false: "audio on"}[a.cfg.Muted])
}

// leaveRoom drops the connection and returns to the menu.
func (a *App) leaveRoom() {
	a.dropSession()
	if a.opts.Backend != nil {
		a.opts.Backend.Close()
	}
	a.game = nil
	a.conn = nil
	a.goTo(scrMenu)
}

// --- connecting --------------------------------------------------------------

func (a *App) startSolo(o SoloOptions) tea.Cmd {
	a.connText = "spinning up a private grid..."
	a.goTo(scrConnecting)
	be := a.opts.Backend
	return func() tea.Msg {
		c, err := be.Solo(o)
		return connectedMsg{conn: c, err: err, mode: "solo"}
	}
}

func (a *App) startHost(o HostOptions) tea.Cmd {
	a.connText = "opening a room..."
	a.goTo(scrConnecting)
	be := a.opts.Backend
	return func() tea.Msg {
		c, err := be.Host(o)
		return connectedMsg{conn: c, err: err, mode: "host"}
	}
}

func (a *App) startJoin(target string) tea.Cmd {
	a.connText = "tracing " + target + " (LAN first, then relay)..."
	a.goTo(scrConnecting)
	be := a.opts.Backend
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		c, err := be.Join(ctx, target)
		return connectedMsg{conn: c, err: err, mode: "join"}
	}
}

func (a *App) onConnected(m connectedMsg) tea.Cmd {
	if m.err != nil {
		a.showError("UPLINK FAILED", m.err.Error())
		return nil
	}
	a.dropSession()
	a.conn = m.conn
	a.mode = m.mode
	a.welcome = protocol.Welcome{}
	a.lobbyMsg = protocol.Lobby{}
	a.lobby = newLobbyScreen()
	hello := protocol.Hello{Name: a.cfg.Name, Room: m.conn.Room, Spectate: a.opts.Spectate && m.mode == "join"}
	if t, ok := a.cfg.Tokens[m.conn.Target]; ok && m.mode == "join" && time.Since(t.Saved) < time.Hour {
		hello.Token = t.Token
	}
	a.sess = NewSession(m.conn.Conn, hello, m.conn.Dial)
	a.connText = "handshake..."
	return a.sess.Wait()
}

func (a *App) onFrame(env protocol.Envelope) tea.Cmd {
	switch env.Type {
	case protocol.TReject:
		var r protocol.Reject
		_ = env.Into(&r)
		a.dropSession()
		a.showError("ACCESS DENIED", r.Reason)
		return nil
	case protocol.TWelcome:
		_ = env.Into(&a.welcome)
		a.rememberToken()
		if a.mode == "solo" || a.mode == "tutorial" {
			a.sess.Send(protocol.TStart, nil)
		}
		if a.scr == scrConnecting {
			a.goTo(scrLobby)
		}
	case protocol.TLobby:
		_ = env.Into(&a.lobbyMsg)
		for _, m := range a.lobbyMsg.Members {
			if m.ID == a.welcome.You {
				a.welcome.Host = m.Host
			}
		}
		if a.scr == scrConnecting {
			a.goTo(scrLobby)
		}
	case protocol.TGameStart:
		var gs protocol.GameStart
		_ = env.Into(&gs)
		if a.sess != nil {
			// Keep trying to rejoin for as long as the host holds our seat.
			a.sess.SetRejoin(time.Duration(gs.Settings.ReconnectS)*time.Second + 15*time.Second)
		}
		a.game = newGameScreen(a, gs)
		a.goTo(scrGame)
		a.sound.Music(audio.SongGame)
	case protocol.TChat, protocol.TEmote:
		switch {
		case a.scr == scrGame && a.game != nil && isBotChat(env):
			// Bot taunts react to events: play them after those events.
			return a.game.onFrame(a, env)
		case a.scr == scrGame && a.game != nil:
			a.game.onChat(a, env)
		default:
			a.lobby.onChat(a, env)
		}
	case protocol.TError:
		var e protocol.Error
		_ = env.Into(&e)
		if e.Code == protocol.ErrStale {
			return nil // the table moved on; the new state is on its way
		}
		a.flash(e.Msg)
		a.sound.SFX(audio.SFXError)
	default:
		if a.game != nil {
			return a.game.onFrame(a, env)
		}
	}
	return nil
}

func (a *App) rememberToken() {
	if a.mode != "join" || a.conn == nil {
		return
	}
	if a.cfg.Tokens == nil {
		a.cfg.Tokens = map[string]config.Token{}
	}
	a.cfg.Tokens[a.conn.Target] = config.Token{Token: a.welcome.Token, Addr: a.conn.Via, Saved: time.Now()}
	a.saveConfig()
}

// --- view --------------------------------------------------------------------

// View renders the current screen.
func (a *App) View() tea.View {
	var body string
	if a.w < MinWidth || a.h < MinHeight {
		body = a.tooSmall()
	} else {
		body = a.screenView()
		body = fit(body, a.w, a.h)
		body = a.postProcess(body)
		if a.toast != "" || a.retrying > 0 {
			body = a.overlayToast(body)
		}
		if a.nowPlaying != "" && a.scr == scrGame {
			body = a.overlayNowPlaying(body)
		}
		if a.help {
			body = overlay(body, a.helpView(), a.w, a.h)
		}
		if a.quitConfirm {
			body = overlay(body, a.confirmView(), a.w, a.h)
		}
		// The compositor trims trailing blanks; keep the frame exactly w x h.
		body = fit(body, a.w, a.h)
	}
	if a.t.ASCII {
		// Last line of defense for --ascii: user text, ellipses, anything.
		body = theme.Asciify(body)
	}
	v := tea.NewView(body)
	v.AltScreen = true
	v.BackgroundColor = theme.Bg
	v.WindowTitle = "NEON//COUP"
	return v
}

func (a *App) screenView() string {
	switch a.scr {
	case scrBoot:
		return a.boot.view(a)
	case scrMenu:
		return a.menu.view(a)
	case scrSolo:
		return a.solo.view(a)
	case scrHost:
		return a.host.view(a)
	case scrJoin:
		return a.join.view(a)
	case scrConnecting:
		return a.centerBox("CONNECTING", a.spinner()+" "+a.connText+"\n\n"+a.t.Faint.Render("esc to cancel"))
	case scrLobby:
		return a.lobby.view(a)
	case scrGame:
		if a.game != nil {
			return a.game.view(a)
		}
	case scrStats:
		return a.stats.view(a)
	case scrReplays:
		return a.replays.view(a)
	case scrReplay:
		if a.replay != nil {
			return a.replay.view(a)
		}
	case scrSettings:
		return a.settings.view(a)
	case scrError:
		return a.centerBox(a.errTitle, a.t.Loss.Render(wrap(a.errText, 60))+"\n\n"+a.t.Faint.Render("enter: back to menu"))
	}
	return ""
}

func (a *App) tooSmall() string {
	msg := fmt.Sprintf("NEON//COUP needs at least %dx%d.\nThis window is %dx%d.\n\nMake it bigger, or zoom out.", MinWidth, MinHeight, a.w, a.h)
	box := lipgloss.NewStyle().Border(a.t.RoundBorder).BorderForeground(theme.Magenta).Padding(1, 2).Foreground(theme.Fg).Render(msg)
	if lipgloss.Width(box) > a.w {
		return fit(msg, a.w, a.h)
	}
	return lipgloss.Place(a.w, a.h, lipgloss.Center, lipgloss.Center, box)
}

func (a *App) spinner() string {
	frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	if a.t.ASCII {
		frames = []string{".", "o", "O", "o"} // |/-\ would read as box borders
	}
	return lipgloss.NewStyle().Foreground(theme.Cyan).Render(frames[(a.frame/2)%len(frames)])
}

// centerBox draws a titled box in the middle of the screen.
func (a *App) centerBox(title, body string) string {
	box := lipgloss.NewStyle().Border(a.t.RoundBorder).BorderForeground(theme.Magenta).Padding(1, 3).Render(
		a.t.Title.Render(title) + "\n\n" + body)
	return lipgloss.Place(a.w, a.h, lipgloss.Center, lipgloss.Center, box)
}

func (a *App) helpView() string {
	rows := [][2]string{
		{"1-8", "actions: income, aid, coup, tax, assassinate, steal, exchange, audit"},
		{"c", "challenge the claim on the table"},
		{"b", "block (then pick the card if there's a choice)"},
		{"p", "pass"},
		{"1 / 2", "pick a card when losing influence or proving a claim"},
		{"tab", "inspect a player: claims, history, face-up cards"},
		{"space", "skip the current beat (solo)    P  pause (solo)"},
		{"enter", "chat    t  taunt menu    pgup/pgdn  scroll the log"},
		{"m", "mute    n  next track    s  CRT scanlines"},
		{"esc", "cancel a choice, or open settings    q  leave"},
		{"?", "this help"},
	}
	var b strings.Builder
	b.WriteString(a.t.Title.Render("CONTROLS") + "\n\n")
	for _, r := range rows {
		b.WriteString(a.t.Key.Render(fmt.Sprintf(" %-5s ", r[0])) + "  " + a.t.Text.Render(r[1]) + "\n")
	}
	b.WriteString("\n" + a.t.Title.Render("RULES IN ONE BREATH") + "\n")
	b.WriteString(a.t.Muted.Render(wrap("Everyone has two hidden cards (influence) and coins. On your turn take one action. "+
		"Any character action may be a bluff: anyone can challenge. A failed challenge costs the challenger a card; a caught bluff costs the bluffer a card. "+
		"7 coins buys a Coup; at 10 you must Coup. Lose both cards and you're out. Last runner standing wins.", 70)))
	b.WriteString("\n\n" + a.t.Title.Render("READING THE TABLE") + "\n")
	b.WriteString(a.t.Muted.Render(wrap("The bottom bar says what's happening, what you can do and what it risks. "+
		"The tracker in the middle counts each character: REVEALED is face up, UNSEEN could be in a rival's hand or the deck. "+
		"UNSEEN 0 means any claim of it is a bluff. Badges on a rival's panel show what they've claimed this game.", 70)))
	b.WriteString("\n\n" + a.t.Faint.Render("any key to close"))
	return lipgloss.NewStyle().Border(a.t.RoundBorder).BorderForeground(theme.Cyan).Padding(1, 2).Background(theme.Panel).Render(b.String())
}

func (a *App) confirmView() string {
	q := "Quit NEON//COUP?"
	if a.scr == scrGame || a.scr == scrLobby {
		q = "Leave this room?"
	}
	return lipgloss.NewStyle().Border(a.t.RoundBorder).BorderForeground(theme.Red).Padding(1, 3).Background(theme.Panel).Render(
		a.t.Warn.Render(q) + "\n\n" + a.t.Text.Render("y / enter: yes    any other key: no"))
}

// overlayNowPlaying shows the new track's title in the top-right corner.
func (a *App) overlayNowPlaying(body string) string {
	msg := a.t.G("♪", "~") + " NOW PLAYING // " + a.nowPlaying
	box := lipgloss.NewStyle().Foreground(theme.Bg).Background(theme.Magenta).Bold(true).Padding(0, 1).Render(a.t.A(msg))
	return overlayAt(body, box, a.w-lipgloss.Width(box)-1, 1)
}

func (a *App) overlayToast(body string) string {
	msg := a.toast
	if a.retrying > 0 {
		msg = fmt.Sprintf("%s uplink lost, reconnecting (attempt %d)...", a.spinner(), a.retrying)
	}
	box := lipgloss.NewStyle().Foreground(theme.Bg).Background(theme.Acid).Bold(true).Padding(0, 2).Render(a.t.A(msg))
	return overlayAt(body, box, a.w-lipgloss.Width(box)-2, a.h-2)
}

// postProcess applies the screen jolt, glitch and CRT scanlines.
func (a *App) postProcess(body string) string {
	if a.scr == scrGame && a.game != nil && !a.cfg.ReduceMotion {
		body = a.game.shake(a, body)
	}
	if a.cfg.CRT {
		body = scanlines(body, a.frame)
	}
	return body
}

// --- layout helpers ------------------------------------------------------------

// fit pads or crops a block to exactly w x h cells.
func fit(s string, w, h int) string {
	lines := strings.Split(s, "\n")
	if len(lines) > h {
		lines = lines[:h]
	}
	for i, l := range lines {
		lw := cellWidth(l)
		switch {
		case lw > w:
			lines[i] = ansi.Cut(l, 0, w)
		case lw < w:
			lines[i] = l + strings.Repeat(" ", w-lw)
		}
	}
	for len(lines) < h {
		lines = append(lines, strings.Repeat(" ", w))
	}
	return strings.Join(lines, "\n")
}

// overlay centers fg on bg.
func overlay(bg, fg string, w, h int) string {
	fw, fh := lipgloss.Size(fg)
	return overlayAt(bg, fg, (w-fw)/2, (h-fh)/2)
}

// overlayAt draws fg over bg with its top-left corner at x, y.
func overlayAt(bg, fg string, x, y int) string {
	base := lipgloss.NewLayer(bg)
	top := lipgloss.NewLayer(fg).X(max(0, x)).Y(max(0, y)).Z(1)
	return lipgloss.NewCompositor(base, top).Render()
}

// scanlines dims every other row, CRT style; a brighter band rolls down slowly.
func scanlines(body string, frame int) string {
	lines := strings.Split(body, "\n")
	band := (frame / 3) % (len(lines) + 12)
	for i, l := range lines {
		if i%2 == 1 && i != band {
			l = "\x1b[2m" + strings.ReplaceAll(strings.ReplaceAll(l, "\x1b[m", "\x1b[m\x1b[2m"), "\x1b[0m", "\x1b[0m\x1b[2m") + "\x1b[m"
			lines[i] = l
		}
	}
	return strings.Join(lines, "\n")
}

// wrap breaks text at spaces to the given width.
func wrap(s string, width int) string {
	var out []string
	for _, para := range strings.Split(s, "\n") {
		line := ""
		for _, word := range strings.Fields(para) {
			if line == "" {
				line = word
			} else if lipgloss.Width(line)+1+lipgloss.Width(word) > width {
				out = append(out, line)
				line = word
			} else {
				line += " " + word
			}
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}
