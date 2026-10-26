package audio

import "strings"

// Original compositions for NEON//COUP. Every pattern is two bars of 4/4 in
// 16th-note steps (32 steps).

// Instruments.
var (
	leadMenu  = Instrument{Duty: Duty50, Vol: 9, Decay: 0.8, Sustain: 0.55, Release: 0.25, Vibrato: 0.18, VibratoRate: 5, VibratoDelay: 0.25}
	arpMenu   = Instrument{Duty: Duty25, Vol: 4, Decay: 0.2, Sustain: 0.25, Release: 0.08}
	lead      = Instrument{Duty: Duty25, Vol: 10, Decay: 0.4, Sustain: 0.6, Release: 0.12, Vibrato: 0.15, VibratoRate: 5.5, VibratoDelay: 0.18}
	arp       = Instrument{Duty: Duty12, Vol: 5, Decay: 0.12, Sustain: 0.3, Release: 0.05}
	arpSoft   = Instrument{Duty: Duty25, Vol: 3, Decay: 0.25, Sustain: 0.3, Release: 0.1}
	counter   = Instrument{Duty: Duty50, Vol: 6, Decay: 0.3, Sustain: 0.5, Release: 0.1}
	bass      = Instrument{Vol: 15, Sustain: 1, Release: 0.02}
	fanfare   = Instrument{Duty: Duty50, Vol: 11, Decay: 0.5, Sustain: 0.7, Release: 0.3, Vibrato: 0.12, VibratoRate: 6, VibratoDelay: 0.2}
	harmony   = Instrument{Duty: Duty25, Vol: 7, Decay: 0.5, Sustain: 0.6, Release: 0.3}
	lament    = Instrument{Duty: Duty50, Vol: 9, Decay: 1.2, Sustain: 0.5, Release: 0.4, Vibrato: 0.35, VibratoRate: 4, VibratoDelay: 0.1}
	lamentLow = Instrument{Duty: Duty12, Vol: 6, Decay: 1.2, Sustain: 0.5, Release: 0.4}
)

// arp16 spells a 16-step up/down arpeggio over four chord tones.
func arp16(a, b, c, d string) string {
	return strings.Join([]string{a, b, c, d, c, b, a, b, c, d, c, b, a, b, c, b}, " ")
}

// oct8 is a driving octave bass in eighth notes.
func oct8(lo, hi string) string {
	return strings.TrimSpace(strings.Repeat(lo+" . "+hi+" . ", 4))
}

func bars(b ...string) string { return strings.Join(b, " ") }

const (
	hatsMenu  = "- - h - - - h - - - h - - - h -"
	drumBeat  = "k - h - s - h - k - k h s - h h"
	drumOpen  = "k - h - s - o - k - k h s - o -"
	drumBusy  = "k - h h s - h h k - k h s - h h"
	drumFill  = "k - h - s - h - k - k - s s s s"
	drumCrash = "c - h - s - h - k - k h s - h h"
	drumHalf  = "k - - - - - - - s - - - - - - -"
)

var menuSong = &SongData{
	Name: "menu",
	BPM:  96,
	Loop: true,
	Patterns: map[string]*Pattern{
		"A": {
			Pulse1: Track{Inst: leadMenu, Steps: bars("E5 . . . . . . . D5 . C5 . . . . .", "C5 . . . . . A4 . . . . . . . - -")},
			Pulse2: Track{Inst: arpMenu, Steps: bars(arp16("A4", "C5", "E5", "A5"), arp16("F4", "A4", "C5", "F5"))},
			Tri:    Track{Inst: bass, Steps: bars("A2 . A2 . A3 . A2 . A2 . A2 . A3 . G2 .", "F2 . F2 . F3 . F2 . F2 . F2 . F3 . E2 .")},
			Noise:  Track{Steps: bars(hatsMenu, hatsMenu)},
		},
		"B": {
			Pulse1: Track{Inst: leadMenu, Steps: bars("E5 . . . G5 . . . E5 . D5 . C5 . . .", "D5 . . . . . . . B4 . . . . . . .")},
			Pulse2: Track{Inst: arpMenu, Steps: bars(arp16("C5", "E5", "G5", "C6"), arp16("G4", "B4", "D5", "G5"))},
			Tri:    Track{Inst: bass, Steps: bars("C3 . C3 . C4 . C3 . C3 . C3 . C4 . B2 .", "G2 . G2 . G3 . G2 . G2 . G2 . G3 . A2 .")},
			Noise:  Track{Steps: bars(hatsMenu, hatsMenu)},
		},
		"C": {
			Pulse1: Track{Inst: leadMenu, Steps: bars("A5 . . . G5 . E5 . . . D5 . E5 . . .", "F5 . . . E5 . C5 . . . A4 . . . . .")},
			Pulse2: Track{Inst: arpMenu, Steps: bars(arp16("A4", "C5", "E5", "A5"), arp16("F4", "A4", "C5", "F5"))},
			Tri:    Track{Inst: bass, Steps: bars("A2 . A2 . A3 . A2 . A2 . A2 . A3 . G2 .", "F2 . F2 . F3 . F2 . F2 . F2 . F3 . E2 .")},
			Noise:  Track{Steps: bars("- - h - - - h - s:6 - h - - - h -", "- - h - - - h - s:6 - h - - - h -")},
		},
		"D": {
			Pulse1: Track{Inst: leadMenu, Steps: bars("F5 . . . E5 . D5 . . . C5 . D5 . . .", "E5 . . . . . . . G#4 . . . B4 . . .")},
			Pulse2: Track{Inst: arpMenu, Steps: bars(arp16("D5", "F5", "A5", "D6"), arp16("E5", "G#5", "B5", "E6"))},
			Tri:    Track{Inst: bass, Steps: bars("D3 . D3 . D4 . D3 . D3 . D3 . D4 . C3 .", "E2 . E2 . E3 . E2 . E2 . G#2 . B2 . E3 .")},
			Noise:  Track{Steps: bars(hatsMenu, "- - h - - - h - s:7 - s:8 - s:9 - s:10 -")},
		},
	},
	Order: []string{"A", "B", "C", "D"},
}

