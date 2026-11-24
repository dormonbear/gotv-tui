package ui

import (
	"bytes"
	"image/png"
	"math"
	"strings"
	"time"

	"github.com/dormonbear/gotv-tui/internal/radar"
	"github.com/dormonbear/gotv-tui/internal/term"
)

const (
	frameEvery = 100 * time.Millisecond // whole-frame images: at most 10 per second
	frameMaxW  = 640                    // iTerm2 frames are scaled down to this width (the terminal scales up)
)

// wholeFrames reports whether the map is sent as one composed image per frame.
func (m *matchModel) wholeFrames() bool {
	return (m.gfx == term.ITerm || m.gfx == term.Sixel) && m.out != nil && m.assets != nil
}

// sendFrames composes each level (radar view + markers) into one image, encodes it
// for the terminal and makes it the output's overlay, redrawn after every TUI frame.
func (m *matchModel) sendFrames() {
	if !m.wholeFrames() || time.Since(m.lastFrame) < frameEvery {
		return
	}
	m.lastFrame = time.Now()
	if m.frameSprites == nil || len(m.frameSprites) > spriteLimit {
		m.frameSprites = map[string]radar.Sprite{}
	}
	cw, ch := term.CellPixels()
	var out strings.Builder
	for _, lv := range m.levels {
		img := m.assets.Upper
		if lv.lower {
			img = m.assets.Lower
		}
		// sixel is shown at its own pixel size, so it must match the cells exactly;
		// iTerm2 images are stretched to the cells, so they can be smaller
		s := 1.0
		if m.gfx == term.ITerm {
			s = math.Min(1, frameMaxW/float64(lv.base.CW*cw))
		}
		vcw, vch := max(2, int(math.Round(float64(cw)*s))), max(4, int(math.Round(float64(ch)*s)))
		frame := radar.ViewImage(img, lv.view, lv.base.CW*vcw, lv.base.CH*vch, imageAlpha)
		for _, u := range radar.Sprites(lv.r, vcw, vch, m.st, lv.opts) {
			sp, ok := m.frameSprites[u.Key]
			if !ok {
				sp = u.Make()
				m.frameSprites[u.Key] = sp
			}
			radar.Stamp(frame, sp, u.X, u.Y)
		}
		var seq string
		if m.gfx == term.Sixel {
			seq = term.SixelImage(frame)
		} else {
			var b bytes.Buffer
			if (&png.Encoder{CompressionLevel: png.BestSpeed}).Encode(&b, frame) != nil {
				continue
			}
			seq = term.ITermImage(b.Bytes(), lv.base.CW, lv.base.CH)
		}
		out.WriteString(term.At(lv.sx, lv.sy, seq))
	}
	m.out.SetOverlay([]byte(out.String()))
}
