package ui

import (
	"math"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/dormonbear/gotv-tui/internal/director"
	"github.com/dormonbear/gotv-tui/internal/radar"
	"github.com/dormonbear/gotv-tui/internal/term"
)

const (
	keyZoom   = 1.25
	wheelZoom = 1.12 // per wheel notch; trackpads send many notches per swipe
	panStep   = 0.2  // of the visible width/height
	easing    = 0.3  // share of the remaining zoom (in log scale) covered per frame
	frameTime = 16 * time.Millisecond
	namesFrom = 1.5 // zoom from which players are labelled by name where it fits
)

// anchor is a radar point (ax, ay) kept at the same place on screen, given as a
// fraction (fx, fy) of the view, while the zoom animates.
type anchor struct{ ax, ay, fx, fy float64 }

type animMsg struct{}

func animTick() tea.Cmd { return tea.Tick(frameTime, func(time.Time) tea.Msg { return animMsg{} }) }

func (m *matchModel) resetCamera() {
	m.zoom, m.zoomTo, m.moving = 1, 1, false
	m.camX, m.camY = m.frame.Center()
}

// camera is the radar area on screen. Both levels share it so they stay aligned.
func (m *matchModel) camera() radar.Box {
	v := m.frame.View(m.camX, m.camY, m.zoom)
	m.camX, m.camY = v.Center() // clamped at the map edge: panning back starts right away
	return v
}

// zoomAround sets a new zoom target, holding radar point (ax, ay) still on screen.
func (m *matchModel) zoomAround(factor, ax, ay float64) tea.Cmd {
	v := m.camera()
	m.zoomTo = min(radar.MaxZoom, max(1, m.zoomTo*factor))
	m.anchor = anchor{ax, ay, (ax - v.X) / v.W, (ay - v.Y) / v.H}
	if m.moving {
		return nil
	}
	m.moving = true
	return animTick()
}

// step eases the zoom towards its target and keeps the anchor in place.
func (m *matchModel) step() tea.Cmd {
	z := m.zoom * math.Pow(m.zoomTo/m.zoom, easing)
	if math.Abs(math.Log(m.zoomTo/z)) < 0.004 {
		z = m.zoomTo
	}
	m.zoom = z
	a := m.anchor
	w, h := m.frame.W/z, m.frame.H/z
	m.camX, m.camY = a.ax-a.fx*w+w/2, a.ay-a.fy*h+h/2
	if z == m.zoomTo {
		m.moving = false
		return nil
	}
	return animTick()
}

// panCells moves the view by (dx, dy) screen cells, the other way: the map follows
// the pointer. It stops a running zoom so the two don't fight.
func (m *matchModel) panCells(dx, dy float64) {
	if len(m.levels) == 0 {
		return
	}
	r := m.levels[0].r
	x0, y0 := r.RadarAt(0, 0)
	x1, y1 := r.RadarAt(2, 4) // one cell
	m.zoomTo, m.moving = m.zoom, false
	m.camX -= dx * (x1 - x0)
	m.camY -= dy * (y1 - y0)
}

// radarAt is the radar point under screen position (x, y) in cells, if it is on a
// map. Whole numbers are a cell's top-left, so callers with cell-only mouse
// positions add 0.5 to aim at the middle.
func (m *matchModel) radarAt(x, y float64) (float64, float64, bool) {
	for _, lv := range m.levels {
		cx, cy := x-float64(lv.sx), y-float64(lv.sy)
		if cx >= 0 && cy >= 0 && cx < float64(lv.base.CW) && cy < float64(lv.base.CH) {
			ax, ay := lv.r.RadarAt(cx*2, cy*4) // 2x4 braille dots per cell
			return ax, ay, true
		}
	}
	return 0, 0, false
}

// pointer is the mouse position in cells: exact when the terminal reports pixels
// (SGR-pixel mode, on with images), else the middle of the reported cell.
func (m *matchModel) pointer(e tea.MouseMsg) (float64, float64) {
	if m.gfx == term.Kitty && m.out != nil { // pixel mouse reports are on with kitty graphics
		cw, ch := term.CellPixels()
		return float64(e.X) / float64(cw), float64(e.Y) / float64(ch)
	}
	return float64(e.X) + 0.5, float64(e.Y) + 0.5
}

func (m *matchModel) cameraKey(k string) tea.Cmd {
	if k == "s" { // scoreboard: live state <-> match stats
		m.stats = !m.stats
		return nil
	}
	if m.assets == nil {
		return nil
	}
	if k == "d" {
		return m.toggleDirector()
	}
	if m.focus != "" {
		switch k {
		case "+", "=":
			m.zoomTo = min(radar.MaxZoom, m.zoomTo*keyZoom)
			return nil
		case "-", "_":
			m.zoomTo = max(1, m.zoomTo/keyZoom)
			return nil
		}
	}
	m.directing, m.focus = false, "" // any manual move takes the camera back
	v := m.camera()
	cx, cy := v.Center()
	switch k {
	case "+", "=":
		return m.zoomAround(keyZoom, cx, cy)
	case "-", "_":
		return m.zoomAround(1/keyZoom, cx, cy)
	case "0":
		return m.zoomAround(1/m.zoomTo, cx, cy)
	case "left", "h":
		m.camX -= v.W * panStep
	case "right", "l":
		m.camX += v.W * panStep
	case "up", "k":
		m.camY -= v.H * panStep
	case "down", "j":
		m.camY += v.H * panStep
	}
	return nil
}

// drag pans so the radar point grabbed stays under the pointer.
type drag struct {
	on   bool
	x, y float64 // last pointer position, in cells
}

