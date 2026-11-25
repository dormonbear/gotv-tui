package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/dormonbear/gotv-tui/internal/gotv"
	"github.com/dormonbear/gotv-tui/internal/radar"
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

func TestMissingRadar(t *testing.T) {
	m := &matchModel{loading: "de_nowhere", st: &gotv.State{Map: "de_nowhere"}}
	m.update(radarMsg{name: "de_nowhere", err: fmt.Errorf("radar de_nowhere: %w", radar.ErrNotFound)})
	if m.noRadar != "de_nowhere" || !m.retryAt.IsZero() {
		t.Fatal("a radar no source has isn't retried")
	}
	if v := ansi.Strip(m.mapView(60, 20, nil)); !strings.Contains(v, "no radar for") {
		t.Fatalf("map area says %q", v)
	}
}

func TestHelpOverlay(t *testing.T) {
	m := testMatch(false)
	m.view()
	key := func(k string) { m.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}) }
	key("?")
	if v := ansi.Strip(m.view()); !strings.Contains(v, "KEYS") || !strings.Contains(v, "tap sync") {
		t.Fatal("? shows the key help")
	}
	was := m.directing
	key("d")
	if m.help || m.directing != was {
		t.Fatal("a key closes the help and does nothing else")
	}
	if strings.Contains(ansi.Strip(m.view()), "tap sync") {
		t.Fatal("help still drawn")
	}
}
