package ui

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/dormonbear/gotv-tui/internal/audio"
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

// delayNote reminds that a relay runs behind the live match (a demo doesn't).
func delayNote(url string) string {
	if strings.HasPrefix(url, DemoPrefix) {
		return ""
	}
	return " · CSTV ~3min delay"
}

// DemoPrefix marks a source that is a .dem file rather than a relay URL.
const DemoPrefix = "demo:"

type matchModel struct {
	url          string
	ctx          context.Context
	cancel       context.CancelFunc
	ch           chan tea.Msg
	st           *gotv.State
	log          []gotv.Event  // newest first
	rounds       []roundResult // results seen since connecting
	notice       string        // app-wide notice (update available) for the footer
	link         link          // connection state, for the footer badge
	help         bool          // the key help is open
	lastData     time.Time     // when the last snapshot arrived
	stats        bool          // scoreboard shows match stats instead of the live state
	directBefore bool          // the director had the camera when the current follow began
	sound        sound         // a stream's audio played along
	send         func(tea.Msg, bool)
	kills        []radar.KillMark
	blasts       []radar.Blast        // HE and flash pops this round
	floor        radar.Floor          // ground height learnt from where players walk
	labelMem     [2]radar.LabelMemory // per level: steady name labels across frames
	status       string
	assets       *radar.Assets
	box          radar.Box // the map's own extent
	frame        radar.Box // box grown to the shape of the map area: what zoom 1 shows
	levels       []*levelMap
	loading      string    // map being loaded
	retryAt      time.Time // after a failed map download, when to try again
	noRadar      string    // a map no radar source has: not asked for again
	aspect       float64   // terminal cell height / width
	zoom         float64   // camera: 1 = whole map
	zoomTo       float64   // zoom animates towards this
	anchor       anchor    // radar point held still while zooming
	moving       bool      // zoom animation running
	camLevel     int       // the level the camera drives (0 upper, 1 lower); the other stays parked
	parked       [2]parked // each level's view while the camera drives the other
	drag         drag
	// director: picks the shot; the camera follows it while directing
	dir            *director.Director
	directing      bool
	shot           *director.Shot
	soon           []playback.Soon[gotv.Event] // events queued ahead of the screen
	settling       bool                        // camera still moving to the last shot
	vel            [3]float64                  // camera spring velocity: x, y, log zoom
	switchX        int                         // screen cell where the director switch starts
	switchY        int
	panelW         int            // width of the scoreboard (clicks there select players)
	switchW        int            // its width; it sits on screen row 0
	badgeX, badgeW int            // the audio badge on the switch row (a click mutes)
	focus          string         // followed player (lowercase), "" for none
	rowPlayer      map[int]string // scoreboard screen row -> player (lowercase)
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
	hs := gotv.Handlers{
		State:  func(s gotv.State) { send(stateMsg(s), true) },
		Event:  func(e gotv.Event) { send(eventMsg(e), false) },
		Status: func(s string) { send(statusMsg(s), false) },
		Ahead: func(soon []playback.Soon[gotv.Event], _ *gotv.State) {
			send(aheadMsg(soon), true)
		},
		Delay: m.sound.delay,
	}
	m.send = send
	if src, ok := strings.CutPrefix(url, DemoPrefix); ok {
		path, speed := src, 1.0
		if i := strings.LastIndexByte(src, '@'); i > 0 { // demo:<path>@<speed>
			if v, err := strconv.ParseFloat(src[i+1:], 64); err == nil {
				path, speed = src[:i], v
			}
		}
		go func() {
			hs.Status(fmt.Sprintf("playing demo at %gx", speed))
			if err := gotv.PlayDemo(ctx, path, speed, hs); err != nil && ctx.Err() == nil {
				hs.Status("demo: " + err.Error())
				return
			}
			hs.Status("demo over")
		}()
	} else {
		go gotv.Connect(ctx, url, hs)
	}
	// the director runs by default; any manual camera move takes over
	m.directing, m.dir, m.moving = true, director.New(), true
	m.sound.auto.on = true
	return m, tea.Batch(m.wait(), tick(), animTick())
}

