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

	"github.com/dormonbear/gotv-tui/internal/director"
	"github.com/dormonbear/gotv-tui/internal/feeds"
	"github.com/dormonbear/gotv-tui/internal/gotv"
	"github.com/dormonbear/gotv-tui/internal/playback"
	"github.com/dormonbear/gotv-tui/internal/radar"
	"github.com/dormonbear/gotv-tui/internal/term"
)

const (
	logLimit = 200
	leftMax  = 80
)

type (
	stateMsg  gotv.State
	aheadMsg  []playback.Soon[gotv.Event]
	eventMsg  gotv.Event
	statusMsg string
	radarMsg  struct {
		name string // which map was asked for: a late answer for another map is dropped
		a    *radar.Assets
		err  error
	}
	tickMsg time.Time
)

// levelMap caches one framed radar level for a size.
type levelMap struct {
	lower  bool
	w, h   int
	view   radar.Box
	opts   radar.Opts // markers to draw this frame
	ox, oy int        // raster's top-left cell inside the map block
	sx, sy int        // ... and on screen, for mouse hits
	r      *radar.Raster
	base   *radar.Base
}

type matchModel struct {
	url     string
	ctx     context.Context
	cancel  context.CancelFunc
	ch      chan tea.Msg
	st      *gotv.State
	log     []gotv.Event  // newest first
	rounds  []roundResult // results seen since connecting
	notice  string        // app-wide notice (update available) for the footer
	kills   []radar.KillMark
	status  string
	assets  *radar.Assets
	box     radar.Box // the map's own extent
	frame   radar.Box // box grown to the shape of the map area: what zoom 1 shows
	levels  []*levelMap
	loading string    // map being loaded
	retryAt time.Time // after a failed map download, when to try again
	aspect  float64   // terminal cell height / width
	zoom    float64   // camera: 1 = whole map
	zoomTo  float64   // zoom animates towards this
	anchor  anchor    // radar point held still while zooming
	moving  bool      // zoom animation running
	drag    drag
	// director: picks the shot; the camera follows it while directing
	dir       *director.Director
	directing bool
	shot      *director.Shot
	soon      []playback.Soon[gotv.Event] // events queued ahead of the screen
	settling  bool                        // camera still moving to the last shot
	vel       [3]float64                  // camera spring velocity: x, y, log zoom
	switchX   int                         // screen cell where the director switch starts
	switchY   int
	panelW    int            // width of the scoreboard (clicks there select players)
	switchW   int            // its width; it sits on screen row 0
	focus     string         // followed player (lowercase), "" for none
	rowPlayer map[int]string // scoreboard screen row -> player (lowercase)
	// kitty graphics: images go to out (nil: draw the map in half blocks)
	out          *term.LockedFile // image output (nil: text map)
	gfx          term.Graphics
	lastFrame    time.Time
	frameSprites map[string]radar.Sprite
	uploaded     string // map whose radar images the terminal holds
	sprites      spriteLayer
	fx           fx
	camX         float64 // camera centre, 1024 radar space
	camY         float64
	w, h         int
}

func newMatch(url string, w, h int) (*matchModel, tea.Cmd) {
	ctx, cancel := context.WithCancel(context.Background())
	m := &matchModel{url: url, ctx: ctx, cancel: cancel, ch: make(chan tea.Msg, 64), status: "connecting...", w: w, h: h, aspect: term.CellAspect()}
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
		Ahead: func(soon []playback.Soon[gotv.Event], _ *gotv.State) {
			send(aheadMsg(soon), true)
		},
	})
	// the director runs by default; any manual camera move takes over
	m.directing, m.dir, m.moving = true, director.New(), true
	return m, tea.Batch(m.wait(), tick(), animTick())
}

// wait delivers the next relay message; it gives up when the match is closed, so
// leaving a quiet relay doesn't strand the command.
func (m *matchModel) wait() tea.Cmd {
	ch, ctx := m.ch, m.ctx
	return func() tea.Msg {
		select {
		case msg := <-ch:
			return msg
		case <-ctx.Done():
			return nil
		}
	}
}

