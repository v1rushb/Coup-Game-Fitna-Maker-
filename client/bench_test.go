package client

import (
	"testing"

	"github.com/v1rushb/neoncoup/engine"
)

// BenchmarkTableFrame measures one 160x45 table frame, the cost that the
// frame rate multiplies.
func BenchmarkTableFrame(b *testing.B) {
	sc := newScripted(b, false)
	sc.do(0, engine.ActionIntent(engine.Tax, -1))
	sc.a.w, sc.a.h = 160, 45
	for b.Loop() {
		_ = sc.a.View()
	}
}
