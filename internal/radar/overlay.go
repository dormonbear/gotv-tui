package radar

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"strings"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"

	"github.com/dormonbear/gotv-tui/internal/gotv"
	"github.com/dormonbear/gotv-tui/internal/term"
)

// Sprites: for terminals that show the radar as an image, markers are small images
// drawn once (per key) and then only *placed* every frame at pixel positions, so they
// move with the map at pixel precision without re-encoding anything per frame.

// Sprite is a marker image; (AX, AY) is the pixel that goes on the marker's position.
type Sprite struct {
	Img    *image.NRGBA
	AX, AY int
}

// Use is one marker this frame: Key identifies its look (same key, same image), Make
// draws it when the key is new, (X, Y) is its position in map pixels, Z its layer.
type Use struct {
	Key  string
	Make func() Sprite
	X, Y float64
	Z    int
}

// Sprites lists the markers of one level for a map drawn at cellW x cellH pixels per
// cell, in the same places Draw would put them, back to front.
func Sprites(r *Raster, cellW, cellH int, st *gotv.State, o Opts) []Use {
	CW := (r.W + 1) / 2
	sx, sy := float64(cellW)/2, float64(cellH)/4 // pixels per braille dot
	at := func(v gotv.Vec) (float64, float64) {
		x, y := r.ToPx(v.X, v.Y)
		return x * sx, y * sy
	}
	here := func(v gotv.Vec) bool { return IsLower(v.Z, o.LowerBelow) == o.Lower }
	x0, _ := at(gotv.Vec{})
	x1, _ := at(gotv.Vec{X: 1000})
	perUnit := math.Abs(x1-x0) / 1000 // pixels per world unit
	m := metrics(cellH)
	var out []Use
	add := func(key string, z int, x, y float64, make func() Sprite) {
		out = append(out, Use{Key: key, Make: make, X: x, Y: y, Z: z})
	}

	if st != nil {
		for _, u := range st.Utility {
			if !here(u.Pos) || (u.Kind != gotv.USmoke && u.Kind != gotv.UFire) {
				continue
			}
			x, y := at(u.Pos)
			col, rad, a := smokeC, smokeR, 0.6
			if u.Kind == gotv.UFire {
				col, rad, a = fireC, fireR, 0.55
			}
			// ~8% size steps: zooming re-draws a few discs, not one per frame
			px := math.Round(math.Pow(1.08, math.Round(math.Log(math.Max(1, rad*perUnit))/math.Log(1.08))))
			add(fmt.Sprintf("disc:%v:%.0f", u.Kind, px), 1, x, y, func() Sprite { return discSprite(px, col, a) })
		}
	}
	if b := o.Boom; b != nil && here(b.Pos) && b.Age < BoomShow {
		x, y := at(b.Pos)
		rad, a, flash := b.ring()
		step := int(b.Age / BoomShow * 16) // 16 frames of animation, each drawn once
		px, w := math.Round(rad*perUnit), math.Max(2, rad*perUnit*0.18)
		add(fmt.Sprintf("boom:%d:%.0f", step, px), 3, x, y, func() Sprite { return boomSprite(px, w, a, flash, boomC) })
	}
	for _, bl := range o.Blasts {
		age := o.Now.Sub(bl.At).Seconds()
		if rad, a, c, ok := bl.fx(age); ok && here(bl.Pos) {
			x, y := at(bl.Pos)
			step := int(age * 20) // ~20 frames a second, each drawn once
			px := math.Round(rad * perUnit)
			add(fmt.Sprintf("bfx:%s:%d:%.0f", bl.Nade, step, px), 3, x, y, func() Sprite { return boomSprite(px, math.Max(2, px*0.3), a, false, c) })
		}
	}
	for i, kl := range o.Kills {
		if o.Now.Sub(kl.At) > killShow || !here(kl.To) {
			continue
		}
		age := o.Now.Sub(kl.At).Seconds()
		vx, vy := at(kl.To)
		if rad, a, c, ok := kl.fx(age); ok {
			step := int(age / killFx * 8) // 8 frames, each drawn once
			px := math.Round(rad * perUnit)
			add(fmt.Sprintf("kfx:%v:%d:%.0f", kl.Headshot, step, px), 3, vx, vy, func() Sprite { return boomSprite(px, math.Max(2, px*0.3), a, false, c) })
		}
		if kl.Headshot && age < hsShow {
			add("hs", 6, vx+m.dot*1.6, vy, func() Sprite { return textSprite("HS", hsC, m.text) })
		}
		if kl.From == nil {
			continue
		}
		ax, ay := at(*kl.From)
		bx, by := at(kl.To)
		dx, dy := math.Round(bx-ax), math.Round(by-ay)
		add(fmt.Sprintf("kill:%d:%.0f:%.0f", i, dx, dy), 2, ax, ay, func() Sprite { return lineSprite(dx, dy, m.stroke, red, 0.9) })
	}
	for _, s := range o.Sites {
		px, py := r.NormPx(s.X, s.Y)
		add("site:"+s.Name, 3, px*sx, py*sy, func() Sprite { return textSprite(s.Name, siteC, m.text+1) })
	}
	if st == nil {
		return out
	}
	for _, u := range st.Utility {
		if col, ok := nadeC[u.Kind]; ok && here(u.Pos) {
			x, y := at(u.Pos)
			add("nade:"+string(u.Kind), 4, x, y, func() Sprite { return discSprite(m.dot*0.35, col, 1) })
		}
	}
	if b := st.Bomb; b != nil && here(b.Pos) {
		x, y := at(b.Pos)
		dropped := b.State == gotv.BombDropped
		add(fmt.Sprintf("bomb:%v", dropped), 4, x, y, func() Sprite { return bombSprite(m, dropped) })
	}
	for _, pl := range st.Players {
		if !pl.Alive && pl.Pos != nil && here(*pl.Pos) {
			x, y := at(*pl.Pos)
			add("dead:"+string(pl.Side), 5, x, y, func() Sprite { return deadSprite(m, teamColor(pl.Side)) })
		}
	}
	toCell := func(v gotv.Vec) (int, int) {
		px, py := r.ToPx(v.X, v.Y)
		return int(math.Floor(px / 2)), int(math.Floor(py / 4))
	}
	labels := playerLabels(st.Players, o, here, toCell, CW, maxName)
	for _, pl := range st.Players {
		if !pl.Alive || pl.Pos == nil || !here(*pl.Pos) {
			continue
		}
		x, y := at(*pl.Pos)
		dir := ((int(math.Round(pl.Yaw/22.5)) % 16) + 16) % 16 // 16 directions
		side, label, bomb := pl.Side, strings.TrimSpace(labels[pl.Name]), pl.HasBomb
		focus := strings.ToLower(pl.Name) == o.Focus
		add(fmt.Sprintf("arrow:%s:%d", side, dir), 6, x, y, func() Sprite { return arrowSprite(m, teamColor(side), dir) })
		if o.Floor.Raised(*pl.Pos) {
			add("raised", 6, x, y-m.dot*2.2, func() Sprite { return raisedSprite(m) })
		}
		blind := math.Ceil(math.Min(pl.Blind, 1.5)/1.5*4) / 4 // 4 steps, so few sprites
		team := blinded(teamColor(side), blind*1.5)
		add(fmt.Sprintf("player:%s:%s:%v:%v:%.2f", side, label, bomb, focus, blind), 7, x, y, func() Sprite { return playerSprite(m, team, label, bomb, focus) })
	}
	return out
}