func tick() tea.Cmd {
	return tea.Tick(250*time.Millisecond, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m *matchModel) close() { m.cancel() }

// applyEvent appends to the log and tracks this round's kills for the map.
func (m *matchModel) applyEvent(e gotv.Event, now time.Time) {
	m.fx.onEvent(e, m.st, now)
	m.log = append([]gotv.Event{e}, m.log...)
	if len(m.log) > logLimit {
		m.log = m.log[:logLimit]
	}
	switch {
	case e.Kind == gotv.EvRoundStart:
		m.kills = nil
	case e.Kind == gotv.EvRoundEnd && m.st != nil:
		m.recordRound(m.st.Round, e) // the state still shows the round that just ended
	case e.Kind == gotv.EvKill && e.To != nil:
		m.kills = append(m.kills, radar.KillMark{From: e.From, To: *e.To, Victim: e.Victim.Side, Headshot: e.Headshot, At: now})
	}
}

func (m *matchModel) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case stateMsg:
		s := gotv.State(msg)
		if m.st != nil && (s.Map != m.st.Map || s.Round < m.st.Round-1) {
			m.rounds = nil // another match
		}
		m.st = &s
		m.fx.onState(m.st, time.Now())
		m.direct()
		if (m.assets == nil || m.assets.Map != s.Map) && m.loading != s.Map && s.Map != "" && time.Now().After(m.retryAt) {
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
		if msg.name != m.loading {
			return nil // asked for by an earlier match or map
		}
		if msg.err != nil {
			m.status = "map: " + msg.err.Error()
			m.loading, m.retryAt = "", time.Now().Add(10*time.Second)
			return nil
		}
		m.assets, m.levels = msg.a, nil
		// both levels use the upper radar's crop so they share scale and line up
		m.box = radar.OpaqueBox(msg.a.Upper)
		m.frame = m.box
		m.resetCamera()
	case tickMsg:
		return tick()
	case tea.KeyMsg:
		return m.cameraKey(msg.String())
	case tea.MouseMsg:
		return m.mouse(msg)
	case aheadMsg:
		m.soon = msg
		return m.wait()
	case animMsg:
		if m.focus != "" {
			return m.followPlayer()
		}
		if m.directing {
			return m.follow()
		}
		return m.step()
	}
	return nil
}

func loadRadar(name string) tea.Cmd {
	return func() tea.Msg {
		a, err := radar.Load(name)
		return radarMsg{name, a, err}
	}
}

func mmss(s float64) string {
	return fmt.Sprintf("%d:%02d", int(s)/60, int(s)%60)
}

// roundStatus is the round clock, phase and bomb timers, shown above the map.
func roundStatus(st *gotv.State) string {
	var clock string
	switch {
	case st.Over:
		return bold("match over")
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
	clock = dim(fmt.Sprintf("R%d ", st.Round)) + clock
	if b := st.Bomb; b != nil && b.State != gotv.BombDropped {
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
		clock += "  \x1b[41;1m " + label + " " + reset
	}
	return clock
}

// header is the score line over the scoreboard; the clock lives above the map.
func header(st *gotv.State) string {
	return fmt.Sprintf(" %s%s%s %d%s : %s%s%d %s%s  %s",
		term.Fg(term.CT), boldOn, st.CT.Name, st.CT.Score, reset,
		term.Fg(term.T), boldOn, st.T.Score, st.T.Name, reset,
		dim(feeds.MapName(st.Map)))
}

var frame = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("8"))

