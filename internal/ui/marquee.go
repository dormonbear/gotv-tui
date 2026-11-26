package ui

import (
	"time"

	"github.com/charmbracelet/x/ansi"
)

const (
	marqueeSpeed = 6.0 // cells a second
	marqueeHold  = 1.5 // seconds paused at each end
)

// marquee fits s into w cells: as is when it fits, else a window sliding across
// it and back, pausing at both ends, so long text can be read in full.
func marquee(s string, w int, now time.Time) string {
	over := ansi.StringWidth(s) - w
	if w <= 0 || over <= 0 {
		return s
	}
	run := float64(over) / marqueeSpeed
	cycle := 2 * (run + marqueeHold)
	t := float64(now.UnixMilli()%int64(cycle*1000)) / 1000
	var at float64
	switch {
	case t < marqueeHold:
		at = 0
	case t < marqueeHold+run:
		at = (t - marqueeHold) * marqueeSpeed
	case t < 2*marqueeHold+run:
		at = float64(over)
	default:
		at = float64(over) - (t-2*marqueeHold-run)*marqueeSpeed
	}
	left := min(over, max(0, int(at)))
	return ansi.Cut(s, left, left+w)
}
