package ui

import (
	"path/filepath"
	"testing"

	"github.com/dormonbear/gotv-tui/internal/feeds"
	"github.com/dormonbear/gotv-tui/internal/gotv"
)

func TestListAutoRefreshKeepsRows(t *testing.T) {
	m, _ := newList(filepath.Join(t.TempDir(), "feeds.json"))
	url := m.feeds[0].URL
	m.update(probeMsg(feeds.Status{URL: url, State: feeds.Live, Map: "de_nuke"}))
	m.update(peekMsg{url, &gotv.Summary{}})

	if _, cmd := m.update(autoMsg{}); cmd == nil {
		t.Fatal("auto refresh scheduled nothing")
	}
	if _, ok := m.status[url]; !ok || m.match[url] == nil {
		t.Fatal("auto refresh cleared the row; it should update it in place")
	}
	// same match, new probe: teams stay (and a live relay is peeked again for the score)
	if _, cmd := m.update(probeMsg(feeds.Status{URL: url, State: feeds.Live, Map: "de_nuke"})); m.match[url] == nil || cmd == nil {
		t.Fatal("same match lost its teams or wasn't re-peeked")
	}
	// another map on the relay: the old teams go
	m.update(probeMsg(feeds.Status{URL: url, State: feeds.Live, Map: "de_inferno"}))
	if m.match[url] != nil {
		t.Fatal("kept the previous match's teams after the map changed")
	}
}