// wait delivers the next relay message; it gives up when the match is closed, so
// leaving a quiet relay doesn't strand the command.
func (m *matchModel) wait() tea.Cmd {
	ch, ctx := m.ch, m.ctx
	return func() tea.Msg {
		select {
		case msg := <-ch:
			return matchMsg{m, msg}
		case <-ctx.Done():
			return nil
		}
	}
}

func tick() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg { return tickMsg(t) }) // drives the waiting animation and the link badge
}

func (m *matchModel) close() {
	m.cancel()
	m.sound.stop()
}

// applyEvent appends to the log and tracks this round's kills for the map.
func (m *matchModel) applyEvent(e gotv.Event, now time.Time) {
	m.fx.onEvent(e, m.st, now)
	if e.Kind == gotv.EvBlast {
		if e.To != nil {
			m.blasts = append(m.blasts, radar.Blast{Nade: e.Nade, Pos: *e.To, At: now})
		}
		return
	}
	m.log = append([]gotv.Event{e}, m.log...)
	if len(m.log) > logLimit {
		m.log = m.log[:logLimit]
	}
	switch {
	case e.Kind == gotv.EvRoundStart:
		m.kills, m.blasts = nil, nil
	case e.Kind == gotv.EvRoundEnd:
		m.recordRound(e.Round, e)
	case e.Kind == gotv.EvKill && e.To != nil:
		m.kills = append(m.kills, radar.KillMark{From: e.From, To: *e.To, Victim: e.Victim.Side, Headshot: e.Headshot, At: now})
	}
}

