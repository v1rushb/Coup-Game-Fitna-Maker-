package audio

import (
	"encoding/binary"
	"math"
	"sync"
	"time"
)

// output is a running audio sink pulling samples from the synth.
type output interface {
	stop()
}

// Device plays the synth through the system's audio output. The synth runs
// on the output's goroutine; every Player method is a non-blocking send.
type Device struct {
	synth *Synth
	out   output

	mu    sync.Mutex
	vol   float64
	muted bool
}

// Open starts audio output and returns a Player. When no device can be
// opened (no sound server, headless box, CI) it returns a silent Nop, so the
// game simply runs muted.
func Open(volume float64, muted bool) Player {
	s := NewSynth(SampleRate, uint64(time.Now().UnixNano()))
	s.SetMuted(muted, volume)
	out, ok := openOutput(s)
	if !ok {
		return Nop{}
	}
	return &Device{synth: s, out: out, vol: volume, muted: muted}
}

// pcmReader adapts the synth to an io.Reader of 32-bit little-endian floats.
type pcmReader struct {
	s   *Synth
	buf []float32
}

func (r *pcmReader) Read(p []byte) (int, error) {
	n := len(p) / 4
	if n == 0 {
		return 0, nil
	}
	if cap(r.buf) < n {
		r.buf = make([]float32, n)
	}
	b := r.buf[:n]
	r.s.Render(b)
	for i, v := range b {
		binary.LittleEndian.PutUint32(p[i*4:], math.Float32bits(v))
	}
	return n * 4, nil
}

// Music implements Player.
func (d *Device) Music(s Song) { d.synth.Music(s) }

// SFX implements Player.
func (d *Device) SFX(e Effect) { d.synth.SFX(e) }

// Stinger implements Player.
func (d *Device) Stinger() { d.synth.Stinger() }

// SetVolume implements Player.
func (d *Device) SetVolume(v float64) {
	d.mu.Lock()
	d.vol = math.Max(0, math.Min(1, v))
	muted := d.muted
	d.mu.Unlock()
	if !muted {
		d.synth.SetVolume(v)
	}
}

// SetMuted implements Player.
func (d *Device) SetMuted(m bool) {
	d.mu.Lock()
	d.muted = m
	v := d.vol
	d.mu.Unlock()
	d.synth.SetMuted(m, v)
}

// SetMusicVolume implements Player.
func (d *Device) SetMusicVolume(v float64) { d.synth.SetMusicVolume(v) }

// SetSFXVolume implements Player.
func (d *Device) SetSFXVolume(v float64) { d.synth.SetSFXVolume(v) }

// NextTrack implements Player.
func (d *Device) NextTrack() { d.synth.NextTrack() }

// SetIntensity implements Player.
func (d *Device) SetIntensity(tense bool) { d.synth.SetIntensity(tense) }

// Notices implements Player.
func (d *Device) Notices() <-chan string { return d.synth.Notices() }

// Muted implements Player.
func (d *Device) Muted() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.muted
}

// Available implements Player.
func (d *Device) Available() bool { return true }

// Close implements Player.
func (d *Device) Close() {
	d.synth.Music(SongNone)
	time.Sleep(40 * time.Millisecond) // let release tails fade instead of clicking
	d.out.stop()
}