// metrics sizes markers from the cell height so they look the same at any font size.
type sizes struct {
	dot, stroke float64
	text        int
}

func metrics(cellH int) sizes {
	return sizes{dot: math.Max(4, float64(cellH)*0.3), stroke: math.Max(1.5, float64(cellH)/14), text: max(1, int(math.Round(float64(cellH)/16)))}
}

// boomSprite is the shock-wave ring (with the white flash inside at first).
func boomSprite(r, w, a float64, flash bool, col term.RGB) Sprite {
	n := int(math.Ceil(r+w))*2 + 2
	c := newCanvas(n, n)
	h := float64(n) / 2
	if flash {
		c.disc(h, h, r, term.White, a*0.8)
	}
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			d := math.Hypot(float64(x)+0.5-h, float64(y)+0.5-h)
			c.blend(x, y, col, a*math.Max(0, 1-math.Abs(d-r)/w))
		}
	}
	return Sprite{c.img, n / 2, n / 2}
}

func discSprite(r float64, col term.RGB, a float64) Sprite {
	n := int(math.Ceil(r))*2 + 2
	c := newCanvas(n, n)
	c.disc(float64(n)/2, float64(n)/2, r, col, a)
	return Sprite{c.img, n / 2, n / 2}
}

// lineSprite runs from its anchor to (dx, dy).
func lineSprite(dx, dy, w float64, col term.RGB, a float64) Sprite {
	pad := int(math.Ceil(w)) + 1
	minX, minY := int(math.Min(0, dx))-pad, int(math.Min(0, dy))-pad
	c := newCanvas(int(math.Abs(dx))+2*pad+1, int(math.Abs(dy))+2*pad+1)
	ox, oy := float64(-minX), float64(-minY)
	c.line(ox, oy, ox+dx, oy+dy, w, col, a)
	return Sprite{c.img, -minX, -minY}
}