func (m *matchModel) mapView(width, height int, nums map[string]int) string {
	a := m.assets
	if a == nil {
		return dim(" loading map...")
	}
	want := []bool{false}
	titleRows := 0 // single-level maps skip the title row (map name is in the header)
	if a.Lower != nil {
		want, titleRows = []bool{false, true}, 1
	}
	// a framed level takes 2 border cols/rows plus its title row
	fit := func(w, h int) (int, int, float64) {
		return w, h, radar.FitScale(m.box, (w-2)*2, (h-2-titleRows)*4, m.aspect)
	}
	levelW, levelH, _ := fit(width, height)
	stacked := false
	if len(want) == 2 { // two levels side by side or stacked, whichever draws bigger
		w1, h1, side := fit(width/2, height)
		w2, h2, stack := fit(width, height/2)
		if stacked = stack > side; stacked {
			levelW, levelH = w2, h2
		} else {
			levelW, levelH = w1, h1
		}
	}
	// zoom 1 shows the map grown to the area's shape: the view fills the frame
	cols, rows := levelW-2, levelH-2-titleRows
	m.frame = m.box.Fill(float64(cols) / (float64(rows) * m.aspect))
	view := m.camera()
	if len(m.levels) != len(want) || m.levels[0].w != levelW || m.levels[0].h != levelH || m.levels[0].r.Aspect != m.aspect || m.levels[0].view != view {
		m.levels = nil
		for _, lower := range want {
			img := a.Upper
			if lower {
				img = a.Lower
			}
			r := radar.Rasterize(img, a.Overview, view, (levelW-2)*2, (levelH-2-titleRows)*4, m.aspect)
			m.levels = append(m.levels, &levelMap{lower: lower, w: levelW, h: levelH, view: view, r: r, base: radar.BuildBase(r, m.zoom)})
		}
	}
	var boxes []string
	for i, lv := range m.levels {
		lv.opts = radar.Opts{
			Kills: m.kills, Nums: nums, Now: time.Now(), Lower: lv.lower,
			LowerBelow: a.Overview.LowerBelow, Sites: radar.SitesOn(a.Map, lv.lower, a.Overview.Sites),
			ImageUnder: m.images(),
			Names:      m.zoom >= namesFrom,
			Focus:      m.focus,
			Boom:       m.fx.boom(time.Now()),
		}
		var lines []string
		if m.images() || m.wholeFrames() { // map and markers are terminal images: the text is blank
			for y := 0; y < lv.base.CH; y++ {
				lines = append(lines, strings.Repeat(" ", lv.base.CW))
			}
		} else {
			lines = radar.Draw(lv.r, lv.base, m.st, lv.opts)
		}
		if titleRows > 0 {
			name := " upper"
			if lv.lower {
				name = " lower"
			}
			lines = append([]string{dim(name)}, lines...)
		}
		box := frame.Render(strings.Join(lines, "\n"))
		lv.ox, lv.oy = 1, 1+titleRows // inside the border and title
		if stacked {
			lv.oy += i * lipgloss.Height(box)
		} else {
			lv.ox += i * lipgloss.Width(box)
		}
		boxes = append(boxes, box)
	}
	if stacked {
		return lipgloss.JoinVertical(lipgloss.Center, boxes...)
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, boxes...)
}

const (
	wideMin    = leftMax + 40 // narrower than this: scoreboard, map and log stacked
	compactMax = 76           // narrower than this: compact scoreboard
	logMin     = 3            // log rows kept under a stacked map
)

// panel is the header and both team tables; it also records which screen row
// shows which player (it always starts at row 0).
func (m *matchModel) panel(nums map[string]int, compact bool) []string {
	if m.st == nil {
		return []string{dim(" " + m.status + " · waiting for game state...")}
	}
	lines := []string{header(m.st)}
	if h := historyLine(m.rounds, m.st.Round, m.panelW); h != "" {
		lines = append(lines, h)
	}
	m.rowPlayer = map[int]string{}
	for _, side := range []struct {
		s gotv.Side
		t gotv.Team
	}{{gotv.CT, m.st.CT}, {gotv.T, m.st.T}} {
		top := len(lines) + 1 // below the team's title row
		for i, p := range teamRows(side.s, m.st.Players, nums) {
			m.rowPlayer[top+i] = strings.ToLower(p.Name)
		}
		lines = append(lines, TeamTable(side.s, side.t, m.st.Players, nums, m.focus, compact)...)
		lines = append(lines, econLine(side.s, m.st.Players, m.st.Round))
	}
	return lines
}

// logLines is the game log title and up to n-1 newest events.
func (m *matchModel) logLines(n int) []string {
	lines := []string{bold("Game log")}
	if len(m.log) == 0 {
		lines = append(lines, dim("events appear from the moment you connect"))
	}
	for i := 0; i < len(m.log) && len(lines) < n; i++ {
		lines = append(lines, FormatEvent(m.log[i]))
	}
	return lines
}

func block(lines []string, w, h int) string {
	for i := range lines {
		lines[i] = ansi.Truncate(lines[i], w, "")
	}
	return lipgloss.NewStyle().Width(w).Height(h).MaxHeight(h).Render(strings.Join(lines, "\n"))
}

