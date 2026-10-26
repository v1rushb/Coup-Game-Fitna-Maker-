package audio

import "math"

// The chip is modeled on the NES 2A03: two pulse channels with selectable
// duty, a 32-step triangle and a 15-bit LFSR noise channel, combined with
// the console's nonlinear mixer. Levels inside the chip use the hardware's
// 0..15 volume scale.

// Duty cycles of the pulse channels.
const (
	Duty12 = 0.125
	Duty25 = 0.25
	Duty50 = 0.5
)

// Instrument shapes a note on a melodic channel.
type Instrument struct {
	Duty    float64 // pulse only
	Vol     float64 // peak level, 0..15
	Decay   float64 // seconds from peak to sustain
	Sustain float64 // fraction of Vol held after the decay
	Release float64 // seconds to silence after note off
	// Vibrato depth in semitones and rate in Hz, starting after Delay seconds.
	Vibrato, VibratoRate, VibratoDelay float64
	// Arp cycles semitone offsets at 60 Hz, the classic chip chord trick.
	Arp []int
	// Slide bends the pitch by this many semitones per second (SFX).
	Slide float64
}

type envState uint8

const (
	envOff envState = iota
	envDecay
	envRelease
)

// envelope is an exponential decay/sustain/release envelope.
type envelope struct {
	level float64
	state envState
}

func (e *envelope) on(inst *Instrument) {
	e.level = inst.Vol
	e.state = envDecay
}

func (e *envelope) off() {
	if e.state != envOff {
		e.state = envRelease
	}
}

func (e *envelope) step(inst *Instrument, dt float64) float64 {
	switch e.state {
	case envDecay:
		target := inst.Vol * inst.Sustain
		if inst.Decay > 0 {
			e.level = target + (e.level-target)*math.Exp(-dt*4/inst.Decay)
		} else {
			e.level = target
		}
		if e.level <= 0.01 && target == 0 {
			e.level, e.state = 0, envOff
		}
	case envRelease:
		if inst.Release > 0 {
			e.level *= math.Exp(-dt * 5 / inst.Release)
		} else {
			e.level = 0
		}
		if e.level < 0.01 {
			e.level, e.state = 0, envOff
		}
	}
	return e.level
}

// voice holds the pitch state shared by melodic channels.
type voice struct {
	inst  Instrument
	note  float64 // MIDI note number
	t     float64 // seconds since note on
	env   envelope
	phase float64
}

func (v *voice) noteOn(note float64, inst Instrument) {
	v.inst = inst
	v.note = note
	v.t = 0
	v.env.on(&v.inst)
}

func (v *voice) freq() float64 {
	n := v.note + v.inst.Slide*v.t
	if len(v.inst.Arp) > 0 {
		n += float64(v.inst.Arp[int(v.t*60)%len(v.inst.Arp)])
	}
	if v.inst.Vibrato > 0 && v.t > v.inst.VibratoDelay {
		n += v.inst.Vibrato * math.Sin(2*math.Pi*v.inst.VibratoRate*(v.t-v.inst.VibratoDelay))
	}
	return midiFreq(n)
}

func midiFreq(n float64) float64 { return 440 * math.Pow(2, (n-69)/12) }

// polyBLEP smooths a discontinuity at phase 0 (t in [0,1), dt per sample).
func polyBLEP(t, dt float64) float64 {
	switch {
	case t < dt:
		t /= dt
		return t + t - t*t - 1
	case t > 1-dt:
		t = (t - 1) / dt
		return t*t + t + t + 1
	}
	return 0
}

// pulse is a band-limited square with variable duty.
type pulse struct{ voice }

func (p *pulse) sample(dt float64) float64 {
	vol := p.env.step(&p.inst, dt)
	if p.env.state == envOff {
		return 0
	}
	f := p.freq()
	inc := f * dt
	if inc >= 0.5 {
		return 0
	}
	duty := p.inst.Duty
	if duty <= 0 {
		duty = Duty50
	}
	// Naive square in {0,1}, then BLEP-corrected edges.
	s := 0.0
	if p.phase < duty {
		s = 1
	}
	s += polyBLEP(p.phase, inc) * 0.5
	t2 := p.phase - duty
	if t2 < 0 {
		t2++
	}
	s -= polyBLEP(t2, inc) * 0.5
	p.phase += inc
	if p.phase >= 1 {
		p.phase--
	}
	p.t += dt
	return math.Max(0, math.Min(1, s)) * vol
}

