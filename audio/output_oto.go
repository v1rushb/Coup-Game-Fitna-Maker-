//go:build !linux || neoncoup_oto

package audio

import (
	"time"

	"github.com/ebitengine/oto/v3"
)

// otoOutput plays through ebitengine/oto: WASAPI on Windows, Core Audio on
// macOS (both without cgo). On Linux oto loads ALSA through purego, which
// makes the binary depend on libc, so Linux builds use output_linux.go unless
// built with -tags neoncoup_oto.
type otoOutput struct{ p *oto.Player }

func openOutput(s *Synth) (output, bool) {
	ctx, ready, err := oto.NewContext(&oto.NewContextOptions{
		SampleRate:      SampleRate,
		ChannelCount:    1,
		Format:          oto.FormatFloat32LE,
		BufferSize:      60 * time.Millisecond,
		ApplicationName: "NEON//COUP",
	})
	if err != nil {
		return nil, false
	}
	select {
	case <-ready:
	case <-time.After(2 * time.Second):
		return nil, false
	}
	if ctx.Err() != nil {
		return nil, false
	}
	p := ctx.NewPlayer(&pcmReader{s: s})
	p.SetBufferSize(SampleRate * 4 * 70 / 1000) // ~70 ms
	p.Play()
	return &otoOutput{p: p}, true
}

func (o *otoOutput) stop() { o.p.Pause() }
