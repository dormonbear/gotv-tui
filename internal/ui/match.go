package ui

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/dormonbear/gotv-tui/internal/gotv"
	"github.com/dormonbear/gotv-tui/internal/radar"
	"github.com/dormonbear/gotv-tui/internal/term"
)

const (
	logLimit = 200
	leftMax  = 80
)

type (
	stateMsg  gotv.State
	eventMsg  gotv.Event
	statusMsg string
	radarMsg  struct {
		a   *radar.Assets
		err error
	}
	tickMsg time.Time
)

// levelMap caches one framed radar level for a size.
type levelMap struct {
	lower bool
	w, h  int
	r     *radar.Raster
	base  *radar.Base
}

type matchModel struct {
	url     string
	cancel  context.CancelFunc
	ch      chan tea.Msg
	st      *gotv.State
	log     []gotv.Event // newest first
	kills   []radar.KillMark
	status  string
	assets  *radar.Assets
	box     radar.Box
	levels  []*levelMap
	loading string // map being loaded
	w, h    int
}

func newMatch(url string, w, h int) (*matchModel, tea.Cmd) {
	ctx, cancel := context.WithCancel(context.Background())
	m := &matchModel{url: url, cancel: cancel, ch: make(chan tea.Msg, 64), status: "connecting...", w: w, h: h}
	send := func(msg tea.Msg, drop bool) {
		if drop { // frames are superseded by the next one; never block the parser on them
			select {
			case m.ch <- msg:
			default:
			}
			return
		}
		select {
		case m.ch <- msg:
		case <-ctx.Done():
		}
	}
	go gotv.Connect(ctx, url, gotv.Handlers{
		State:  func(s gotv.State) { send(stateMsg(s), true) },
		Event:  func(e gotv.Event) { send(eventMsg(e), false) },
		Status: func(s string) { send(statusMsg(s), false) },
	})
	return m, tea.Batch(m.wait(), tick())
}

func (m *matchModel) wait() tea.Cmd {
	ch := m.ch
	return func() tea.Msg { return <-ch }
}

func tick() tea.Cmd {
	return tea.Tick(250*time.Millisecond, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m *matchModel) close() { m.cancel() }

// applyEvent appends to the log and tracks this round's kills for the map.
func (m *matchModel) applyEvent(e gotv.Event, now time.Time) {
	m.log = append([]gotv.Event{e}, m.log...)
	if len(m.log) > logLimit {
		m.log = m.log[:logLimit]
	}
	switch {
	case e.Kind == gotv.EvRoundStart:
		m.kills = nil
	case e.Kind == gotv.EvKill && e.To != nil:
		m.kills = append(m.kills, radar.KillMark{From: e.From, To: *e.To, Victim: e.Victim.Side, At: now})
	}
}

func (m *matchModel) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case stateMsg:
		s := gotv.State(msg)
		m.st = &s
		if (m.assets == nil || m.assets.Map != s.Map) && m.loading != s.Map && s.Map != "" {
			m.loading = s.Map
			return tea.Batch(m.wait(), loadRadar(s.Map))
		}
		return m.wait()
	case eventMsg:
		m.applyEvent(gotv.Event(msg), time.Now())
		return m.wait()
	case statusMsg:
		m.status = string(msg)
		return m.wait()
	case radarMsg:
		if msg.err != nil {
			m.status = "map: " + msg.err.Error()
			return nil
		}
		m.assets, m.levels = msg.a, nil
		// both levels use the upper radar's crop so they share scale and line up
		m.box = radar.OpaqueBox(msg.a.Upper)
	case tickMsg:
		return tick()
	}
	return nil
}

func loadRadar(name string) tea.Cmd {
	return func() tea.Msg {
		a, err := radar.Load(name)
		return radarMsg{a, err}
	}
}

func mmss(s float64) string {
	return fmt.Sprintf("%d:%02d", int(s)/60, int(s)%60)
}

