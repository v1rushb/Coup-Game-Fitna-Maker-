package audio

// fxStep is one event in a sound effect, at a time offset from its start.
type fxStep struct {
	at    float64 // seconds
	ch    byte    // '1', '2' pulse; 't' triangle; 'n' noise
	note  float64 // MIDI note, or noise period index
	inst  Instrument
	short bool    // noise: short (metallic) LFSR mode
	slide float64 // noise: period index change per second
	off   bool
}

type effect []fxStep

var (
	blip  = Instrument{Duty: Duty50, Vol: 11, Decay: 0.06, Sustain: 0.5, Release: 0.05}
	blip2 = Instrument{Duty: Duty50, Vol: 11, Decay: 0.12, Sustain: 0, Release: 0.05}
	thud  = Instrument{Duty: Duty50, Vol: 12, Decay: 0.08, Sustain: 0, Release: 0.03}
)

var effects = map[Effect]effect{
	// Two rising blips, the universal sound of money.
	SFXCoin: {
		{at: 0, ch: '1', note: 83, inst: blip},
		{at: 0.06, ch: '1', note: 88, inst: blip2},
	},
	// A short papery swish with a falling click.
	SFXFlip: {
		{at: 0, ch: 'n', note: 2, inst: Instrument{Vol: 8, Decay: 0.06, Sustain: 0}, slide: 25},
		{at: 0, ch: '2', note: 76, inst: Instrument{Duty: Duty12, Vol: 5, Decay: 0.05, Sustain: 0, Slide: -60}},
	},
	// A falling alarm: someone just called a bluff.
	SFXChallenge: {
		{at: 0, ch: '1', note: 84, inst: Instrument{Duty: Duty12, Vol: 12, Decay: 0.25, Sustain: 0.2, Slide: -40, Release: 0.05}},
		{at: 0, ch: '2', note: 79, inst: Instrument{Duty: Duty25, Vol: 9, Decay: 0.25, Sustain: 0.1, Slide: -40, Release: 0.05}},
		{at: 0, ch: 'n', note: 4, inst: Instrument{Vol: 10, Decay: 0.2, Sustain: 0}},
		{at: 0.14, ch: '1', note: 72, inst: Instrument{Duty: Duty12, Vol: 12, Decay: 0.3, Sustain: 0, Slide: -24, Release: 0.05}},
	},
	// Two low thuds and a shield hit.
	SFXBlock: {
		{at: 0, ch: '1', note: 50, inst: thud},
		{at: 0, ch: 'n', note: 9, inst: Instrument{Vol: 9, Decay: 0.1, Sustain: 0}},
		{at: 0.09, ch: '1', note: 55, inst: thud},
	},
	// Bitcrushed garbage for a flatline.
	SFXGlitch: glitchFX(),
	// UI.
	SFXSelect: {{at: 0, ch: '2', note: 88, inst: Instrument{Duty: Duty25, Vol: 6, Decay: 0.03, Sustain: 0}}},
	SFXTick:   {{at: 0, ch: 'n', note: 0, inst: Instrument{Vol: 4, Decay: 0.015, Sustain: 0}}},
	SFXError: {
		{at: 0, ch: '1', note: 45, inst: Instrument{Duty: Duty12, Vol: 10, Decay: 0.15, Sustain: 0.6, Release: 0.05}},
		{at: 0.15, ch: '1', off: true},
	},
}

func glitchFX() effect {
	// A fixed pseudo-random walk so the effect is the same every time.
	notes := []float64{91, 62, 97, 58, 86, 70, 101, 55, 79, 66, 94, 50}
	periods := []float64{1, 7, 3, 12, 0, 9, 5, 14, 2, 11, 4, 13}
	var e effect
	for i := range notes {
		at := float64(i) * 0.045
		e = append(e,
			fxStep{at: at, ch: '1', note: notes[i], inst: Instrument{Duty: Duty12, Vol: 10, Decay: 0.04, Sustain: 0.3}},
			fxStep{at: at, ch: 'n', note: periods[i], short: i%3 == 0, inst: Instrument{Vol: 9, Decay: 0.04, Sustain: 0.2}},
		)
	}
	e = append(e, fxStep{at: float64(len(notes)) * 0.045, ch: '1', off: true}, fxStep{at: float64(len(notes)) * 0.045, ch: 'n', off: true})
	return e
}

// stingerLen is how long the tension stinger plays (and the music ducks).
const stingerLen = 1.15

// stinger is the tension sting for an opening challenge window: a trembling
// tritone over a rising pulse and a swelling noise bed.
func stinger() effect {
	var e effect
	trem := Instrument{Duty: Duty12, Vol: 9, Decay: 0.06, Sustain: 0.8}
	for i := 0; float64(i)*0.06 < 1.0; i++ {
		note := 76.0
		if i%2 == 1 {
			note = 70
		}
		e = append(e, fxStep{at: float64(i) * 0.06, ch: '1', note: note, inst: trem})
	}
	e = append(e,
		fxStep{at: 0, ch: '2', note: 64, inst: Instrument{Duty: Duty25, Vol: 6, Decay: 1.2, Sustain: 0.9, Slide: 10, Release: 0.2}},
		fxStep{at: 0, ch: 't', note: 40, inst: bass},
	)
	for i, v := range []float64{3, 5, 7, 9} {
		e = append(e, fxStep{at: float64(i) * 0.25, ch: 'n', note: 1, inst: Instrument{Vol: v, Decay: 0.25, Sustain: 0.9}})
	}
	return append(e,
		fxStep{at: 1.0, ch: '1', off: true},
		fxStep{at: 1.1, ch: '2', off: true},
		fxStep{at: 1.1, ch: 't', off: true},
		fxStep{at: 1.1, ch: 'n', off: true},
	)
}

// apply fires one step on a chip.
func (s fxStep) apply(c *chip) {
	switch s.ch {
	case '1':
		if s.off {
			c.p1.env.off()
		} else {
			c.p1.noteOn(s.note, s.inst)
		}
	case '2':
		if s.off {
			c.p2.env.off()
		} else {
			c.p2.noteOn(s.note, s.inst)
		}
	case 't':
		if s.off {
			c.tri.env.off()
		} else {
			c.tri.noteOn(s.note, s.inst)
		}
	case 'n':
		if s.off {
			c.nz.env.off()
		} else {
			c.nz.hit(s.note, s.short, s.inst, s.slide)
		}
	}
}

// fxPlayer schedules effect steps on a chip.
type fxPlayer struct {
	pending []timedStep
}

type timedStep struct {
	at   float64 // absolute synth time
	step fxStep
}

func (p *fxPlayer) play(e effect, now float64) {
	for _, s := range e {
		p.pending = append(p.pending, timedStep{at: now + s.at, step: s})
	}
	// Bound the queue in case effects are spammed.
	if len(p.pending) > 256 {
		p.pending = p.pending[len(p.pending)-256:]
	}
}

func (p *fxPlayer) run(c *chip, now float64) {
	if len(p.pending) == 0 {
		return
	}
	keep := p.pending[:0]
	for _, ts := range p.pending {
		if ts.at <= now {
			ts.step.apply(c)
		} else {
			keep = append(keep, ts)
		}
	}
	p.pending = keep
}
