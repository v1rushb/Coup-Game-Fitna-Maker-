package audio

import (
	"math"
	"math/rand/v2"
	"slices"
)

// SampleRate is the synth's output rate.
const SampleRate = 44100

// Crossfades.
const (
	trackFade = 3.0  // between in-game tracks
	skipFade  = 1.5  // when the player skips a track
	cutFade   = 0.25 // switching to menu music or a jingle
)

type cmdKind uint8

const (
	cmdMusic cmdKind = iota
	cmdSFX
	cmdStinger
	cmdVolume
	cmdMute
	cmdMusicVolume
	cmdSFXVolume
	cmdNext
	cmdIntensity
)

type command struct {
	kind   cmdKind
	song   Song
	effect Effect
	value  float64
	flag   bool
}

// deck is one music voice: a chip, its sequencer and a fader. There are two,
// so one track can fade in while the other fades out.
type deck struct {
	chip   chip
	seq    sequencer
	song   Song
	gain   float64
	target float64
	rate   float64 // gain change per second
}

func (d *deck) audible() bool { return d.gain > 0 || d.target > 0 }

// Synth renders the game's audio. It is fed commands through a channel and
// pulls samples on demand, so it can run on the audio device's goroutine
// while the UI only ever does a non-blocking send.
type Synth struct {
	rate float64
	dt   float64
	now  float64

	decks       [2]deck
	active      int // the deck playing (or fading in) the current song
	fx, sting   chip
	fxq, stingq fxPlayer
	filt        filter
	rng         *rand.Rand

	// The in-game playlist.
	playlist bool
	bag      []Song // the rest of this shuffled round
	tense    bool
	notices  chan string

	vol, volTarget   float64
	music, musicT    float64
	sfx, sfxT        float64
	duck, duckTarget float64
	duckUntil        float64

	cmds chan command
}

// NewSynth creates a synth at the given sample rate.
func NewSynth(rate int, seed uint64) *Synth {
	return &Synth{
		rate:       float64(rate),
		dt:         1 / float64(rate),
		rng:        rand.New(rand.NewPCG(seed, 0x1985)),
		vol:        0.6,
		volTarget:  0.6,
		music:      1,
		musicT:     1,
		sfx:        1,
		sfxT:       1,
		duck:       1,
		duckTarget: 1,
		cmds:       make(chan command, 128),
		notices:    make(chan string, 4),
	}
}

// deck is the deck playing the current song.
func (s *Synth) deck() *deck { return &s.decks[s.active] }

// send queues a command, dropping it rather than blocking when full.
func (s *Synth) send(c command) {
	select {
	case s.cmds <- c:
	default:
	}
}

func (s *Synth) apply(c command) {
	switch c.kind {
	case cmdMusic:
		s.music1(c.song)
	case cmdSFX:
		if e, ok := effects[c.effect]; ok {
			s.fxq.play(e, s.now)
		}
	case cmdStinger:
		s.stingq.play(stinger(), s.now)
		s.duckTarget = 0.3
		s.duckUntil = s.now + stingerLen
	case cmdVolume:
		s.volTarget = clamp01(c.value)
	case cmdMute:
		if c.flag {
			s.volTarget = 0
		} else {
			s.volTarget = clamp01(c.value)
		}
	case cmdMusicVolume:
		s.musicT = clamp01(c.value)
	case cmdSFXVolume:
		s.sfxT = clamp01(c.value)
	case cmdNext:
		if s.playlist && !s.fading() {
			s.startTrack(s.pickNext(), skipFade)
		}
	case cmdIntensity:
		s.tense = c.flag
		s.deck().seq.extra = s.tense && s.playlist
	}
}

func (s *Synth) music1(song Song) {
	if song == SongGame {
		if !s.playlist {
			s.playlist = true
			s.bag = nil
			s.startTrack(s.firstTrack(), cutFade)
		}
		return
	}
	cur := s.deck()
	if !s.playlist && song == cur.song && song != SongVictory && song != SongDefeat && cur.audible() {
		return // already playing; don't restart the loop
	}
	s.playlist = false
	if song == SongNone {
		for i := range s.decks {
			s.decks[i].target, s.decks[i].rate = 0, 1/0.03
		}
		cur.song = SongNone
		return
	}
	s.startTrack(song, cutFade)
}

// startTrack starts a song on the quieter deck and crossfades to it.
func (s *Synth) startTrack(song Song, fade float64) {
	next := 1 - s.active
	if s.decks[next].gain > s.decks[s.active].gain {
		next = s.active // mid-fade: reuse whichever is quieter
	}
	old := s.deck()
	old.target, old.rate = 0, 1/fade
	old.seq.extra = false
	d := &s.decks[next]
	d.seq.start(songs[song], &d.chip, s.rng)
	d.song = song
	d.gain, d.target, d.rate = 0, 1, 1/fade
	d.seq.extra = s.tense && s.playlist
	s.active = next
	if s.playlist {
		if sd := songs[song]; sd != nil && sd.Title != "" {
			select {
			case s.notices <- sd.Title:
			default:
			}
		}
	}
}

