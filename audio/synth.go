package audio

import (
	"math"
	"math/rand/v2"
)

// SampleRate is the synth's output rate.
const SampleRate = 44100

type cmdKind uint8

const (
	cmdMusic cmdKind = iota
	cmdSFX
	cmdStinger
	cmdVolume
	cmdMute
)

type command struct {
	kind   cmdKind
	song   Song
	effect Effect
	value  float64
	flag   bool
}

// Synth renders the game's audio. It is fed commands through a channel and
// pulls samples on demand, so it can run on the audio device's goroutine
// while the UI only ever does a non-blocking send.
type Synth struct {
	rate float64
	dt   float64
	now  float64

	music, fx, sting chip
	seq              sequencer
	fxq, stingq      fxPlayer
	filt             filter
	rng              *rand.Rand
	song             Song

	vol, volTarget   float64
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
		duck:       1,
		duckTarget: 1,
		cmds:       make(chan command, 128),
	}
}

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
		if c.song == s.song && c.song != SongVictory && c.song != SongDefeat {
			return // already playing; don't restart the loop
		}
		s.song = c.song
		s.seq.start(songs[c.song], &s.music, s.rng)
	case cmdSFX:
		if e, ok := effects[c.effect]; ok {
			s.fxq.play(e, s.now)
		}
	case cmdStinger:
		s.stingq.play(stinger(), s.now)
		s.duckTarget = 0.3
		s.duckUntil = s.now + stingerLen
	case cmdVolume:
		s.volTarget = math.Max(0, math.Min(1, c.value))
	case cmdMute:
		if c.flag {
			s.volTarget = 0
		} else {
			s.volTarget = math.Max(0, math.Min(1, c.value))
		}
	}
}

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
	for i := range out {
		s.now += s.dt
		s.seq.advance(&s.music, s.dt)
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
		s.vol += (s.volTarget - s.vol) * math.Min(1, 30*s.dt)

		m := s.music.sample(s.dt)*s.duck*0.9 + s.fx.sample(s.dt)*0.8 + s.sting.sample(s.dt)*0.75
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
