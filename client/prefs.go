package client

import (
	"strconv"

	tea "charm.land/bubbletea/v2"

	"github.com/v1rushb/neoncoup/client/art"
	"github.com/v1rushb/neoncoup/client/theme"
	"github.com/v1rushb/neoncoup/config"
	"github.com/v1rushb/neoncoup/protocol"
)

// Preferences shared by the Settings screen and the in-game settings
// overlay (esc): speed, hints and accessibility. Audio levels sit with the
// other audio fields.

// prefFields edits c. In a game, speed changes the running solo game (and is
// read-only at a table someone else hosts); in the menu it is the default for
// new games.
func (a *App) prefFields(c *config.Config, inGame bool) []field {
	var fs []field
	speed := field{label: "Speed", value: func() string { return speedTitle(c.Speed) },
		left:  func() { a.setSpeed(c, cycle(protocol.Speeds, protocol.ParseSpeed(c.Speed), -1), inGame) },
		right: func() { a.setSpeed(c, cycle(protocol.Speeds, protocol.ParseSpeed(c.Speed), 1), inGame) },
		help:  speedHelp}
	if inGame && a.game != nil && !a.game.soloControls(a) {
		speed = field{label: "Speed", value: func() string { return speedTitle(a.game.speed) + "  (the host's choice)" },
			help: "The host picks the speed for everyone at an online table."}
	}
	fs = append(fs, speed,
		field{label: "Hints", value: func() string { return onOff(!c.NoHints) },
			left: func() { a.toggleHints(c) }, right: func() { a.toggleHints(c) },
			help: "A tip the first time each situation comes up. Turning hints back on shows them all again."},
		field{label: "Reduce motion", value: func() string { return onOff(c.ReduceMotion) },
			left: func() { c.ReduceMotion = !c.ReduceMotion }, right: func() { c.ReduceMotion = !c.ReduceMotion },
			help: "No sliding cards, glitches, pulses or screen jolts. Every beat still plays out in full."})
	if !a.opts.Remote {
		fs = append(fs, field{label: "Palette", value: func() string { return theme.PaletteTitle(c.Palette) },
			left:  func() { a.setPalette(c, cycle(theme.PaletteNames, paletteName(c.Palette), -1)) },
			right: func() { a.setPalette(c, cycle(theme.PaletteNames, paletteName(c.Palette), 1)) },
			help:  "Colorblind-safe swaps red/green for vermillion and bluish green. High contrast is black, white and full color."})
	}
	return fs
}

func paletteName(p string) string {
	if p == "" {
		return "neon"
	}
	return p
}

func (a *App) setSpeed(c *config.Config, s string, inGame bool) {
	c.Speed = s
	if inGame && a.sess != nil {
		a.sess.Send(protocol.TControl, protocol.Control{Speed: s})
	}
}

func (a *App) toggleHints(c *config.Config) {
	c.NoHints = !c.NoHints
	if !c.NoHints {
		c.HintsSeen = nil
	}
}

// setPalette switches colors at once, rebuilding the styles that hold them.
func (a *App) setPalette(c *config.Config, p string) {
	c.Palette = p
	a.applyPalette(p)
}

func (a *App) applyPalette(p string) {
	if a.opts.Remote {
		return // the palette is process-wide; SSH sessions share one
	}
	theme.Use(p)
	a.t = theme.New(a.t.ASCII)
	a.art = art.NewRenderer(a.t)
}

// prefsOverlay is the in-game settings box.
type prefsOverlay struct {
	form
	paused bool // opening it paused a solo game
}

func (g *gameScreen) openPrefs(a *App) {
	g.prefs = &prefsOverlay{}
	if g.soloControls(a) && !g.paused {
		g.setPaused(a, true)
		g.prefs.paused = true
	}
}

func (a *App) inGameFields() []field {
	c := &a.cfg
	fs := a.prefFields(c, true)
	if !a.opts.Remote {
		pct := func(v *float64) func() string { return func() string { return pctLabel(*v) } }
		fs = append(fs,
			field{label: "Master volume", value: pct(&c.Volume),
				left: func() { c.Volume = max(0, c.Volume-0.1); a.sound.SetVolume(c.Volume) }, right: func() { c.Volume = min(1, c.Volume+0.1); a.sound.SetVolume(c.Volume) }},
			field{label: "Music volume", value: pct(&c.MusicVolume),
				left:  func() { c.MusicVolume = max(0, c.MusicVolume-0.1); a.sound.SetMusicVolume(c.MusicVolume) },
				right: func() { c.MusicVolume = min(1, c.MusicVolume+0.1); a.sound.SetMusicVolume(c.MusicVolume) },
				help:  "n skips to the next track."},
			field{label: "SFX volume", value: pct(&c.SFXVolume),
				left:  func() { c.SFXVolume = max(0, c.SFXVolume-0.1); a.sound.SetSFXVolume(c.SFXVolume) },
				right: func() { c.SFXVolume = min(1, c.SFXVolume+0.1); a.sound.SetSFXVolume(c.SFXVolume) }},
			field{label: "Muted", value: func() string { return onOff(c.Muted) },
				left: func() { c.Muted = !c.Muted; a.sound.SetMuted(c.Muted) }, right: func() { c.Muted = !c.Muted; a.sound.SetMuted(c.Muted) }})
	}
	return append(fs, field{label: "CRT scanlines", value: func() string { return onOff(c.CRT) },
		left: func() { c.CRT = !c.CRT }, right: func() { c.CRT = !c.CRT }})
}

func (p *prefsOverlay) key(a *App, g *gameScreen, msg tea.KeyPressMsg) {
	switch p.form.key(a, a.inGameFields(), msg) {
	case "back", "submit":
		a.saveConfig()
		g.prefs = nil
		if p.paused {
			g.setPaused(a, false)
		}
	}
}

func (p *prefsOverlay) view(a *App) string {
	sub := "saved to " + a.configPath()
	if p.paused {
		sub = "game paused · " + sub
	}
	return p.box(a, "SETTINGS", sub, a.inGameFields(), "to close")
}

func pctLabel(v float64) string { return strconv.Itoa(int(v*100+0.5)) + "%" }
