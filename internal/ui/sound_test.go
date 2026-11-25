package ui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/dormonbear/gotv-tui/internal/gotv"
	"github.com/dormonbear/gotv-tui/internal/playback"
	"github.com/dormonbear/gotv-tui/internal/radar"
)

func TestTapSync(t *testing.T) {
	now := time.Now()
	m := testMatch(false)
	m.kills = []radar.KillMark{{At: now.Add(-15 * time.Second)}, {At: now.Add(-3 * time.Second)}}
	m.tapSync(now) // heard 3 s after it was shown: the audio is later, the data waits
	if m.sound.offset != 3 || m.sound.delay() != 3 {
		t.Fatalf("offset %.1f, data delay %.1f; want 3, 3", m.sound.offset, m.sound.delay())
	}
	m.kills = nil
	m.soon = []playback.Soon[gotv.Event]{{In: 2, E: gotv.Event{Kind: gotv.EvKill}}, {In: 1, E: gotv.Event{Kind: gotv.EvPlant}}}
	m.tapSync(now) // heard 2 s before it shows: 3 - 2
	if m.sound.offset != 1 {
		t.Fatalf("offset %.1f, want 1", m.sound.offset)
	}
	m.soon = nil
	m.tapSync(now) // nothing near: unchanged
	if m.sound.offset != 1 {
		t.Fatal("moved without a kill nearby")
	}
}

func TestTypingSourceKeepsKeys(t *testing.T) {
	a := New("", nil, 0)
	a.match = testMatch(false)
	key := func(k string) { a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}) }
	key("v")
	if !a.match.sound.picking {
		t.Fatal("v didn't open the platform picker")
	}
	a.Update(tea.KeyMsg{Type: tea.KeyRight})
	a.Update(tea.KeyMsg{Type: tea.KeyRight})
	if platforms[a.match.sound.platform].name != "Bilibili" {
		t.Fatalf("→→ picked %s", platforms[a.match.sound.platform].name)
	}
	if f := ansi.Strip(a.match.soundFooter()); !strings.Contains(f, "Huya") || !strings.Contains(f, "Twitch") {
		t.Fatalf("picker footer %q", f)
	}
	key("2") // straight to Twitch
	if a.match.sound.input == nil || !strings.Contains(ansi.Strip(a.match.soundFooter()), "Twitch channel") {
		t.Fatal("2 should open the Twitch channel input")
	}
	for _, k := range "bqu" {
		a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{k}})
	}
	if a.match == nil || a.quitting || a.match.sound.input.Value() != "bqu" {
		t.Fatal("keys typed into the input acted as commands")
	}
	a.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if a.match == nil || a.match.sound.typing() {
		t.Fatal("esc should close the input, not leave the match")
	}
}

func TestPickOffStopsAudio(t *testing.T) {
	m := testMatch(false)
	m.sound.src = "huya 1"
	m.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("v")})
	m.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("5")})
	if m.sound.typing() || m.sound.src != "" || m.status != "audio off" {
		t.Fatal("Off stops the audio and closes the picker")
	}
}
