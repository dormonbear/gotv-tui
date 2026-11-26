package ui

import (
	"fmt"
	"math"
	"strings"
	"sync/atomic"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/dormonbear/gotv-tui/internal/audio"
	"github.com/dormonbear/gotv-tui/internal/gotv"
)

// sound is a stream's audio played along with the match, and the offset that
// lines the two up. offset > 0: the audio is later than the match data, so the
// data waits; offset < 0: the audio is earlier, so the audio waits.
type sound struct {
	player    *audio.Player
	src       string
	url       string       // the stream page src resolves to
	offset    float64      // seconds
	dataDelay atomic.Int64 // ms, read by the playback goroutine
	input     *textinput.Model
	picking   bool              // choosing the platform, before the input
	platform  int               // index into platforms
	last      map[string]string // last room typed per platform, offered again
	history   []string          // the chosen platform's rooms that played, for ↑/↓
	histAt    int               // -1: not browsing the history
	pending   [2]string         // platform and room just started; saved once it plays
	muted     bool
	auto      autoSync
}

// audioChoice is what a match was playing, to restore when the source is reopened.
type audioChoice struct {
	typed  string
	offset float64
}

// startPlayer starts the audio pipeline (swapped out in tests).
var startPlayer = audio.Start

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
	s.auto.stop()
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
	case "a":
		if s.player == nil {
			return nil, true
		}
		s.auto.on = !s.auto.on
		if s.auto.on {
			m.status = "auto sync on: reading the stream's round clock..."
			m.startAutoSync(s.url)
		} else {
			s.auto.stop()
			m.status = "auto sync off"
		}
		return nil, true
	case "[", "]", "{", "}":
		if s.player == nil {
			return nil, true
		}
		s.manual()
		step := map[string]float64{"[": -1, "]": 1, "{": -0.1, "}": 0.1}[k]
		m.setOffset(math.Round((s.offset+step)*10) / 10)
		return nil, true
	case "t":
		if s.player != nil {
			s.manual()
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
		if s.last == nil {
			s.last = map[string]string{}
		}
		s.last[p.name] = v
		if off, ok := loadOffset(m.url, typed); ok { // lined up before: start lined up
			s.offset = off
		}
		m.startAudio(typed, [2]string{p.name, v})
		return nil
	case "up", "down":
		if len(s.history) == 0 {
			return nil
		}
		if msg.String() == "up" {
			s.histAt = min(s.histAt+1, len(s.history)-1)
		} else {
			s.histAt = max(s.histAt-1, -1)
		}
		v := ""
		if s.histAt >= 0 {
			v = s.history[s.histAt]
		}
		s.input.SetValue(v)
		s.input.CursorEnd()
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
	m.setOffset(math.Round((m.sound.offset+best)*10) / 10)
	m.status = fmt.Sprintf("tap sync: moved %+.1fs", best)
}

// soundNote is the footer's audio part.
func (m *matchModel) soundNote() string {
	s := &m.sound
	if s.player == nil {
		return ""
	}
	auto := ""
	if s.auto.cancel != nil {
		auto = " auto"
	}
	return fmt.Sprintf(" offset%s: a [ ] t ·", auto)
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
		s.history, s.histAt = loadRooms()[p.name], -1
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
	line := " " + label + s.input.View()
	if len(s.history) > 0 {
		line += dim("   ↑/↓ history: " + strings.Join(s.history, " · "))
	}
	return line
}

// startAudio plays typed ("huya 123321", a URL, ...); pick is the platform and
// room to remember once it actually plays.
func (m *matchModel) startAudio(typed string, pick [2]string) {
	s := &m.sound
	s.stop()
	src, err := audio.Source(typed)
	if err != nil {
		m.status = err.Error()
		return
	}
	send := m.send
	player, err := startPlayer(src, func(st string) { send(soundStatusMsg(st), false) })
	if err != nil {
		m.status = err.Error()
		return
	}
	s.player, s.src, s.url, s.pending = player, typed, src, pick
	s.apply()
	m.startAutoSync(src)
}

// audioChoice is what to restore if this source is opened again.
func (m *matchModel) audioChoice() *audioChoice {
	if m.sound.src == "" {
		return nil
	}
	return &audioChoice{m.sound.src, m.sound.offset}
}

// restoreAudio starts the audio a source had when it was left.
func (m *matchModel) restoreAudio(c *audioChoice) {
	m.sound.offset = c.offset
	m.startAudio(c.typed, [2]string{})
}

// manual: a hand adjustment turns auto sync off, so it isn't undone.
func (s *sound) manual() {
	s.auto.stop()
	s.auto.on = false
}

var soundOn = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("0")).Background(lipgloss.Color("13"))

// soundBadge is the "now playing" badge next to the director switch; the note
// alternates so it reads as playing.
func (m *matchModel) soundBadge() string {
	s := &m.sound
	if s.player == nil {
		return ""
	}
	note := "♪"
	if time.Now().UnixMilli()/500%2 == 1 {
		note = "♫"
	}
	label := fmt.Sprintf(" %s %s  %+.1fs ", note, strings.ToUpper(s.src), s.offset)
	if s.auto.cancel != nil {
		label += "AUTO "
	}
	badge := soundOn.Render(label)
	if s.muted {
		badge = switchOff.Render(fmt.Sprintf(" ✕ MUTED %s ", strings.ToUpper(s.src)))
	}
	m.badgeW = lipgloss.Width(badge)
	return badge
}

// streamClock is what the stream shows as heard now ("STREAM R18 0:58"), next to
// the data's own round and clock, so a gap between them is plain to see.
func (m *matchModel) streamClock() string {
	a := &m.sound.auto
	if a.cancel == nil {
		return ""
	}
	r, secs, ok := a.streamNow(time.Now(), time.Duration(max(-m.sound.offset, 0)*float64(time.Second)))
	sync := ""
	if !a.locked {
		sync = dim(" syncing…")
		if r := a.lockTarget(); r != nil { // the stream is ahead: say what we wait for
			sync = dim(fmt.Sprintf(" stream ahead · locks when the radar reaches R%d %d:%02d", r.Round, r.Secs/60, r.Secs%60))
		}
	}
	if !ok {
		return dim(" STREAM --:--") + sync
	}
	round := ""
	if r > 0 {
		round = fmt.Sprintf("R%d ", r)
	}
	return fmt.Sprintf(" STREAM %s%d:%02d", round, secs/60, secs%60) + sync
}

// setOffset applies a new offset and remembers it for this relay and stream.
func (m *matchModel) setOffset(v float64) {
	m.sound.offset = v
	m.sound.apply()
	saveOffset(m.url, m.sound.src, v)
}

// toggleMute stops or resumes the sound; the stream keeps being received, so it
// comes back lined up.
func (m *matchModel) toggleMute() {
	s := &m.sound
	if s.player == nil {
		return
	}
	s.muted = !s.muted
	s.player.SetMuted(s.muted)
}