var gameSong = &SongData{
	Name: "game",
	BPM:  132,
	Loop: true,
	Patterns: map[string]*Pattern{
		// A: the main hook.
		"A1": {
			Pulse1: Track{Inst: lead, Steps: bars("B4 . . . E5 . . . G5 . F#5 . E5 . . .", "E5 . D5 . C5 . . . B4 . . . G4 . . .")},
			Pulse2: Track{Inst: arp, Steps: bars(arp16("E4", "G4", "B4", "E5"), arp16("C4", "E4", "G4", "C5"))},
			Tri:    Track{Inst: bass, Steps: bars(oct8("E2", "E3"), oct8("C2", "C3"))},
			Noise:  Track{Steps: bars(drumBeat, drumBeat)},
		},
		"A2": {
			Pulse1: Track{Inst: lead, Steps: bars("D5 . . . G5 . . . B5 . A5 . G5 . . .", "F#5 . . . . . E5 . D5 . . . A4 . . .")},
			Pulse2: Track{Inst: arp, Steps: bars(arp16("G4", "B4", "D5", "G5"), arp16("D4", "F#4", "A4", "D5"))},
			Tri:    Track{Inst: bass, Steps: bars(oct8("G2", "G3"), "D2 . D3 . D2 . D3 . D2 . D3 . F#2 . A2 .")},
			Noise:  Track{Steps: bars(drumBeat, drumFill)},
		},
		// B: the climb.
		"B1": {
			Pulse1: Track{Inst: lead, Steps: bars("A5 . . . G5 . E5 . C5 . . . E5 . . .", "G5 . . . F#5 . E5 . B4 . . . . . . .")},
			Pulse2: Track{Inst: arp, Steps: bars(arp16("A4", "C5", "E5", "A5"), arp16("E4", "G4", "B4", "E5"))},
			Tri:    Track{Inst: bass, Steps: bars(oct8("A2", "A3"), oct8("E2", "E3"))},
			Noise:  Track{Steps: bars(drumOpen, drumOpen)},
		},
		"B2": {
			Pulse1: Track{Inst: lead, Steps: bars("C6 . . . B5 . G5 . E5 . . . G5 . . .", "F#5 . . . D#5 . . . B4 . . . - - - -")},
			Pulse2: Track{Inst: arp, Steps: bars(arp16("C5", "E5", "G5", "C6"), arp16("B4", "D#5", "F#5", "B5"))},
			Tri:    Track{Inst: bass, Steps: bars(oct8("C3", "C4"), "B2 . B3 . B2 . B3 . B2 . B3 . B2 . D#3 .")},
			Noise:  Track{Steps: bars(drumCrash, drumFill)},
		},
		// C: the breakdown, everyone holding their breath.
		"C1": {
			Pulse2: Track{Inst: arpSoft, Steps: bars(arp16("E4", "G4", "B4", "E5"), arp16("C4", "E4", "G4", "C5"))},
			Tri:    Track{Inst: bass, Steps: bars("E2 . . . . . . . . . . . . . . .", "C2 . . . . . . . . . . . . . . .")},
			Noise:  Track{Steps: bars(drumHalf, drumHalf)},
		},
		"C2": {
			Pulse2: Track{Inst: arpSoft, Steps: bars(arp16("G4", "B4", "D5", "G5"), arp16("D4", "F#4", "A4", "D5"))},
			Tri:    Track{Inst: bass, Steps: bars("G2 . . . . . . . . . . . . . . .", "D2 . . . . . . . D2 . D3 . D2 . D3 .")},
			Noise:  Track{Steps: bars(drumHalf, "k - h - k - h - s s s s s:12 s:13 s:14 s:15")},
		},
		// D: the hook again with a countermelody instead of the arpeggio.
		"D1": {
			Pulse1: Track{Inst: lead, Steps: bars("B4 . . . E5 . . . G5 . F#5 . E5 . . .", "E5 . D5 . C5 . . . B4 . . . G4 . . .")},
			Pulse2: Track{Inst: counter, Steps: bars("G4 . . . B4 . . . E5 . D5 . B4 . . .", "C5 . B4 . A4 . . . G4 . . . E4 . . .")},
			Tri:    Track{Inst: bass, Steps: bars(oct8("E2", "E3"), oct8("C2", "C3"))},
			Noise:  Track{Steps: bars(drumBusy, drumBusy)},
		},
		"D2": {
			Pulse1: Track{Inst: lead, Steps: bars("D5 . . . G5 . . . B5 . A5 . G5 . . .", "F#5 . . . . . E5 . D5 . . . A4 . . .")},
			Pulse2: Track{Inst: counter, Steps: bars("B4 . . . D5 . . . G5 . F#5 . D5 . . .", "D5 . . . . . C#5 . A4 . . . F#4 . . .")},
			Tri:    Track{Inst: bass, Steps: bars(oct8("G2", "G3"), "D2 . D3 . D2 . D3 . D2 . D3 . F#2 . A2 .")},
			Noise:  Track{Steps: bars(drumBusy, drumFill)},
		},
	},
	Order: []string{"A1"},
	Branch: map[string][]Next{
		"A1": {{"A2", 1}},
		"A2": {{"A1", 0.25}, {"B1", 0.45}, {"D1", 0.2}, {"C1", 0.1}},
		"B1": {{"B2", 1}},
		"B2": {{"A1", 0.35}, {"D1", 0.35}, {"C1", 0.3}},
		"C1": {{"C2", 1}},
		"C2": {{"D1", 0.6}, {"A1", 0.4}},
		"D1": {{"D2", 1}},
		"D2": {{"B1", 0.6}, {"A1", 0.4}},
	},
}

