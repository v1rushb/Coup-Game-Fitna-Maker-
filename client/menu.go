package client

import (
	"fmt"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/v1rushb/neoncoup/audio"
	"github.com/v1rushb/neoncoup/client/art"
	"github.com/v1rushb/neoncoup/client/theme"
	"github.com/v1rushb/neoncoup/config"
	"github.com/v1rushb/neoncoup/engine"
	"github.com/v1rushb/neoncoup/protocol"
)

// --- main menu -----------------------------------------------------------------

type menuItem struct {
	key, title, desc string
	local            bool // hidden over SSH
}

var menuItems = []menuItem{
	{"solo", "SOLO RUN", "you vs bots, right now", false},
	{"host", "HOST ROOM", "open a room for LAN or internet friends", false},
	{"join", "JOIN ROOM", "room code (NEON-XXXX) or host:port", false},
	{"tutorial", "TUTORIAL", "a scripted run that teaches the con", false},
	{"replays", "REPLAYS", "step through saved games", true},
	{"stats", "STATS", "your record: wins, bluffs, challenges", true},
	{"settings", "SETTINGS", "name, audio, CRT, ASCII", false},
	{"quit", "QUIT", "jack out", false},
}

type menuScreen struct{ sel int }

func newMenu() menuScreen { return menuScreen{} }

func (m *menuScreen) items(a *App) []menuItem {
	var out []menuItem
	for _, it := range menuItems {
		if it.local && a.opts.Remote {
			continue
		}
		out = append(out, it)
	}
	return out
}

func (m *menuScreen) key(a *App, msg tea.KeyPressMsg) tea.Cmd {
	items := m.items(a)
	switch k := msg.String(); k {
	case "up", "k", "shift+tab":
		m.sel = (m.sel + len(items) - 1) % len(items)
		a.sound.SFX(audio.SFXSelect)
	case "down", "j", "tab":
		m.sel = (m.sel + 1) % len(items)
		a.sound.SFX(audio.SFXSelect)
	case "q", "esc":
		a.quitConfirm = true
	case "enter", "space", "l", "right":
		return m.activate(a, items[m.sel].key)
	default:
		if n, err := strconv.Atoi(k); err == nil && n >= 1 && n <= len(items) {
			m.sel = n - 1
			return m.activate(a, items[m.sel].key)
		}
	}
	return nil
}

func (m *menuScreen) activate(a *App, key string) tea.Cmd {
	a.sound.SFX(audio.SFXSelect)
	switch key {
	case "solo":
		a.goTo(scrSolo)
	case "host":
		a.goTo(scrHost)
	case "join":
		a.goTo(scrJoin)
	case "tutorial":
		return a.startTutorial()
	case "replays":
		a.goTo(scrReplays)
	case "stats":
		a.goTo(scrStats)
	case "settings":
		a.settings = newSettingsScreen(a.cfg)
		a.goTo(scrSettings)
	case "quit":
		a.quitConfirm = true
	}
	return nil
}

func (m *menuScreen) view(a *App) string {
	var b strings.Builder
	for i, it := range m.items(a) {
		cursor := "  "
		title := a.t.Text.Render(fmt.Sprintf(" %-10s ", it.title))
		if i == m.sel {
			cursor = a.t.Hot.Render(a.t.G("▶ ", "> "))
			title = lipgloss.NewStyle().Foreground(theme.Bg).Background(theme.Magenta).Bold(true).Render(fmt.Sprintf(" %-10s ", it.title))
		}
		fmt.Fprintf(&b, "%s%s %s  %s\n", cursor, a.t.Faint.Render(strconv.Itoa(i+1)), title, a.t.Muted.Render(it.desc))
	}
	sub := a.t.Muted.Render("a bluffing game for the terminal // ") + a.t.Accent.Render("v"+a.opts.Version)
	footer := a.t.Faint.Render(fmt.Sprintf("runner: %s   ↑↓ select · enter go · ? help · m mute %s", a.cfg.Name, a.muteIcon()))
	body := lipgloss.JoinVertical(lipgloss.Center,
		a.logo(0), "", sub, "", "", lipgloss.NewStyle().Width(64).Render(b.String()), "", footer)
	return lipgloss.Place(a.w, a.h, lipgloss.Center, lipgloss.Center, a.t.A(body))
}

