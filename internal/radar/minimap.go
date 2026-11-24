package radar

import (
	"math"
	"strings"
	"time"

	"github.com/dormonbear/gotv-tui/internal/gotv"
	"github.com/dormonbear/gotv-tui/internal/term"
)

type KillMark struct {
	From   *gotv.Vec
	To     gotv.Vec
	Victim gotv.Side
	At     time.Time
}

const (
	killShow = 4 * time.Second
	smokeR   = 144.0 // world units
	fireR    = 120.0
)

var (
	red    = term.RGB{235, 60, 60}
	smokeC = term.RGB{175, 178, 186}
	fireC  = term.RGB{235, 115, 35}
	siteC  = term.RGB{255, 150, 60}
	nadeC  = map[gotv.UtilityKind]term.RGB{gotv.UHE: red, gotv.UFlash: term.White, gotv.UMolotov: fireC, gotv.UDecoy: {120, 200, 120}, gotv.USmokeNade: smokeC}
	dotBit = [4][2]byte{{0x1, 0x8}, {0x2, 0x10}, {0x4, 0x20}, {0x40, 0x80}} // braille bit per (dy, dx)
	arrows = []string{"→", "↗", "↑", "↖", "←", "↙", "↓", "↘"}
)

func teamColor(s gotv.Side) term.RGB {
	if s == gotv.CT {
		return term.CT
	}
	return term.T
}

// IsLower reports whether z is on the lower radar.
func IsLower(z float64, lowerBelow *float64) bool { return lowerBelow != nil && z < *lowerBelow }

// Facing returns the arrow glyph and neighbouring cell offset for a yaw (0 = east, 90 = north).
func Facing(yaw float64) (string, int, int) {
	i := ((int(math.Round(yaw/45)) % 8) + 8) % 8
	a := float64(i) * math.Pi / 4
	return arrows[i], int(math.Round(math.Cos(a))), -int(math.Round(math.Sin(a)))
}

// Base is the static map layer, computed once per radar.
type Base struct {
	CW, CH int
	bg     []*term.RGB
	dots   []byte
	ink    []term.RGB
}

func sat(p []byte, i int) int {
	r, g, b := int(p[i]), int(p[i+1]), int(p[i+2])
	return max(r, g, b) - min(r, g, b)
}

func absd(a, b byte) int {
	if a > b {
		return int(a - b)
	}
	return int(b - a)
}

