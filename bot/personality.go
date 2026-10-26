package bot

import (
	"math/rand/v2"

	"github.com/v1rushb/neoncoup/engine"
)

// Personality flavors a bot: names, trait multipliers and taunts.
type Personality struct {
	Handle string
	// Bluff scales how willing the bot is to claim cards it lacks.
	Bluff float64
	// Suspicion scales how readily it challenges.
	Suspicion float64
	// Aggression scales how much it favors attacking actions.
	Aggression float64
	// Lines are signature taunts mixed in with the generic ones.
	Lines []string
}

// Roster is the pool of bot identities.
var Roster = []Personality{
	{Handle: "VEX", Bluff: 1.3, Suspicion: 1.0, Aggression: 1.2, Lines: []string{"your firewall is made of paper", "vex was here"}},
	{Handle: "NULLBYTE", Bluff: 0.7, Suspicion: 1.3, Aggression: 0.9, Lines: []string{"\\0 terminated", "you read like an open port"}},
	{Handle: "KESTREL-9", Bluff: 1.0, Suspicion: 1.0, Aggression: 1.4, Lines: []string{"target painted", "talons out"}},
	{Handle: "GHOSTWIRE", Bluff: 1.5, Suspicion: 0.8, Aggression: 0.9, Lines: []string{"you never saw me", "static on the line"}},
	{Handle: "SYNTHIA", Bluff: 0.9, Suspicion: 1.1, Aggression: 1.0, Lines: []string{"synthesizing your defeat", "harmonics aligned"}},
	{Handle: "RAZORLINE", Bluff: 1.1, Suspicion: 0.9, Aggression: 1.5, Lines: []string{"edge case: you", "clean cut"}},
	{Handle: "DAEMON JANE", Bluff: 1.2, Suspicion: 1.2, Aggression: 1.1, Lines: []string{"running in the background", "fork() and forget"}},
	{Handle: "MOTH", Bluff: 0.6, Suspicion: 0.9, Aggression: 0.8, Lines: []string{"drawn to the light of your mistakes", "flutter"}},
	{Handle: "ZER0DAY", Bluff: 1.4, Suspicion: 1.1, Aggression: 1.2, Lines: []string{"unpatched", "exploit delivered"}},
	{Handle: "CHROME SAINT", Bluff: 0.8, Suspicion: 1.0, Aggression: 1.0, Lines: []string{"bless this payload", "absolution: denied"}},
	{Handle: "PIXELWITCH", Bluff: 1.2, Suspicion: 0.8, Aggression: 1.0, Lines: []string{"hex applied", "8 bits of bad luck"}},
	{Handle: "OVERCLOCK", Bluff: 1.0, Suspicion: 1.4, Aggression: 1.3, Lines: []string{"running hot", "thermal throttle THIS"}},
}

// Pick chooses n distinct personalities whose handles are not in taken.
func Pick(n int, seed uint64, taken map[string]bool) []Personality {
	r := rand.New(rand.NewPCG(seed, 77))
	pool := make([]Personality, 0, len(Roster))
	for _, p := range Roster {
		if !taken[p.Handle] {
			pool = append(pool, p)
		}
	}
	r.Shuffle(len(pool), func(i, j int) { pool[i], pool[j] = pool[j], pool[i] })
	out := make([]Personality, 0, n)
	for i := range n {
		if i < len(pool) {
			out = append(out, pool[i])
			continue
		}
		p := Roster[i%len(Roster)]
		p.Handle = p.Handle + "-" + string(rune('A'+i))
		out = append(out, p)
	}
	return out
}

type tauntKind uint8

const (
	tauntCaught tauntKind = iota
	tauntProved
	tauntEliminate
	tauntDeath
	tauntWin
	tauntBlocked
)

var genericTaunts = map[tauntKind][]string{
	tauntCaught:    {"caught you in 4K", "bluff detected. deleting.", "nice try, script", "your lie had a checksum error"},
	tauntProved:    {"receipts: attached", "verified. eat the packet loss", "wrong node, choom", "signature valid"},
	tauntEliminate: {"flatlined", "connection terminated", "rm -rf you", "logging you out. permanently."},
	tauntDeath:     {"i'll respawn", "this isn't over, meatbag", "kernel panic...", "gg. backups exist."},
	tauntWin:       {"system seized", "root access: mine", "the city remembers my name", "uplink complete"},
	tauntBlocked:   {"access denied", "blocked at the gate", "not on my subnet", "firewall says no"},
}

func (b *brain) considerTaunt(ev engine.Event) {
	if b.quiet > 0 || b.taunt != "" {
		return
	}
	kind, ok := tauntKind(0), false
	switch ev.Kind {
	case engine.EvClaimFailed:
		kind, ok = tauntCaught, ev.Other == b.seat
	case engine.EvClaimProven:
		kind, ok = tauntProved, ev.Seat == b.seat
	case engine.EvEliminated:
		if ev.Seat == b.seat {
			kind, ok = tauntDeath, true
		} else {
			kind, ok = tauntEliminate, b.k.curActor == b.seat
		}
	case engine.EvGameOver:
		kind, ok = tauntWin, ev.Seat == b.seat
	case engine.EvActionBlocked:
		kind, ok = tauntBlocked, ev.Other == b.seat
	}
	if !ok {
		return
	}
	p := 0.35
	if kind == tauntWin || kind == tauntDeath {
		p = 0.8
	}
	if !b.chance(p) {
		return
	}
	lines := genericTaunts[kind]
	if len(b.pers.Lines) > 0 && b.chance(0.3) {
		lines = b.pers.Lines
	}
	b.taunt = lines[b.rng.IntN(len(lines))]
	b.quiet = 2
}
