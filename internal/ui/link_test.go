package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
)

func TestLinkBadge(t *testing.T) {
	for status, want := range map[string]link{
		"connected · Mirage": linkLive, "waiting for the broadcast to start...": linkWaiting,
		"relay offline, retrying...": linkDown, "relay ended (EOF), reconnecting...": linkDown,
		"playing demo at 8x": linkDemo, "audio off": linkNone,
	} {
		if got := linkOf(status); got != want {
			t.Errorf("linkOf(%q) = %v, want %v", status, got, want)
		}
	}
	now := time.Now()
	badge := func(l link, last time.Time) string { return ansi.Strip(linkBadge(l, last, now)) }
	if !strings.Contains(badge(linkLive, now), "LIVE") || !strings.Contains(badge(linkLive, now.Add(-6*time.Second)), "STALLED") {
		t.Fatal("live until snapshots stop for 5 s")
	}
	a, b := linkBadge(linkWaiting, now, now), linkBadge(linkWaiting, now, now.Add(100*time.Millisecond))
	if a == b {
		t.Fatal("the waiting spinner should move")
	}
	if badge(linkNone, now) != "" {
		t.Fatal("no badge before any status")
	}
}
