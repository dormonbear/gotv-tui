package ui

import (
	"image"
	"math"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

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
	m := &matchModel{w: 178, h: 40, aspect: 2.16, assets: a, box: radar.Box{X: 112, Y: 112, W: 800, H: 800}}
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
	// 1400 px is more than the 800 px map: the shot needs the whole map
	if tx, ty, tz := m.shotTarget(); tz != 1 || !near(m.camera(), m.box.View(tx, ty, tz)) {
		t.Fatalf("view %+v, want the whole map", m.camera())
	}
	m.shot = &director.Shot{X: 300, Y: -400, Radius: 120} // 240 px wide: ~3.3x on (300, 400)
	for i := 0; i < 120; i++ {
		m.follow()
	}
	if tx, ty, tz := m.shotTarget(); tz < 3.3 || !near(m.camera(), m.box.View(tx, ty, tz)) {
		t.Fatalf("view %+v x%.2f, want %+v", m.camera(), m.zoom, m.box.View(tx, ty, tz))
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