// triangle is the NES's quantized 32-step triangle. Like the hardware it has
// no volume control, only on and off (with a tiny fade to avoid clicks).
type triangle struct {
	voice
	gate float64
}

var triSteps = func() [32]float64 {
	var s [32]float64
	for i := range 16 {
		s[i] = float64(15 - i)
		s[16+i] = float64(i)
	}
	return s
}()

func (tr *triangle) sample(dt float64) float64 {
	tr.env.step(&tr.inst, dt)
	target := 0.0
	if tr.env.state != envOff {
		target = 1
	}
	tr.gate += (target - tr.gate) * math.Min(1, dt*800)
	if tr.gate < 0.001 {
		return 0
	}
	inc := tr.freq() * dt
	tr.phase += inc
	if tr.phase >= 1 {
		tr.phase -= math.Floor(tr.phase)
	}
	tr.t += dt
	return triSteps[int(tr.phase*32)%32] * tr.gate
}

// noise is the 15-bit LFSR noise channel.
type noise struct {
	lfsr    uint16
	acc     float64
	short   bool
	inst    Instrument
	env     envelope
	slide   float64 // period index change per second
	periodI float64
	t       float64
}

// noisePeriods are the NTSC noise timer periods in CPU cycles.
var noisePeriods = [16]float64{4, 8, 16, 32, 64, 96, 128, 160, 202, 254, 380, 508, 762, 1016, 2034, 4068}

const cpuHz = 1789773.0

func (n *noise) hit(periodIndex float64, short bool, inst Instrument, slide float64) {
	if n.lfsr == 0 {
		n.lfsr = 1
	}
	n.periodI = periodIndex
	n.short = short
	n.inst = inst
	n.slide = slide
	n.t = 0
	n.env.on(&n.inst)
}

func (n *noise) sample(dt float64) float64 {
	vol := n.env.step(&n.inst, dt)
	if n.env.state == envOff {
		return 0
	}
	idx := math.Max(0, math.Min(15, n.periodI+n.slide*n.t))
	lo := int(idx)
	hi := min(15, lo+1)
	frac := idx - float64(lo)
	per := (noisePeriods[lo]*(1-frac) + noisePeriods[hi]*frac) / cpuHz
	n.acc += dt
	for n.acc >= per {
		n.acc -= per
		tap := uint16(1)
		if n.short {
			tap = 6
		}
		fb := (n.lfsr ^ (n.lfsr >> tap)) & 1
		n.lfsr = (n.lfsr >> 1) | (fb << 14)
	}
	n.t += dt
	if n.lfsr&1 == 0 {
		return vol
	}
	return 0
}

// mix is the NES nonlinear mixer; inputs are 0..15 levels, output ~0..1.
func mix(p1, p2, tri, nz float64) float64 {
	out := 0.0
	if p := p1 + p2; p > 0 {
		out += 95.88 / (8128/p + 100)
	}
	if tri > 0 || nz > 0 {
		out += 159.79 / (1/(tri/8227+nz/12241) + 100)
	}
	return out
}

// chip is one 2A03: music and effects each get their own.
type chip struct {
	p1, p2 pulse
	tri    triangle
	nz     noise
}

func (c *chip) sample(dt float64) float64 {
	return mix(c.p1.sample(dt), c.p2.sample(dt), c.tri.sample(dt), c.nz.sample(dt))
}

func (c *chip) silence() {
	c.p1.env.off()
	c.p2.env.off()
	c.tri.env.off()
	c.nz.env.off()
}

// filter removes DC (high-pass ~90 Hz) and softens the top (low-pass ~12 kHz),
// roughly like the NES output stage.
type filter struct {
	hpPrevIn, hpPrevOut float64
	lp                  float64
}

func (f *filter) apply(x, dt float64) float64 {
	rc := 1 / (2 * math.Pi * 90)
	a := rc / (rc + dt)
	hp := a * (f.hpPrevOut + x - f.hpPrevIn)
	f.hpPrevIn, f.hpPrevOut = x, hp
	rc2 := 1 / (2 * math.Pi * 12000)
	b := dt / (rc2 + dt)
	f.lp += b * (hp - f.lp)
	return f.lp
}
