package ui

import (
	"math"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/dormonbear/gotv-tui/internal/gotv"
	"github.com/dormonbear/gotv-tui/internal/radar"
)

const (
	focusZoom = 2.5  // following from the whole map zooms in at least this far
	focusEase = 0.25 // share of the way to the player covered per frame: stays centred
	hitRadius = 1.2  // a click this many cell widths from a player selects them
)

// focusOn starts following player name (lowercase), or stops if already followed.
func (m *matchModel) focusOn(name string) tea.Cmd {
	if m.focus == name {
		return m.unfollow()
	}
	if m.focus == "" { // remember whether to hand back to the director afterwards
		m.directBefore = m.directing
	}
	m.focus, m.directing = name, false
	m.zoomTo = math.Max(m.zoomTo, focusZoom)
	if m.moving {
		return nil
	}
	m.moving = true
	return animTick()
}

// unfollow stops following; the director takes over again if it had the camera
// before the follow began.
func (m *matchModel) unfollow() tea.Cmd {
	m.focus = ""
	if !m.directBefore {
		return nil
	}
	m.directBefore = false
	return m.resumeDirector()
}

// focusPos is the followed player's radar position (their death spot once dead).
func (m *matchModel) focusPos() (float64, float64, bool) {
	if m.st == nil || m.assets == nil {
		return 0, 0, false
	}
	ov := m.assets.Overview
	for _, p := range m.st.Players {
		if strings.ToLower(p.Name) == m.focus && p.Pos != nil {
			m.driveLevel(m.levelOf(p.Pos.Z), true) // follow them onto their level
			return (p.Pos.X - ov.PosX) / ov.Scale, (ov.PosY - p.Pos.Y) / ov.Scale, true
		}
	}
	return 0, 0, false
}

// followPlayer keeps the followed player centred, easing zoom towards zoomTo.
func (m *matchModel) followPlayer() tea.Cmd {
	tx, ty, ok := m.focusPos()
	if !ok { // left the server: stop following
		m.focus, m.moving = "", false
		return nil
	}
	m.camX += (tx - m.camX) * focusEase
	m.camY += (ty - m.camY) * focusEase
	m.zoom *= math.Pow(m.zoomTo/m.zoom, focusEase)
	return animTick()
}

// playerAt is the alive player whose map marker is at screen position (x, y) cells.
func (m *matchModel) playerAt(x, y float64) string {
	if m.st == nil || m.assets == nil {
		return ""
	}
	best, bestD := "", hitRadius
	for _, lv := range m.levels {
		for _, p := range m.st.Players {
			if !p.Alive || p.Pos == nil || radar.IsLower(p.Pos.Z, m.assets.Overview.LowerBelow) != lv.lower {
				continue
			}
			px, py := lv.r.ToPx(p.Pos.X, p.Pos.Y) // braille dots: 2 per cell across, 4 down
			dx := x - (float64(lv.sx) + px/2)
			dy := (y - (float64(lv.sy) + py/4)) * m.aspect // rows are taller than columns are wide
			if d := math.Hypot(dx, dy); d < bestD {
				best, bestD = strings.ToLower(p.Name), d
			}
		}
	}
	return best
}

// followKey follows a player by number key: 1-5 the CT table's badges, 6-0 the
// T table's. The same key again stops following.
func (m *matchModel) followKey(i int) tea.Cmd {
	side, n := gotv.CT, i%5+1
	if i >= 5 {
		side = gotv.T
	}
	nums := PlayerNumbers(m.st.Players)
	for _, p := range m.st.Players {
		if name := strings.ToLower(p.Name); p.Side == side && nums[name] == n {
			return m.focusOn(name)
		}
	}
	return nil
}