func (a *App) muteIcon() string {
	switch {
	case !a.sound.Available():
		return a.t.Loss.Render(a.t.G("♪✖ no audio", "~x no audio"))
	case a.cfg.Muted:
		return a.t.Warn.Render(a.t.G("♪✖ muted", "~x muted"))
	}
	bars := int(a.cfg.Volume*5 + 0.5)
	return a.t.Accent.Render(a.t.G("♪ ", "~ ") + strings.Repeat(a.t.G("▮", "|"), bars) + strings.Repeat(a.t.G("▯", "."), 5-bars))
}

// --- forms -------------------------------------------------------------------

type field struct {
	label string
	value func() string
	left  func()
	right func()
	text  *lineInput
	help  string
}

type form struct {
	sel     int
	editing bool
}

// key handles navigation and editing; it returns "submit" or "back".
func (f *form) key(a *App, fields []field, msg tea.KeyPressMsg) string {
	k := msg.String()
	cur := &fields[f.sel]
	if f.editing && cur.text != nil {
		switch cur.text.handle(msg) {
		case "enter", "esc":
			f.editing = false
		}
		return ""
	}
	switch k {
	case "up", "k", "shift+tab":
		f.sel = (f.sel + len(fields) - 1) % len(fields)
	case "down", "j", "tab":
		f.sel = (f.sel + 1) % len(fields)
	case "left", "h", "-":
		if cur.left != nil {
			cur.left()
			a.sound.SFX(audio.SFXSelect)
		}
	case "right", "l", "+", "=", "space":
		if cur.right != nil {
			cur.right()
			a.sound.SFX(audio.SFXSelect)
		} else if cur.text != nil && k == "space" {
			f.editing = true
		}
	case "e":
		if cur.text != nil {
			f.editing = true
		}
	case "enter":
		if cur.text != nil && !f.editing && f.sel != len(fields)-1 {
			f.editing = true
			return ""
		}
		return "submit"
	case "esc", "q":
		return "back"
	}
	return ""
}

func (f *form) view(a *App, title, sub string, fields []field, submit string) string {
	return lipgloss.Place(a.w, a.h, lipgloss.Center, lipgloss.Center, f.box(a, title, sub, fields, submit))
}

// box renders the form on its own, for overlays.
func (f *form) box(a *App, title, sub string, fields []field, submit string) string {
	var rows []string
	for i, fl := range fields {
		label := a.t.Muted.Render(fmt.Sprintf("%-16s", fl.label))
		var val string
		if fl.text != nil {
			val = fl.text.view(a.t, 32, f.editing && i == f.sel)
		} else {
			arrowL, arrowR := a.t.G("◀ ", "< "), a.t.G(" ▶", " >")
			val = a.t.Faint.Render(arrowL) + a.t.Bold.Render(fl.value()) + a.t.Faint.Render(arrowR)
		}
		cursor := "  "
		if i == f.sel {
			cursor = a.t.Hot.Render(a.t.G("▶ ", "> "))
			label = a.t.Accent.Render(fmt.Sprintf("%-16s", fl.label))
		}
		rows = append(rows, cursor+label+val)
	}
	help := ""
	if f.sel < len(fields) && fields[f.sel].help != "" {
		help = a.t.Faint.Render(fields[f.sel].help)
	}
	keys := "↑↓ field · ←→ change · enter " + submit + " · esc back"
	if f.editing {
		keys = "type, then enter to keep · esc to stop editing"
	}
	body := lipgloss.JoinVertical(lipgloss.Left,
		a.t.Title.Render(title), a.t.Muted.Render(sub), "",
		strings.Join(rows, "\n"), "", help, "", a.t.Faint.Render(a.t.A(keys)))
	box := lipgloss.NewStyle().Border(a.t.RoundBorder).BorderForeground(theme.Magenta).Background(theme.Panel).Padding(1, 3).Width(72).Render(body)
	return a.t.A(box)
}

