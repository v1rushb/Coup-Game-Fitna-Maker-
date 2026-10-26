package main

import (
	"github.com/v1rushb/neoncoup/audio"
	"github.com/v1rushb/neoncoup/config"
)

// openAudio starts the chiptune engine, or returns a silent player when no
// audio device is available.
func openAudio(cfg config.Config) audio.Player {
	return audio.Open(cfg.Volume, cfg.Muted)
}