func (m *matchModel) mouse(e tea.MouseMsg) tea.Cmd {
	if m.assets == nil {
		return nil
	}
	if e.Action == tea.MouseActionPress && e.Button == tea.MouseButtonLeft {
		x, y := m.pointer(e)
		if int(y) == m.switchY && int(x) >= m.switchX && int(x) < m.switchX+m.switchW {
			return m.toggleDirector()
		}
		if name := m.rowPlayer[int(y)]; name != "" && int(x) < m.panelW { // scoreboard row
			return m.focusOn(name)
		}
		if name := m.playerAt(x, y); name != "" {
			return m.focusOn(name)
		}
	}
	if m.focus != "" && (e.Button == tea.MouseButtonWheelUp || e.Button == tea.MouseButtonWheelDown) {
		if e.Button == tea.MouseButtonWheelUp { // zoom around the followed player
			m.zoomTo = min(radar.MaxZoom, m.zoomTo*wheelZoom)
		} else {
			m.zoomTo = max(1, m.zoomTo/wheelZoom)
		}
		return nil
	}
	if e.Action == tea.MouseActionPress {
		m.directing, m.focus = false, "" // a click elsewhere, a drag: manual camera
	}
	switch {
	case e.Button == tea.MouseButtonLeft && e.Action == tea.MouseActionPress:
		x, y := m.pointer(e)
		if _, _, ok := m.radarAt(x, y); ok {
			m.drag = drag{on: true, x: x, y: y}
		}
		return nil
	case e.Action == tea.MouseActionRelease:
		m.drag.on = false
		return nil
	case e.Action == tea.MouseActionMotion && m.drag.on:
		x, y := m.pointer(e)
		m.panCells(x-m.drag.x, y-m.drag.y)
		m.drag.x, m.drag.y = x, y
		return nil
	case e.Action != tea.MouseActionPress:
		return nil
	}
	f := 0.0
	switch e.Button {
	case tea.MouseButtonWheelUp:
		f = wheelZoom
	case tea.MouseButtonWheelDown:
		f = 1 / wheelZoom
	default:
		return nil
	}
	ax, ay, ok := m.radarAt(m.pointer(e))
	if !ok { // off the map: zoom around the middle
		ax, ay = m.camera().Center()
	}
	return m.zoomAround(f, ax, ay)
}

const (
	followTime = 0.45 // spring smoothing time, seconds: ~1 s to settle, no velocity jumps
	holdMove   = 0.12 // dead zone: ignore shot moves under this share of the view...
	holdZoom   = 0.15 // ...and zoom changes under this (log scale)
)

// smoothDamp moves cur towards target on a critically damped spring (Game Programming
// Gems 4): velocity stays continuous, so starts, retargets and stops never jerk.
func smoothDamp(cur, target float64, vel *float64, dt float64) float64 {
	omega := 2 / followTime
	x := omega * dt
	e := 1 / (1 + x + 0.48*x*x + 0.235*x*x*x)
	change := cur - target
	temp := (*vel + omega*change) * dt
	*vel = (*vel - omega*temp) * e
	return target + (change+temp)*e
}

func (m *matchModel) toggleDirector() tea.Cmd {
	m.directing, m.focus, m.vel = !m.directing, "", [3]float64{}
	if !m.directing {
		return nil
	}
	if m.dir == nil {
		m.dir = director.New()
	}
	m.direct()
	m.moving = true // keep the frame loop going while directing
	return animTick()
}

// direct asks the director for a shot on the latest state.
func (m *matchModel) direct() {
	if !m.directing || m.assets == nil {
		return
	}
	m.shot = m.dir.Update(time.Now(), m.st, m.soon, m.assets.Overview.LowerBelow)
}

// shotTarget is the camera centre (radar space) and zoom that frame the shot.
func (m *matchModel) shotTarget() (x, y, z float64) {
	s, ov := m.shot, m.assets.Overview
	if s == nil {
		x, y = m.frame.Center()
		return x, y, 1
	}
	side := 2 * s.Radius / ov.Scale // radar px the shot needs both ways
	z = min(radar.MaxZoom, max(1, min(m.frame.W, m.frame.H)/side))
	return (s.X - ov.PosX) / ov.Scale, (ov.PosY - s.Y) / ov.Scale, z
}

// follow eases the camera towards the shot, holding still for small moves.
func (m *matchModel) follow() tea.Cmd {
	if !m.directing {
		m.moving = false
		return nil
	}
	if m.assets == nil { // the map isn't loaded yet: keep the loop waiting for it
		return animTick()
	}
	tx, ty, tz := m.shotTarget()
	v := m.camera()
	far := math.Abs(tx-m.camX) > holdMove*v.W || math.Abs(ty-m.camY) > holdMove*v.H || math.Abs(math.Log(tz/m.zoom)) > holdZoom
	if far || m.settling {
		m.settling = true
		dt := frameTime.Seconds()
		m.camX = smoothDamp(m.camX, tx, &m.vel[0], dt)
		m.camY = smoothDamp(m.camY, ty, &m.vel[1], dt)
		m.zoom = math.Exp(smoothDamp(math.Log(m.zoom), math.Log(tz), &m.vel[2], dt)) // zoom eases in log space
		m.zoomTo = m.zoom
		// once moving, go all the way: stopping at the dead-zone edge looks unfinished
		if math.Abs(tx-m.camX) < 0.005*v.W && math.Abs(ty-m.camY) < 0.005*v.H && math.Abs(math.Log(tz/m.zoom)) < 0.005 {
			m.settling, m.vel = false, [3]float64{}
		}
	}
	return animTick()
}