func cycle[T comparable](opts []T, cur T, d int) T {
	for i, o := range opts {
		if o == cur {
			return opts[(i+d+len(opts))%len(opts)]
		}
	}
	return opts[0]
}

var levels = []string{"easy", "normal", "hard"}

func levelTitle(l string) string {
	switch l {
	case "easy":
		return "SCRIPT KIDDIE (easy)"
	case "hard":
		return "BLACK ICE (hard)"
	}
	return "FIXER (normal)"
}

func onOff(b bool) string {
	if b {
		return "ON"
	}
	return "OFF"
}

// --- solo setup ----------------------------------------------------------------

type soloSetup struct {
	form
	bots       int
	level      string
	magistrate bool
	windowMS   int // 0 = auto
	speed      string
}

func newSoloSetup(c config.Config) soloSetup {
	return soloSetup{bots: max(1, min(5, c.Bots)), level: c.BotLevel, magistrate: c.Magistrate, windowMS: c.WindowMS, speed: protocol.ParseSpeed(c.Speed)}
}

// windowOptions are the reaction windows a player can pick; 0 is auto.
var windowOptions = []int{0, 4000, 6000, 8000, 10000, 12000, 15000, 20000, 30000}

// windowLabel describes a reaction window setting.
func windowLabel(ms int, speed string) string {
	if ms == 0 {
		return fmt.Sprintf("AUTO (%ds)", int(protocol.AutoWindow(speed).Seconds()))
	}
	return fmt.Sprintf("%ds", ms/1000)
}

func speedTitle(s string) string {
	switch protocol.ParseSpeed(s) {
	case protocol.SpeedNormal:
		return "NORMAL"
	case protocol.SpeedFast:
		return "FAST"
	case protocol.SpeedInstant:
		return "INSTANT"
	}
	return "RELAXED"
}

const speedHelp = "How fast events play out. Relaxed gives every move time to land; Fast is for veterans."

func (s *soloSetup) fields() []field {
	return []field{
		{label: "Bots", value: func() string { return strconv.Itoa(s.bots) },
			left: func() { s.bots = max(1, s.bots-1) }, right: func() { s.bots = min(engine.MaxPlayers-1, s.bots+1) },
			help: "1 to 5 opponents. Magistrate is recommended for 5+ players."},
		{label: "Difficulty", value: func() string { return levelTitle(s.level) },
			left: func() { s.level = cycle(levels, s.level, -1) }, right: func() { s.level = cycle(levels, s.level, 1) },
			help: "Script Kiddie plays honest and random. Fixer counts cards. Black ICE reads you."},
		{label: "Magistrate", value: func() string { return onOff(s.magistrate) },
			left: func() { s.magistrate = !s.magistrate }, right: func() { s.magistrate = !s.magistrate },
			help: "Variant: 18-card deck. Magistrate audits rich players (-2¢) and blocks Tax."},
		{label: "Speed", value: func() string { return speedTitle(s.speed) },
			left: func() { s.speed = cycle(protocol.Speeds, s.speed, -1) }, right: func() { s.speed = cycle(protocol.Speeds, s.speed, 1) },
			help: speedHelp},
		{label: "Reaction window", value: func() string { return windowLabel(s.windowMS, s.speed) },
			left: func() { s.windowMS = cycle(windowOptions, s.windowMS, -1) }, right: func() { s.windowMS = cycle(windowOptions, s.windowMS, 1) },
			help: "Time to challenge or block before the table moves on. Auto follows the speed."},
	}
}