// header puts the clocks first: when the line is cut to the column width only the
// team names and map lose characters, never a countdown.
func header(st *gotv.State) string {
	var clock string
	switch {
	case st.Over:
		clock = bold("match over")
	case st.SecondsLeft < 0:
		clock = bold("--:--")
	case st.Warmup:
		clock = bold(mmss(st.SecondsLeft)) + dim(" warmup")
	case st.Paused:
		clock = bold(mmss(st.SecondsLeft)) + dim(" paused")
	case st.Freeze:
		clock = bold(mmss(st.SecondsLeft)) + dim(" freeze")
	default:
		clock = bold(mmss(st.SecondsLeft))
	}
	bomb := ""
	if b := st.Bomb; b != nil && b.State != gotv.BombDropped && !st.Over {
		label := "BOMB"
		if b.SecondsLeft >= 0 {
			label += fmt.Sprintf(" %ds", int(math.Ceil(b.SecondsLeft)))
		}
		if b.State == gotv.BombDefusing {
			label += " · DEFUSING"
			if b.DefuseLeft >= 0 {
				label += fmt.Sprintf(" %.1fs", b.DefuseLeft)
			}
		}
		bomb = "  \x1b[41;1m " + label + " " + reset
	}
	return fmt.Sprintf(" %s%s  %s%s%s %d%s : %s%s%d %s%s  %s",
		clock, bomb,
		term.Fg(term.CT), boldOn, st.CT.Name, st.CT.Score, reset,
		term.Fg(term.T), boldOn, st.T.Score, st.T.Name, reset,
		dim(fmt.Sprintf("R%d %s", st.Round, st.Map)))
}

var frame = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("8"))

func (m *matchModel) mapView(width, height int, nums map[string]int) string {
	a := m.assets
	if a == nil {
		return dim(" loading map...")
	}
	want := []bool{false}
	if a.Lower != nil {
		want = []bool{false, true} // two-level maps sit side by side
	}
	levelW := width / len(want)
	titled := a.Lower != nil // single-level maps skip the title row (map name is in the header)
	titleRows := 0
	if titled {
		titleRows = 1
	}
	if len(m.levels) != len(want) || m.levels[0].w != levelW || m.levels[0].h != height {
		m.levels = nil
		for _, lower := range want {
			img := a.Upper
			if lower {
				img = a.Lower
			}
			r := radar.Rasterize(img, a.Overview, m.box, (levelW-2)*2, (height-2-titleRows)*4)
			m.levels = append(m.levels, &levelMap{lower: lower, w: levelW, h: height, r: r, base: radar.BuildBase(r)})
		}
	}
	var boxes []string
	for _, lv := range m.levels {
		lines := radar.Draw(lv.r, lv.base, m.st, radar.Opts{
			Kills: m.kills, Nums: nums, Now: time.Now(), Lower: lv.lower,
			LowerBelow: a.Overview.LowerBelow, Sites: radar.SitesOn(a.Map, lv.lower, a.Overview.Sites),
		})
		if titled {
			name := " upper"
			if lv.lower {
				name = " lower"
			}
			lines = append([]string{dim(name)}, lines...)
		}
		boxes = append(boxes, frame.Render(strings.Join(lines, "\n")))
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, boxes...)
}

// Layout: left = header + scoreboard, game log below (fixed width); right = minimap.
func (m *matchModel) view() string {
	leftW := min(leftMax, m.w/2) // scoreboard needs ~76 cols; the map gets the rest
	mapW := m.w - leftW - 1
	body := m.h - 1 // footer
	var left []string
	var nums map[string]int
	if m.st == nil {
		left = append(left, dim(" "+m.status+" · waiting for game state..."))
	} else {
		nums = PlayerNumbers(m.st.Players)
		left = append(left, header(m.st))
		left = append(left, TeamTable(gotv.CT, m.st.CT, m.st.Players, nums)...)
		left = append(left, TeamTable(gotv.T, m.st.T, m.st.Players, nums)...)
	}
	left = append(left, "", bold("Game log"))
	if len(m.log) == 0 {
		left = append(left, dim("events appear from the moment you connect"))
	}
	for i := 0; i < len(m.log) && len(left) < body; i++ {
		left = append(left, FormatEvent(m.log[i]))
	}
	for i := range left {
		left[i] = ansi.Truncate(left[i], leftW, "")
	}
	leftBlock := lipgloss.NewStyle().Width(leftW).Height(body).MaxHeight(body).Render(strings.Join(left, "\n"))
	mapBlock := lipgloss.Place(mapW, body, lipgloss.Center, lipgloss.Center, m.mapView(mapW, body, nums))
	footer := dim(ansi.Truncate(fmt.Sprintf(" q quit · b sources · %s · CSTV ~3min delay", m.status), m.w, ""))
	return lipgloss.JoinHorizontal(lipgloss.Top, leftBlock, " ", mapBlock) + "\n" + footer
}
