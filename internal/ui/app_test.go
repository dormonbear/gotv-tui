package ui

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/dormonbear/gotv-tui/internal/feeds"
)

func TestUpdateNoticeAndKey(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // the list reads ~/.config/gotv-tui
	a := New("", nil, 0)
	a.Version = "0.1.0"
	a.list, _ = newList(filepath.Join(t.TempDir(), "feeds.json"))
	a.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	if _, cmd := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("u")}); cmd != nil && a.updating {
		t.Fatal("u updated with no newer release known")
	}
	if !strings.Contains(plain(a.View()), "v0.1.0 · checking for updates") {
		t.Fatal("no version line before the check answers")
	}
	a.Update(latestMsg{v: "0.2.0"})
	if !strings.Contains(plain(a.View()), "v0.2.0 available · press u to update") {
		t.Fatal("no update notice on the list")
	}
	if _, cmd := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("u")}); cmd == nil || !a.updating {
		t.Fatal("u didn't start the update")
	}
	if !strings.Contains(plain(a.View()), "updating to v0.2.0") {
		t.Fatal("no progress shown")
	}
	a.Update(updatedMsg{err: errors.New("offline")})
	if a.updating || a.Restart || !strings.Contains(plain(a.View()), "update failed: offline") {
		t.Fatal("a failed update should say so and keep running")
	}
}

func TestStaleMatchMessagesAreDropped(t *testing.T) {
	a := New("", nil, 0)
	old, cur := testMatch(false), testMatch(false)
	a.match = cur
	a.Update(matchMsg{old, statusMsg("from the closed match")})
	if cur.status == "from the closed match" {
		t.Fatal("a closed match's message reached the open one")
	}
	a.Update(matchMsg{cur, statusMsg("ours")})
	if cur.status != "ours" {
		t.Fatal("the open match's own message was dropped")
	}
}

func TestAutoRefreshRestartsAfterAMatch(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m, _ := newList(filepath.Join(t.TempDir(), "feeds.json"))
	m.resume() // back from a match: new loop
	if _, cmd := m.update(autoMsg{0}); cmd != nil {
		t.Fatal("the old loop's tick kept a second loop going")
	}
	if _, cmd := m.update(autoMsg{m.gen}); cmd == nil {
		t.Fatal("the current loop stopped")
	}
}

func TestVersionLineStates(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	a := New("", nil, 0)
	a.Version = "0.20.0"
	a.list, _ = newList(filepath.Join(t.TempDir(), "feeds.json"))
	a.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	a.Update(latestMsg{})
	if !strings.Contains(plain(a.View()), "v0.20.0 · up to date") {
		t.Fatal("no 'up to date' when the check found nothing newer")
	}
	a.Update(latestMsg{err: errors.New("rate limited")})
	if !strings.Contains(plain(a.View()), "v0.20.0 · couldn't check for updates") {
		t.Fatal("a failed check should say so")
	}
	a.Version = "dev"
	if !strings.Contains(plain(a.View()), "source build") {
		t.Fatal("dev builds don't check")
	}
}

func TestClickInAnotherPaneKeepsTheDirector(t *testing.T) {
	a := New("", nil, 0)
	a.match = testMatch(false)
	a.match.view()
	a.match.toggleDirector()
	// what Ghostty sends for a click in the split pane above ours (pixel reports, y < 0)
	for _, k := range []tea.KeyMsg{
		{Type: tea.KeyRunes, Runes: []rune("["), Alt: true},
		{Type: tea.KeyRunes, Runes: []rune("<0;2256;-6M")},
		{Type: tea.KeyRunes, Runes: []rune("<0;-17;-20m")},
	} {
		a.Update(k)
	}
	if !a.match.directing {
		t.Fatal("a click in another pane turned the director off")
	}
	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")}) // not a camera key either
	if !a.match.directing {
		t.Fatal("an unrelated key turned the director off")
	}
	a.Update(tea.KeyMsg{Type: tea.KeyLeft})
	if a.match.directing {
		t.Fatal("a camera key should still take the camera")
	}
}

func TestListMouse(t *testing.T) {
	a := New("", nil, 0)
	a.list, _ = newList(filepath.Join(t.TempDir(), "feeds.json"))
	a.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	a.list.feeds = []feeds.Feed{{Name: "one", URL: "https://a.example/x"}, {Name: "two", URL: "https://b.example/y"}}
	view := ansi.Strip(a.list.view(""))
	rows := strings.Split(view, "\n")
	if !strings.Contains(rows[a.list.rowY+1], "two") {
		t.Fatalf("row %d is %q, want the second source", a.list.rowY+1, rows[a.list.rowY+1])
	}
	click := func(y int) {
		a.Update(tea.MouseMsg{X: 5, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	}
	click(a.list.rowY + 1)
	if a.list.sel != 1 || a.match != nil {
		t.Fatal("first click selects")
	}
	a.Update(tea.MouseMsg{Button: tea.MouseButtonWheelUp, Action: tea.MouseActionPress})
	if a.list.sel != 0 {
		t.Fatal("wheel moves the selection")
	}
	click(a.list.rowY)
	if a.match == nil {
		t.Fatal("clicking the selected source opens it")
	}
	a.match.close()
}
