package audio

import (
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func render(s *Synth, seconds float64) []float32 {
	out := make([]float32, int(seconds*SampleRate))
	// Render in device-sized chunks, as oto would.
	for i := 0; i < len(out); i += 2048 {
		s.Render(out[i:min(len(out), i+2048)])
	}
	return out
}

func stats(buf []float32) (peak, rms float64) {
	sum := 0.0
	for _, v := range buf {
		f := float64(v)
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return math.NaN(), math.NaN()
		}
		peak = math.Max(peak, math.Abs(f))
		sum += f * f
	}
	return peak, math.Sqrt(sum / float64(len(buf)))
}

func TestNoteParsing(t *testing.T) {
	for in, want := range map[string]float64{"A4": 69, "C4": 60, "C#4": 61, "Db4": 61, "B3": 59, "E2": 40, "G#5": 80} {
		if got, err := parseNote(in); err != nil || got != want {
			t.Errorf("parseNote(%q) = %v, %v", in, got, err)
		}
	}
	for _, bad := range []string{"", "H4", "C", "Cx4"} {
		if _, err := parseNote(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
	if math.Abs(midiFreq(69)-440) > 1e-9 || math.Abs(midiFreq(81)-880) > 1e-9 {
		t.Fatal("tuning is off")
	}
}

func TestPatternsLineUp(t *testing.T) {
	// compile() ran in init; check every pattern is two bars.
	for id, sd := range songs {
		for name, p := range sd.Patterns {
			if p.len != 32 {
				t.Errorf("song %d pattern %s has %d steps", id, name, p.len)
			}
		}
	}
	bad := &SongData{Name: "bad", BPM: 120, Order: []string{"X"}, Patterns: map[string]*Pattern{
		"X": {Pulse1: Track{Steps: "C4 . . ."}, Tri: Track{Steps: "C3 ."}},
	}}
	if err := bad.compile(); err == nil {
		t.Fatal("misaligned tracks accepted")
	}
}

func TestSongsRenderCleanly(t *testing.T) {
	for _, song := range []Song{SongMenu, SongGame, SongVictory, SongDefeat, SongTheme, SongRain, SongOverclock, SongRemix} {
		s := NewSynth(SampleRate, 1)
		s.SetVolume(1)
		s.Music(song)
		buf := render(s, 6)
		peak, rms := stats(buf)
		if math.IsNaN(peak) {
			t.Fatalf("song %d produced NaN/Inf", song)
		}
		if peak > 1 {
			t.Fatalf("song %d clips: peak %v", song, peak)
		}
		if rms < 0.02 {
			t.Fatalf("song %d is (nearly) silent: rms %v", song, rms)
		}
	}
}

func TestJinglesEnd(t *testing.T) {
	for _, song := range []Song{SongVictory, SongDefeat} {
		s := NewSynth(SampleRate, 1)
		s.SetVolume(1)
		s.Music(song)
		render(s, 6)
		if !s.deck().seq.done {
			t.Fatalf("jingle %d still playing after 6s", song)
		}
		_, rms := stats(render(s, 1))
		if rms > 0.002 {
			t.Fatalf("jingle %d not silent after ending: %v", song, rms)
		}
	}
}

func TestSequencerTiming(t *testing.T) {
	s := NewSynth(SampleRate, 1)
	s.Music(SongMenu)
	stepLen := 60 / menuSong.BPM / 4 // 16th note at 96 BPM
	// Render to the middle of step 5.
	render(s, stepLen*5.5)
	seq := &s.deck().seq
	if seq.step != 5 || seq.patName != "A" {
		t.Fatalf("at %.3fs the sequencer is on %s/%d, want A/5", s.Now(), seq.patName, seq.step)
	}
	render(s, stepLen*32)
	if seq.patName != "B" || seq.step != 5 {
		t.Fatalf("after a pattern: %s/%d, want B/5", seq.patName, seq.step)
	}
}

func TestGameLoopVaries(t *testing.T) {
	s := NewSynth(SampleRate, 42)
	s.Music(SongTheme)
	render(s, 140)
	seen := map[string]bool{}
	for _, p := range s.deck().seq.played {
		seen[p] = true
	}
	for _, want := range []string{"A1", "A2", "B1", "B2", "C1", "D1"} {
		if !seen[want] {
			t.Errorf("section %s never played in 150s: %v", want, s.deck().seq.played)
		}
	}
	// Sections stay paired (x1 is always followed by x2).
	h := s.deck().seq.played
	for i := 0; i+1 < len(h); i++ {
		if h[i][1] == '1' && h[i+1] != string(h[i][0])+"2" {
			t.Fatalf("%s followed by %s", h[i], h[i+1])
		}
	}
	// Re-requesting the same song does not restart it.
	pos := s.deck().seq.played
	s.Music(SongTheme)
	render(s, 0.1)
	if len(s.deck().seq.played) < len(pos) {
		t.Fatal("song restarted")
	}
}

func TestEffectsMakeSound(t *testing.T) {
	for e := range effects {
		s := NewSynth(SampleRate, 1)
		s.SetVolume(1)
		s.SFX(e)
		// UI ticks are deliberately tiny, so judge effects by their peak.
		peak, _ := stats(render(s, 0.7))
		if math.IsNaN(peak) || peak > 1 || peak < 0.03 {
			t.Errorf("effect %d: peak %v", e, peak)
		}
	}
	s := NewSynth(SampleRate, 1)
	s.SetVolume(1)
	s.Stinger()
	if peak, rms := stats(render(s, stingerLen)); math.IsNaN(peak) || rms < 0.01 {
		t.Fatalf("stinger: peak %v rms %v", peak, rms)
	}
}

func TestStingerDucksMusic(t *testing.T) {
	s := NewSynth(SampleRate, 1)
	s.Music(SongGame)
	render(s, 1)
	if s.duck < 0.99 {
		t.Fatal("music ducked without a stinger")
	}
	s.Stinger()
	render(s, 0.3)
	if s.duck > 0.4 {
		t.Fatalf("duck %v during the stinger", s.duck)
	}
	render(s, stingerLen+2)
	if s.duck < 0.95 {
		t.Fatalf("music did not recover: duck %v", s.duck)
	}
}

func TestMuteAndVolume(t *testing.T) {
	s := NewSynth(SampleRate, 1)
	s.SetVolume(1)
	s.Music(SongGame)
	_, loud := stats(render(s, 2))
	s.SetMuted(true, 1)
	render(s, 0.3)
	if _, rms := stats(render(s, 1)); rms > 0.001 {
		t.Fatalf("muted output rms %v", rms)
	}
	s.SetMuted(false, 0.3)
	render(s, 0.3)
	_, quiet := stats(render(s, 2))
	if !(quiet < loud*0.6 && quiet > 0.005) {
		t.Fatalf("volume 0.3 rms %v vs volume 1 rms %v", quiet, loud)
	}
}

func TestDeterministic(t *testing.T) {
	a, b := NewSynth(SampleRate, 9), NewSynth(SampleRate, 9)
	for _, s := range []*Synth{a, b} {
		s.Music(SongGame)
		s.SFX(SFXCoin)
	}
	x, y := render(a, 3), render(b, 3)
	for i := range x {
		if x[i] != y[i] {
			t.Fatalf("diverged at sample %d", i)
		}
	}
}

func TestCommandsNeverBlock(t *testing.T) {
	s := NewSynth(SampleRate, 1)
	done := make(chan struct{})
	go func() {
		for range 100000 {
			s.SFX(SFXCoin)
			s.Stinger()
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("commands blocked without a reader")
	}
	render(s, 0.1) // and the synth survives the flood
}

func TestPCMReader(t *testing.T) {
	s := NewSynth(SampleRate, 1)
	s.SFX(SFXCoin)
	r := &pcmReader{s: s}
	p := make([]byte, 4002) // not a multiple of 4
	n, err := r.Read(p)
	if err != nil || n != 4000 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	nonzero := false
	for i := 0; i < n; i += 4 {
		v := math.Float32frombits(binary.LittleEndian.Uint32(p[i:]))
		if v < -1 || v > 1 {
			t.Fatalf("sample %v out of range", v)
		}
		nonzero = nonzero || v != 0
	}
	if !nonzero {
		t.Fatal("silence from an effect")
	}
}

func TestNopPlayer(t *testing.T) {
	var p Player = Nop{}
	p.Music(SongMenu)
	p.SFX(SFXCoin)
	p.Stinger()
	p.SetVolume(1)
	p.SetMuted(false)
	if p.Available() || !p.Muted() {
		t.Fatal("Nop must report no device and muted")
	}
	p.Close()
}

// TestWriteWAV renders every song and effect to WAV files when
// NEONCOUP_WAV is set to a directory, for listening outside the game.
func TestWriteWAV(t *testing.T) {
	dir := os.Getenv("NEONCOUP_WAV")
	if dir == "" {
		t.Skip("set NEONCOUP_WAV=dir to export audio")
	}
	write := func(name string, buf []float32) {
		f, err := os.Create(filepath.Join(dir, name+".wav"))
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		data := make([]byte, len(buf)*2)
		for i, v := range buf {
			binary.LittleEndian.PutUint16(data[i*2:], uint16(int16(v*32767)))
		}
		hdr := make([]byte, 44)
		copy(hdr[0:], "RIFF")
		binary.LittleEndian.PutUint32(hdr[4:], uint32(36+len(data)))
		copy(hdr[8:], "WAVEfmt ")
		binary.LittleEndian.PutUint32(hdr[16:], 16)
		binary.LittleEndian.PutUint16(hdr[20:], 1)
		binary.LittleEndian.PutUint16(hdr[22:], 1)
		binary.LittleEndian.PutUint32(hdr[24:], SampleRate)
		binary.LittleEndian.PutUint32(hdr[28:], SampleRate*2)
		binary.LittleEndian.PutUint16(hdr[32:], 2)
		binary.LittleEndian.PutUint16(hdr[34:], 16)
		copy(hdr[36:], "data")
		binary.LittleEndian.PutUint32(hdr[40:], uint32(len(data)))
		_, _ = f.Write(append(hdr, data...))
	}
	for name, song := range map[string]Song{"menu": SongMenu, "theme": SongTheme, "rain": SongRain, "overclock": SongOverclock,
		"remix": SongRemix, "victory": SongVictory, "defeat": SongDefeat} {
		s := NewSynth(SampleRate, 3)
		s.SetVolume(0.8)
		s.Music(song)
		write("song-"+name, render(s, 40))
	}
	names := map[Effect]string{SFXCoin: "coin", SFXFlip: "flip", SFXChallenge: "challenge", SFXBlock: "block", SFXGlitch: "glitch", SFXSelect: "select", SFXTick: "tick", SFXError: "error", SFXYourTurn: "your_turn"}
	for e, name := range names {
		s := NewSynth(SampleRate, 3)
		s.SetVolume(0.8)
		s.SFX(e)
		write("sfx-"+name, render(s, 1))
	}
	s := NewSynth(SampleRate, 3)
	s.SetVolume(0.8)
	s.Music(SongGame)
	render(s, 2)
	s.Stinger()
	write("stinger-over-game", render(s, 4))
}
