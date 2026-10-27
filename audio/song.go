package audio

import (
	"fmt"
	"math"
	"math/rand/v2"
	"strconv"
	"strings"
)

// Songs are written as text patterns, one token per 16th-note step:
//
//	C4  D#5  Bb3   note on (optionally "C4:9" for a volume of 9)
//	.              hold the previous note
//	-              note off / rest
//
// The noise track uses drum letters instead of notes: k kick, s snare,
// h closed hat, o open hat, c crash (".", "-" and ":v" work the same).

type stepKind uint8

const (
	stepHold stepKind = iota
	stepOff
	stepNote
	stepDrum
)

type step struct {
	kind stepKind
	note float64
	vol  float64
	drum byte
}

// Track is one channel of a pattern.
type Track struct {
	Inst  Instrument
	Steps string
	steps []step
}

// Pattern is a block of steps on the four channels.
type Pattern struct {
	Pulse1, Pulse2, Tri, Noise Track
	len                        int
}

// Next is a weighted transition to another pattern.
type Next struct {
	Pattern string
	Weight  float64
}

// SongData describes a piece of music.
type SongData struct {
	Name string
	// Title is shown when the track starts ("now playing").
	Title    string
	BPM      float64
	Patterns map[string]*Pattern
	// Order is played in sequence; with Loop it repeats.
	Order []string
	Loop  bool
	// Branch, when set, replaces Order after the first pattern: the next
	// pattern is drawn from the weights for the current one. This is how the
	// in-game loop keeps varying.
	Branch map[string][]Next
	// MaxPatterns ends a branching song after this many patterns (0 plays
	// forever), so it can take its turn in the playlist.
	MaxPatterns int
}

var noteBase = map[byte]int{'C': 0, 'D': 2, 'E': 4, 'F': 5, 'G': 7, 'A': 9, 'B': 11}

// parseNote reads "C4", "F#3", "Bb5" as a MIDI note number.
func parseNote(s string) (float64, error) {
	if len(s) < 2 {
		return 0, fmt.Errorf("bad note %q", s)
	}
	base, ok := noteBase[s[0]]
	if !ok {
		return 0, fmt.Errorf("bad note %q", s)
	}
	rest := s[1:]
	switch rest[0] {
	case '#':
		base++
		rest = rest[1:]
	case 'b':
		base--
		rest = rest[1:]
	}
	oct, err := strconv.Atoi(rest)
	if err != nil {
		return 0, fmt.Errorf("bad octave in %q", s)
	}
	return float64((oct+1)*12 + base), nil
}

func parseSteps(s string, drums bool) ([]step, error) {
	var out []step
	for _, tok := range strings.Fields(s) {
		vol := 0.0
		if i := strings.IndexByte(tok, ':'); i >= 0 {
			v, err := strconv.Atoi(tok[i+1:])
			if err != nil || v < 0 || v > 15 {
				return nil, fmt.Errorf("bad volume in %q", tok)
			}
			vol = float64(v)
			tok = tok[:i]
		}
		switch {
		case tok == ".":
			out = append(out, step{kind: stepHold})
		case tok == "-":
			out = append(out, step{kind: stepOff})
		case drums:
			switch tok {
			case "k", "s", "h", "o", "c":
			default:
				return nil, fmt.Errorf("bad drum %q", tok)
			}
			out = append(out, step{kind: stepDrum, drum: tok[0], vol: vol})
		default:
			n, err := parseNote(tok)
			if err != nil {
				return nil, err
			}
			out = append(out, step{kind: stepNote, note: n, vol: vol})
		}
	}
	return out, nil
}

// compile parses every pattern and checks the tracks line up.
func (sd *SongData) compile() error {
	for name, p := range sd.Patterns {
		p.len = -1
		for i, tr := range []*Track{&p.Pulse1, &p.Pulse2, &p.Tri, &p.Noise} {
			if strings.TrimSpace(tr.Steps) == "" {
				continue
			}
			st, err := parseSteps(tr.Steps, i == 3)
			if err != nil {
				return fmt.Errorf("song %s pattern %s track %d: %w", sd.Name, name, i, err)
			}
			tr.steps = st
			if p.len >= 0 && len(st) != p.len {
				return fmt.Errorf("song %s pattern %s: track %d has %d steps, want %d", sd.Name, name, i, len(st), p.len)
			}
			p.len = len(st)
		}
		if p.len <= 0 {
			return fmt.Errorf("song %s pattern %s is empty", sd.Name, name)
		}
	}
	for _, o := range sd.Order {
		if sd.Patterns[o] == nil {
			return fmt.Errorf("song %s: order names unknown pattern %s", sd.Name, o)
		}
	}
	for from, nexts := range sd.Branch {
		if sd.Patterns[from] == nil {
			return fmt.Errorf("song %s: branch from unknown pattern %s", sd.Name, from)
		}
		for _, n := range nexts {
			if sd.Patterns[n.Pattern] == nil {
				return fmt.Errorf("song %s: branch to unknown pattern %s", sd.Name, n.Pattern)
			}
		}
	}
	return nil
}

