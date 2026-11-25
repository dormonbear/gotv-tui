package ui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
)

// roomHistory is each platform's rooms that played, newest first, kept across runs
// in ~/.config/gotv-tui/audio.json.
type roomHistory map[string][]string

const roomsKept = 5

var roomsFile = func() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "gotv-tui", "audio.json")
}

func loadRooms() roomHistory {
	h := roomHistory{}
	if b, err := os.ReadFile(roomsFile()); err == nil {
		_ = json.Unmarshal(b, &h) // a broken file is just no history
	}
	return h
}

// addRoom puts room first in platform's history and saves it.
func addRoom(platform, room string) {
	h := loadRooms()
	list := slices.DeleteFunc(slices.Clone(h[platform]), func(r string) bool { return r == room })
	h[platform] = append([]string{room}, list...)[:min(roomsKept, len(list)+1)]
	b, err := json.MarshalIndent(h, "", "  ")
	if err != nil {
		return
	}
	file := roomsFile()
	_ = os.MkdirAll(filepath.Dir(file), 0o755)
	if tmp := file + ".tmp"; os.WriteFile(tmp, b, 0o644) == nil {
		_ = os.Rename(tmp, file)
	}
}