func (s *soloSetup) key(a *App, msg tea.KeyPressMsg) tea.Cmd {
	switch s.form.key(a, s.fields(), msg) {
	case "back":
		a.goTo(scrMenu)
	case "submit":
		a.cfg.Bots, a.cfg.BotLevel, a.cfg.Magistrate, a.cfg.WindowMS, a.cfg.Speed = s.bots, s.level, s.magistrate, s.windowMS, s.speed
		a.saveConfig()
		return a.startSolo(SoloOptions{Bots: s.bots, Level: s.level, Magistrate: s.magistrate, WindowMS: s.windowMS, Speed: s.speed})
	}
	return nil
}

func (s *soloSetup) view(a *App) string {
	return s.form.view(a, "SOLO RUN", "a private grid, you against the machine", s.fields(), "to jack in")
}

// --- host setup ------------------------------------------------------------------

type hostSetup struct {
	form
	st    protocol.Settings
	port  lineInput
	relay lineInput
}

func newHostSetup(c config.Config) hostSetup {
	st := protocol.DefaultSettings()
	st.BotLevel = c.BotLevel
	st.WindowMS = c.WindowMS
	st.TurnMS = c.TurnMS
	st.Magistrate = c.Magistrate
	st.Speed = c.Speed
	h := hostSetup{st: st.Clamp(), port: newLineInput("", 5), relay: newLineInput("", 120)}
	h.port.Set(strconv.Itoa(c.Port))
	h.relay.Set(c.Relay)
	h.relay.hint = "none (LAN + direct only)"
	return h
}

var turnOptions = []int{0, 30000, 60000, 120000}

func (h *hostSetup) fields() []field {
	st := &h.st
	return []field{
		{label: "Seats", value: func() string { return strconv.Itoa(st.MaxPlayers) },
			left: func() { st.MaxPlayers = max(2, st.MaxPlayers-1) }, right: func() { st.MaxPlayers = min(6, st.MaxPlayers+1) },
			help: "Total seats at the table, humans and bots."},
		{label: "Fill with bots", value: func() string { return onOff(st.FillBots) },
			left: func() { st.FillBots = !st.FillBots }, right: func() { st.FillBots = !st.FillBots },
			help: "Empty seats get bots when you start."},
		{label: "Bot difficulty", value: func() string { return levelTitle(st.BotLevel) },
			left: func() { st.BotLevel = cycle(levels, st.BotLevel, -1) }, right: func() { st.BotLevel = cycle(levels, st.BotLevel, 1) }},
		{label: "Speed", value: func() string { return speedTitle(st.Speed) },
			left: func() { st.Speed = cycle(protocol.Speeds, st.Speed, -1) }, right: func() { st.Speed = cycle(protocol.Speeds, st.Speed, 1) },
			help: speedHelp + " Everyone at the table plays at this speed."},
		{label: "Reaction window", value: func() string { return windowLabel(st.WindowMS, st.Speed) },
			left: func() { st.WindowMS = cycle(windowOptions, st.WindowMS, -1) }, right: func() { st.WindowMS = cycle(windowOptions, st.WindowMS, 1) },
			help: "Seconds to challenge or block. Auto follows the speed."},
		{label: "Turn timer", value: func() string {
			if st.TurnMS == 0 {
				return "OFF"
			}
			return fmt.Sprintf("%ds", st.TurnMS/1000)
		}, left: func() { st.TurnMS = cycle(turnOptions, st.TurnMS, -1) }, right: func() { st.TurnMS = cycle(turnOptions, st.TurnMS, 1) },
			help: "How long a player may think on their turn before the host acts for them."},
		{label: "Magistrate", value: func() string { return onOff(st.Magistrate) },
			left: func() { st.Magistrate = !st.Magistrate }, right: func() { st.Magistrate = !st.Magistrate },
			help: "Variant for 5+ players: Audit and Tax blocking."},
		{label: "Seat hold", value: func() string { return fmt.Sprintf("%ds", st.ReconnectS) },
			left: func() { st.ReconnectS = max(10, st.ReconnectS-10) }, right: func() { st.ReconnectS = min(300, st.ReconnectS+10) },
			help: "A dropped player's seat is held this long before a bot takes over."},
		{label: "Port", text: &h.port, help: "TCP port for LAN/direct joins (e to edit)."},
		{label: "Relay", text: &h.relay, help: "wss:// URL of a neoncoup relay for players behind NAT (e to edit)."},
	}
}