// BuildBase, on the supersampled image:
//   - roofs (low-saturation grey) covering most of a cell paint it as a light block, so
//     buildings read as solid shapes and small crates (under half a cell) drop out;
//   - other cells take the average floor colour, darkened so markers pop;
//   - braille dots trace the outline / gaps between rooms and saturated painted lines
//     (bombsite / spawn boxes), in their colour.
func BuildBase(r *Raster) *Base {
	W, H, p := r.W, r.H, r.Pixels
	IW, IH := W*SS, H*SS
	floor := func(x, y int) bool { return x >= 0 && y >= 0 && x < IW && y < IH && IsFloor(p, y*IW+x) }
	dist := func(i, j int) int { return absd(p[i], p[j]) + absd(p[i+1], p[j+1]) + absd(p[i+2], p[j+2]) }
	roof := func(i int) bool { return sat(p, i) < 16 && int(p[i])+int(p[i+1])+int(p[i+2]) > 240 } // olive floor ~212, roofs ~280
	nb := [4][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}}
	edge := func(x, y int) bool {
		i := (y*IW + x) * 4
		for _, d := range nb {
			if !floor(x+d[0], y+d[1]) {
				return true
			}
			j := ((y+d[1])*IW + x + d[0]) * 4
			if (sat(p, i) > 60 || sat(p, j) > 60) && dist(i, j) > 60 {
				return true
			}
		}
		return false
	}
	CW, CH := (W+1)/2, (H+3)/4
	b := &Base{CW: CW, CH: CH, bg: make([]*term.RGB, CW*CH), dots: make([]byte, CW*CH), ink: make([]term.RGB, CW*CH)}
	half := SS * SS / 2
	for cy := 0; cy < CH; cy++ {
		for cx := 0; cx < CW; cx++ {
			floorDots, n, rn, en := 0, 0, 0, 0
			var code byte
			var sum, rsum, esum [3]int
			for dy := 0; dy < 4; dy++ {
				for dx := 0; dx < 2; dx++ {
					fp, ep := 0, 0
					for sy := 0; sy < SS; sy++ {
						for sx := 0; sx < SS; sx++ {
							x, y := (cx*2+dx)*SS+sx, (cy*4+dy)*SS+sy
							if !floor(x, y) {
								continue
							}
							i := (y*IW + x) * 4
							fp++
							n++
							for c := 0; c < 3; c++ {
								sum[c] += int(p[i+c])
							}
							if roof(i) {
								rn++
								for c := 0; c < 3; c++ {
									rsum[c] += int(p[i+c])
								}
							}
							if edge(x, y) {
								ep++
								en++
								for c := 0; c < 3; c++ {
									esum[c] += int(p[i+c])
								}
							}
						}
					}
					if fp >= half {
						floorDots++
					}
					if ep > 0 {
						code |= dotBit[dy][dx]
					}
				}
			}
			i := cy*CW + cx
			b.dots[i] = code
			avg := func(s [3]int, k int) term.RGB {
				return term.RGB{uint8(s[0] / k), uint8(s[1] / k), uint8(s[2] / k)}
			}
			switch {
			case rn > 0 && rn*2 >= n:
				c := term.Scale(avg(rsum, rn), 0.95) // building
				b.bg[i] = &c
			case floorDots >= 4:
				c := term.Scale(avg(sum, n), 0.62)
				b.bg[i] = &c
			}
			if en > 0 { // the line's own colour, lifted so it reads on the darkened floor
				b.ink[i] = term.Mix(avg(esum, en), term.White, 0.35)
			}
		}
	}
	return b
}

type glyph struct {
	ch   string
	fg   term.RGB
	bg   *term.RGB
	bold bool
}

type Opts struct {
	Kills      []KillMark
	Nums       map[string]int // lowercased player name -> scoreboard number
	Now        time.Time
	Lower      bool // which level this map shows
	LowerBelow *float64
	Sites      []Site
}

