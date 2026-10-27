// Package config stores settings, stats and replays in the OS config
// directory (e.g. ~/.config/neoncoup on Linux).
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/v1rushb/neoncoup/protocol"
)

// Config is the persisted user configuration.
type Config struct {
	Name       string  `json:"name"`
	Volume     float64 `json:"volume"`
	Muted      bool    `json:"muted"`
	CRT        bool    `json:"crt"`
	ASCII      bool    `json:"ascii"`
	Relay      string  `json:"relay,omitempty"`
	WindowMS   int     `json:"window_ms"`
	TurnMS     int     `json:"turn_ms"`
	BotLevel   string  `json:"bot_level"`
	Bots       int     `json:"bots"`
	Magistrate bool    `json:"magistrate,omitempty"`
	Port       int     `json:"port"`
	// ReduceMotion turns off screen jolts and glitches.
	ReduceMotion bool `json:"reduce_motion,omitempty"`
	// SkipBoot skips the BIOS boot sequence.
	SkipBoot bool `json:"skip_boot,omitempty"`
	// Tokens maps room codes to rejoin tokens, so a crashed client can reclaim its seat.
	Tokens map[string]Token `json:"tokens,omitempty"`
	// Speed is the presentation speed for solo games and rooms you host.
	Speed string `json:"speed"`
	// MusicVolume and SFXVolume sit under the master Volume.
	MusicVolume float64 `json:"music_volume"`
	SFXVolume   float64 `json:"sfx_volume"`
	// Palette is neon, colorblind or contrast.
	Palette string `json:"palette,omitempty"`
	// NoHints turns off the first-game hints; HintsSeen remembers which
	// hints were already shown.
	NoHints   bool            `json:"no_hints,omitempty"`
	HintsSeen map[string]bool `json:"hints_seen,omitempty"`
	// Version marks the config format; files without one predate pacing.
	Version int `json:"config_version"`
}

// configVersion is the current config format.
const configVersion = 2

// Token is a saved rejoin token.
type Token struct {
	Token string    `json:"token"`
	Addr  string    `json:"addr"`
	Saved time.Time `json:"saved"`
}

// Default returns the configuration used when no file exists.
func Default() Config {
	return Config{
		Name:     "RUNNER",
		Volume:   0.6,
		TurnMS:   60000,
		BotLevel: "normal",
		Bots:     3,
		Port:     7777,
		Speed:    protocol.SpeedRelaxed,
		Version:  configVersion,

		MusicVolume: 0.8,
		SFXVolume:   1,
	}
}

// Store reads and writes the config directory.
type Store struct {
	dir string
	mu  sync.Mutex
}

// Open uses dir; when dir is empty it uses NEONCOUP_HOME, then the OS
// config directory.
func Open(dir string) (*Store, error) {
	if dir == "" {
		dir = os.Getenv("NEONCOUP_HOME")
	}
	if dir == "" {
		base, err := os.UserConfigDir()
		if err != nil {
			return nil, fmt.Errorf("config: no config directory: %w", err)
		}
		dir = filepath.Join(base, "neoncoup")
	}
	if err := os.MkdirAll(filepath.Join(dir, "replays"), 0o755); err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	return &Store{dir: dir}, nil
}

// Dir is the directory the store uses.
func (s *Store) Dir() string { return s.dir }

func (s *Store) path(name string) string { return filepath.Join(s.dir, name) }

// Load reads the config, falling back to defaults for a missing or broken file.
func (s *Store) Load() Config {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := Default()
	b, err := os.ReadFile(s.path("config.json"))
	if err != nil {
		return c
	}
	c.Version = 0
	_ = json.Unmarshal(b, &c)
	if c.Version < 2 {
		// Before pacing the reaction window was always explicit (8 s by
		// default); move old configs to the speed-based default.
		c.WindowMS = 0
	}
	c.Version = configVersion
	c.Speed = protocol.ParseSpeed(c.Speed)
	if c.Volume < 0 || c.Volume > 1 {
		c.Volume = Default().Volume
	}
	if c.MusicVolume < 0 || c.MusicVolume > 1 {
		c.MusicVolume = Default().MusicVolume
	}
	if c.SFXVolume < 0 || c.SFXVolume > 1 {
		c.SFXVolume = Default().SFXVolume
	}
	if n := protocol.CleanName(c.Name); n != "" {
		c.Name = n
	} else {
		c.Name = Default().Name
	}
	if c.Port <= 0 || c.Port > 65535 {
		c.Port = Default().Port
	}
	return c
}

// Save writes the config atomically.
func (s *Store) Save(c Config) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Forget rejoin tokens older than a day.
	for k, t := range c.Tokens {
		if time.Since(t.Saved) > 24*time.Hour {
			delete(c.Tokens, k)
		}
	}
	return writeJSON(s.path("config.json"), c)
}