func (m *matchModel) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case stateMsg:
		m.lastData = time.Now()
		s := gotv.State(msg)
		if m.st != nil && (s.Map != m.st.Map || s.Round < m.st.Round-1) {
			m.rounds = nil // another match
		}
		if m.floor == nil || (m.st != nil && s.Map != m.st.Map) {
			m.floor = radar.KnownFloor(s.Map)
		}
		m.floor.Observe(s.Players)
		m.st = &s
		m.recordClock(m.st, time.Now())
		m.fx.onState(m.st, time.Now())
		m.direct()
		if (m.assets == nil || m.assets.Map != s.Map) && m.loading != s.Map && s.Map != "" && s.Map != m.noRadar && time.Now().After(m.retryAt) {
			m.loading = s.Map
			return tea.Batch(m.wait(), m.loadRadar(s.Map))
		}
		return m.wait()
	case eventMsg:
		m.applyEvent(gotv.Event(msg), time.Now())
		return m.wait()
	case clockReadMsg:
		m.onClockRead(audio.ClockRead(msg))
		return m.wait()
	case soundStatusMsg:
		m.status = term.Clean(string(msg), 120)
		if p := m.sound.pending; string(msg) == "audio: playing" && p[1] != "" { // it plays: worth keeping
			addRoom(p[0], p[1])
			m.sound.pending = [2]string{}
		}
		return m.wait()
	case statusMsg:
		m.status = term.Clean(string(msg), 120) // may carry relay-side error text
		m.link = linkOf(m.status)
		return m.wait()
	case radarMsg:
		if msg.name != m.loading || (m.st != nil && m.st.Map != msg.name) {
			return nil // asked for by an earlier match or map
		}
		if errors.Is(msg.err, radar.ErrNotFound) {
			m.loading, m.noRadar = "", msg.name
			return nil
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
		if m.sound.typing() {
			return m.typeSource(msg)
		}
		if k := msg.String(); k == "?" || m.help { // ? toggles the key help; any key closes it
			m.help = k == "?" && !m.help
			return nil
		}
		if cmd, ok := m.soundKey(msg.String()); ok {
			return cmd
		}
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

func (m *matchModel) loadRadar(name string) tea.Cmd {
	return func() tea.Msg {
		a, err := radar.Load(name)
		return matchMsg{m, radarMsg{name, a, err}}
	}
}

// matchMsg is a message from one match's background work; the app drops it when
// that match is no longer the open one.
type matchMsg struct {
	m   *matchModel
	msg tea.Msg
}

func mmss(s float64) string {
	return fmt.Sprintf("%d:%02d", int(s)/60, int(s)%60)
}

// roundStatus is the round clock, phase and bomb timers, shown above the map.
func roundStatus(st *gotv.State) string {
	var clock string
	switch {
	case st.Over:
		return bold("MATCH OVER")
	case st.Paused: // a yellow badge, with the team's timeout counting down
		label := st.Pause
		if st.PauseLeft >= 0 {
			label += " " + mmss(math.Ceil(st.PauseLeft))
		}
		clock = bold(mmss(max(0, st.SecondsLeft))) + "  \x1b[43;30;1m " + label + " " + reset
	case st.SecondsLeft < 0:
		clock = bold("--:--")
	case st.Warmup:
		clock = bold(mmss(st.SecondsLeft)) + dim(" WARMUP")
	case st.Freeze:
		clock = bold(mmss(st.SecondsLeft)) + dim(" FREEZE")
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
// Team names fall back to initials when the line doesn't fit w cells.
func header(st *gotv.State, w int) string {
	line := func(ct, t string) string {
		return fmt.Sprintf(" %s%s%s %d%s : %s%s%d %s%s  %s",
			term.Fg(term.CT), boldOn, ct, st.CT.Score, reset,
			term.Fg(term.T), boldOn, st.T.Score, t, reset,
			dim(feeds.MapName(st.Map)))
	}
	if s := line(st.CT.Name, st.T.Name); ansi.StringWidth(s) <= w {
		return s
	}
	return line(abbrev(st.CT.Name, 12), abbrev(st.T.Name, 12))
}

var frame = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("8"))

func (m *matchModel) mapView(width, height int, nums map[string]int) string {
	a := m.assets
	if a == nil {
		if m.st != nil && m.st.Map == m.noRadar {
			return dim(" no radar for " + feeds.MapName(m.noRadar) + " yet · scoreboard and log still update")
		}
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
	views := make([]radar.Box, len(want))
	rebuild := len(m.levels) != len(want) || m.levels[0].w != levelW || m.levels[0].h != levelH || m.levels[0].r.Aspect != m.aspect
	for i := range want {
		views[i] = m.levelView(i)
		rebuild = rebuild || m.levels[i].view != views[i]
	}
	if rebuild {
		m.levels = nil
		for i, lower := range want {
			view := views[i]
			img := a.Upper
			if lower {
				img = a.Lower
			}
			cw, ch := (levelW-2)*2, (levelH-2-titleRows)*4
			lv := &levelMap{lower: lower, w: levelW, h: levelH, view: view}
			if m.images() || m.wholeFrames() { // the terminal draws the map: skip the text one (~17 ms a frame while the camera moves)
				lv.r = radar.Mapping(a.Overview, view, cw, ch, m.aspect)
				lv.base = radar.Size(lv.r)
			} else {
				lv.r = radar.Rasterize(img, a.Overview, view, cw, ch, m.aspect)
				lv.base = radar.BuildBase(lv.r, m.levelZoom(i))
			}
			m.levels = append(m.levels, lv)
		}
	}
	var boxes []string
	for i, lv := range m.levels {
		lv.opts = radar.Opts{
			Kills: m.kills, Nums: nums, Now: time.Now(), Lower: lv.lower,
			LowerBelow: a.Overview.LowerBelow, Sites: radar.SitesOn(a.Map, lv.lower, a.Overview.Sites),
			ImageUnder: m.images(),
			Names:      m.levelZoom(i) >= namesFrom,
			Focus:      m.focus,
			Boom:       m.fx.boom(time.Now()),
			Blasts:     m.blasts,
			Floor:      m.floor,
			Labels:     m.labelMemory(lv.lower),
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
			name := " UPPER"
			if lv.lower {
				name = " LOWER"
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
		return append([]string{m.withBadge("")}, waiting(time.Now(), m.status)...)
	}
	lines := []string{m.withBadge(header(m.st, m.panelW-ansi.StringWidth(linkBadge(m.link, m.lastData, time.Now()))))}
	if h := historyLine(m.st.History, m.rounds, m.st.Round, m.panelW); h != "" {
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
		lines = append(lines, TeamTable(side.s, side.t, m.st.Players, nums, m.focus, compact, m.stats)...)
		lines = append(lines, econLine(side.s, m.st.Players, m.st.Round))
	}
	return lines
}

// logLines is the game log title and up to n-1 newest events.
func (m *matchModel) logLines(n int) []string {
	if n <= 0 { // no room left under the scoreboard
		return nil
	}
	lines := []string{bold(" GAME LOG")}
	if len(m.log) == 0 {
		lines = append(lines, dim(" events appear from the moment you connect"))
	}
	for i := 0; i < len(m.log) && len(lines) < n; i++ {
		lines = append(lines, " "+FormatEvent(m.log[i]))
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
	if m.help { // the help covers the screen: no map images over it
		m.hideImages()
		screen = lipgloss.Place(m.w, m.h-1, lipgloss.Center, lipgloss.Center, helpBox())
	} else {
		if m.gfx == term.Kitty {
			m.placeImages()
		}
		m.sendFrames()
	}
	zoom := ""
	if m.zoom > 1.01 {
		zoom = fmt.Sprintf(" %.1fx ·", m.zoom)
	}
	footer := fmt.Sprintf("%s ? keys · q quit · b back · s stats · d director ·%s %s%s", m.soundNote(), zoom, m.status, delayNote(m.url))
	if m.notice != "" { // first, so a narrow footer keeps it
		footer = " " + m.notice + " ·" + footer
	}
	footer = dim(marquee(footer, m.w, time.Now()))
	if m.sound.typing() {
		footer = m.soundFooter()
	}
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
	if b := m.soundBadge(); b != "" {
		note = "  " + b
	}
	pre := ""
	if m.st != nil {
		pre = " " + roundStatus(m.st) + "   "
		m.switchX += lipgloss.Width(pre)
	}
	if m.focus != "" {
		sw = switchFocus.Render(" ◎ FOLLOWING " + m.focusName() + " ")
		m.switchW = 0 // following, not the director: the bar isn't a switch now
		m.badgeX = m.switchX + lipgloss.Width(sw) + 2
		clock, sync := m.streamClock()
		return fitTail(pre+sw+note+clock, sync, width)
	}
	if m.directing {
		sw = switchOn.Render(" ● DIRECTOR ON ")
	} else {
		sw = switchOff.Render(" ○ DIRECTOR OFF ")
	}
	m.switchW = lipgloss.Width(sw)
	m.badgeX = m.switchX + m.switchW + 2
	clock, sync := m.streamClock()
	return fitTail(pre+sw+note+clock, sync, width)
}

var waitQuips = []string{
	"warming up", "buying utility", "checking crosshairs", "tactical timeout",
	"waiting for the coach", "fixing a stuck peripheral", "eco round for the casters",
}

// waiting animates the empty match screen: a T carrying the bomb runs up and
// down a lane, under a rotating line of quips; steps every 250 ms.
func waiting(now time.Time, status string) []string {
	const lane = 28
	f := int(now.UnixMilli() / 250)
	pos := f % (2 * lane)
	runner := "▶✱"
	if pos >= lane {
		pos, runner = 2*lane-1-pos, "✱◀"
	}
	track := strings.Repeat("·", pos) + runner + strings.Repeat("·", lane-pos)
	return []string{
		"",
		" " + lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Render(track),
		dim(" " + waitQuips[f/12%len(waitQuips)] + strings.Repeat(".", f%4)),
		dim(" " + status),
	}
}

// labelMemory is the label memory of the upper (false) or lower level map.
func (m *matchModel) labelMemory(lower bool) radar.LabelMemory {
	i := 0
	if lower {
		i = 1
	}
	if m.labelMem[i] == nil {
		m.labelMem[i] = radar.LabelMemory{}
	}
	return m.labelMem[i]
}

// withBadge puts the connection badge at the right end of a panel row.
func (m *matchModel) withBadge(row string) string {
	badge := linkBadge(m.link, m.lastData, time.Now())
	room := m.panelW - ansi.StringWidth(badge)
	if badge == "" || room < 1 {
		return row
	}
	row = ansi.Truncate(row, room, "")
	return row + strings.Repeat(" ", room-ansi.StringWidth(row)) + badge
}

// fitTail keeps head fixed (it holds click targets) and scrolls tail in the rest
// of width when it doesn't fit.
func fitTail(head, tail string, width int) string {
	room := width - ansi.StringWidth(head)
	if room <= 0 {
		return ansi.Truncate(head, width, "")
	}
	return head + marquee(tail, room, time.Now())
}
