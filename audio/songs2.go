package audio

import (
	"strconv"
	"strings"
)

// The rest of the in-game playlist. Like every song here these are original
// compositions, written as two-bar patterns of 16th-note steps.
//
//	Rain on Chrome  90 BPM, C minor: moody arpeggios, a slow lead, sparse
//	                hats. The calm opener.
//	Overclock       150 BPM, D minor: galloping triangle bass, a punchy
//	                pulse lead, busy drums. Late-game energy.
//	Neon Protocol (Chrome Rain Remix)
//	                120 BPM, A minor: the original theme (E minor, 132 BPM)
//	                moved down a key and a gear, laced with Rain's arpeggios
//	                and Overclock's riff and gallop, with a new half-time
//	                section built on the theme's hook.

// mel expands a compact line: "G4*4 Eb5*2 -*2" is G4 held for four steps,
// Eb5 for two, then two steps of rest. Volumes ("C5:9*4") pass through. A
// wrong length shows up as a misaligned pattern when the song compiles.
func mel(s string) string {
	var out []string
	for _, tok := range strings.Fields(s) {
		n := 1
		if i := strings.LastIndexByte(tok, '*'); i >= 0 {
			n, _ = strconv.Atoi(tok[i+1:])
			tok = tok[:i]
		}
		for k := range n {
			if k == 0 {
				out = append(out, tok)
			} else {
				out = append(out, ".")
			}
		}
	}
	return strings.Join(out, " ")
}

// rarp is Rain's arpeggio: a rising figure that turns back on itself, one
// bar over four chord tones.
func rarp(c [4]string) string {
	a, b, x, d := c[0], c[1], c[2], c[3]
	return strings.Join([]string{a, x, b, d, x, b, d, x, a, x, b, d, x, d, x, b}, " ")
}

// gallop is Overclock's bass: a 16th-note gallop between two octaves.
func gallop(lo, hi string) string {
	return strings.TrimSpace(strings.Repeat(lo+" . "+lo+" "+hi+" ", 4))
}

// stab hits one note on every offbeat 8th.
func stab(n string) string {
	return strings.Join([]string{"-", "-", n, "-", "-", "-", n, "-", "-", "-", n, "-", "-", "-", n, "-"}, " ")
}

func arp4(c [4]string) string { return arp16(c[0], c[1], c[2], c[3]) }

func pat(p1, p2, tri, nz Track) *Pattern {
	return &Pattern{Pulse1: p1, Pulse2: p2, Tri: tri, Noise: nz}
}

func drumTrack(b ...string) Track { return Track{Steps: bars(b...)} }

var (
	rainLead  = Instrument{Duty: Duty50, Vol: 7, Decay: 1.2, Sustain: 0.5, Release: 0.5, Vibrato: 0.22, VibratoRate: 4.5, VibratoDelay: 0.35}
	rainArp   = Instrument{Duty: Duty25, Vol: 4, Decay: 0.35, Sustain: 0.25, Release: 0.2}
	rainThin  = Instrument{Duty: Duty12, Vol: 3, Decay: 0.4, Sustain: 0.2, Release: 0.25}
	bassSoft  = Instrument{Vol: 13, Sustain: 1, Release: 0.1}
	ocLead    = Instrument{Duty: Duty25, Vol: 11, Decay: 0.18, Sustain: 0.55, Release: 0.06, Vibrato: 0.12, VibratoRate: 6, VibratoDelay: 0.15}
	ocStab    = Instrument{Duty: Duty12, Vol: 6, Decay: 0.07, Sustain: 0.15, Release: 0.03}
	ocArp     = Instrument{Duty: Duty25, Vol: 5, Decay: 0.1, Sustain: 0.3, Release: 0.04}
	ocCounter = Instrument{Duty: Duty50, Vol: 7, Decay: 0.3, Sustain: 0.6, Release: 0.08}
)

// --- Rain on Chrome --------------------------------------------------------

var (
	rCm  = [4]string{"C4", "Eb4", "G4", "Bb4"}
	rAb  = [4]string{"Ab3", "C4", "Eb4", "G4"}
	rEb  = [4]string{"Eb4", "G4", "Bb4", "D5"}
	rBb  = [4]string{"Bb3", "D4", "F4", "Bb4"}
	rFm  = [4]string{"F4", "Ab4", "C5", "Eb5"}
	rG   = [4]string{"G3", "B3", "D4", "F4"}
	rCm2 = [4]string{"C4", "Eb4", "G4", "C5"}

	rainHats  = "- - - - h:3 - - - - - - - h:3 - - -"
	rainHatsB = "- - h:2 - h:4 - - - - - h:2 - h:4 - o:2 -"
)

