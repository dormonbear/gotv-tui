package ui

import (
	"fmt"
	"math"
	"strings"
	"sync/atomic"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/dormonbear/gotv-tui/internal/audio"
	"github.com/dormonbear/gotv-tui/internal/gotv"
)

// sound is a stream's audio played along with the match, and the offset that
// lines the two up. offset > 0: the audio is later than the match data, so the
// data waits; offset < 0: the audio is earlier, so the audio waits.
type sound struct {
	player    *audio.Player
	src       string
	offset    float64      // seconds
	dataDelay atomic.Int64 // ms, read by the playback goroutine
	input     *textinput.Model
	last      string // last source typed, offered again
}

type soundStatusMsg string

func (s *sound) typing() bool { return s.input != nil }

// apply splits the offset between the data and the audio.
func (s *sound) apply() {
	s.dataDelay.Store(int64(max(s.offset, 0) * 1000))
	if s.player != nil {
		s.player.SetDelay(time.Duration(max(-s.offset, 0) * float64(time.Second)))
	}
}

func (s *sound) delay() float64 { return float64(s.dataDelay.Load()) / 1000 }

func (s *sound) stop() {
	if s.player != nil {
		s.player.Stop()
		s.player = nil
	}
	s.src = ""
}

// soundKey handles the audio keys; ok reports whether k was one.
func (m *matchModel) soundKey(k string) (tea.Cmd, bool) {
	s := &m.sound
	switch k {
	case "v":
		in := textinput.New()
		in.Placeholder = "stream URL, or bili 6 / twitch name / douyin 123 (empty: stop)"
		in.SetValue(s.last)
		in.CharLimit = 300
		in.Width = max(20, m.w-24)
		s.input = &in
		return in.Focus(), true
	case "[", "]", "{", "}":
		if s.player == nil {
			return nil, true
		}
		step := map[string]float64{"[": -1, "]": 1, "{": -0.1, "}": 0.1}[k]
		s.offset = math.Round((s.offset+step)*10) / 10
		s.apply()
		return nil, true
	case "t":
		if s.player != nil {
			m.tapSync(time.Now())
		}
		return nil, true
	}
	return nil, false
}

// typeSource feeds a key to the source input.
func (m *matchModel) typeSource(msg tea.KeyMsg) tea.Cmd {
	s := &m.sound
	switch msg.String() {
	case "esc":
		s.input = nil
		return nil
	case "enter":
		v := strings.TrimSpace(s.input.Value())
		s.input = nil
		s.stop()
		if v == "" {
			m.status = "audio off"
			return nil
		}
		src, err := audio.Source(v)
		if err != nil {
			m.status = err.Error()
			return nil
		}
		s.last = v
		send := m.send
		p, err := audio.Start(src, func(st string) { send(soundStatusMsg(st), false) })
		if err != nil {
			m.status = err.Error()
			return nil
		}
		s.player, s.src = p, v
		s.apply()
		return nil
	}
	var cmd tea.Cmd
	*s.input, cmd = s.input.Update(msg)
	return cmd
}

// tapSync lines the audio up on a kill: pressed when the commentary calls one,
// the nearest kill (already shown, or queued to show) says how far off we are.
func (m *matchModel) tapSync(now time.Time) {
	best, found := 0.0, false
	try := func(c float64) {
		if math.Abs(c) <= 20 && (!found || math.Abs(c) < math.Abs(best)) {
			best, found = c, true
		}
	}
	for _, k := range m.kills { // shown at k.At: heard c seconds after
		try(now.Sub(k.At).Seconds())
	}
	for _, s := range m.soon { // shown in s.In seconds: heard that much before
		if s.E.Kind == gotv.EvKill {
			try(-s.In)
		}
	}
	if !found {
		m.status = "tap sync: no kill within 20 s of now; press t right as a kill is called"
		return
	}
	m.sound.offset = math.Round((m.sound.offset+best)*10) / 10
	m.sound.apply()
	m.status = fmt.Sprintf("tap sync: moved %+.1fs", best)
}

// soundNote is the footer's audio part.
func (m *matchModel) soundNote() string {
	s := &m.sound
	if s.player == nil {
		return ""
	}
	return fmt.Sprintf(" ♪ %s · offset %+.1fs ([ ] { } t) ·", s.src, s.offset)
}
