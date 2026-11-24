package ui

import (
	"math"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/dormonbear/gotv-tui/internal/director"
	"github.com/dormonbear/gotv-tui/internal/radar"
)

const (
	keyZoom   = 1.25
	wheelZoom = 1.12 // per wheel notch; trackpads send many notches per swipe
	panStep   = 0.2  // of the visible width/height
	easing    = 0.3  // share of the remaining zoom (in log scale) covered per frame
	frameTime = 16 * time.Millisecond
)

// anchor is a radar point (ax, ay) kept at the same place on screen, given as a
// fraction (fx, fy) of the view, while the zoom animates.
type anchor struct{ ax, ay, fx, fy float64 }

type animMsg struct{}

func animTick() tea.Cmd { return tea.Tick(frameTime, func(time.Time) tea.Msg { return animMsg{} }) }

func (m *matchModel) resetCamera() {
	m.zoom, m.zoomTo, m.moving = 1, 1, false
	m.camX, m.camY = m.box.Center()
}

// camera is the radar area on screen. Both levels share it so they stay aligned.
func (m *matchModel) camera() radar.Box {
	v := m.box.View(m.camX, m.camY, m.zoom)
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
	w, h := m.box.W/z, m.box.H/z
	m.camX, m.camY = a.ax-a.fx*w+w/2, a.ay-a.fy*h+h/2
	if z == m.zoomTo {
		m.moving = false
		return nil
	}
	return animTick()
}

// panCells moves the view by (dx, dy) screen cells, the other way: the map follows
// the pointer. It stops a running zoom so the two don't fight.
func (m *matchModel) panCells(dx, dy int) {
	if len(m.levels) == 0 {
		return
	}
	r := m.levels[0].r
	x0, y0 := r.RadarAt(0, 0)
	x1, y1 := r.RadarAt(2, 4) // one cell
	m.zoomTo, m.moving = m.zoom, false
	m.camX -= float64(dx) * (x1 - x0)
	m.camY -= float64(dy) * (y1 - y0)
}

// radarAt is the radar point under screen cell (x, y), if it is on a map.
func (m *matchModel) radarAt(x, y int) (float64, float64, bool) {
	for _, lv := range m.levels {
		cx, cy := x-lv.sx, y-lv.sy
		if cx >= 0 && cy >= 0 && cx < lv.base.CW && cy < lv.base.CH {
			ax, ay := lv.r.RadarAt(float64(cx*2+1), float64(cy*4+2)) // middle of the cell
			return ax, ay, true
		}
	}
	return 0, 0, false
}

func (m *matchModel) cameraKey(k string) tea.Cmd {
	if m.assets == nil {
		return nil
	}
	if k == "d" {
		return m.toggleDirector()
	}
	m.directing = false // any manual move takes the camera back
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
	x, y int // last pointer cell
}

func (m *matchModel) mouse(e tea.MouseMsg) tea.Cmd {
	if m.assets == nil {
		return nil
	}
	if e.Action == tea.MouseActionPress {
		m.directing = false // any manual move takes the camera back
	}
	switch {
	case e.Button == tea.MouseButtonLeft && e.Action == tea.MouseActionPress:
		if _, _, ok := m.radarAt(e.X, e.Y); ok {
			m.drag = drag{on: true, x: e.X, y: e.Y}
		}
		return nil
	case e.Action == tea.MouseActionRelease:
		m.drag.on = false
		return nil
	case e.Action == tea.MouseActionMotion && m.drag.on:
		m.panCells(e.X-m.drag.x, e.Y-m.drag.y)
		m.drag.x, m.drag.y = e.X, e.Y
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
	ax, ay, ok := m.radarAt(e.X, e.Y)
	if !ok { // off the map: zoom around the middle
		ax, ay = m.camera().Center()
	}
	return m.zoomAround(f, ax, ay)
}

const (
	followEase = 0.12 // share of the way to the shot covered per frame (~0.6 s to settle)
	holdMove   = 0.12 // dead zone: ignore shot moves under this share of the view...
	holdZoom   = 0.15 // ...and zoom changes under this (log scale)
)

func (m *matchModel) toggleDirector() tea.Cmd {
	m.directing = !m.directing
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
		x, y = m.box.Center()
		return x, y, 1
	}
	side := 2 * s.Radius / ov.Scale // radar px the shot needs both ways
	z = min(radar.MaxZoom, max(1, min(m.box.W, m.box.H)/side))
	return (s.X - ov.PosX) / ov.Scale, (ov.PosY - s.Y) / ov.Scale, z
}

// follow eases the camera towards the shot, holding still for small moves.
func (m *matchModel) follow() tea.Cmd {
	if !m.directing {
		m.moving = false
		return nil
	}
	tx, ty, tz := m.shotTarget()
	v := m.camera()
	far := math.Abs(tx-m.camX) > holdMove*v.W || math.Abs(ty-m.camY) > holdMove*v.H || math.Abs(math.Log(tz/m.zoom)) > holdZoom
	if far || m.settling {
		m.settling = true
		m.camX += (tx - m.camX) * followEase
		m.camY += (ty - m.camY) * followEase
		m.zoom *= math.Pow(tz/m.zoom, followEase)
		m.zoomTo = m.zoom
		// once moving, go all the way: stopping at the dead-zone edge looks unfinished
		if math.Abs(tx-m.camX) < 0.01*v.W && math.Abs(ty-m.camY) < 0.01*v.H && math.Abs(math.Log(tz/m.zoom)) < 0.01 {
			m.settling = false
		}
	}
	return animTick()
}