func rainArps(a, b [4]string) Track { return Track{Inst: rainArp, Steps: bars(rarp(a), rarp(b))} }
func rainBass(s string) Track       { return Track{Inst: bassSoft, Steps: mel(s)} }
func rainMel(s string) Track        { return Track{Inst: rainLead, Steps: mel(s)} }

var rainSong = &SongData{
	Name:  "rain",
	Title: "RAIN ON CHROME",
	BPM:   90,
	Patterns: map[string]*Pattern{
		"I1": pat(Track{}, rainArps(rCm, rAb), rainBass("C2*12 G2*4 Ab1*12 Eb2*4"), Track{}),
		"I2": pat(Track{}, rainArps(rEb, rBb), rainBass("Eb2*12 Bb2*4 Bb1*12 F2*4"), drumTrack(rainHats, rainHats)),
		"A1": pat(rainMel("G4*4 Eb5*4 D5*2 C5*6 C5*8 Bb4*2 Ab4*6"), rainArps(rCm, rAb),
			rainBass("C2*12 G2*4 Ab1*12 Eb2*4"), drumTrack(rainHats, rainHats)),
		"A2": pat(rainMel("G4*4 Bb4*4 Eb5*4 D5*4 D5*8 F4*6 -*2"), rainArps(rEb, rBb),
			rainBass("Eb2*12 Bb2*4 Bb1*12 F2*4"), drumTrack(rainHats, rainHats)),
		"A3": pat(rainMel("C5*4 Eb5*2 G5*6 F5*2 Eb5*2 Eb5*4 D5*2 C5*4 Ab4*6"), rainArps(rCm, rAb),
			rainBass("C2*12 G2*4 Ab1*12 Eb2*4"), drumTrack(rainHats, rainHats)),
		"A4": pat(rainMel("Bb4*4 Eb5*4 G5*4 F5*2 Eb5*2 D5*12 -*4"), rainArps(rEb, rBb),
			rainBass("Eb2*12 Bb2*4 Bb1*12 F2*4"), drumTrack(rainHats, rainHatsB)),
		"B1": pat(rainMel("Ab5*6 G5*2 F5*4 Eb5*4 G5*8 Eb5*4 C5*4"), rainArps(rFm, rCm),
			rainBass("F2*12 C3*4 C2*12 G2*4"), drumTrack(rainHatsB, rainHatsB)),
		"B2": pat(rainMel("C6*6 Bb5*2 Ab5*4 G5*4 B4*8 D5*4 G5*4"), rainArps(rAb, rG),
			rainBass("Ab1*12 Eb2*4 G1*12 D2*4"), drumTrack(rainHatsB, rainHatsB)),
		// The bridge: the lead drops out and a thin echo of the arpeggio
		// floats above a pedal.
		"R1": pat(Track{Inst: rainThin, Steps: bars(rarp([4]string{"Ab5", "C6", "Eb6", "G6"}), rarp([4]string{"Bb5", "D6", "F6", "Bb6"}))},
			Track{Inst: rainArp, Steps: mel("Eb5*16 F5*16")}, rainBass("Ab1*16 Bb1*16"), Track{}),
		"R2": pat(Track{Inst: rainThin, Steps: bars(rarp([4]string{"C6", "Eb6", "G6", "Bb6"}), rarp([4]string{"B5", "D6", "F6", "G6"}))},
			Track{Inst: rainArp, Steps: mel("G5*16 F5*16")}, rainBass("C2*16 G1*16"), drumTrack("- - - - - - - - - - - - - - - -", rainHats)),
		"O1": pat(rainMel("G5*8 Eb5*8 C5*16"), rainArps(rCm, rAb), rainBass("C2*12 G2*4 Ab1*16"), drumTrack(rainHats, rainHats)),
		"O2": pat(Track{}, Track{Inst: rainThin, Steps: bars(rarp(rCm2), "C5 . . . . . . . - - - - - - - -")},
			rainBass("C2*16 C2*8 -*8"), Track{}),
	},
	Order: []string{"I1", "I2", "A1", "A2", "A3", "A4", "A1", "A2", "B1", "B2", "B1", "B2",
		"R1", "R2", "A3", "A4", "A1", "A2", "B1", "B2", "A3", "A4", "O1", "O2"},
}

