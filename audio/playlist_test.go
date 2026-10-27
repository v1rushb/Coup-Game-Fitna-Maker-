package audio

import (
	"math"
	"testing"
)

// trackSeconds is how long a track plays before the playlist moves on.
func trackSeconds(sd *SongData) float64 {
	step := 60 / sd.BPM / 4
	if sd.MaxPatterns > 0 {
		return float64(sd.MaxPatterns*32) * step
	}
	n := 0
	for _, name := range sd.Order {
		n += sd.Patterns[name].len
	}
	return float64(n) * step
}

func TestPlaylistTracksRunOneAndAHalfToThreeMinutes(t *testing.T) {
	for _, tr := range GameTracks {
		sd := songs[tr]
		if sd.Title == "" {
			t.Errorf("track %d has no title", tr)
		}
		if sd.Loop {
			t.Errorf("%s loops on its own; playlist tracks must end", sd.Title)
		}
		if d := trackSeconds(sd); d < 90 || d > 180 {
			t.Errorf("%s runs %.0fs, want 90-180s", sd.Title, d)
		}
		// Sections, not a short loop: plenty of distinct patterns.
		if len(sd.Patterns) < 8 {
			t.Errorf("%s has only %d patterns", sd.Title, len(sd.Patterns))
		}
	}
	if songs[SongRain].BPM != 90 || songs[SongOverclock].BPM != 150 {
		t.Fatal("tempos drifted from the brief")
	}
	if songs[SongRemix].BPM == songs[SongTheme].BPM {
		t.Fatal("the remix should change the tempo")
	}
}

func TestPlaylistRotatesAndNeverRepeatsBackToBack(t *testing.T) {
	s := NewSynth(SampleRate, 5)
	s.playlist = true
	s.deck().song = s.firstTrack()
	if s.deck().song != SongRain {
		t.Fatal("a calm game should open with Rain on Chrome")
	}
	counts := map[Song]int{}
	prev := s.deck().song
	for range 4000 {
		next := s.pickNext()
		if next == prev {
			t.Fatalf("%d played twice in a row", next)
		}
		counts[next]++
		s.deck().song, prev = next, next
	}
	for _, tr := range GameTracks {
		if c := counts[tr]; c < 800 {
			t.Fatalf("track %d played %d times in 4000; the rotation is lopsided: %v", tr, c, counts)
		}
	}
}

func TestTenseGamesFavorOverclock(t *testing.T) {
	s := NewSynth(SampleRate, 5)
	s.playlist = true
	s.deck().song = SongRain
	s.tense = true
	if got := s.pickNext(); got != SongOverclock {
		t.Fatalf("tense: next is %d, want Overclock", got)
	}
	s.deck().song = SongOverclock
	if got := s.pickNext(); got == SongOverclock {
		t.Fatal("Overclock repeated back to back")
	}
}

// nearEnd moves the active track to its last two patterns.
func nearEnd(s *Synth) {
	d := s.deck()
	d.seq.orderAt = len(d.seq.song.Order) - 2
	d.seq.enter(d.seq.song.Order[d.seq.orderAt])
}

func TestTracksCrossfadeSmoothly(t *testing.T) {
	s := NewSynth(SampleRate, 2)
	s.SetVolume(1)
	s.Music(SongGame)
	render(s, 1)
	if got := <-s.Notices(); got != "RAIN ON CHROME" {
		t.Fatalf("first notice %q", got)
	}
	nearEnd(s)
	first := s.active
	var gains [][2]float64
	var buf []float32
	for range 150 { // 15 s in 0.1 s chunks
		buf = append(buf, render(s, 0.1)...)
		gains = append(gains, [2]float64{s.decks[first].gain, s.decks[1-first].gain})
	}
	select {
	case next := <-s.Notices():
		if next == "RAIN ON CHROME" {
			t.Fatal("the next track repeated the first")
		}
	default:
		t.Fatal("no next track before the first ended")
	}
	// The outgoing deck only falls and the incoming one only rises, over
	// about three seconds, with both audible in between.
	both, fadeChunks := false, 0
	for i := 1; i < len(gains); i++ {
		if gains[i][0] > gains[i-1][0]+1e-9 || gains[i][1] < gains[i-1][1]-1e-9 {
			t.Fatalf("gains jumped at chunk %d: %v -> %v", i, gains[i-1], gains[i])
		}
		if gains[i][0] > 0 && gains[i][1] > 0 {
			both = true
			fadeChunks++
		}
	}
	if !both || fadeChunks < 25 || fadeChunks > 32 {
		t.Fatalf("crossfade overlapped for %d chunks of 0.1s, want about 3s", fadeChunks)
	}
	if peak, _ := stats(buf); math.IsNaN(peak) || peak > 1 {
		t.Fatalf("crossfade output peak %v", peak)
	}
}

func TestNextTrackSkips(t *testing.T) {
	s := NewSynth(SampleRate, 3)
	s.Music(SongGame)
	render(s, 1)
	<-s.Notices()
	first := s.deck().song
	s.NextTrack()
	s.NextTrack() // a second press during the fade is ignored
	render(s, 0.5)
	if s.deck().song == first {
		t.Fatal("NextTrack did not change the track")
	}
	<-s.Notices()
	select {
	case n := <-s.Notices():
		t.Fatalf("a double press skipped twice (%q)", n)
	default:
	}
	render(s, skipFade+0.5)
	if s.decks[1-s.active].gain != 0 {
		t.Fatal("the skipped track never faded out")
	}
}

func TestTensionAddsAPercussionLayer(t *testing.T) {
	s := NewSynth(SampleRate, 4)
	s.Music(SongGame)
	render(s, 4)
	if s.deck().seq.extraHits != 0 {
		t.Fatal("percussion layer playing in a calm game")
	}
	s.SetIntensity(true)
	render(s, 4)
	if s.deck().seq.extraHits == 0 {
		t.Fatal("tension added no percussion")
	}
	s.SetIntensity(false)
	render(s, 0.1)
	hits := s.deck().seq.extraHits
	render(s, 3)
	if s.deck().seq.extraHits != hits {
		t.Fatal("the layer kept playing after the tension passed")
	}
}

func TestMusicAndEffectsHaveSeparateVolumes(t *testing.T) {
	level := func(music, sfx float64) float64 {
		s := NewSynth(SampleRate, 1)
		s.SetVolume(1)
		s.SetMusicVolume(music)
		s.SetSFXVolume(sfx)
		s.Music(SongMenu)
		render(s, 1)
		s.SFX(SFXChallenge)
		_, rms := stats(render(s, 0.4))
		return rms
	}
	full, noMusic, noSFX, none := level(1, 1), level(0, 1), level(1, 0), level(0, 0)
	if !(noMusic < full && noSFX < full && none < 0.001 && noMusic > 0.005 && noSFX > 0.005) {
		t.Fatalf("rms: both %v, sfx only %v, music only %v, neither %v", full, noMusic, noSFX, none)
	}
}
