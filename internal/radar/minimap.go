package radar

import (
	"math"
	"sort"
	"strings"
	"time"

	"github.com/dormonbear/gotv-tui/internal/gotv"
	"github.com/dormonbear/gotv-tui/internal/term"
)

type KillMark struct {
	From     *gotv.Vec
	To       gotv.Vec
	Victim   gotv.Side
	Headshot bool
	At       time.Time
}

// killFx is the hit ring at the victim, hsShow how long a headshot stays marked.
const killFx, hsShow = 0.6, 1.5

var hsC = term.RGB{255, 215, 80}

// fx is the kill's hit ring at age seconds: world radius, strength, colour (ok=false when over).
func (k KillMark) fx(age float64) (rad, a float64, c term.RGB, ok bool) {
	if age < 0 || age >= killFx {
		return 0, 0, c, false
	}
	t := age / killFx
	c = red
	if k.Headshot {
		c = hsC
	}
	return 40 + 160*t, 0.85 * (1 - t), c, true
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
//
// detail is the zoom: from DetailFrom up, a second, dimmer layer of lines also traces
// where the radar's shading changes (height levels, crates, cover), with a lower
// threshold the further in, so zooming in shows more of the map.
func BuildBase(r *Raster, detail float64) *Base {
	W, H, p := r.W, r.H, r.Pixels
	lum := func(i int) int { return (int(p[i])*30 + int(p[i+1])*59 + int(p[i+2])*11) / 100 }
	step := 1 << 30 // shading change that counts as a detail line; off below DetailFrom
	if detail >= DetailFrom {
		step = int(DetailStep / detail)
	}
	IW, IH := W*SS, H*SS
	floor := func(x, y int) bool { return x >= 0 && y >= 0 && x < IW && y < IH && IsFloor(p, y*IW+x) }
	dist := func(i, j int) int { return absd(p[i], p[j]) + absd(p[i+1], p[j+1]) + absd(p[i+2], p[j+2]) }
	roof := func(i int) bool { return sat(p, i) < 16 && int(p[i])+int(p[i+1])+int(p[i+2]) > 240 } // olive floor ~212, roofs ~280
	nb := [4][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}}
	// edge: 2 = outline, gap between rooms or painted site/spawn box; 1 = detail
	// (shading step); 0 = none
	edge := func(x, y int) int {
		i := (y*IW + x) * 4
		kind := 0
		for _, d := range nb {
			nx, ny := x+d[0], y+d[1]
			if nx < 0 || ny < 0 || nx >= IW || ny >= IH {
				continue // the view's edge isn't a wall (matters when zoomed in)
			}
			if !floor(nx, ny) {
				return 2
			}
			j := ((y+d[1])*IW + x + d[0]) * 4
			if (sat(p, i) > 60 || sat(p, j) > 60) && dist(i, j) > 60 {
				return 2
			}
			// shading steps are compared a dot apart: zoomed in, the radar is
			// up-scaled and a step spreads over several neighbouring samples
			fx, fy := x+d[0]*SS, y+d[1]*SS
			if fx >= 0 && fy >= 0 && fx < IW && fy < IH && floor(fx, fy) {
				if l := lum(i) - lum((fy*IW+fx)*4); l > step || -l > step {
					kind = 1
				}
			}
		}
		return kind
	}
	CW, CH := (W+1)/2, (H+3)/4
	b := &Base{CW: CW, CH: CH, bg: make([]*term.RGB, CW*CH), dots: make([]byte, CW*CH), ink: make([]term.RGB, CW*CH)}
	half := SS * SS / 2
	for cy := 0; cy < CH; cy++ {
		for cx := 0; cx < CW; cx++ {
			floorDots, n, rn, en, dn := 0, 0, 0, 0, 0
			var code byte
			var sum, rsum, esum, dsum [3]int
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
							switch edge(x, y) {
							case 2:
								ep++
								en++
								for c := 0; c < 3; c++ {
									esum[c] += int(p[i+c])
								}
							case 1:
								ep++
								dn++
								for c := 0; c < 3; c++ {
									dsum[c] += int(p[i+c])
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
			switch {
			case en > 0: // the line's own colour, lifted so it reads on the darkened floor
				b.ink[i] = term.Mix(avg(esum, en), term.White, 0.35)
			case dn > 0: // detail: fainter, so the outline still reads first
				b.ink[i] = term.Mix(avg(dsum, dn), term.White, 0.12)
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
	ImageUnder bool   // the terminal shows the radar image under blank cells
	Names      bool   // label players by name where it fits (zoomed in)
	Focus      string // followed player (lowercase): drawn highlighted
	Boom       *Boom  // a C4 explosion in progress
}

// Draw overlays the markers on this level over the static base, as ANSI rows.
func Draw(r *Raster, base *Base, st *gotv.State, o Opts) []string {
	CW, CH := base.CW, base.CH
	cellBg := append([]*term.RGB(nil), base.bg...)
	tinted := make([]bool, CW*CH)
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
		for yy := int(math.Floor(float64(cy) - rc/r.Aspect)); float64(yy) <= float64(cy)+rc/r.Aspect; yy++ {
			for xx := int(math.Floor(float64(cx) - rc)); float64(xx) <= float64(cx)+rc; xx++ {
				dy := float64(yy-cy) * r.Aspect // rows are taller than columns are wide
				d := float64((xx-cx)*(xx-cx)) + dy*dy
				if xx < 0 || yy < 0 || xx >= CW || yy >= CH || d > rc*rc {
					continue
				}
				i := yy*CW + xx
				tinted[i] = true              // drawn as text: the image can't be tinted per frame
				k := t * (1 - 0.35*d/(rc*rc)) // soft edge
				under := term.Black
				if cellBg[i] != nil {
					under = *cellBg[i]
				}
				m := term.Mix(under, c, k)
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
	if b := o.Boom; b != nil && here(b.Pos) && b.Age < BoomShow {
		cx, cy := toCell(b.Pos)
		rad, a, flash := b.ring()
		rc := math.Max(1, rad*unit)
		w := math.Max(1, rc*0.18) // ring thickness, cells
		for yy := int(float64(cy) - (rc+w)/r.Aspect); float64(yy) <= float64(cy)+(rc+w)/r.Aspect; yy++ {
			for xx := int(float64(cx) - rc - w); float64(xx) <= float64(cx)+rc+w; xx++ {
				if xx < 0 || yy < 0 || xx >= CW || yy >= CH {
					continue
				}
				d := math.Hypot(float64(xx-cx), float64(yy-cy)*r.Aspect)
				k := a * math.Max(0, 1-math.Abs(d-rc)/w) // brightest on the wave front
				c := boomC
				if flash && d < rc {
					c, k = term.White, a
				}
				if k <= 0 {
					continue
				}
				i := yy*CW + xx
				tinted[i] = true
				under := term.Black
				if cellBg[i] != nil {
					under = *cellBg[i]
				}
				m := term.Mix(under, c, k)
				cellBg[i] = &m
			}
		}
	}
	lineBits := map[int]byte{} // kill lines as braille dots: a thin line, not a staircase of cells
	for _, k := range o.Kills {
		if o.Now.Sub(k.At) > killShow || !here(k.To) {
			continue
		}
		if k.From != nil {
			ax, ay := r.ToPx(k.From.X, k.From.Y)
			bx, by := r.ToPx(k.To.X, k.To.Y)
			n := int(math.Max(math.Abs(bx-ax), math.Abs(by-ay)))
			for i := 2; i < n-1; i++ { // leave the two ends to the player and the cross
				t := float64(i) / float64(n)
				x, y := int(math.Floor(ax+(bx-ax)*t)), int(math.Floor(ay+(by-ay)*t))
				if x < 0 || y < 0 || x/2 >= CW || y/4 >= CH {
					continue
				}
				lineBits[(y/4)*CW+x/2] |= dotBit[y%4][x%2]
			}
		}
		age := o.Now.Sub(k.At).Seconds()
		if rad, a, c, ok := k.fx(age); ok {
			tint(k.To, rad, c, a)
		}
		if k.Headshot && age < hsShow {
			cx, cy := toCell(k.To)
			putCell(cx, cy, glyph{ch: "✸", fg: hsC, bold: true}, true)
			putCell(cx+1, cy, glyph{ch: "H", fg: hsC, bold: true}, true)
			putCell(cx+2, cy, glyph{ch: "S", fg: hsC, bold: true}, true)
		}
	}
	for i, b := range lineBits {
		putCell(i%CW, i/CW, glyph{ch: string(rune(0x2800 + int(b))), fg: red, bold: true}, false)
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
			put(b.Pos, glyph{ch: "✱", fg: c, bold: true}, true) // not "B": reads as the B site label
		}
		for _, pl := range st.Players {
			if !pl.Alive && pl.Pos != nil {
				put(*pl.Pos, glyph{ch: "✕", fg: term.Mix(teamColor(pl.Side), term.Black, 0.2), bold: true}, false)
			}
		}
		labels := playerLabels(st.Players, o, here, toCell, CW)
		for _, pl := range st.Players {
			if pl.Alive && pl.Pos != nil && here(*pl.Pos) {
				cx, cy := toCell(*pl.Pos)
				ch, dx, dy := Facing(pl.Yaw)
				if w := len([]rune(labels[pl.Name])); w > 1 && dx > 0 {
					cx += w - 1 // facing right: the arrow goes after the name
				}
				putCell(cx+dx, cy+dy, glyph{ch: ch, fg: teamColor(pl.Side), bold: true}, false)
			}
		}
		for _, pl := range st.Players {
			if !pl.Alive || pl.Pos == nil || !here(*pl.Pos) {
				continue
			}
			fg := term.Black
			if pl.HasBomb {
				fg = red
			}
			c := teamColor(pl.Side)
			if strings.ToLower(pl.Name) == o.Focus {
				c = term.White // the followed player stands out
			}
			cx, cy := toCell(*pl.Pos)
			for i, r := range []rune(labels[pl.Name]) {
				putCell(cx+i, cy, glyph{ch: string(r), fg: fg, bg: &c, bold: true}, true)
			}
		}
	}

	rows := make([]string, CH)
	var sb strings.Builder
	for cy := 0; cy < CH; cy++ {
		sb.Reset()
		for cx := 0; cx < CW; cx++ {
			i := cy*CW + cx
			g, has := glyphs[i]
			if o.ImageUnder && !has && !tinted[i] {
				sb.WriteByte(' ') // blank, no background: the image shows through
				continue
			}
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
			case base.dots[i] != 0: // outline: braille dots in the line's colour
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

// maxName is the longest name shown on the map; longer ones are cut.
const maxName = 10

// playerLabels picks what marks each alive player on this level: the name when
// Opts.Names is on and it fits without touching another player or label, else the
// scoreboard number. Players are labelled left to right so the result is stable.
func playerLabels(players []gotv.Player, o Opts, here func(gotv.Vec) bool, toCell func(gotv.Vec) (int, int), CW int) map[string]string {
	type mark struct {
		name   string
		cx, cy int
	}
	var ms []mark
	for _, pl := range players {
		if pl.Alive && pl.Pos != nil && here(*pl.Pos) {
			cx, cy := toCell(*pl.Pos)
			ms = append(ms, mark{pl.Name, cx, cy})
		}
	}
	sort.Slice(ms, func(i, j int) bool { return ms[i].cx < ms[j].cx })
	taken := map[[2]int]bool{}
	for _, m := range ms {
		taken[[2]int{m.cx, m.cy}] = true
	}
	out := map[string]string{}
	for _, m := range ms {
		num := "●"
		if n, ok := o.Nums[strings.ToLower(m.name)]; ok {
			num = string(rune('0' + n))
		}
		out[m.name] = num
		if !o.Names {
			continue
		}
		name := []rune(m.name)
		if len(name) > maxName {
			name = name[:maxName]
		}
		label := " " + string(name) + " "
		w := len([]rune(label))
		if m.cx+w > CW {
			continue
		}
		fits := true
		for x := m.cx + 1; x < m.cx+w+1 && fits; x++ { // +1: a gap before the next mark
			fits = !taken[[2]int{x, m.cy}]
		}
		if !fits {
			continue
		}
		for x := m.cx; x < m.cx+w; x++ {
			taken[[2]int{x, m.cy}] = true
		}
		out[m.name] = label
	}
	return out
}

// Boom is a C4 explosion at Pos, Age seconds ago.
type Boom struct {
	Pos gotv.Vec
	Age float64
}

const (
	BoomShow = 1.5   // seconds the explosion is shown
	boomMax  = 1100. // world units the shock wave reaches
)

var boomC = term.RGB{255, 120, 30}

// ring is the shock wave now: radius (world units), opacity, and whether the
// first white flash is still on.
func (b *Boom) ring() (float64, float64, bool) {
	p := math.Min(1, b.Age/BoomShow)
	out := 1 - (1-p)*(1-p) // fast, then slowing down
	return 120 + (boomMax-120)*out, 1 - p, b.Age < 0.2
}

// Detail lines (see BuildBase) appear from DetailFrom zoom, at shading steps of
// DetailStep/zoom luminance: 2x ~ 45, 4x ~ 22.
const (
	DetailFrom = 1.5
	DetailStep = 90.0
)