func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Stats are lifetime numbers for the local player.
type Stats struct {
	Games         int            `json:"games"`
	Wins          int            `json:"wins"`
	Eliminations  int            `json:"eliminations"`
	Bluffs        int            `json:"bluffs"`
	BluffsCaught  int            `json:"bluffs_caught"`
	Challenges    int            `json:"challenges"`
	ChallengesWon int            `json:"challenges_won"`
	Blocks        int            `json:"blocks"`
	CoinsEarned   int            `json:"coins_earned"`
	Coups         int            `json:"coups"`
	WinsByMode    map[string]int `json:"wins_by_mode,omitempty"`
	GamesByMode   map[string]int `json:"games_by_mode,omitempty"`
	LongestGame   int            `json:"longest_game_turns"`
	LastPlayed    time.Time      `json:"last_played"`
	CurrentStreak int            `json:"current_streak"`
	BestStreak    int            `json:"best_streak"`
}

// GameStats is what one game contributes.
type GameStats struct {
	Mode          string
	Won           bool
	Eliminated    bool
	Turns         int
	Bluffs        int
	BluffsCaught  int
	Challenges    int
	ChallengesWon int
	Blocks        int
	CoinsEarned   int
	Coups         int
}

// BluffRate is the share of bluffs that were not caught.
func (s Stats) BluffRate() float64 {
	if s.Bluffs == 0 {
		return 0
	}
	return float64(s.Bluffs-s.BluffsCaught) / float64(s.Bluffs)
}

// ChallengeAccuracy is the share of challenges that caught a bluff.
func (s Stats) ChallengeAccuracy() float64 {
	if s.Challenges == 0 {
		return 0
	}
	return float64(s.ChallengesWon) / float64(s.Challenges)
}

// WinRate is wins per game.
func (s Stats) WinRate() float64 {
	if s.Games == 0 {
		return 0
	}
	return float64(s.Wins) / float64(s.Games)
}

// LoadStats reads lifetime stats.
func (s *Store) LoadStats() Stats {
	s.mu.Lock()
	defer s.mu.Unlock()
	var st Stats
	if b, err := os.ReadFile(s.path("stats.json")); err == nil {
		_ = json.Unmarshal(b, &st)
	}
	return st
}

// AddGame folds one game into the lifetime stats and saves them.
func (s *Store) AddGame(g GameStats) (Stats, error) {
	st := s.LoadStats()
	s.mu.Lock()
	defer s.mu.Unlock()
	st.Games++
	if st.GamesByMode == nil {
		st.GamesByMode = map[string]int{}
	}
	if st.WinsByMode == nil {
		st.WinsByMode = map[string]int{}
	}
	st.GamesByMode[g.Mode]++
	if g.Won {
		st.Wins++
		st.WinsByMode[g.Mode]++
		st.CurrentStreak++
		st.BestStreak = max(st.BestStreak, st.CurrentStreak)
	} else {
		st.CurrentStreak = 0
	}
	if g.Eliminated {
		st.Eliminations++
	}
	st.Bluffs += g.Bluffs
	st.BluffsCaught += g.BluffsCaught
	st.Challenges += g.Challenges
	st.ChallengesWon += g.ChallengesWon
	st.Blocks += g.Blocks
	st.CoinsEarned += g.CoinsEarned
	st.Coups += g.Coups
	st.LongestGame = max(st.LongestGame, g.Turns)
	st.LastPlayed = time.Now()
	return st, writeJSON(s.path("stats.json"), st)
}

// ReplayInfo describes a saved replay file.
type ReplayInfo struct {
	Path    string
	Name    string
	Created time.Time
	Players int
	Winner  string
	Turns   int
}

// SaveReplay writes a replay and returns its path.
func (s *Store) SaveReplay(r protocol.Replay) (string, error) {
	created := r.Created
	if created.IsZero() {
		created = time.Now()
	}
	name := fmt.Sprintf("%s-%s.json", created.Local().Format("20060102-150405"), strings.TrimPrefix(r.Room, protocol.RoomPrefix))
	name = strings.ReplaceAll(name, "-.json", ".json")
	p := filepath.Join(s.dir, "replays", name)
	return p, writeJSON(p, r)
}

// LoadReplay reads a replay file.
func LoadReplay(path string) (protocol.Replay, error) {
	var r protocol.Replay
	b, err := os.ReadFile(path)
	if err != nil {
		return r, err
	}
	if err := json.Unmarshal(b, &r); err != nil {
		return r, fmt.Errorf("replay %s: %w", filepath.Base(path), err)
	}
	if r.Format != protocol.ReplayFormat {
		return r, fmt.Errorf("replay %s has format %d, this build reads %d", filepath.Base(path), r.Format, protocol.ReplayFormat)
	}
	return r, nil
}

// Replays lists saved replays, newest first.
func (s *Store) Replays() ([]ReplayInfo, error) {
	dir := filepath.Join(s.dir, "replays")
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []ReplayInfo
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		p := filepath.Join(dir, e.Name())
		r, err := LoadReplay(p)
		if err != nil {
			continue
		}
		info := ReplayInfo{Path: p, Name: e.Name(), Created: r.Created, Players: len(r.Seats), Turns: len(r.Steps)}
		if r.Winner >= 0 && r.Winner < len(r.Seats) {
			info.Winner = r.Seats[r.Winner].Name
		}
		out = append(out, info)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Created.After(out[j].Created) })
	return out, nil
}