// view lays the screen out side by side (scoreboard + log | map) when it is wide
// enough, else stacked (scoreboard / map / log), e.g. on a phone.
func (m *matchModel) view() string {
	body := m.h - 1 // footer
	var nums map[string]int
	if m.st != nil {
		nums = PlayerNumbers(m.st.Players)
	}
	var screen string
	var mapX, mapY int // screen cell of the map block (under the director switch)
	if m.w >= wideMin {
		leftW := min(leftMax, m.w/2) // scoreboard needs ~76 cols; the map gets the rest
		mapW, mapH := m.w-leftW-1, body-1
		left := m.panel(nums, false)
		left = append(left, "")
		left = append(left, m.logLines(body-len(left))...)
		m.panelW, m.switchX, m.switchY = leftW, leftW+1, 0
		mapX, mapY = leftW+1, 1
		toggle := m.directorSwitch(mapW)
		mapStr := m.fx.overlay(m.mapView(mapW, mapH, nums), time.Now())
		right := toggle + "\n" + lipgloss.Place(mapW, mapH, lipgloss.Left, lipgloss.Top, mapStr)
		screen = lipgloss.JoinHorizontal(lipgloss.Top, block(left, leftW, body), " ", right)
	} else {
		top := m.panel(nums, m.w < compactMax)
		logN := max(logMin, (body-len(top))/5)
		mapH := max(4, body-len(top)-1-logN)
		m.panelW, m.switchX, m.switchY = m.w, 0, len(top)
		mapX, mapY = 0, len(top)+1
		toggle := m.directorSwitch(m.w)
		mapStr := m.fx.overlay(m.mapView(m.w, mapH, nums), time.Now())
		screen = strings.Join([]string{
			block(top, m.w, len(top)),
			toggle,
			lipgloss.Place(m.w, mapH, lipgloss.Left, lipgloss.Top, mapStr),
			block(m.logLines(body-len(top)-1-mapH), m.w, body-len(top)-1-mapH),
		}, "\n")
	}
	for _, lv := range m.levels {
		lv.sx, lv.sy = mapX+lv.ox, mapY+lv.oy
	}
	if m.gfx == term.Kitty {
		m.placeImages()
	}
	m.sendFrames()
	zoom := ""
	if m.zoom > 1.01 {
		zoom = fmt.Sprintf(" %.1fx ·", m.zoom)
	}
	footer := fmt.Sprintf(" q quit · b/esc sources · d director · wheel/+- zoom · drag/←↑↓→ pan · 0 reset ·%s %s · CSTV ~3min delay", zoom, m.status)
	if m.notice != "" { // first, so a narrow footer keeps it
		footer = " " + m.notice + " ·" + footer
	}
	footer = dim(ansi.Truncate(footer, m.w, ""))
	return screen + "\n" + footer
}

var (
	switchOn    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("0")).Background(lipgloss.Color("2"))
	switchOff   = lipgloss.NewStyle().Foreground(lipgloss.Color("7")).Background(lipgloss.Color("8"))
	switchFocus = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("0")).Background(lipgloss.Color("15"))
)

// focusName is the followed player's name as shown in game.
func (m *matchModel) focusName() string {
	if m.st != nil {
		for _, p := range m.st.Players {
			if strings.ToLower(p.Name) == m.focus {
				return p.Name
			}
		}
	}
	return m.focus
}

// directorSwitch is the director toggle above the map: what it shows when on, a
// hint when off. Clicking it toggles too.
func (m *matchModel) directorSwitch(width int) string {
	var sw, note string
	pre := ""
	if m.st != nil {
		pre = " " + roundStatus(m.st) + "   "
		m.switchX += lipgloss.Width(pre)
	}
	if m.focus != "" {
		sw = switchFocus.Render(" ◎ FOLLOWING " + m.focusName() + " ")
		note = dim(" click them again or elsewhere to stop")
		m.switchW = 0 // following, not the director: the bar isn't a switch now
		return ansi.Truncate(pre+sw+note, width, "")
	}
	if m.directing {
		sw = switchOn.Render(" ● DIRECTOR ON ")
		note = " whole map"
		if m.shot != nil {
			note = " " + m.shot.Reason
		}
	} else {
		sw = switchOff.Render(" ○ DIRECTOR OFF ")
		note = dim(" press d or click")
	}
	m.switchW = lipgloss.Width(sw)
	return ansi.Truncate(pre+sw+note, width, "")
}