func textSprite(s string, col term.RGB, k int) Sprite {
	w := font.MeasureString(basicfont.Face7x13, s).Ceil()
	c := newCanvas(w*k, 13*k)
	c.text(0, 0, s, col, k)
	return Sprite{c.img, w * k / 2, 13 * k / 2}
}

func bombSprite(m sizes, dropped bool) Sprite {
	col := red
	if dropped {
		col = term.RGB{200, 200, 200}
	}
	n := int(m.dot*1.2) + 2
	c := newCanvas(n, n)
	c.disc(float64(n)/2, float64(n)/2, m.dot*0.55, term.Black, 0.8)
	c.disc(float64(n)/2, float64(n)/2, m.dot*0.4, col, 1)
	return Sprite{c.img, n / 2, n / 2}
}

func deadSprite(m sizes, team term.RGB) Sprite {
	d := m.dot * 0.6
	n := int(2*d+2*m.stroke) + 2
	c := newCanvas(n, n)
	h := float64(n) / 2
	col := term.Mix(team, term.Black, 0.2)
	c.line(h-d, h-d, h+d, h+d, m.stroke, col, 1)
	c.line(h-d, h+d, h+d, h-d, m.stroke, col, 1)
	return Sprite{c.img, n / 2, n / 2}
}

func arrowSprite(m sizes, team term.RGB, dir int) Sprite {
	l := m.dot * 2.1
	n := int(2*l+2*m.stroke) + 2
	c := newCanvas(n, n)
	h := float64(n) / 2
	a := float64(dir) * math.Pi / 8 // 0 = east, counter-clockwise, screen y is down
	c.line(h, h, h+math.Cos(a)*l, h-math.Sin(a)*l, m.stroke, team, 1)
	return Sprite{c.img, n / 2, n / 2}
}

// raisedSprite is the white ▴ above a player standing above the ground.
func raisedSprite(m sizes) Sprite {
	s := m.dot * 0.8
	n := int(2*s+2*m.stroke) + 2
	c := newCanvas(n, n)
	h := float64(n) / 2
	c.line(h-s, h+s/2, h, h-s/2, m.stroke, term.White, 1)
	c.line(h, h-s/2, h+s, h+s/2, m.stroke, term.White, 1)
	return Sprite{c.img, n / 2, n / 2}
}

// playerSprite is the dot (red ring for the bomb carrier) with the number inside, or
// with a name tag on its right.
func playerSprite(m sizes, team term.RGB, label string, bomb, focus bool) Sprite {
	k := m.text
	named := len([]rune(label)) > 1
	r := m.dot + m.stroke
	if focus {
		r += 1.5 * m.stroke // room for the halo
	}
	tagW := 0
	if named {
		tagW = font.MeasureString(basicfont.Face7x13, label).Ceil()*k + 4*k
	}
	d := int(math.Ceil(2*r)) + 2
	w, h := d+tagW+int(2*m.stroke), max(d, 13*k)
	c := newCanvas(w, h)
	cx, cy := float64(d)/2, float64(h)/2
	if focus { // the followed player: a white halo
		c.disc(cx, cy, r+1.5*m.stroke, term.White, 1)
	}
	c.disc(cx, cy, r, term.Black, 0.85) // outline
	ring := team
	if bomb {
		ring = red
	}
	c.disc(cx, cy, m.dot, ring, 1)
	if bomb {
		c.disc(cx, cy, m.dot-m.stroke, team, 1)
	}
	if !named {
		tw := font.MeasureString(basicfont.Face7x13, label).Ceil() * k
		c.text(int(cx)-tw/2, int(cy)-13*k/2, label, term.Black, k)
		return Sprite{c.img, int(cx), int(cy)}
	}
	tx := d + int(m.stroke)
	c.rect(tx, int(cy)-13*k/2, tagW, 13*k, team, 0.95)
	c.text(tx+2*k, int(cy)-13*k/2, label, term.Black, k)
	return Sprite{c.img, int(cx), int(cy)}
}

