//go:build linux && !neoncoup_oto

package audio

import (
	"io"
	"os/exec"
	"time"

	pa "github.com/jfreymuth/pulse"
)

// On Linux the game talks to PulseAudio (or PipeWire's Pulse server) with a
// pure-Go client, the same one oto uses first, so the binary stays fully
// static. ALSA-only systems fall back to piping PCM into aplay.

type pulseOutput struct {
	c *pa.Client
	s *pa.PlaybackStream
}

func openOutput(s *Synth) (output, bool) {
	if o, ok := openPulse(s); ok {
		return o, true
	}
	return openAplay(s)
}

func openPulse(s *Synth) (output, bool) {
	type result struct {
		o   *pulseOutput
		err error
	}
	done := make(chan result, 1)
	go func() {
		c, err := pa.NewClient(pa.ClientApplicationName("NEON//COUP"))
		if err != nil {
			done <- result{err: err}
			return
		}
		st, err := c.NewPlayback(pa.Float32Reader(func(out []float32) (int, error) {
			s.Render(out)
			return len(out), nil
		}), pa.PlaybackSampleRate(SampleRate), pa.PlaybackMono, pa.PlaybackLatency(0.07), pa.PlaybackMediaName("NEON//COUP"))
		if err != nil {
			c.Close()
			done <- result{err: err}
			return
		}
		st.Start()
		done <- result{o: &pulseOutput{c: c, s: st}}
	}()
	select {
	case r := <-done:
		if r.err != nil {
			return nil, false
		}
		return r.o, true
	case <-time.After(2 * time.Second):
		return nil, false
	}
}

func (p *pulseOutput) stop() {
	p.s.Stop()
	p.s.Close()
	p.c.Close()
}

// aplayOutput feeds raw float PCM to aplay. Blocking writes pace the synth.
type aplayOutput struct {
	cmd  *exec.Cmd
	in   io.WriteCloser
	done chan struct{}
}

func openAplay(s *Synth) (output, bool) {
	path, err := exec.LookPath("aplay")
	if err != nil {
		return nil, false
	}
	cmd := exec.Command(path, "-q", "-t", "raw", "-f", "FLOAT_LE", "-r", "44100", "-c", "1", "--buffer-time=80000", "-")
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, false
	}
	if err := cmd.Start(); err != nil {
		return nil, false
	}
	o := &aplayOutput{cmd: cmd, in: in, done: make(chan struct{})}
	go func() {
		r := &pcmReader{s: s}
		buf := make([]byte, 1024*4)
		for {
			select {
			case <-o.done:
				return
			default:
			}
			n, _ := r.Read(buf)
			if _, err := in.Write(buf[:n]); err != nil {
				return
			}
		}
	}()
	// aplay exits at once when there is no usable device.
	time.Sleep(150 * time.Millisecond)
	if cmd.ProcessState != nil {
		return nil, false
	}
	return o, true
}

func (o *aplayOutput) stop() {
	close(o.done)
	_ = o.in.Close()
	_ = o.cmd.Process.Kill()
	_ = o.cmd.Wait()
}
