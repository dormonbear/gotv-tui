package ui

import (
	"time"

	"github.com/dormonbear/gotv-tui/internal/radar"
	"github.com/dormonbear/gotv-tui/internal/term"
)

const (
	imageID    = 0x475401 // + level index; also the placeholders' foreground colour
	imageAlpha = 0.85     // dim the map a little so markers stand out
	// while the camera animates, upload at most this often (each upload is a PNG)
	sendEvery = 80 * time.Millisecond
)

type sentImage struct {
	ok         bool
	view       radar.Box
	cols, rows int
}

// sendImages uploads each level's visible area when it changed since the last upload.
func (m *matchModel) sendImages() {
	if m.out == nil || m.assets == nil {
		return
	}
	if len(m.sent) != len(m.levels) {
		m.sent = make([]sentImage, len(m.levels))
	}
	animating := m.moving || m.settling || m.drag.on
	cw, ch := term.CellPixels()
	for i, lv := range m.levels {
		want := sentImage{true, lv.view, lv.base.CW, lv.base.CH}
		if m.sent[i] == want || (animating && m.sent[i].ok && time.Since(m.lastSend) < sendEvery) {
			continue
		}
		img := m.assets.Upper
		if lv.lower {
			img = m.assets.Lower
		}
		// no more pixels than the radar has in view: the terminal scales it up
		src := lv.view.W * float64(img.Rect.Dx()) / 1024
		w, h := want.cols*cw, want.rows*ch
		if k := src / float64(w); k < 1 {
			w, h = int(float64(w)*k), int(float64(h)*k)
		}
		png, err := radar.ImagePNG(img, lv.view, max(1, w), max(1, h), imageAlpha)
		if err != nil || term.SendImage(m.out, imageID+uint32(i), png, want.cols, want.rows) != nil {
			continue // keeps drawing half blocks for this level
		}
		m.sent[i] = want
		m.lastSend = time.Now()
	}
}
