// Package feeds manages the list of CSTV relays and probes their state.
package feeds

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type Feed struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// Presets are relays organizers have published for in-game viewing (playcast "<url>").
// Sources: PGL Singapore 2026 community guidelines (pglesports.com), BLAST.tv Major
// community guidelines (blast.tv/major/community-guidelines), HLTV stream list for
// PGL Copenhagen 2024. Organizers reuse/replace these per event; edit as needed.
var Presets = []Feed{
	{Name: "PGL Major · Stream A", URL: "https://cstv.fragnet.net/a"},
	{Name: "PGL Major · Stream B", URL: "https://cstv.fragnet.net/b"},
	{Name: "BLAST Major · Stream A", URL: "https://gotv.blast.tv/major-a"},
	{Name: "BLAST Major · Stream B", URL: "https://gotv.blast.tv/major-b"},
}

// File is where the user's source list is saved (~/.config on every OS).
func File() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".config", "gotv-tui", "feeds.json")
}

// Load returns the saved list, or the presets when the file is missing or broken.
func Load(file string) []Feed {
	b, err := os.ReadFile(file)
	if err != nil {
		return append([]Feed(nil), Presets...)
	}
	var list []Feed
	if json.Unmarshal(b, &list) != nil {
		return append([]Feed(nil), Presets...)
	}
	out := list[:0]
	for _, f := range list {
		if f.Name != "" && f.URL != "" {
			out = append(out, f)
		}
	}
	return out
}

func Save(file string, list []Feed) error {
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(file, append(b, '\n'), 0o644)
}

var playcast = regexp.MustCompile(`^playcast\s+`)

// NormalizeURL accepts a bare URL or a pasted `playcast "<url>"` command.
func NormalizeURL(u string) string {
	u = strings.TrimSpace(u)
	u = playcast.ReplaceAllString(u, "")
	u = strings.Trim(u, `"`)
	return strings.TrimRight(u, "/")
}

var relayURL = regexp.MustCompile(`^https?://[^\s/]+`)

func IsRelayURL(u string) bool { return relayURL.MatchString(NormalizeURL(u)) }

// State: live = relay is receiving data; stale = answers but nothing new for a while
// (match over); waiting = relay reset for the next match and has no fragment yet
// (HTTP 405 "check back soon"); offline = unreachable.
type State string

const (
	Live    State = "live"
	Stale   State = "stale"
	Waiting State = "waiting"
	Offline State = "offline"
)

const staleAfter = 60 // seconds since the relay last received data

type Sync struct {
	Tick   *int     `json:"tick"`
	RcvAge *float64 `json:"rcvage"`
	Map    string   `json:"map"`
	TPS    float64  `json:"tps"`
}

func StateOf(s *Sync) State {
	if s == nil || s.Tick == nil {
		return Offline
	}
	if s.RcvAge != nil && *s.RcvAge > staleAfter {
		return Stale
	}
	return Live
}

var ErrWaiting = errors.New("waiting for the broadcast to start")

// FetchSync reads <url>/sync. A 405 means the relay is between matches.
func FetchSync(client *http.Client, url string) (*Sync, error) {
	r, err := client.Get(url + "/sync")
	if err != nil {
		return nil, err
	}
	defer r.Body.Close()
	if r.StatusCode == http.StatusMethodNotAllowed {
		return nil, ErrWaiting
	}
	if r.StatusCode != http.StatusOK {
		return nil, errors.New(r.Status)
	}
	var s Sync
	if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
		return nil, err
	}
	return &s, nil
}

type Status struct {
	Feed
	State State
	Map   string
}

func Probe(f Feed) Status {
	s, err := FetchSync(&http.Client{Timeout: 8 * time.Second}, f.URL)
	switch {
	case errors.Is(err, ErrWaiting):
		return Status{Feed: f, State: Waiting}
	case err != nil:
		return Status{Feed: f, State: Offline}
	}
	return Status{Feed: f, State: StateOf(s), Map: s.Map}
}
