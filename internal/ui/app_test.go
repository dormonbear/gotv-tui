package ui

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestUpdateNoticeAndKey(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // the list reads ~/.config/gotv-tui
	a := New("", nil, 0)
	a.Version = "0.1.0"
	a.list, _ = newList(filepath.Join(t.TempDir(), "feeds.json"))
	if _, cmd := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("u")}); cmd != nil && a.updating {
		t.Fatal("u updated with no newer release known")
	}
	a.Update(latestMsg("0.2.0"))
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