// --- Overclock -------------------------------------------------------------

var (
	oDm = [4]string{"D4", "F4", "A4", "D5"}
	oBb = [4]string{"Bb3", "D4", "F4", "Bb4"}
	oC  = [4]string{"C4", "E4", "G4", "C5"}
	oA  = [4]string{"A3", "C#4", "E4", "A4"}

	ocBeat  = "k h h h s h h h k h k h s h h h"
	ocBeat2 = "k h o h s h h k k h k h s h o h"
	ocFill  = "k h h h s h h h k s k s s s s s"
	ocFour  = "k - h - k - h - k - h - k - h -"
	ocHalf  = "k - - - - - h - s - - - - - h -"
	ocCrash = "c h h h s h h h k h k h s h h h"
)

func ocMel(s string) Track        { return Track{Inst: ocLead, Steps: mel(s)} }
func ocStabs(a, b string) Track   { return Track{Inst: ocStab, Steps: bars(stab(a), stab(b))} }
func ocArps(a, b [4]string) Track { return Track{Inst: ocArp, Steps: bars(arp4(a), arp4(b))} }
func ocCounterMel(s string) Track { return Track{Inst: ocCounter, Steps: mel(s)} }

// bassBars is a bass track from two one-bar lines; bassMel from a mel line.
func bassBars(a, b string) Track { return Track{Inst: bass, Steps: bars(a, b)} }
func bassMel(s string) Track     { return Track{Inst: bass, Steps: mel(s)} }

var overclockSong = &SongData{
	Name:  "overclock",
	Title: "OVERCLOCK",
	BPM:   150,
	Patterns: map[string]*Pattern{
		"I1": pat(Track{}, Track{}, bassBars(gallop("D2", "D3"), gallop("Bb1", "Bb2")), drumTrack(ocFour, ocFour)),
		"I2": pat(Track{}, ocStabs("E4", "C#4"), bassBars(gallop("C2", "C3"), gallop("A1", "A2")), drumTrack(ocFour, ocFill)),
		"A1": pat(ocMel("D5*2 D5 F5 A5*2 D5*2 C6*2 A5*2 G5*2 F5*2 F5*2 G5*2 F5*2 D5*2 Bb4*4 D5*4"),
			ocStabs("F4", "D4"), bassBars(gallop("D2", "D3"), gallop("Bb1", "Bb2")), drumTrack(ocBeat, ocBeat)),
		"A2": pat(ocMel("E5*2 G5*2 C6*2 G5*2 E5*2 G5*2 C6*4 C#6*4 A5*2 E5*2 C#5*4 E5*4"),
			ocStabs("E4", "C#4"), bassBars(gallop("C2", "C3"), gallop("A1", "A2")), drumTrack(ocBeat, ocFill)),
		"A3": pat(ocMel("A5*2 A5 G5 F5*2 D5*2 F5*2 G5*2 A5*4 Bb5*2 A5*2 G5*2 F5*2 D5*8"),
			ocArps(oDm, oBb), bassBars(gallop("D2", "D3"), gallop("Bb1", "Bb2")), drumTrack(ocBeat2, ocBeat2)),
		"A4": pat(ocMel("G5*2 E5*2 C5*2 E5*2 G5*4 A5*4 E5*4 C#5*4 A4*8"),
			ocArps(oC, oA), bassBars(gallop("C2", "C3"), gallop("A1", "A2")), drumTrack(ocBeat2, ocFill)),
		"B1": pat(ocMel("Bb5*4 A5*2 G5*2 D5*4 G5*4 F5*4 E5*2 D5*2 A4*8"), ocCounterMel("D5*8 Bb4*8 A4*8 F4*8"),
			bassBars(oct8("G1", "G2"), oct8("D2", "D3")), drumTrack(ocCrash, ocBeat)),
		"B2": pat(ocMel("D6*4 C6*2 Bb5*2 F5*4 Bb5*4 A5*4 G5*2 E5*2 C#5*8"), ocCounterMel("F5*8 D5*8 E5*8 C#5*8"),
			bassBars(oct8("Bb1", "Bb2"), oct8("A1", "A2")), drumTrack(ocBeat, ocFill)),
		// The breakdown: half-time drums under the arpeggio, then a build.
		"K1": pat(Track{}, ocArps(oDm, oBb), bassMel("D2*16 Bb1*16"), drumTrack(ocHalf, ocHalf)),
		"K2": pat(ocCounterMel("A4*8 C5*8 C#5*8 E5*8"), ocArps(oC, oA), bassMel("C2*16 A1*16"),
			drumTrack(ocHalf, "k - h - s - h - k h k h s s s s")),
		"C1": pat(ocMel("A5*6 C6*2 F6*4 E6*4 E6*6 D6*2 C6*4 G5*4"), ocStabs("A4", "E4"),
			bassBars(gallop("F2", "F3"), gallop("C2", "C3")), drumTrack(ocCrash, ocBeat2)),
		"C2": pat(ocMel("D6*6 Bb5*2 G5*4 D6*4 C#6*8 E6*8"), ocStabs("Bb4", "C#5"),
			bassBars(gallop("G1", "G2"), gallop("A1", "A2")), drumTrack(ocBeat2, ocFill)),
		"O1": pat(ocMel("D6*8 A5*8 F5*8 D5*8"), ocArps(oDm, oBb), bassBars(gallop("D2", "D3"), gallop("Bb1", "Bb2")), drumTrack(ocBeat, ocBeat)),
		"O2": pat(ocMel("E5*8 C#5*8 D5*12 -*4"), ocStabs("C#4", "D4"), bassMel("A1*16 D2*12 -*4"),
			drumTrack(ocFill, "c - - - - - - - - - - - - - - -")),
	},
	Order: []string{"I1", "I2", "A1", "A2", "A3", "A4", "A1", "A2", "A3", "A4", "B1", "B2", "B1", "B2",
		"A1", "A2", "K1", "K2", "K1", "K2", "A3", "A4", "A1", "A2", "C1", "C2", "C1", "C2",
		"B1", "B2", "B1", "B2", "A3", "A4", "C1", "C2", "A1", "A2", "O1", "O2"},
}

