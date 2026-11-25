package ui

import (
	"errors"
	"image"
	"math"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/dormonbear/gotv-tui/internal/director"
	"github.com/dormonbear/gotv-tui/internal/gotv"
	"github.com/dormonbear/gotv-tui/internal/radar"
)

// testMatch has a fully-floored 1024x1024 radar with world == radar coords (y flipped).
func testMatch(lower bool) *matchModel {
	img := image.NewNRGBA(image.Rect(0, 0, 256, 256))
	for i := 0; i < len(img.Pix); i += 4 {
		copy(img.Pix[i:], []byte{80, 100, 110, 255})
	}
	lb := -500.0
	a := &radar.Assets{Map: "de_test", Overview: radar.Overview{Scale: 1, LowerBelow: &lb}, Upper: img}
	if lower {
		a.Lower = img
	}
	box := radar.Box{X: 112, Y: 112, W: 800, H: 800}
	m := &matchModel{w: 178, h: 40, aspect: 2.16, assets: a, box: box, frame: box} // as when the map loads
	m.resetCamera()
	return m
}

// cellOf finds the screen cell showing glyph ch right of column minX (the map side).
func cellOf(t *testing.T, screen string, ch rune, minX int) (int, int) {
	for y, line := range strings.Split(plain(screen), "\n") {
		for x, r := range []rune(line) {
			if r == ch && x >= minX {
				return x, y
			}
		}
	}
	t.Fatalf("%q not on screen", ch)
	return 0, 0
}

func TestMouseHitsTheRightRadarPoint(t *testing.T) {
	for _, lower := range []bool{false, true} {
		m := testMatch(lower)
		z := 0.0
		if lower {
			z = -600 // on the lower map, which is drawn second
		}
		m.st = &gotv.State{Players: []gotv.Player{{Name: "p", Side: gotv.T, Alive: true, Pos: &gotv.Vec{X: 600, Y: -300, Z: z}}}}
		x, y := cellOf(t, m.view(), '1', leftMax)
		ax, ay, ok := m.radarAt(float64(x)+0.5, float64(y)+0.5)
		// the player is somewhere in that cell, so the hit is within half a cell
		r := m.levels[len(m.levels)-1].r
		x0, y0 := r.RadarAt(0, 0)
		x1, y1 := r.RadarAt(2, 4) // one cell = 2x4 dots
		if !ok || math.Abs(ax-600) > (x1-x0)/2 || math.Abs(ay-300) > (y1-y0)/2 {
			t.Fatalf("lower=%v: cell (%d,%d) -> (%.0f,%.0f) ok=%v, want ~(600,300)", lower, x, y, ax, ay, ok)
		}
	}
}

func TestZoomEasesInAndHoldsTheAnchor(t *testing.T) {
	m := testMatch(false)
	m.zoomAround(2, 300, 400)
	frac := func() (float64, float64) {
		v := m.camera()
		return (300 - v.X) / v.W, (400 - v.Y) / v.H
	}
	fx, fy := m.anchor.fx, m.anchor.fy
	frames := 0
	for m.moving {
		m.step()
		if gx, gy := frac(); math.Abs(gx-fx) > 1e-9 || math.Abs(gy-fy) > 1e-9 {
			t.Fatalf("anchor drifted to (%.3f,%.3f) from (%.3f,%.3f)", gx, gy, fx, fy)
		}
		if frames++; frames > 60 {
			t.Fatal("animation never settles")
		}
	}
	if m.zoom != 2 || frames < 5 {
		t.Fatalf("zoom %.3f after %d frames", m.zoom, frames)
	}
}

func TestDragKeepsTheGrabbedPointUnderThePointer(t *testing.T) {
	for _, lower := range []bool{false, true} {
		dragOn(t, lower)
	}
}