// fading reports whether a crossfade is in progress.
func (s *Synth) fading() bool {
	other := &s.decks[1-s.active]
	return other.gain > 0 || s.deck().gain < 1
}

// firstTrack opens the playlist: calm unless the game is already tense.
func (s *Synth) firstTrack() Song {
	first := SongRain
	if s.tense {
		first = SongOverclock
	}
	s.bag = s.shuffled(first)
	return first
}

// pickNext chooses the next in-game track: Overclock when the game is tense,
// otherwise the next of a shuffled round of all four, never the track that
// is playing now.
func (s *Synth) pickNext() Song {
	cur := s.deck().song
	if s.tense && cur != SongOverclock {
		s.bag = slices.DeleteFunc(s.bag, func(t Song) bool { return t == SongOverclock })
		return SongOverclock
	}
	for range 2 {
		if len(s.bag) == 0 {
			s.bag = s.shuffled(SongNone)
		}
		for i, t := range s.bag {
			if t != cur {
				s.bag = slices.Delete(s.bag, i, i+1)
				return t
			}
		}
		s.bag = nil // only the current track was left in this round
	}
	return GameTracks[0]
}

// shuffled is a new round of every game track except skip.
func (s *Synth) shuffled(skip Song) []Song {
	var out []Song
	for _, t := range GameTracks {
		if t != skip {
			out = append(out, t)
		}
	}
	s.rng.Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
	return out
}

func clamp01(v float64) float64 { return math.Max(0, math.Min(1, v)) }

// Render fills out with mono samples in [-1, 1].
func (s *Synth) Render(out []float32) {
	for {
		select {
		case c := <-s.cmds:
			s.apply(c)
			continue
		default:
		}
		break
	}
	if s.playlist && !s.fading() && s.deck().seq.remaining() <= trackFade {
		s.startTrack(s.pickNext(), trackFade)
	}
	for i := range out {
		s.now += s.dt
		music := 0.0
		for k := range s.decks {
			d := &s.decks[k]
			if !d.audible() {
				continue
			}
			d.seq.advance(&d.chip, s.dt)
			if d.gain != d.target {
				step := d.rate * s.dt
				if d.gain < d.target {
					d.gain = math.Min(d.target, d.gain+step)
				} else {
					d.gain = math.Max(d.target, d.gain-step)
				}
			}
			// An equal-power curve keeps a crossfade from dipping.
			music += d.chip.sample(s.dt) * math.Sin(d.gain*math.Pi/2)
		}
		s.fxq.run(&s.fx, s.now)
		s.stingq.run(&s.sting, s.now)
		if s.duckTarget < 1 && s.now >= s.duckUntil {
			s.duckTarget = 1
		}
		// Duck fast, recover slowly.
		rate := 40.0
		if s.duckTarget > s.duck {
			rate = 2.5
		}
		s.duck += (s.duckTarget - s.duck) * math.Min(1, rate*s.dt)
		smooth := math.Min(1, 30*s.dt)
		s.vol += (s.volTarget - s.vol) * smooth
		s.music += (s.musicT - s.music) * smooth
		s.sfx += (s.sfxT - s.sfx) * smooth

		m := music*s.duck*0.9*s.music + (s.fx.sample(s.dt)*0.8+s.sting.sample(s.dt)*0.75)*s.sfx
		x := s.filt.apply(m, s.dt) * 2.2 * s.vol
		out[i] = float32(math.Tanh(x))
	}
}

// Now is the synth clock in seconds (tests).
func (s *Synth) Now() float64 { return s.now }

// Commands. They never block.

// Music switches songs.
func (s *Synth) Music(song Song) { s.send(command{kind: cmdMusic, song: song}) }

// SFX plays an effect.
func (s *Synth) SFX(e Effect) { s.send(command{kind: cmdSFX, effect: e}) }

// Stinger plays the tension sting and ducks the music.
func (s *Synth) Stinger() { s.send(command{kind: cmdStinger}) }

// SetVolume sets the master volume (0..1).
func (s *Synth) SetVolume(v float64) { s.send(command{kind: cmdVolume, value: v}) }

// SetMuted silences output; v is the volume to return to when unmuting.
func (s *Synth) SetMuted(m bool, v float64) { s.send(command{kind: cmdMute, flag: m, value: v}) }

// SetMusicVolume sets the music level (0..1).
func (s *Synth) SetMusicVolume(v float64) { s.send(command{kind: cmdMusicVolume, value: v}) }

// SetSFXVolume sets the effects level (0..1).
func (s *Synth) SetSFXVolume(v float64) { s.send(command{kind: cmdSFXVolume, value: v}) }

// NextTrack skips to the next playlist track.
func (s *Synth) NextTrack() { s.send(command{kind: cmdNext}) }

// SetIntensity marks the game tense or calm.
func (s *Synth) SetIntensity(tense bool) { s.send(command{kind: cmdIntensity, flag: tense}) }

// Notices delivers playlist track titles as they start.
func (s *Synth) Notices() <-chan string { return s.notices }
