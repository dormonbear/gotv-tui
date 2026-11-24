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

// Base is the static map layer: the radar image itself, two pixels per cell (upper
// and lower half block). nil where the radar is empty, so the terminal shows through.
type Base struct {
	CW, CH   int
	top, bot []*term.RGB
}

// mapAlpha dims the radar so markers stand out on it, like a radar overlay.
const mapAlpha = 0.78

// BuildBase averages the supersampled image into each half cell. A half that is mostly
// empty (transparent, or the dark backdrop of lower-level radars) stays empty.
func BuildBase(r *Raster) *Base {
	W, H, p := r.W, r.H, r.Pixels
	IW := W * SS
	CW, CH := (W+1)/2, (H+3)/4
	b := &Base{CW: CW, CH: CH, top: make([]*term.RGB, CW*CH), bot: make([]*term.RGB, CW*CH)}
	half := func(cx, cy, dy0 int) *term.RGB { // dots dy0, dy0+1 of the cell's 4 rows
		var sum [3]int
		n, all := 0, 0
		for y := (cy*4 + dy0) * SS; y < (cy*4+dy0+2)*SS && y < H*SS; y++ {
			for x := cx * 2 * SS; x < (cx*2+2)*SS && x < IW; x++ {
				all++
				if !IsFloor(p, y*IW+x) {
					continue
				}
				i := (y*IW + x) * 4
				sum[0], sum[1], sum[2] = sum[0]+int(p[i]), sum[1]+int(p[i+1]), sum[2]+int(p[i+2])
				n++
			}
		}
		if n == 0 || n*2 < all {
			return nil
		}
		c := term.Scale(term.RGB{uint8(sum[0] / n), uint8(sum[1] / n), uint8(sum[2] / n)}, mapAlpha)
		return &c
	}
	for cy := 0; cy < CH; cy++ {
		for cx := 0; cx < CW; cx++ {
			b.top[cy*CW+cx] = half(cx, cy, 0)
			b.bot[cy*CW+cx] = half(cx, cy, 2)
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
}

// Draw overlays the markers on this level over the static base, as ANSI rows.
func Draw(r *Raster, base *Base, st *gotv.State, o Opts) []string {
	CW, CH := base.CW, base.CH
	top := append([]*term.RGB(nil), base.top...)
	bot := append([]*term.RGB(nil), base.bot...)
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
				for _, px := range []*[]*term.RGB{&top, &bot} {
					under := term.Black
					if (*px)[i] != nil {
						under = *(*px)[i]
					}
					m := term.Mix(under, c, k)
					(*px)[i] = &m
				}
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
			t, b := top[i], bot[i]
			if o.ImageUnder && !has && !tinted[i] {
				sb.WriteByte(' ') // blank, no background: the image shows through
				continue
			}
			switch {
			case has:
				bg := g.bg
				if bg == nil { // the map under the marker: blend of the two halves
					switch {
					case t != nil && b != nil:
						m := term.Mix(*t, *b, 0.5)
						bg = &m
					case t != nil:
						bg = t
					default:
						bg = b
					}
				}
				if bg != nil {
					sb.WriteString(term.Bg(*bg))
				}
				sb.WriteString(term.Fg(g.fg))
				if g.bold {
					sb.WriteString("\x1b[1m")
				}
				sb.WriteString(g.ch)
				sb.WriteString(term.Reset)
			case t != nil && b != nil:
				sb.WriteString(term.Fg(*t) + term.Bg(*b) + "▀" + term.Reset)
			case t != nil:
				sb.WriteString(term.Fg(*t) + "▀" + term.Reset)
			case b != nil:
				sb.WriteString(term.Fg(*b) + "▄" + term.Reset)
			default:
				sb.WriteByte(' ')
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
