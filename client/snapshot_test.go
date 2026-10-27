package client

import (
	"flag"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/v1rushb/neoncoup/engine"
	"github.com/v1rushb/neoncoup/protocol"
)

var update = flag.Bool("update", false, "rewrite the golden table snapshots in testdata/")

// snapshotScenes set up a table worth pinning down.
var snapshotScenes = []struct {
	name  string
	setup func(sc *scripted)
}{
	{"challenge-window", func(sc *scripted) {
		sc.do(0, engine.ActionIntent(engine.Tax, -1))
	}},
	{"announcement", func(sc *scripted) {
		sc.s.speed = protocol.SpeedRelaxed
		sc.queueFrames(sc.apply(0, engine.ActionIntent(engine.Steal, 2)))
		sc.s.pump(sc.a)
		sc.a.now = sc.a.now.Add(600 * time.Millisecond) // after the glitch-in
	}},
	{"your-turn", func(sc *scripted) {
		sc.do(0, engine.ActionIntent(engine.Income, -1))
		sc.s.turnAlert = time.Time{} // the pulse alternates; pin it off
	}},
	{"results", func(sc *scripted) {
		_, rep := recordedGame(sc.t.(*testing.T))
		sc.s.onGameOver(sc.a, protocol.GameOver{Winner: 1, Names: []string{"VEX", "RUNNER", "MOTH", "GHOSTWIRE", "NULLBYTE"}, Replay: rep})
		sc.s.overAt = sc.a.now.Add(-5 * time.Second)
		sc.s.replayPath = "~/.config/neoncoup/replays/20870101-000000-7K2Q.json"
	}},
}

// TestTableSnapshots pins the table's look at the two sizes that matter most:
// the 100x30 minimum and a roomy 160x45.
func TestTableSnapshots(t *testing.T) {
	for _, sz := range [][2]int{{100, 30}, {160, 45}} {
		for _, sc := range snapshotScenes {
			name := fmt.Sprintf("%s-%dx%d.txt", sc.name, sz[0], sz[1])
			t.Run(name, func(t *testing.T) {
				s := newScripted(t, false)
				s.a.rng = rand.New(rand.NewPCG(1, 2))
				sc.setup(s)
				s.a.w, s.a.h = sz[0], sz[1]
				checkFrame(t, s.a, name)
				got := shot(s.a)
				path := filepath.Join("testdata", "snapshots", name)
				if *update {
					if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
						t.Fatal(err)
					}
					return
				}
				want, err := os.ReadFile(path)
				if err != nil {
					t.Fatalf("%v (run with -update to create it)", err)
				}
				if got != string(want) {
					t.Fatalf("%s changed (run with -update if intended):\n--- got\n%s--- want\n%s", name, got, want)
				}
			})
		}
	}
}