type canvas struct{ img *image.NRGBA }

func newCanvas(w, h int) *canvas {
	return &canvas{image.NewNRGBA(image.Rect(0, 0, max(1, w), max(1, h)))}
}

// blend composites colour c at alpha a over pixel (x, y).
func (c *canvas) blend(x, y int, col term.RGB, a float64) {
	if !image.Pt(x, y).In(c.img.Rect) || a <= 0 {
		return
	}
	p := c.img.Pix[c.img.PixOffset(x, y):]
	da := float64(p[3]) / 255
	oa := a + da*(1-a)
	for i := 0; i < 3; i++ {
		p[i] = uint8((float64(col[i])*a + float64(p[i])*da*(1-a)) / oa)
	}
	p[3] = uint8(oa * 255)
}

// disc fills a circle with a one-pixel anti-aliased edge.
func (c *canvas) disc(cx, cy, r float64, col term.RGB, a float64) {
	for y := int(cy - r - 1); y <= int(cy+r+1); y++ {
		for x := int(cx - r - 1); x <= int(cx+r+1); x++ {
			d := math.Hypot(float64(x)+0.5-cx, float64(y)+0.5-cy)
			c.blend(x, y, col, a*math.Min(1, math.Max(0, r-d+0.5)))
		}
	}
}

// line strokes a segment of width w with anti-aliased edges.
func (c *canvas) line(x0, y0, x1, y1, w float64, col term.RGB, a float64) {
	dx, dy := x1-x0, y1-y0
	l2 := dx*dx + dy*dy
	for y := int(math.Min(y0, y1) - w - 1); y <= int(math.Max(y0, y1)+w+1); y++ {
		for x := int(math.Min(x0, x1) - w - 1); x <= int(math.Max(x0, x1)+w+1); x++ {
			px, py := float64(x)+0.5, float64(y)+0.5
			t := 0.0
			if l2 > 0 {
				t = math.Max(0, math.Min(1, ((px-x0)*dx+(py-y0)*dy)/l2))
			}
			d := math.Hypot(px-(x0+t*dx), py-(y0+t*dy))
			c.blend(x, y, col, a*math.Min(1, math.Max(0, w/2-d+0.5)))
		}
	}
}

func (c *canvas) rect(x, y, w, h int, col term.RGB, a float64) {
	for yy := y; yy < y+h; yy++ {
		for xx := x; xx < x+w; xx++ {
			c.blend(xx, yy, col, a)
		}
	}
}

// text draws s with the 7x13 bitmap font scaled k times, top-left at (x, y).
func (c *canvas) text(x, y int, s string, col term.RGB, k int) {
	w := font.MeasureString(basicfont.Face7x13, s).Ceil()
	if w == 0 {
		return
	}
	mask := image.NewAlpha(image.Rect(0, 0, w, 13))
	d := font.Drawer{Dst: mask, Src: image.NewUniform(color.Alpha{255}), Face: basicfont.Face7x13, Dot: fixed.P(0, 11)}
	d.DrawString(s)
	for my := 0; my < 13; my++ {
		for mx := 0; mx < w; mx++ {
			if a := mask.AlphaAt(mx, my).A; a > 0 {
				for j := 0; j < k*k; j++ {
					c.blend(x+mx*k+j%k, y+my*k+j/k, col, float64(a)/255)
				}
			}
		}
	}
}

// Stamp composites sprite s onto dst with its anchor at (x, y), for terminals that
// get the whole frame as one image.
func Stamp(dst *image.NRGBA, s Sprite, x, y float64) {
	c := canvas{dst}
	ox, oy := int(math.Round(x))-s.AX, int(math.Round(y))-s.AY
	b := s.Img.Rect
	for sy := 0; sy < b.Dy(); sy++ {
		for sx := 0; sx < b.Dx(); sx++ {
			p := s.Img.Pix[s.Img.PixOffset(sx, sy):]
			if p[3] > 0 {
				c.blend(ox+sx, oy+sy, term.RGB{p[0], p[1], p[2]}, float64(p[3])/255)
			}
		}
	}
}
