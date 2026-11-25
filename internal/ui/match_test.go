package ui

import (
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/dormonbear/gotv-tui/internal/gotv"
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

func TestBlastsAreMapOnly(t *testing.T) {
	m := &matchModel{}
	now := time.Now()
	m.applyEvent(gotv.Event{Kind: gotv.EvBlast, Nade: gotv.HE, To: &gotv.Vec{X: 1}}, now)
	if len(m.log) != 0 || len(m.blasts) != 1 {
		t.Fatalf("log %d, blasts %d: a blast is drawn, not logged", len(m.log), len(m.blasts))
	}
	m.applyEvent(gotv.Event{Kind: gotv.EvRoundStart, Round: 2}, now)
	if len(m.blasts) != 0 {
		t.Fatal("a new round clears the blasts")
	}
}
