package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/v1rushb/neoncoup/engine"
	"github.com/v1rushb/neoncoup/protocol"
)

func TestConfigRoundTripAndDefaults(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if c := s.Load(); c.Name != "RUNNER" || c.Volume != 0.6 {
		t.Fatalf("defaults %+v", c)
	}
	c := s.Load()
	c.Name = "\x1b[31mVEX"
	c.Volume = 0.25
	c.Muted = true
	if err := s.Save(c); err != nil {
		t.Fatal(err)
	}
	got := s.Load()
	if got.Name != "VEX" || got.Volume != 0.25 || !got.Muted {
		t.Fatalf("%+v", got)
	}
	_ = os.WriteFile(filepath.Join(s.Dir(), "config.json"), []byte("{broken"), 0o644)
	if s.Load().Name != "RUNNER" {
		t.Fatal("broken file should fall back to defaults")
	}
}

func TestStatsAccumulate(t *testing.T) {
	s, _ := Open(t.TempDir())
	s.AddGame(GameStats{Mode: "solo", Won: true, Bluffs: 4, BluffsCaught: 1, Challenges: 2, ChallengesWon: 1, Turns: 30})
	st, err := s.AddGame(GameStats{Mode: "online", Won: false, Eliminated: true, Bluffs: 0, Turns: 12})
	if err != nil {
		t.Fatal(err)
	}
	if st.Games != 2 || st.Wins != 1 || st.WinsByMode["solo"] != 1 || st.LongestGame != 30 || st.CurrentStreak != 0 || st.BestStreak != 1 {
		t.Fatalf("%+v", st)
	}
	if st.BluffRate() != 0.75 || st.ChallengeAccuracy() != 0.5 || st.WinRate() != 0.5 {
		t.Fatalf("rates %v %v %v", st.BluffRate(), st.ChallengeAccuracy(), st.WinRate())
	}
}

func TestReplaysSaveAndList(t *testing.T) {
	s, _ := Open(t.TempDir())
	r := protocol.Replay{Format: protocol.ReplayFormat, Created: time.Now(), Room: "NEON-AB23",
		Config: engine.Config{Players: 2, Seed: 1}, Seats: []protocol.Seat{{Name: "A"}, {Name: "B"}}, Winner: 1}
	p, err := s.SaveReplay(r)
	if err != nil {
		t.Fatal(err)
	}
	got, err := LoadReplay(p)
	if err != nil || got.Config.Seed != 1 {
		t.Fatal(got, err)
	}
	list, err := s.Replays()
	if err != nil || len(list) != 1 || list[0].Winner != "B" {
		t.Fatalf("%+v %v", list, err)
	}
	bad := r
	bad.Format = 99
	_ = writeJSON(filepath.Join(s.Dir(), "replays", "future.json"), bad)
	if _, err := LoadReplay(filepath.Join(s.Dir(), "replays", "future.json")); err == nil {
		t.Fatal("future replay format accepted")
	}
}
