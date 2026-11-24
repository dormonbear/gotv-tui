package ui

import (
	"bytes"
	"fmt"
	"image/png"
	"io"
	"math"
	"strings"

	"github.com/dormonbear/gotv-tui/internal/radar"
	"github.com/dormonbear/gotv-tui/internal/term"
)

const (
	spriteIDs   = 0x475500 // first sprite image id
	spriteLimit = 400      // looks kept in the terminal before the cache is reset
)

type spriteImg struct {
	id     uint32
	ax, ay int
	w, h   int
}

type placement struct{ id, p uint32 }

// spriteLayer keeps marker sprites in the terminal and places them every frame.
type spriteLayer struct {
	cache  map[string]spriteImg
	next   uint32
	placed map[placement]bool
}

// mapArea is where one level's map sits on screen and how its sprites go on it.
type mapArea struct {
	x, y       int // screen cell of the map's top-left
	cols, rows int
	uses       []radar.Use
}

// frame uploads new looks and places all markers of all levels, removing the
// placements of the last frame that are gone. cw, ch: pixels per cell.
func (s *spriteLayer) frame(w io.Writer, areas []mapArea, cw, ch int) {
	if s.cache == nil || len(s.cache) > spriteLimit {
		s.reset(w)
	}
	var b strings.Builder
	b.WriteString("\x1b7")
	cur := map[placement]bool{}
	n := map[uint32]uint32{} // placements per image this frame
	for _, a := range areas {
		mapW, mapH := a.cols*cw, a.rows*ch
		for _, u := range a.uses {
			img, ok := s.cache[u.Key]
			if !ok {
				sp := u.Make()
				var buf bytes.Buffer
				if (&png.Encoder{CompressionLevel: png.BestSpeed}).Encode(&buf, sp.Img) != nil {
					continue
				}
				img = spriteImg{spriteIDs + s.next, sp.AX, sp.AY, sp.Img.Rect.Dx(), sp.Img.Rect.Dy()}
				if term.UploadImage(w, img.id, buf.Bytes()) != nil {
					continue
				}
				s.next++
				s.cache[u.Key] = img
			}
			// top-left in map pixels, clipped to the map
			x, y := int(math.Round(u.X))-img.ax, int(math.Round(u.Y))-img.ay
			src := term.Rect{W: img.w, H: img.h}
			if x < 0 {
				src.X, src.W, x = -x, src.W+x, 0
			}
			if y < 0 {
				src.Y, src.H, y = -y, src.H+y, 0
			}
			src.W, src.H = min(src.W, mapW-x), min(src.H, mapH-y)
			if src.W <= 0 || src.H <= 0 {
				continue
			}
			n[img.id]++
			pl := placement{img.id, n[img.id]}
			cur[pl] = true
			fmt.Fprintf(&b, "\x1b[%d;%dH\x1b_Ga=p,i=%d,p=%d,X=%d,Y=%d,x=%d,y=%d,w=%d,h=%d,z=%d,C=1,q=2\x1b\\",
				a.y+y/ch+1, a.x+x/cw+1, pl.id, pl.p, x%cw, y%ch, src.X, src.Y, src.W, src.H, u.Z)
		}
	}
	for pl := range s.placed {
		if !cur[pl] {
			fmt.Fprintf(&b, "\x1b_Ga=d,d=i,i=%d,p=%d,q=2\x1b\\", pl.id, pl.p)
		}
	}
	b.WriteString("\x1b8")
	io.WriteString(w, b.String()) // one write: never split by a TUI frame
	s.placed = cur
}

// hide removes every sprite placement (the images stay cached).
func (s *spriteLayer) hide(w io.Writer) {
	var b strings.Builder
	for pl := range s.placed {
		fmt.Fprintf(&b, "\x1b_Ga=d,d=i,i=%d,p=%d,q=2\x1b\\", pl.id, pl.p)
	}
	io.WriteString(w, b.String())
	s.placed = nil
}

// reset frees all sprite images, e.g. when many one-off looks (kill lines) piled up.
func (s *spriteLayer) reset(w io.Writer) {
	var b strings.Builder
	for _, img := range s.cache {
		fmt.Fprintf(&b, "\x1b_Ga=d,d=I,i=%d,q=2\x1b\\", img.id)
	}
	io.WriteString(w, b.String())
	s.cache, s.placed, s.next = map[string]spriteImg{}, nil, 0
}
