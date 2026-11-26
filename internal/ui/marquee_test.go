package ui

import (
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
)

func TestMarquee(t *testing.T) {
	s := "stream ahead · locks when the radar reaches R14 0:51"
	if got := marquee("short", 10, time.Now()); got != "short" {
		t.Fatalf("fits: %q", got)
	}
	w := 20
	over := ansi.StringWidth(s) - w
	run := float64(over) / marqueeSpeed
	at := func(sec float64) string { return marquee(s, w, time.UnixMilli(int64(sec*1000))) }
	if got := at(0.5); got != ansi.Cut(s, 0, w) {
		t.Fatalf("holds the start first: %q", got)
	}
	if got := at(marqueeHold + run + 0.5); got != ansi.Cut(s, over, over+w) {
		t.Fatalf("reaches the end: %q", got)
	}
	for sec := 0.0; sec < 3*(run+marqueeHold); sec += 0.1 {
		if n := ansi.StringWidth(at(sec)); n != w {
			t.Fatalf("at %.1fs width %d, want %d", sec, n, w)
		}
	}
}