// Draw overlays the markers on this level over the static base, as ANSI rows.
func Draw(r *Raster, base *Base, st *gotv.State, o Opts) []string {
	CW, CH := base.CW, base.CH
	cellBg := append([]*term.RGB(nil), base.bg...)
	toCell := func(v gotv.Vec) (int, int) {
		px, py := r.ToPx(v.X, v.Y)
		return int(math.Floor(px / 2)), int(math.Floor(py / 4))
	}
	here := func(v gotv.Vec) bool { return IsLower(v.Z, o.LowerBelow) == o.Lower }
	x0, _ := r.ToPx(0, 0)
	x1, _ := r.ToPx(1000, 0)
	unit := math.Abs(x1-x0) / 1000 / 2 // cells per world unit
	tint := func(v gotv.Vec, rad float64, c term.RGB, t float64) {
		cx, cy := toCell(v)
		rc := math.Max(1, rad*unit)
		for yy := int(math.Floor(float64(cy) - rc/2)); float64(yy) <= float64(cy)+rc/2; yy++ {
			for xx := int(math.Floor(float64(cx) - rc)); float64(xx) <= float64(cx)+rc; xx++ {
				d := float64((xx-cx)*(xx-cx) + 4*(yy-cy)*(yy-cy))
				if xx < 0 || yy < 0 || xx >= CW || yy >= CH || d > rc*rc {
					continue
				}
				i := yy*CW + xx
				under := term.Black
				if cellBg[i] != nil {
					under = *cellBg[i]
				}
				m := term.Mix(under, c, t*(1-0.35*d/(rc*rc))) // soft edge
				cellBg[i] = &m
			}
		}
	}
	glyphs := map[int]glyph{}
	putCell := func(cx, cy int, g glyph, over bool) {
		if cx < 0 || cx >= CW || cy < 0 || cy >= CH {
			return
		}
		i := cy*CW + cx
		if _, taken := glyphs[i]; taken && !over {
			return
		}
		glyphs[i] = g
	}
	put := func(v gotv.Vec, g glyph, over bool) {
		if here(v) { // the other level is drawn on its own map
			cx, cy := toCell(v)
			putCell(cx, cy, g, over)
		}
	}

	for _, s := range o.Sites {
		px, py := r.NormPx(s.X, s.Y)
		putCell(int(px/2), int(py/4), glyph{ch: s.Name, fg: siteC, bold: true}, true)
	}
	if st != nil {
		for _, u := range st.Utility {
			if !here(u.Pos) {
				continue
			}
			switch u.Kind {
			case gotv.USmoke:
				tint(u.Pos, smokeR, smokeC, 0.75)
			case gotv.UFire:
				tint(u.Pos, fireR, fireC, 0.85)
			}
		}
	}
	for _, k := range o.Kills {
		if o.Now.Sub(k.At) > killShow || k.From == nil {
			continue
		}
		for i := 1; i < 24; i++ {
			t := float64(i) / 24
			put(gotv.Vec{X: k.From.X + (k.To.X-k.From.X)*t, Y: k.From.Y + (k.To.Y-k.From.Y)*t, Z: k.To.Z}, glyph{ch: "·", fg: red, bold: true}, false)
		}
	}
	if st != nil {
		for _, u := range st.Utility {
			if c, ok := nadeC[u.Kind]; ok {
				put(u.Pos, glyph{ch: "•", fg: c, bold: true}, true)
			}
		}
		if b := st.Bomb; b != nil {
			c := red
			if b.State == gotv.BombDropped {
				c = term.RGB{200, 200, 200}
			}
			put(b.Pos, glyph{ch: "B", fg: term.Black, bg: &c, bold: true}, true)
		}
		for _, pl := range st.Players {
			if !pl.Alive && pl.Pos != nil {
				put(*pl.Pos, glyph{ch: "✕", fg: term.Mix(teamColor(pl.Side), term.Black, 0.2), bold: true}, false)
			}
		}
		for _, pl := range st.Players {
			if pl.Alive && pl.Pos != nil && here(*pl.Pos) {
				cx, cy := toCell(*pl.Pos)
				ch, dx, dy := Facing(pl.Yaw)
				putCell(cx+dx, cy+dy, glyph{ch: ch, fg: teamColor(pl.Side), bold: true}, false)
			}
		}
		for _, pl := range st.Players {
			if !pl.Alive || pl.Pos == nil {
				continue
			}
			ch := "●"
			if n, ok := o.Nums[strings.ToLower(pl.Name)]; ok {
				ch = string(rune('0' + n))
			}
			fg := term.Black
			if pl.HasBomb {
				fg = red
			}
			c := teamColor(pl.Side)
			put(*pl.Pos, glyph{ch: ch, fg: fg, bg: &c, bold: true}, true)
		}
	}

	rows := make([]string, CH)
	var sb strings.Builder
	for cy := 0; cy < CH; cy++ {
		sb.Reset()
		for cx := 0; cx < CW; cx++ {
			i := cy*CW + cx
			g, has := glyphs[i]
			b := cellBg[i]
			if has && g.bg != nil {
				b = g.bg
			}
			if b != nil {
				sb.WriteString(term.Bg(*b))
			}
			switch {
			case has:
				sb.WriteString(term.Fg(g.fg))
				if g.bold {
					sb.WriteString("\x1b[1m")
				}
				sb.WriteString(g.ch)
			case base.dots[i] != 0:
				sb.WriteString(term.Fg(base.ink[i]))
				sb.WriteRune(rune(0x2800 + int(base.dots[i])))
			default:
				sb.WriteByte(' ')
			}
			if has || b != nil || base.dots[i] != 0 {
				sb.WriteString(term.Reset)
			}
		}
		rows[cy] = sb.String()
	}
	return rows
}