// --- Neon Protocol (Chrome Rain Remix) --------------------------------------

var (
	xAm = [4]string{"A3", "C4", "E4", "A4"}
	xF  = [4]string{"F3", "A3", "C4", "F4"}
	xC  = [4]string{"C4", "E4", "G4", "C5"}
	xG  = [4]string{"G3", "B3", "D4", "G4"}
	xDm = [4]string{"D4", "F4", "A4", "D5"}
	xE  = [4]string{"E3", "G#3", "B3", "E4"}

	rmBeat  = "k - h h s - h - k h k - s - h h"
	rmBeat2 = "k - h h s - h h k h k h s - o -"
	rmFill  = "k - h h s - h - k s k s s s s s"
	rmHalf  = "k - h - s - h - k - h - s - h -"

	// The theme's hook moved from E minor to A minor.
	hookA = "E5*4 A5*4 C6*2 B5*2 A5*4 A5*2 G5*2 F5*4 E5*4 C5*4"
	hookB = "G5*4 C6*4 E6*2 D6*2 C6*4 B5*6 A5*2 G5*4 D5*4"
)

func rmLead(s string) Track       { return Track{Inst: lead, Steps: mel(s)} }
func rmArps(a, b [4]string) Track { return Track{Inst: rainArp, Steps: bars(rarp(a), rarp(b))} }
func rmCounter(s string) Track    { return Track{Inst: counter, Steps: mel(s)} }

