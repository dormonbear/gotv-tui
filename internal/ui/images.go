package ui

import (
	"image"
	"math"

	"github.com/dormonbear/gotv-tui/internal/radar"
	"github.com/dormonbear/gotv-tui/internal/term"
)

const (
	imageID    = 0x475401 // + level index
	imageAlpha = 0.85     // dim the map a little so markers stand out
)

// images reports whether the radar is drawn as a terminal image under the text.
func (m *matchModel) images() bool {
	return m.gfx == term.Kitty && m.out != nil && m.assets != nil && m.uploaded == m.assets.Map
}

// placeImages shows each level's current view. The images are uploaded once per map;
// every frame then only sends a placement (position + source rectangle), so zoom
// and pan follow the camera exactly, and a screen clear is repaired next frame.
func (m *matchModel) placeImages() {
	if m.gfx != term.Kitty || m.out == nil || m.assets == nil {
		return
	}
	if m.uploaded != m.assets.Map {
		m.hideImages()
		levels := []*image.NRGBA{m.assets.Upper}
		if m.assets.Lower != nil {
			levels = append(levels, m.assets.Lower)
		}
		for i, src := range levels {
			png, err := radar.ImagePNG(src, radar.Box{W: 1024, H: 1024}, src.Rect.Dx(), src.Rect.Dy(), imageAlpha)
			if err != nil || term.UploadImage(m.out, imageID+uint32(i), png) != nil {
				return // stay in half blocks
			}
		}
		m.uploaded = m.assets.Map
	}
	cw, ch := term.CellPixels()
	var areas []mapArea
	for i, lv := range m.levels {
		areas = append(areas, mapArea{lv.sx, lv.sy, lv.base.CW, lv.base.CH, radar.Sprites(lv.r, cw, ch, m.st, lv.opts)})
		img := m.assets.Upper
		if lv.lower {
			img = m.assets.Lower
		}
		kx, ky := float64(img.Rect.Dx())/1024, float64(img.Rect.Dy())/1024
		r := func(v float64) int { return int(math.Round(v)) }
		src := term.Rect{X: r(lv.view.X * kx), Y: r(lv.view.Y * ky), W: max(1, r(lv.view.W*kx)), H: max(1, r(lv.view.H*ky))}
		term.PlaceImage(m.out, imageID+uint32(i), lv.sx, lv.sy, lv.base.CW, lv.base.CH, src)
	}
	m.sprites.frame(m.out, areas, cw, ch)
}

// hideImages takes the radar images off screen (keeping them uploaded).
func (m *matchModel) hideImages() {
	if m.out == nil {
		return
	}
	if m.gfx != term.Kitty {
		m.out.SetOverlay(nil) // whole frames: stop redrawing them; the list's text covers them
		return
	}
	for i := 0; i < 2; i++ {
		term.HideImage(m.out, imageID+uint32(i))
	}
	m.sprites.hide(m.out)
}