// dragOn grabs the last level shown (the lower map on two-level maps): both levels
// share one camera, so the other level must move with it.
func dragOn(t *testing.T, lower bool) {
	m := testMatch(lower)
	m.zoom, m.zoomTo = 3, 3
	m.view()
	lv := m.levels[len(m.levels)-1]
	x, y := lv.sx+20, lv.sy+8
	ax, ay, _ := m.radarAt(float64(x)+0.5, float64(y)+0.5)
	m.mouse(tea.MouseMsg{X: x, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m.mouse(tea.MouseMsg{X: x + 5, Y: y + 2, Button: tea.MouseButtonLeft, Action: tea.MouseActionMotion})
	m.mouse(tea.MouseMsg{X: x + 7, Y: y + 3, Button: tea.MouseButtonLeft, Action: tea.MouseActionMotion})
	m.mouse(tea.MouseMsg{X: x + 7, Y: y + 3, Button: tea.MouseButtonLeft, Action: tea.MouseActionRelease})
	m.view()
	if bx, by, _ := m.radarAt(float64(x+7)+0.5, float64(y+3)+0.5); math.Abs(bx-ax) > 1e-6 || math.Abs(by-ay) > 1e-6 {
		t.Fatalf("lower=%v: grabbed (%.1f,%.1f), now under pointer (%.1f,%.1f)", lower, ax, ay, bx, by)
	}
	if m.levels[0].view != m.levels[len(m.levels)-1].view {
		t.Fatal("levels no longer share the camera")
	}
	m.mouse(tea.MouseMsg{X: x + 30, Y: y, Button: tea.MouseButtonNone, Action: tea.MouseActionMotion})
	if m.drag.on {
		t.Fatal("still dragging after release")
	}
}

func TestDirectorCameraFollowsTheShot(t *testing.T) {
	m := testMatch(false)
	m.view()
	m.toggleDirector()
	m.shot = &director.Shot{X: 300, Y: -400, Radius: 700} // radar (300, 400), 1400 px wide
	for i := 0; i < 120; i++ {
		m.follow()
	}
	near := func(a, b radar.Box) bool {
		tol := 0.02 * b.W // the camera settles within ~1% of the shot
		return math.Abs(a.X-b.X) < tol && math.Abs(a.Y-b.Y) < tol && math.Abs(a.W-b.W) < tol && math.Abs(a.H-b.H) < tol
	}
	// 1400 px is more than the 800 px map: the shot needs the whole (filled) frame
	if tx, ty, tz := m.shotTarget(); tz != 1 || !near(m.camera(), m.frame.View(tx, ty, tz)) {
		t.Fatalf("view %+v, want the whole map", m.camera())
	}
	m.shot = &director.Shot{X: 300, Y: -400, Radius: 120} // 240 px wide: ~3.3x on (300, 400)
	for i := 0; i < 120; i++ {
		m.follow()
	}
	if tx, ty, tz := m.shotTarget(); tz < 3.3 || !near(m.camera(), m.frame.View(tx, ty, tz)) {
		t.Fatalf("view %+v x%.2f, want %+v", m.camera(), m.zoom, m.frame.View(tx, ty, tz))
	}
	x, y := m.camX, m.camY
	m.shot.X += 10 // small move: inside the dead zone, the camera holds
	for i := 0; i < 30; i++ {
		m.follow()
	}
	if m.camX != x || m.camY != y {
		t.Fatalf("camera chased a small move: (%.2f,%.2f) -> (%.2f,%.2f)", x, y, m.camX, m.camY)
	}
	m.cameraKey("left")
	if m.directing {
		t.Fatal("manual input should end directing")
	}
}

func TestDirectorSwitchClickAndLabel(t *testing.T) {
	m := testMatch(false)
	if !strings.Contains(plain(m.view()), "DIRECTOR OFF") {
		t.Fatal("switch should show OFF")
	}
	click := tea.MouseMsg{X: m.switchX + 2, Y: 0, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}
	m.mouse(click)
	if !m.directing || !strings.Contains(plain(m.view()), "DIRECTOR ON") {
		t.Fatal("click should turn the director on")
	}
	m.mouse(click)
	if m.directing {
		t.Fatal("second click should turn it off")
	}
}

func TestClickAPlayerToFollowThem(t *testing.T) {
	m := testMatch(false)
	m.st = &gotv.State{Players: []gotv.Player{
		{Name: "s1n", Side: gotv.T, Alive: true, Pos: &gotv.Vec{X: 600, Y: -300}},
		{Name: "dgt", Side: gotv.CT, Alive: true, Pos: &gotv.Vec{X: 300, Y: -700}},
	}}
	m.view()
	click := func(x, y float64) {
		m.mouse(tea.MouseMsg{X: int(x), Y: int(y), Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
		m.mouse(tea.MouseMsg{X: int(x), Y: int(y), Button: tea.MouseButtonLeft, Action: tea.MouseActionRelease})
	}
	lv := m.levels[0]
	px, py := lv.r.ToPx(600, -300)
	sx, sy := float64(lv.sx)+px/2, float64(lv.sy)+py/4
	click(sx, sy)
	if m.focus != "s1n" {
		t.Fatalf("clicking s1n's marker: focus %q", m.focus)
	}
	for i := 0; i < 60; i++ {
		m.followPlayer()
	}
	if v := m.camera(); math.Abs(m.zoom-focusZoom) > 0.01 || math.Abs(v.X+v.W/2-600) > 3 || math.Abs(v.Y+v.H/2-300) > 3 {
		t.Fatalf("camera %+v x%.2f, want centred on (600,300) at x%.1f", v, m.zoom, focusZoom)
	}
	if !strings.Contains(plain(m.view()), "FOLLOWING s1n") {
		t.Fatal("bar should say who is followed")
	}
	px, py = m.levels[0].r.ToPx(600, -300) // the camera moved: s1n is centred now
	click(float64(m.levels[0].sx)+px/2, float64(m.levels[0].sy)+py/4)
	if m.focus != "" {
		t.Fatal("clicking the followed player again should stop")
	}
	for row, name := range m.rowPlayer { // a scoreboard row selects too
		if name == "dgt" {
			click(3, float64(row))
		}
	}
	if m.focus != "dgt" {
		t.Fatalf("clicking dgt's scoreboard row: focus %q", m.focus)
	}
	click(float64(m.levels[0].sx+1), float64(m.levels[0].sy+1)) // empty map corner
	if m.focus != "" {
		t.Fatal("clicking elsewhere on the map should stop following")
	}
}

func TestDirectorWaitsForTheMap(t *testing.T) {
	m := &matchModel{directing: true, moving: true}
	if m.follow() == nil {
		t.Fatal("the director loop should keep running until the map loads")
	}
}

func TestNarrowScreensStackTheLayout(t *testing.T) {
	for _, size := range [][2]int{{100, 40}, {50, 45}} {
		m := testMatch(false)
		m.w, m.h = size[0], size[1]
		m.st = &gotv.State{Map: "de_test", Players: []gotv.Player{
			{Name: "s1n", Side: gotv.T, Alive: true, HP: 100, Money: 4750, Gun: "AK-47", Pos: &gotv.Vec{X: 600, Y: -300}},
			{Name: "dgt", Side: gotv.CT, Alive: true, HP: 100, Pos: &gotv.Vec{X: 300, Y: -700}},
		}}
		lines := strings.Split(m.view(), "\n")
		for i, l := range lines {
			if w := ansi.StringWidth(l); w > m.w {
				t.Fatalf("%dx%d: line %d is %d wide: %q", m.w, m.h, i, w, plain(l))
			}
		}
		if len(lines) != m.h {
			t.Fatalf("%dx%d: %d lines", m.w, m.h, len(lines))
		}
		compact := m.w < compactMax
		if strings.Contains(plain(lines[1]), "ADR") == compact {
			t.Fatalf("%dx%d: compact=%v but header %q", m.w, m.h, compact, plain(lines[1]))
		}
		// header, CT title, dgt, T title, s1n: the switch is the next row
		if m.switchY != 5 || !strings.Contains(plain(lines[5]), "DIRECTOR") {
			t.Fatalf("%dx%d: switch at row %d: %q", m.w, m.h, m.switchY, plain(lines[m.switchY]))
		}
		m.mouse(tea.MouseMsg{X: m.switchX + 1, Y: m.switchY, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
		if m.directing != true {
			t.Fatalf("%dx%d: clicking the switch should turn the director on", m.w, m.h)
		}
		m.directing = false
		m.view()
		lv := m.levels[0]
		px, py := lv.r.ToPx(600, -300)
		m.mouse(tea.MouseMsg{X: int(float64(lv.sx) + px/2), Y: int(float64(lv.sy) + py/4), Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
		if m.focus != "s1n" {
			t.Fatalf("%dx%d: clicking s1n on the stacked map: focus %q", m.w, m.h, m.focus)
		}
	}
}

func TestDirectorCameraHasNoVelocityJumps(t *testing.T) {
	m := testMatch(false)
	m.view()
	m.toggleDirector()
	m.shot = &director.Shot{X: 300, Y: -400, Radius: 120}
	for i := 0; i < 200; i++ {
		m.follow()
	}
	var steps []float64
	x := m.camX
	for i := 0; i < 90; i++ {
		if i == 0 {
			m.shot.X = 700 // leaves the dead zone
		}
		if i == 20 {
			m.shot.X = 500 // retarget mid-flight
		}
		m.follow()
		steps = append(steps, m.camX-x)
		x = m.camX
	}
	top := 0.0
	for _, s := range steps {
		top = max(top, math.Abs(s))
	}
	if math.Abs(steps[0]) > 0.15*top {
		t.Fatalf("camera jumped off the mark: first step %.2f of max %.2f", steps[0], top)
	}
	for i := 1; i < len(steps); i++ {
		if math.Abs(steps[i]-steps[i-1]) > 0.2*top {
			t.Fatalf("velocity jump at frame %d: %.2f -> %.2f (max %.2f)", i, steps[i-1], steps[i], top)
		}
	}
}

func TestRadarAnswersForAnotherMapAreDropped(t *testing.T) {
	m := testMatch(false)
	m.loading = "de_nuke"
	other := &radar.Assets{Map: "de_dust2", Upper: m.assets.Upper}
	m.update(radarMsg{"de_dust2", other, nil}) // a slow download from the previous match
	if m.assets.Map != "de_test" {
		t.Fatalf("took a radar nobody asked for: %s", m.assets.Map)
	}
	m.update(radarMsg{"de_nuke", nil, errors.New("timeout")})
	if m.loading != "" || !m.retryAt.After(time.Now()) {
		t.Fatal("a failed download should clear loading and schedule a retry")
	}
}
