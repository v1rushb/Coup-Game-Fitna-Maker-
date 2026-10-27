package client

import (
	"sync"

	"charm.land/lipgloss/v2"
)

// Measuring a styled line's width means segmenting it into graphemes, and
// at 20 fps the same lines are measured over and over: most of a frame is
// the same as the last one. cellWidth remembers recent answers. The cache is
// shared by every session in the process (SSH), hence the lock.
var widths = struct {
	sync.Mutex
	m map[string]int
}{m: map[string]int{}}

const widthCacheMax = 8192

func cellWidth(s string) int {
	if len(s) < 24 {
		return lipgloss.Width(s) // cheap enough, and not worth a slot
	}
	widths.Lock()
	w, ok := widths.m[s]
	widths.Unlock()
	if ok {
		return w
	}
	w = lipgloss.Width(s)
	widths.Lock()
	if len(widths.m) >= widthCacheMax {
		widths.m = map[string]int{}
	}
	widths.m[s] = w
	widths.Unlock()
	return w
}
