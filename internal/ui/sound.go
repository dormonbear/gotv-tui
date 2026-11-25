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
	picking   bool              // choosing the platform, before the input
	platform  int               // index into platforms
	last      map[string]string // last room typed per platform, offered again
}

// platforms the audio picker offers; prefix is what audio.Source takes before the room.
var platforms = []struct{ name, prefix, hint string }{
	{"Huya", "huya", "room number or name, e.g. 123321"},
	{"Twitch", "twitch", "channel name, e.g. eslcs"},
	{"Bilibili", "bili", "room number, e.g. 6"},
	{"URL", "", "a stream or media URL"},
	{"Off", "", ""},
}

type soundStatusMsg string

func (s *sound) typing() bool { return s.input != nil || s.picking }

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
		s.picking = true
		return nil, true
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
	if s.picking {
		return m.pickPlatform(msg.String())
	}
	switch msg.String() {
	case "esc":
		s.input = nil
		return nil
	case "enter":
		p := platforms[s.platform]
		v := strings.TrimSpace(s.input.Value())
		s.input = nil
		if v == "" {
			return nil
		}
		s.stop()
		typed := v
		if p.prefix != "" {
			typed = p.prefix + " " + v
		}
		src, err := audio.Source(typed)
		if err != nil {
			m.status = err.Error()
			return nil
		}
		if s.last == nil {
			s.last = map[string]string{}
		}
		s.last[p.name] = v
		send := m.send
		player, err := audio.Start(src, func(st string) { send(soundStatusMsg(st), false) })
		if err != nil {
			m.status = err.Error()
			return nil
		}
		s.player, s.src = player, typed
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

// pickPlatform: ←/→ (or the number) choose, enter goes on to the room, esc cancels.
func (m *matchModel) pickPlatform(k string) tea.Cmd {
	s := &m.sound
	switch k {
	case "left", "h", "shift+tab":
		s.platform = (s.platform + len(platforms) - 1) % len(platforms)
	case "right", "l", "tab":
		s.platform = (s.platform + 1) % len(platforms)
	case "esc":
		s.picking = false
	case "1", "2", "3", "4", "5":
		s.platform = int(k[0]-'1') % len(platforms)
		return m.pickPlatform("enter")
	case "enter":
		s.picking = false
		p := platforms[s.platform]
		if p.name == "Off" {
			s.stop()
			m.status = "audio off"
			return nil
		}
		in := textinput.New()
		in.Placeholder = p.hint
		in.SetValue(s.last[p.name])
		in.CharLimit = 300
		in.Width = max(20, m.w-30)
		s.input = &in
		return in.Focus()
	}
	return nil
}

// soundFooter is the footer while picking a platform or typing the room.
func (m *matchModel) soundFooter() string {
	s := &m.sound
	if s.picking {
		var parts []string
		for i, p := range platforms {
			label := fmt.Sprintf(" %d %s ", i+1, p.name)
			if i == s.platform {
				label = switchOn.Render(label)
			}
			parts = append(parts, label)
		}
		return " audio from: " + strings.Join(parts, " ") + dim("   ←/→ choose · enter · esc")
	}
	p := platforms[s.platform]
	label := p.name + " room: "
	if p.prefix == "" {
		label = "URL: "
	} else if p.name == "Twitch" {
		label = "Twitch channel: "
	}
	return " " + label + s.input.View()
}