func (h *hostSetup) key(a *App, msg tea.KeyPressMsg) tea.Cmd {
	switch h.form.key(a, h.fields(), msg) {
	case "back":
		a.goTo(scrMenu)
	case "submit":
		port, err := strconv.Atoi(strings.TrimSpace(h.port.Value()))
		if err != nil || port < 0 || port > 65535 {
			a.flash("port must be a number from 0 to 65535")
			return nil
		}
		a.cfg.Port, a.cfg.Relay = port, strings.TrimSpace(h.relay.Value())
		a.cfg.BotLevel, a.cfg.WindowMS, a.cfg.TurnMS, a.cfg.Magistrate, a.cfg.Speed = h.st.BotLevel, h.st.WindowMS, h.st.TurnMS, h.st.Magistrate, h.st.Speed
		a.saveConfig()
		return a.startHost(HostOptions{Settings: h.st.Clamp(), Port: port, Relay: a.cfg.Relay})
	}
	return nil
}

func (h *hostSetup) view(a *App) string {
	return h.form.view(a, "HOST ROOM", "you run the table; friends join with a room code", h.fields(), "to open the room")
}

// --- join ------------------------------------------------------------------

type joinScreen struct {
	input    lineInput
	spectate bool
}

func newJoinScreen(c config.Config) joinScreen {
	in := newLineInput("> ", 80)
	in.hint = "NEON-7K2Q or 192.168.1.20:7777"
	return joinScreen{input: in}
}

func (j *joinScreen) key(a *App, msg tea.KeyPressMsg) tea.Cmd {
	if msg.String() == "tab" {
		j.spectate = !j.spectate
		return nil
	}
	switch j.input.handle(msg) {
	case "esc":
		a.goTo(scrMenu)
	case "enter":
		target := strings.TrimSpace(j.input.Value())
		if target == "" {
			a.flash("enter a room code or host:port")
			return nil
		}
		a.opts.Spectate = j.spectate
		return a.startJoin(target)
	}
	return nil
}

func (j *joinScreen) view(a *App) string {
	spec := "as a player"
	if j.spectate {
		spec = "as a spectator"
	}
	relay := a.cfg.Relay
	if relay == "" {
		relay = "none configured (settings)"
	}
	body := lipgloss.JoinVertical(lipgloss.Left,
		a.t.Title.Render("JOIN ROOM"),
		a.t.Muted.Render("room codes are found on your LAN first, then through the relay"), "",
		j.input.view(a.t, 60, true), "",
		a.t.Text.Render("joining ")+a.t.Accent.Render(spec)+a.t.Faint.Render("  (tab to switch)"),
		a.t.Faint.Render("relay: "+relay), "",
		a.t.Faint.Render("enter connect · esc back"))
	box := lipgloss.NewStyle().Border(a.t.RoundBorder).BorderForeground(theme.Magenta).Padding(1, 3).Width(72).Render(body)
	return lipgloss.Place(a.w, a.h, lipgloss.Center, lipgloss.Center, a.t.A(box))
}

// --- settings --------------------------------------------------------------

type settingsScreen struct {
	form
	name  lineInput
	relay lineInput
	cfg   config.Config
}

func newSettingsScreen(c config.Config) settingsScreen {
	s := settingsScreen{name: newLineInput("", protocol.MaxNameLen), relay: newLineInput("", 120), cfg: c}
	s.name.Set(c.Name)
	s.relay.Set(c.Relay)
	s.relay.hint = "none"
	return s
}