// Drum sounds on the noise channel.
var drums = map[byte]struct {
	period float64
	short  bool
	slide  float64
	inst   Instrument
}{
	'k': {10, false, 45, Instrument{Vol: 14, Decay: 0.09, Sustain: 0, Release: 0.02}},
	's': {5, false, 12, Instrument{Vol: 12, Decay: 0.16, Sustain: 0, Release: 0.04}},
	'h': {0, false, 0, Instrument{Vol: 5, Decay: 0.035, Sustain: 0, Release: 0.01}},
	'o': {1, false, 0, Instrument{Vol: 5, Decay: 0.18, Sustain: 0, Release: 0.05}},
	'c': {3, false, 1.5, Instrument{Vol: 9, Decay: 0.7, Sustain: 0, Release: 0.2}},
}

// sequencer plays a song on a chip.
type sequencer struct {
	song    *SongData
	pat     *Pattern
	patName string
	step    int
	acc     float64
	orderAt int
	entered int // patterns started, for MaxPatterns
	done    bool
	rng     *rand.Rand
	played  []string // pattern history, for tests
	// extra adds a percussion layer: soft hats in the noise channel's
	// silences (tense moments). extraHits counts them (tests).
	extra     bool
	extraHits int
}

func (s *sequencer) start(sd *SongData, c *chip, rng *rand.Rand) {
	c.silence()
	*s = sequencer{song: sd, rng: rng}
	if sd == nil || len(sd.Order) == 0 {
		s.done = true
		return
	}
	s.enter(sd.Order[0])
	s.trigger(c)
}

func (s *sequencer) enter(name string) {
	s.patName = name
	s.pat = s.song.Patterns[name]
	s.step = 0
	s.entered++
	s.played = append(s.played, name)
	if len(s.played) > 64 {
		s.played = s.played[len(s.played)-64:]
	}
}

func (s *sequencer) stepLen() float64 { return 60 / s.song.BPM / 4 }

func (s *sequencer) advance(c *chip, dt float64) {
	if s.song == nil || s.done {
		return
	}
	s.acc += dt
	for s.acc >= s.stepLen() && !s.done {
		s.acc -= s.stepLen()
		s.step++
		if s.step >= s.pat.len {
			if !s.nextPattern() {
				s.done = true
				c.silence()
				return
			}
		}
		s.trigger(c)
	}
}

// remaining is roughly how many seconds of the song are left (+Inf when it
// loops forever).
func (s *sequencer) remaining() float64 {
	if s.song == nil || s.done {
		return 0
	}
	sd := s.song
	steps := float64(s.pat.len-s.step) - s.acc/s.stepLen()
	switch {
	case len(sd.Branch) > 0:
		if sd.MaxPatterns == 0 {
			return math.Inf(1)
		}
		steps += float64(max(0, sd.MaxPatterns-s.entered) * s.pat.len)
	case sd.Loop:
		return math.Inf(1)
	default:
		for _, name := range sd.Order[min(len(sd.Order), s.orderAt+1):] {
			steps += float64(sd.Patterns[name].len)
		}
	}
	return steps * s.stepLen()
}

func (s *sequencer) nextPattern() bool {
	sd := s.song
	if sd.MaxPatterns > 0 && s.entered >= sd.MaxPatterns {
		return false
	}
	if nexts := sd.Branch[s.patName]; len(nexts) > 0 && len(s.played) > 0 {
		total := 0.0
		for _, n := range nexts {
			total += n.Weight
		}
		r := s.rng.Float64() * total
		for _, n := range nexts {
			if r -= n.Weight; r <= 0 {
				s.enter(n.Pattern)
				return true
			}
		}
		s.enter(nexts[len(nexts)-1].Pattern)
		return true
	}
	s.orderAt++
	if s.orderAt >= len(sd.Order) {
		if !sd.Loop {
			return false
		}
		s.orderAt = 0
	}
	s.enter(sd.Order[s.orderAt])
	return true
}

func (s *sequencer) trigger(c *chip) {
	p := s.pat
	melodic := func(tr *Track, v *voice) {
		if len(tr.steps) == 0 {
			return
		}
		st := tr.steps[s.step]
		switch st.kind {
		case stepNote:
			inst := tr.Inst
			if st.vol > 0 {
				inst.Vol = st.vol
			}
			v.noteOn(st.note, inst)
		case stepOff:
			v.env.off()
		}
	}
	melodic(&p.Pulse1, &c.p1.voice)
	melodic(&p.Pulse2, &c.p2.voice)
	melodic(&p.Tri, &c.tri.voice)
	drummed := false
	if len(p.Noise.steps) > 0 {
		st := p.Noise.steps[s.step]
		switch st.kind {
		case stepDrum:
			d := drums[st.drum]
			inst := d.inst
			if st.vol > 0 {
				inst.Vol = st.vol
			}
			c.nz.hit(d.period, d.short, inst, d.slide)
			drummed = true
		case stepOff:
			c.nz.env.off()
		}
	}
	if s.extra && !drummed && s.step%2 == 1 && c.nz.env.state == envOff {
		// The tension layer: offbeat 16th hats wherever the drums leave a
		// gap, accented on the "and" of each beat.
		h := drums['h']
		inst := h.inst
		inst.Vol = 3
		if s.step%4 == 3 {
			inst.Vol = 5
		}
		c.nz.hit(h.period, h.short, inst, h.slide)
		s.extraHits++
	}
}
