package ui

import (
	"image"
	"math"
	"strings"
	"testing"

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
		ax, ay, ok := m.radarAt(x, y)
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