func (s *settingsScreen) fields(a *App) []field {
	c := &s.cfg
	fs := []field{{label: "Nickname", text: &s.name, help: "Shown to other players (e to edit)."}}
	if !a.opts.Remote {
		pct := func(v float64) func() string { return func() string { return fmt.Sprintf("%d%%", int(v*100+0.5)) } }
		fs = append(fs,
			field{label: "Master volume", value: func() string { return pct(c.Volume)() },
				left:  func() { c.Volume = max(0, c.Volume-0.1); a.sound.SetVolume(c.Volume); a.sound.SFX(audio.SFXCoin) },
				right: func() { c.Volume = min(1, c.Volume+0.1); a.sound.SetVolume(c.Volume); a.sound.SFX(audio.SFXCoin) }},
			field{label: "Music volume", value: func() string { return pct(c.MusicVolume)() },
				left:  func() { c.MusicVolume = max(0, c.MusicVolume-0.1); a.sound.SetMusicVolume(c.MusicVolume) },
				right: func() { c.MusicVolume = min(1, c.MusicVolume+0.1); a.sound.SetMusicVolume(c.MusicVolume) },
				help:  "In game, n skips to the next track."},
			field{label: "SFX volume", value: func() string { return pct(c.SFXVolume)() },
				left: func() {
					c.SFXVolume = max(0, c.SFXVolume-0.1)
					a.sound.SetSFXVolume(c.SFXVolume)
					a.sound.SFX(audio.SFXCoin)
				},
				right: func() {
					c.SFXVolume = min(1, c.SFXVolume+0.1)
					a.sound.SetSFXVolume(c.SFXVolume)
					a.sound.SFX(audio.SFXCoin)
				}},
			field{label: "Muted", value: func() string { return onOff(c.Muted) },
				left: func() { c.Muted = !c.Muted; a.sound.SetMuted(c.Muted) }, right: func() { c.Muted = !c.Muted; a.sound.SetMuted(c.Muted) }})
	}
	fs = append(fs, a.prefFields(c, false)...)
	fs = append(fs,
		field{label: "CRT scanlines", value: func() string { return onOff(c.CRT) },
			left: func() { c.CRT = !c.CRT }, right: func() { c.CRT = !c.CRT }, help: "Also toggled in game with s."},
		field{label: "ASCII mode", value: func() string { return onOff(c.ASCII) },
			left: func() { c.ASCII = !c.ASCII }, right: func() { c.ASCII = !c.ASCII }, help: "Plain ASCII instead of box drawing (same as --ascii)."},
		field{label: "Skip boot", value: func() string { return onOff(c.SkipBoot) },
			left: func() { c.SkipBoot = !c.SkipBoot }, right: func() { c.SkipBoot = !c.SkipBoot }},
	)
	if !a.opts.Remote {
		fs = append(fs, field{label: "Relay URL", text: &s.relay, help: "Default relay for hosting and joining by code (e to edit)."})
	}
	return fs
}

func (s *settingsScreen) key(a *App, msg tea.KeyPressMsg) tea.Cmd {
	switch s.form.key(a, s.fields(a), msg) {
	case "back", "submit":
		if n := protocol.CleanName(s.name.Value()); n != "" {
			s.cfg.Name = n
		}
		s.cfg.Relay = strings.TrimSpace(s.relay.Value())
		asciiChanged := s.cfg.ASCII != a.cfg.ASCII
		a.cfg = s.cfg
		if asciiChanged && !a.opts.ASCII {
			a.t = theme.New(a.cfg.ASCII)
			a.art = art.NewRenderer(a.t)
		}
		a.saveConfig()
		a.flash("settings saved")
		a.goTo(scrMenu)
	}
	return nil
}

func (s *settingsScreen) view(a *App) string {
	return s.form.view(a, "SETTINGS", "saved to "+a.configPath(), s.fields(a), "to save")
}

func (a *App) configPath() string {
	if a.opts.Store == nil {
		return "this session only"
	}
	return a.opts.Store.Dir()
}
