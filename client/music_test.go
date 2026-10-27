package client

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/v1rushb/neoncoup/audio"
	"github.com/v1rushb/neoncoup/engine"
)

// recPlayer is an audio.Player that records what the UI asks for.
type recPlayer struct {
	audio.Nop
	intensity []bool
	nexts     int
	notices   chan string
}

func (p *recPlayer) SetIntensity(t bool)    { p.intensity = append(p.intensity, t) }
func (p *recPlayer) NextTrack()             { p.nexts++ }
func (p *recPlayer) Available() bool        { return true }
func (p *recPlayer) Muted() bool            { return false }
func (p *recPlayer) Notices() <-chan string { return p.notices }

func TestTenseTable(t *testing.T) {
	pl := func(coins, hidden int) engine.PublicPlayer {
		return engine.PublicPlayer{Coins: coins, Hidden: hidden, Alive: hidden > 0}
	}
	for _, tc := range []struct {
		players []engine.PublicPlayer
		want    bool
	}{
		{[]engine.PublicPlayer{pl(2, 2), pl(3, 2), pl(4, 2)}, false},
		{[]engine.PublicPlayer{pl(2, 2), pl(3, 1), pl(4, 2)}, true}, // one influence left
		{[]engine.PublicPlayer{pl(7, 2), pl(3, 2), pl(4, 2)}, true}, // a coup is armed
		{[]engine.PublicPlayer{pl(2, 2), pl(0, 0), pl(4, 2)}, true}, // down to two
		{[]engine.PublicPlayer{pl(6, 2), pl(0, 0), pl(4, 2), pl(1, 2)}, false},
	} {
		if got := tenseTable(engine.PlayerView{Players: tc.players}); got != tc.want {
			t.Errorf("%+v: tense=%v, want %v", tc.players, got, tc.want)
		}
	}
}

func TestMusicFollowsTheTablesMood(t *testing.T) {
	sc := newScripted(t, false)
	rec := &recPlayer{}
	sc.a.sound = rec
	sc.do(0, engine.ActionIntent(engine.Tax, -1))
	sc.do(1, engine.ChallengeIntent())
	sc.do(0, engine.RevealIntent(0)) // VEX was bluffing: down to one influence
	if len(rec.intensity) != 1 || !rec.intensity[0] {
		t.Fatalf("intensity calls %v, want one switch to tense", rec.intensity)
	}
	sc.do(1, engine.ActionIntent(engine.Income, -1))
	if len(rec.intensity) != 1 {
		t.Fatal("intensity re-sent without a change")
	}
}

func TestNowPlayingToastAndSkip(t *testing.T) {
	sc := newScripted(t, false)
	rec := &recPlayer{notices: make(chan string, 1)}
	sc.a.sound = rec
	sc.a.Update(audioNoticeMsg("RAIN ON CHROME"))
	out := plain(sc.a.View().Content)
	if !strings.Contains(out, "♪ NOW PLAYING // RAIN ON CHROME") {
		t.Fatalf("no now-playing toast:\n%s", out)
	}
	sc.a.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
	if rec.nexts != 1 {
		t.Fatal("n did not skip the track")
	}
	// The toast clears itself.
	sc.a.now = sc.a.now.Add(5e9)
	sc.a.onTick()
	if strings.Contains(plain(sc.a.View().Content), "NOW PLAYING") {
		t.Fatal("toast never cleared")
	}
}
