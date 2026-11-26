package ui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
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
	writeJSON(roomsFile(), h)
}

// offsets remembers the audio offset per relay and stream ("<relay> <stream>"), so
// reopening a pair starts lined up; ~/.config/gotv-tui/offsets.json.
var offsetsFile = func() string { return filepath.Join(filepath.Dir(roomsFile()), "offsets.json") }

func loadOffsets() map[string]float64 {
	o := map[string]float64{}
	if b, err := os.ReadFile(offsetsFile()); err == nil {
		_ = json.Unmarshal(b, &o)
	}
	return o
}

func loadOffset(relay, stream string) (float64, bool) {
	v, ok := loadOffsets()[relay+" "+stream]
	return v, ok
}

func saveOffset(relay, stream string, v float64) {
	if relay == "" || stream == "" || strings.HasPrefix(relay, DemoPrefix) {
		return
	}
	o := loadOffsets()
	o[relay+" "+stream] = v
	writeJSON(offsetsFile(), o)
}

func writeJSON(file string, v any) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(file), 0o755)
	if tmp := file + ".tmp"; os.WriteFile(tmp, b, 0o644) == nil {
		_ = os.Rename(tmp, file)
	}
}