var victorySong = &SongData{
	Name: "victory",
	BPM:  150,
	Patterns: map[string]*Pattern{
		"V": {
			Pulse1: Track{Inst: fanfare, Steps: bars("C5 E5 G5 C6 . . G5 . C6 . E6 . . . . .", "D6 . . . C6 . . . . . . . . . - -")},
			Pulse2: Track{Inst: harmony, Steps: bars("E4 G4 C5 E5 . . C5 . E5 . G5 . . . . .", "B5 . . . G5 . . . . . . . . . - -")},
			Tri:    Track{Inst: bass, Steps: bars("C3 . . . . . . . G2 . . . C3 . . .", "G2 . . . C3 . . . . . . . . . - -")},
			Noise:  Track{Steps: bars("k - h - s - h - k - k - s - c -", "k - - - c - - - - - - - - - - -")},
		},
	},
	Order: []string{"V"},
}

var defeatSong = &SongData{
	Name: "defeat",
	BPM:  84,
	Patterns: map[string]*Pattern{
		"D": {
			Pulse1: Track{Inst: lament, Steps: bars("B4 . . . A#4 . . . A4 . . . G#4 . . .", "G4 . . . . . . . . . . . . . - -")},
			Pulse2: Track{Inst: lamentLow, Steps: bars("E4 . . . D#4 . . . D4 . . . C#4 . . .", "C4 . . . . . . . . . . . . . - -")},
			Tri:    Track{Inst: bass, Steps: bars("E2 . . . . . . . D#2 . . . . . . .", "D2 . . . . . . . . . . . . . - -")},
			Noise:  Track{Steps: bars("- - - - - - - - - - - - - - - -", "o:4 - - - - - - - - - - - - - - -")},
		},
	},
	Order: []string{"D"},
}

var songs = map[Song]*SongData{
	SongMenu:    menuSong,
	SongGame:    gameSong,
	SongVictory: victorySong,
	SongDefeat:  defeatSong,
}

func init() {
	for _, s := range songs {
		if err := s.compile(); err != nil {
			panic(err)
		}
	}
}
