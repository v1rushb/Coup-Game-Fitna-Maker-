// Package audio is NEON//COUP's chiptune engine: an NES-style synth (two
// pulse channels, a triangle and a noise channel), a pattern sequencer, the
// songs and the sound effects. Nothing is loaded from files; every sound is
// synthesized in code.
package audio

// Song identifies a piece of music.
type Song uint8

// Songs. SongGame is the in-game playlist, which rotates the four tracks
// after it.
const (
	SongNone Song = iota
	SongMenu
	SongGame
	SongVictory
	SongDefeat
	SongTheme     // "Neon Protocol", the original in-game theme
	SongRain      // "Rain on Chrome"
	SongOverclock // "Overclock"
	SongRemix     // "Neon Protocol (Chrome Rain Remix)"
)

// GameTracks are the tracks the in-game playlist rotates.
var GameTracks = []Song{SongTheme, SongRain, SongOverclock, SongRemix}

// Effect identifies a sound effect.
type Effect uint8

// Sound effects.
const (
	SFXCoin Effect = iota
	SFXFlip
	SFXChallenge
	SFXBlock
	SFXGlitch
	SFXSelect
	SFXTick
	SFXError
	SFXYourTurn
)

// Player is what the UI talks to. Implementations never block the caller.
type Player interface {
	// Music switches to a song (SongNone stops the music).
	Music(Song)
	// SFX plays an effect over the music.
	SFX(Effect)
	// Stinger plays the tension sting for an opening challenge window,
	// ducking the music underneath it.
	Stinger()
	// SetVolume sets the master volume, 0..1.
	SetVolume(float64)
	// SetMusicVolume and SetSFXVolume set the music and effects levels
	// under the master volume, 0..1.
	SetMusicVolume(float64)
	SetSFXVolume(float64)
	// NextTrack skips to the next in-game track.
	NextTrack()
	// SetIntensity tells the playlist whether the game is tense: tense
	// games favor Overclock and add a percussion layer.
	SetIntensity(bool)
	// Notices delivers each in-game track's title as it starts; nil when
	// there is no audio.
	Notices() <-chan string
	// SetMuted mutes or unmutes everything.
	SetMuted(bool)
	// Muted reports whether output is muted (or unavailable).
	Muted() bool
	// Available reports whether an audio device is in use.
	Available() bool
	// Close releases the device.
	Close()
}

// Nop is a silent Player, used with --no-audio, over SSH, and when no
// audio device can be opened.
type Nop struct{}

func (Nop) Music(Song)             {}
func (Nop) SFX(Effect)             {}
func (Nop) Stinger()               {}
func (Nop) SetVolume(float64)      {}
func (Nop) SetMusicVolume(float64) {}
func (Nop) SetSFXVolume(float64)   {}
func (Nop) NextTrack()             {}
func (Nop) SetIntensity(bool)      {}
func (Nop) Notices() <-chan string { return nil }
func (Nop) SetMuted(bool)          {}
func (Nop) Muted() bool            { return true }
func (Nop) Available() bool        { return false }
func (Nop) Close()                 {}