var remixSong = &SongData{
	Name:  "remix",
	Title: "NEON PROTOCOL (CHROME RAIN REMIX)",
	BPM:   120,
	Patterns: map[string]*Pattern{
		// Intro on Rain's arpeggio.
		"X1": pat(Track{}, rmArps(xAm, xF), Track{Inst: bassSoft, Steps: mel("A1*12 E2*4 F1*12 C2*4")}, drumTrack(rainHats, rainHats)),
		"X2": pat(Track{}, rmArps(xC, xG), Track{Inst: bassSoft, Steps: mel("C2*12 G2*4 G1*12 D2*4")}, drumTrack(rainHats, rmFill)),
		// The hook over Rain's arpeggio and Overclock's gallop.
		"H1": pat(rmLead(hookA), rmArps(xAm, xF), bassBars(gallop("A1", "A2"), gallop("F1", "F2")), drumTrack(rmBeat, rmBeat)),
		"H2": pat(rmLead(hookB), rmArps(xC, xG), bassBars(gallop("C2", "C3"), gallop("G1", "G2")), drumTrack(rmBeat, rmFill)),
		// The hook again with the theme's own countermelody.
		"H3": pat(rmLead(hookA), rmCounter("C5*4 E5*4 A5*2 G5*2 E5*4 F5*2 E5*2 D5*4 C5*4 A4*4"),
			bassBars(gallop("A1", "A2"), gallop("F1", "F2")), drumTrack(rmBeat2, rmBeat2)),
		"H4": pat(rmLead(hookB), rmCounter("E5*4 G5*4 C6*2 B5*2 G5*4 G5*6 F#5*2 D5*4 B4*4"),
			bassBars(gallop("C2", "C3"), gallop("G1", "G2")), drumTrack(rmBeat2, rmFill)),
		// Motifs: Overclock's riff, then Rain's falling line.
		"M1": pat(rmLead("A5*2 A5 C6 E6*2 A5*2 G6*2 E6*2 D6*2 C6*2 C6*2 D6*2 C6*2 A5*2 F5*4 A5*4"),
			Track{Inst: ocStab, Steps: bars(stab("C5"), stab("A4"))}, bassBars(gallop("A1", "A2"), gallop("F1", "F2")), drumTrack(rmBeat2, rmBeat)),
		"M2": pat(rmLead("E5*4 C6*4 B5*2 A5*6 A5*8 G#5*2 E5*6"), rmArps(xDm, xE),
			Track{Inst: bassSoft, Steps: mel("D2*12 A2*4 E2*12 B2*4")}, drumTrack(rainHats, rmFill)),
		// New: the hook at half speed, reharmonized, over stabs.
		"N1": pat(rmLead("E5*8 A5*8 C6*4 B5*4 A5*8"), Track{Inst: ocStab, Steps: bars(stab("A4"), stab("C5"))},
			bassBars(gallop("F1", "F2"), gallop("A1", "A2")), drumTrack(rmHalf, rmHalf)),
		"N2": pat(rmLead("A5*4 G5*4 F5*8 E5*8 C5*8"), Track{Inst: ocStab, Steps: bars(stab("F4"), stab("C5"))},
			bassBars(gallop("D2", "D3"), gallop("A1", "A2")), drumTrack(rmHalf, rmHalf)),
		"N3": pat(rmLead("G5*8 C6*8 E6*4 D6*4 C6*8"), Track{Inst: ocStab, Steps: bars(stab("E4"), stab("C5"))},
			bassBars(gallop("C2", "C3"), gallop("A1", "A2")), drumTrack(rmHalf, rmBeat)),
		"N4": pat(rmLead("B5*12 A5*4 G#5*8 E5*8"), rmArps(xE, xE),
			bassBars(gallop("E1", "E2"), gallop("E1", "E2")), drumTrack(rmBeat, rmFill)),
		// Breakdown: the hook sinks into the triangle.
		"B1": pat(Track{}, rmArps(xDm, xE), bassMel("E3*4 A3*4 C4*2 B3*2 A3*4 A3*2 G#3*2 F3*4 E3*8"), drumTrack(ocHalf, ocHalf)),
		"B2": pat(rmCounter("A5*16 G#5*16"), rmArps(xF, xE), bassMel("F3*4 A3*4 C4*4 A3*4 G#3*8 B3*8"), drumTrack(ocHalf, rmFill)),
		"O1": pat(rmLead("E5*4 A5*4 C6*2 B5*2 A5*4 A5*16"), rmArps(xAm, xF),
			bassBars(gallop("A1", "A2"), mel("F1*16")), drumTrack(rmBeat, "k - h - s - h - k - - - - - - -")),
		"O2": pat(rmLead("A5*16 -*16"), rmCounter("E5*16 -*16"), bassMel("A1*16 -*16"),
			drumTrack("c - - - - - - - - - - - - - - -", "- - - - - - - - - - - - - - - -")),
	},
	Order: []string{"X1", "X2", "H1", "H2", "H1", "H2", "M1", "M2", "H3", "H4", "N1", "N2", "N3", "N4",
		"H1", "H2", "B1", "B2", "M1", "M2", "H3", "H4", "N1", "N2", "N3", "N4", "H1", "H2", "H3", "H4",
		"M1", "M2", "O1", "O2"},
}
