package term

import (
	"encoding/base64"
	"fmt"
	"image"
	"sort"
	"strings"
)

// Whole-frame image protocols: the image is part of the screen content, so text
// written over it erases it; callers redraw it after every TUI frame (SetOverlay).

// At wraps an image sequence so it is drawn with its top-left at screen cell (x, y)
// (0-based) and the cursor is left where the TUI had it.
func At(x, y int, seq string) string {
	return fmt.Sprintf("\x1b7\x1b[%d;%dH%s\x1b8", y+1, x+1, seq)
}

// ITermImage shows png stretched to cols x rows cells (iTerm2 inline images,
// also WezTerm and VS Code).
func ITermImage(png []byte, cols, rows int) string {
	return fmt.Sprintf("\x1b]1337;File=inline=1;size=%d;width=%d;height=%d;preserveAspectRatio=0:%s\x07",
		len(png), cols, rows, base64.StdEncoding.EncodeToString(png))
}

// SixelImage encodes img with a palette of its 255 most common colours (5 bits per
// channel; the rest map to the nearest); transparent pixels are left unpainted so
// the terminal background shows through.
func SixelImage(img *image.NRGBA) string {
	w, h := img.Rect.Dx(), img.Rect.Dy()
	bucket := func(x, y int) int { // 15-bit colour, -1 for transparent
		p := img.Pix[img.PixOffset(x, y):]
		if p[3] < 128 {
			return -1
		}
		return int(p[0]>>3)<<10 | int(p[1]>>3)<<5 | int(p[2]>>3)
	}
	count := map[int]int{}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if c := bucket(x, y); c >= 0 {
				count[c]++
			}
		}
	}
	pal := make([]int, 0, len(count))
	for c := range count {
		pal = append(pal, c)
	}
	sort.Slice(pal, func(i, j int) bool {
		return count[pal[i]] > count[pal[j]] || count[pal[i]] == count[pal[j]] && pal[i] < pal[j]
	})
	if len(pal) > 255 {
		pal = pal[:255]
	}
	idx := map[int]int{} // bucket -> palette index (nearest for buckets left out)
	for i, c := range pal {
		idx[c] = i
	}
	rgb := func(c int) (int, int, int) { return c >> 10 & 31, c >> 5 & 31, c & 31 }
	index := func(c int) int {
		if i, ok := idx[c]; ok {
			return i
		}
		r, g, b := rgb(c)
		best, bd := 0, 1<<30
		for i, p := range pal {
			pr, pg, pb := rgb(p)
			if d := (r-pr)*(r-pr) + (g-pg)*(g-pg) + (b-pb)*(b-pb); d < bd {
				best, bd = i, d
			}
		}
		idx[c] = best
		return best
	}

	var b strings.Builder
	fmt.Fprintf(&b, "\x1bP0;1;0q\"1;1;%d;%d", w, h)
	for i, c := range pal {
		r, g, bl := rgb(c)
		fmt.Fprintf(&b, "#%d;2;%d;%d;%d", i, (r*8+4)*100/255, (g*8+4)*100/255, (bl*8+4)*100/255)
	}
	rows := make([][]byte, len(pal)) // per colour: one sixel column bitmask per x, this band
	for y0 := 0; y0 < h; y0 += 6 {
		var used []int
		for dy := 0; dy < 6 && y0+dy < h; dy++ {
			for x := 0; x < w; x++ {
				c := bucket(x, y0+dy)
				if c < 0 {
					continue
				}
				i := index(c)
				if rows[i] == nil {
					rows[i] = make([]byte, w)
					used = append(used, i)
				}
				rows[i][x] |= 1 << dy
			}
		}
		for k, i := range used {
			if k > 0 {
				b.WriteByte('$') // back to the band's start for the next colour
			}
			fmt.Fprintf(&b, "#%d", i)
			writeRuns(&b, rows[i])
			rows[i] = nil
		}
		b.WriteByte('-')
	}
	b.WriteString("\x1b\\")
	return b.String()
}

// writeRuns writes sixel characters with run-length compression (!n<char>).
func writeRuns(b *strings.Builder, bits []byte) {
	for x := 0; x < len(bits); {
		n := 1
		for x+n < len(bits) && bits[x+n] == bits[x] {
			n++
		}
		c := byte(63 + bits[x])
		if n > 3 {
			fmt.Fprintf(b, "!%d%c", n, c)
		} else {
			b.WriteString(strings.Repeat(string(c), n))
		}
		x += n
	}
}
