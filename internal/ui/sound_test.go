package ui

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/dormonbear/gotv-tui/internal/audio"
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

func TestRoomHistoryAndRestore(t *testing.T) {
	dir := t.TempDir()
	roomsFile = func() string { return filepath.Join(dir, "audio.json") }
	var started []string
	startPlayer = func(src string, _ func(string)) (*audio.Player, error) {
		started = append(started, src)
		return nil, nil
	}
	defer func() { startPlayer = audio.Start }()

	for _, r := range []string{"1", "2", "3", "4", "5", "6", "3"} {
		addRoom("Huya", r)
	}
	if got := loadRooms()["Huya"]; strings.Join(got, ",") != "3,6,5,4,2" {
		t.Fatalf("newest first, no repeats, 5 kept: %v", got)
	}

	a := New("", nil, 0)
	a.list, _ = newList(filepath.Join(dir, "feeds.json"))
	a.match = testMatch(false)
	a.match.url = "https://relay.example/a"
	a.match.cancel = func() {}
	m := a.match
	m.startAudio("twitch eslcs", [2]string{"Twitch", "eslcs"})
	if len(loadRooms()["Twitch"]) != 0 {
		t.Fatal("a room is only remembered once it plays")
	}
	m.update(soundStatusMsg("audio: playing"))
	if got := loadRooms()["Twitch"]; len(got) != 1 || got[0] != "eslcs" {
		t.Fatalf("played room saved: %v", got)
	}
	m.sound.offset = 2.5
	a.Update(tea.KeyMsg{Type: tea.KeyEsc}) // back to the list
	a.open("https://relay.example/a", nil)
	if n := len(started); n != 2 || started[1] != "https://www.twitch.tv/eslcs" || a.match.sound.offset != 2.5 {
		t.Fatalf("reopening the source restores its audio: %v offset %v", started, a.match.sound.offset)
	}
	a.match.close()
	a.open("https://relay.example/b", nil)
	if len(started) != 2 {
		t.Fatal("another source starts silent")
	}
	a.match.close()
	if New("", nil, 0).audio != nil {
		t.Fatal("a new run restores nothing")
	}
}
