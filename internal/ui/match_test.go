package ui

import (
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
)

func TestWaitingAnimates(t *testing.T) {
	t0 := time.UnixMilli(0)
	a, b := waiting(t0, "s"), waiting(t0.Add(250*time.Millisecond), "s")
	if a[1] == b[1] {
		t.Fatal("runner didn't move between ticks")
	}
	for i := 0; i < 200; i++ { // bounces within the lane, never wider
		if w := lipgloss.Width(waiting(t0.Add(time.Duration(i)*250*time.Millisecond), "s")[1]); w != 31 {
			t.Fatalf("frame %d is %d wide", i, w)
		}
	}
}
