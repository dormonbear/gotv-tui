package audio

// Reading a broadcast's round clock ("01:13") from its picture, without an OCR
// tool. Ported from Fy1Sync (https://github.com/Fy1ng/Fy1Sync, GPL-3.0, commit
// 269cb3d): the glyph priors (glyphpriors.json), glyph normalisation to 32x48,
// hole topology, and splitting touching glyphs at projection valleys.

import (
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"image"
	"math"
	"slices"
	"sort"
)

const (
	inkLevel   = 200 // the clock is white; score digits and backdrops are greyer
	mw, mh     = 32, 48
	minScore   = 0.72 // IoU against the best prior
	minLead    = 0.03 // over the best other character
	sessionMin = 0.80 // IoU against a glyph learned from this stream
)

type mask [mw * mh]bool

//go:embed glyphpriors.json
var priorsJSON []byte

type prior struct {
	char  byte
	m     mask
	holes int
}

var priors = loadPriors()

func loadPriors() []prior {
	var raw map[string][]string
	if err := json.Unmarshal(priorsJSON, &raw); err != nil {
		panic(err)
	}
	var out []prior
	for ch, list := range raw {
		for _, enc := range list {
			b, err := base64.StdEncoding.DecodeString(enc)
			if err != nil || len(b)*8 < mw*mh {
				continue
			}
			var m mask
			for i := range m {
				m[i] = b[i>>3]&(0x80>>(i&7)) != 0
			}
			out = append(out, prior{ch[0], m, holes(&m)})
		}
	}
	return out
}

type box struct{ x0, y0, x1, y1 int }

func (b box) w() int { return b.x1 - b.x0 + 1 }
func (b box) h() int { return b.y1 - b.y0 + 1 }

// clockReader reads the clock frame by frame. The priors only find the clock:
// once two reads agree with its pace, the clock's place on screen is locked and
// each digit slot is read in place. The clock's pace then predicts each frame's
// value, which labels glyphs the priors couldn't read, so this stream's own font
// is learned within seconds.
type clockReader struct {
	session []prior
	lay     *layout
	misses  int // frames in a row without a read while locked
	lastPts float64
	lastV   int
	hasLast bool
	jump    *[2]float64 // a read that broke the pace, kept until the next confirms it
	seen    []token     // the last frame's colon and digit glyphs, to keep as variants
}

// keep adds the glyphs of a frame read as v as more variants of their characters
// (compression makes each frame's glyphs a little different).
func (c *clockReader) keep(v int) {
	d := clockDigits(v, len(c.lay.digits))
	for _, t := range c.seen {
		if t.char == ':' {
			c.learn(':', t.m)
		} else if int(t.char) < len(d) {
			c.learn(byte('0'+d[t.char]), t.m)
		}
	}
}

// layout is where the clock sits: its line and the centre of each slot (digits
// left to right, the colon between the minutes and the seconds).
type layout struct {
	y0, y1 int
	digits []int
	colon  int
	pitch  int // half the distance between digit centres
}

const (
	sessionCap = 12  // templates kept per character
	relock     = 120 // missed frames (about 2 minutes of key frames) before searching again
)

// read returns the clock at stream time pts.
func (c *clockReader) read(pts float64, img *image.Gray) (int, bool) {
	if c.lay != nil {
		v, ok := c.readLocked(pts, img)
		if ok {
			c.keep(v)
			c.misses = 0
			c.lastPts, c.lastV, c.hasLast = pts, v, true
			return v, true
		}
		if c.misses++; c.misses > relock {
			c.lay, c.hasLast = nil, false
		}
		return 0, false
	}
	v, toks, ok := readClock(img, c.session)
	if !ok {
		return 0, false
	}
	fits := c.fits(pts, v)
	c.lastPts, c.lastV, c.hasLast = pts, v, true
	if !fits {
		return 0, false
	}
	for _, t := range toks {
		c.learn(t.char, t.m)
	}
	c.lay = newLayout(img, toks)
	return v, true
}

// expect is what the clock may show at pts, going by the last read. The HUD's
// ticks don't keep exact time with the video, so a second either way is allowed.
func (c *clockReader) expect(pts float64) []int {
	if !c.hasLast || pts < c.lastPts || pts-c.lastPts > 10 {
		return nil
	}
	v := c.lastV - int(math.Round(pts-c.lastPts))
	return []int{v - 1, v, v + 1}
}

// pick decides which of digits an unread glyph is: digits whose shape has a
// different number of holes are out, so are digits this stream's glyph for is
// already known (it would have matched), and of what is left the priors must
// clearly prefer one.
func (c *clockReader) pick(m mask, digits []int) (int, bool) {
	nh := holes(&m)
	var left []int
	for _, d := range digits {
		ch := byte('0' + d)
		if !slices.ContainsFunc(priors, func(p prior) bool { return p.char == ch && p.holes == nh }) ||
			slices.ContainsFunc(c.session, func(p prior) bool { return p.char == ch }) {
			continue
		}
		left = append(left, d)
	}
	switch len(left) {
	case 0:
		return 0, false
	case 1:
		return left[0], true
	}
	score := func(d int) float64 {
		best := 0.0
		for i := range priors {
			if priors[i].char == byte('0'+d) {
				best = max(best, iou(&m, &priors[i].m))
			}
		}
		return best
	}
	slices.SortFunc(left, func(a, b int) int { return cmpF(score(b), score(a)) })
	if score(left[0])-score(left[1]) < 0.05 {
		return 0, false
	}
	return left[0], true
}

func cmpF(a, b float64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// newLayout records the slots of a confirmed read; a lone minute digit gets a
// slot for the leading zero the priors may have missed.
func newLayout(img *image.Gray, toks []token) *layout {
	l := &layout{y0: toks[0].b.y0, y1: toks[0].b.y1}
	var xs []int
	for _, t := range toks {
		l.y0, l.y1 = min(l.y0, t.b.y0), max(l.y1, t.b.y1)
		cx := (t.b.x0 + t.b.x1) / 2
		if t.char == ':' {
			l.colon = cx
		} else {
			xs = append(xs, cx)
		}
	}
	l.pitch = max(4, (xs[len(xs)-1]-xs[len(xs)-2])/2)
	if len(xs) == 3 {
		xs = append([]int{xs[0] - 2*l.pitch}, xs...)
	}
	l.digits = xs
	_ = img
	return l
}

// cells cuts the clock's span into exactly its glyphs: the minute digits, the
// colon, two second digits. HUD fonts are proportional and bold digits touch, so
// the cuts are chosen together (least ink cut through) such that every piece has
// a plausible width for what it must be.
func (l *layout) cells(img *image.Gray) (colon box, digits []box, ok bool) {
	h := l.y1 - l.y0 + 1
	L := max(0, l.digits[0]-l.pitch*13/10)
	R := min(img.Rect.Dx(), l.digits[len(l.digits)-1]+l.pitch*13/10+1)
	n := R - L
	ink := make([]int, n)
	for x := range ink {
		ink[x] = colInk(img, L+x, l.y0, l.y1)
	}
	first, last := make([]int, n+1), make([]int, n+1) // first inked column >= x, last inked < x
	first[n] = n
	for x := n - 1; x >= 0; x-- {
		if first[x] = first[x+1]; ink[x] > 0 {
			first[x] = x
		}
	}
	last[0] = -1
	for x := 1; x <= n; x++ {
		if last[x] = last[x-1]; ink[x-1] > 0 {
			last[x] = x - 1
		}
	}
	nl := len(l.digits) - 2
	kinds := make([]bool, 0, nl+3) // true: the colon
	for i := 0; i < nl; i++ {
		kinds = append(kinds, false)
	}
	kinds = append(kinds, true, false, false)
	fits := func(colon bool, s, e int) bool { // columns s..e-1
		a, b := first[s], last[e]
		if a >= e || b < s {
			return false
		}
		w := b - a + 1
		if colon {
			return w*100 >= h*8 && w*100 <= h*35
		}
		return w*100 >= h*25 && w*100 <= h*90
	}
	const inf = 1 << 30
	cost := make([][]int, len(kinds)) // cost[k][e]: pieces 0..k placed, piece k ends before e
	from := make([][]int, len(kinds))
	for k := range kinds {
		cost[k], from[k] = make([]int, n+1), make([]int, n+1)
		for e := range cost[k] {
			cost[k][e] = inf
		}
	}
	for e := 1; e <= n; e++ {
		if fits(kinds[0], 0, e) {
			cost[0][e] = 0
		}
	}
	for k := 1; k < len(kinds); k++ {
		for s := 1; s < n; s++ {
			if cost[k-1][s] == inf {
				continue
			}
			for e := s + 1; e <= n; e++ {
				if c := cost[k-1][s] + ink[s]; c < cost[k][e] && fits(kinds[k], s, e) {
					cost[k][e], from[k][e] = c, s
				}
			}
		}
	}
	end := len(kinds) - 1
	if cost[end][n] == inf {
		return box{}, nil, false
	}
	var pieces []box
	for k, e := end, n; k >= 0; k-- {
		s := 0
		if k > 0 {
			s = from[k][e]
		}
		b, ok := trim(img, box{L + s, l.y0, L + e - 1, l.y1})
		if !ok {
			return box{}, nil, false
		}
		pieces = append([]box{b}, pieces...)
		e = s
	}
	return pieces[nl], append(pieces[:nl:nl], pieces[nl+1:]...), true
}

// readLocked reads the clock in place: every slot it can classify must agree with
// one of the values the clock's pace allows, or (without a prediction) all must read.
func (c *clockReader) readLocked(pts float64, img *image.Gray) (int, bool) {
	l := c.lay
	h := l.y1 - l.y0 + 1
	b, cells, ok := l.cells(img)
	if !ok {
		return 0, false // the clock is hidden (bomb, round end banner)
	}
	ch, cm, ok := classify(img, b, priors, c.session)
	if !ok || ch != ':' {
		return 0, false
	}
	c.seen = c.seen[:0]
	c.seen = append(c.seen, token{char: ':', m: cm})
	type seen struct {
		ch   byte
		ok   bool
		m    mask
		real bool // a digit-sized glyph is there
	}
	slots := make([]seen, len(l.digits))
	for i, b := range cells {
		if b.h()*4 < h*3 {
			continue
		}
		ch, m, ok := classify(img, b, nil, c.session)
		slots[i] = seen{ch, ok && ch != ':', m, true}
		c.seen = append(c.seen, token{char: byte(i), m: m}) // char: the slot, until the value is known
	}
	cands := c.expect(pts)
	known := 0
	for _, s := range slots {
		if s.ok {
			known++
		}
	}
	match := func(v int) bool {
		if v < 0 || v > 115 {
			return false
		}
		d := clockDigits(v, len(slots))
		for i, s := range slots {
			if s.ok && s.ch-'0' != byte(d[i]) {
				return false
			}
		}
		return true
	}
	if known == len(slots) {
		v := 0
		for _, sl := range slots {
			v = v*10 + int(sl.ch-'0')
		}
		v = v/100*60 + v%100
		if len(cands) == 0 || slices.Contains(cands, v) {
			c.jump = nil
			return v, true
		}
		// off the pace: a new round or a pause, or a misread. Believe it once the
		// next frame keeps pace with it.
		j := c.jump
		c.jump = &[2]float64{pts, float64(v)}
		if j != nil && pts-j[0] <= 10 && math.Abs(j[1]-float64(v)-(pts-j[0])) <= 1.5 {
			c.jump = nil
			return v, true
		}
		return 0, false
	}
	if known < 2 {
		return 0, false
	}
	var fit []int
	for _, v := range cands {
		if match(v) {
			fit = append(fit, v)
		}
	}
	if len(fit) == 0 {
		return 0, false
	}
	for i, sl := range slots { // work out each unread glyph among what fits
		if !sl.real || sl.ok {
			continue
		}
		var ds []int
		for _, v := range fit {
			if d := clockDigits(v, len(slots))[i]; !slices.Contains(ds, d) {
				ds = append(ds, d)
			}
		}
		d, ok := c.pick(sl.m, ds)
		if !ok {
			return 0, false
		}
		c.learn(byte('0'+d), sl.m)
		slots[i].ch, slots[i].ok = byte('0'+d), true
	}
	var still []int
	for _, v := range fit {
		if match(v) {
			still = append(still, v)
		}
	}
	if len(still) != 1 {
		return 0, false
	}
	return still[0], true
}

// clockDigits writes seconds as the HUD's digits: 73 -> [0 1 1 3] (n=4) or [1 1 3].
func clockDigits(secs, n int) []int {
	m, s := secs/60, secs%60
	d := []int{m / 10, m % 10, s / 10, s % 10}
	return d[4-n:]
}

func (c *clockReader) learn(ch byte, m mask) {
	n := 0
	for i := range c.session {
		if c.session[i].char != ch {
			continue
		}
		if iou(&c.session[i].m, &m) >= 0.95 {
			return // already known
		}
		n++
	}
	if n < sessionCap {
		c.session = append(c.session, prior{ch, m, holes(&m)})
	}
}

// readClock finds the clock in a HUD strip and returns its value in seconds.
func readClock(img *image.Gray, session []prior) (int, []token, bool) {
	tried := map[[2]int]bool{}
	for _, b := range blobs(img) { // each tall bright blob proposes a text line
		if b.h() < 20 || b.h() > 120 {
			continue
		}
		key := [2]int{b.y0 / 3, b.y1 / 3}
		if tried[key] {
			continue
		}
		tried[key] = true
		if v, toks, ok := clockIn(img, b.y0, b.y1, session); ok {
			return v, toks, true
		}
	}
	return 0, nil, false
}

type token struct {
	char byte
	b    box
	m    mask
}

// clockIn reads the line y0..y1 as characters and looks for d(d):dd.
func clockIn(img *image.Gray, y0, y1 int, session []prior) (int, []token, bool) {
	h := y1 - y0 + 1
	var toks []token
	for _, s := range segments(img, y0, y1) {
		toks = append(toks, readSegment(img, s, h, session)...)
	}
	near := func(a, b token) bool { return b.b.x0-a.b.x1 <= h*3/5 }
	isD := func(t token) bool { return t.char >= '0' && t.char <= '9' }
	for i := 1; i+2 < len(toks); i++ {
		if toks[i].char != ':' || !isD(toks[i-1]) || !isD(toks[i+1]) || !isD(toks[i+2]) ||
			!near(toks[i-1], toks[i]) || !near(toks[i], toks[i+1]) || !near(toks[i+1], toks[i+2]) {
			continue
		}
		m, used := int(toks[i-1].char-'0'), toks[i-1:i+3]
		if i >= 2 && isD(toks[i-2]) && near(toks[i-2], toks[i-1]) {
			m += 10 * int(toks[i-2].char-'0')
			used = toks[i-2 : i+3]
		}
		s := int(toks[i+1].char-'0')*10 + int(toks[i+2].char-'0')
		if s > 59 || m*60+s > 115 { // CS2 round clocks never pass 1:55
			continue
		}
		return m*60 + s, used, true
	}
	return 0, nil, false
}

// segments cuts the line into runs of inked columns, each trimmed to its ink.
func segments(img *image.Gray, y0, y1 int) []box {
	var out []box
	W := img.Rect.Dx()
	for x := 0; x < W; {
		if colInk(img, x, y0, y1) == 0 {
			x++
			continue
		}
		x0 := x
		for x < W && colInk(img, x, y0, y1) > 0 {
			x++
		}
		if b, ok := trim(img, box{x0, y0, x - 1, y1}); ok {
			out = append(out, b)
		}
	}
	return out
}

// readSegment recognises a segment as one character, or splits touching
// characters at projection valleys (up to three pieces).
func readSegment(img *image.Gray, s box, lineH int, session []prior) []token {
	if c, m, ok := classify(img, s, priors, session); ok && s.w() <= lineH {
		return []token{{c, s, m}}
	}
	if s.w() < lineH/2 || s.w() > lineH*3 {
		return nil
	}
	proj := make([]int, s.w())
	for x := range proj {
		proj[x] = colInk(img, s.x0+x, s.y0, s.y1)
	}
	maxValley := max(1, s.h()/4)
	var cuts []int
	for c := 2; c < s.w()-2; c++ {
		if min(proj[c-1], proj[c]) <= maxValley {
			cuts = append(cuts, c)
		}
	}
	sort.Slice(cuts, func(i, j int) bool {
		return min(proj[cuts[i]-1], proj[cuts[i]]) < min(proj[cuts[j]-1], proj[cuts[j]])
	})
	if len(cuts) > 12 {
		cuts = cuts[:12]
	}
	piece := func(a, b int) (token, bool) {
		bx, ok := trim(img, box{s.x0 + a, s.y0, s.x0 + b - 1, s.y1})
		if !ok || bx.w() > lineH {
			return token{}, false
		}
		c, m, ok := classify(img, bx, priors, session)
		return token{c, bx, m}, ok
	}
	for _, c := range cuts { // two pieces
		if l, ok := piece(0, c); ok {
			if r, ok := piece(c, s.w()); ok {
				return []token{l, r}
			}
		}
	}
	for _, c1 := range cuts { // three pieces
		for _, c2 := range cuts {
			if c2 <= c1+2 {
				continue
			}
			a, ok1 := piece(0, c1)
			b, ok2 := piece(c1, c2)
			c, ok3 := piece(c2, s.w())
			if ok1 && ok2 && ok3 {
				return []token{a, b, c}
			}
		}
	}
	return nil
}

// classify matches a glyph against the priors: best IoU, clear of the runner-up
// character, and with the same number of enclosed holes.
// Priors (another font's digits) pass at minScore; this stream's own learned
// glyphs must match closely.
func classify(img *image.Gray, b box, fonts, session []prior) (byte, mask, bool) {
	m := normalise(img, b)
	nh := holes(&m)
	best := map[byte]float64{}
	for k, set := range [][]prior{fonts, session} {
		for i := range set {
			p := &set[i]
			s := iou(&m, &p.m)
			if k == 1 { // map a learned glyph's score onto the priors' scale
				s -= sessionMin - minScore
			}
			if s > best[p.char] && (p.holes == nh || p.char == ':') {
				best[p.char] = s
			}
		}
	}
	var top, second float64
	var ch byte
	for c, s := range best {
		if s > top {
			top, second, ch = s, top, c
		} else if s > second {
			second = s
		}
	}
	if ch == ':' {
		return ch, m, top >= minScore
	}
	return ch, m, top >= minScore && top-second >= minLead
}

// trim shrinks b to the ink inside it.
func trim(img *image.Gray, b box) (box, bool) {
	out := box{b.x1, b.y1, b.x0, b.y0}
	for y := b.y0; y <= b.y1; y++ {
		for x := b.x0; x <= b.x1; x++ {
			if img.Pix[y*img.Stride+x] >= inkLevel {
				out.x0, out.x1, out.y0, out.y1 = min(out.x0, x), max(out.x1, x), min(out.y0, y), max(out.y1, y)
			}
		}
	}
	return out, out.x0 <= out.x1
}

// normalise scales the glyph to mh rows keeping its aspect, centred in mw columns,
// by area coverage (ported from Fy1Sync's normalize_glyph_coverage).
func normalise(img *image.Gray, b box) mask {
	var m mask
	tw := int(math.Max(1, math.Min(mw, math.Floor(float64(b.w())*mh/float64(b.h())+0.5))))
	ox := (mw - tw) / 2
	sx, sy := float64(b.w())/float64(tw), float64(b.h())/mh
	spanx, spany := math.Max(sx, 1), math.Max(sy, 1)
	for ty := 0; ty < mh; ty++ {
		cy := (float64(ty) + 0.5) * sy
		y0, y1 := math.Max(cy-spany/2, 0), math.Min(cy+spany/2, float64(b.h()))
		for tx := 0; tx < tw; tx++ {
			cx := (float64(tx) + 0.5) * sx
			x0, x1 := math.Max(cx-spanx/2, 0), math.Min(cx+spanx/2, float64(b.w()))
			var ink, total float64
			for yy := int(y0); float64(yy) < y1; yy++ {
				wy := math.Min(float64(yy+1), y1) - math.Max(float64(yy), y0)
				for xx := int(x0); float64(xx) < x1; xx++ {
					w := wy * (math.Min(float64(xx+1), x1) - math.Max(float64(xx), x0))
					if w <= 0 {
						continue
					}
					total += w
					if img.Pix[(b.y0+yy)*img.Stride+b.x0+xx] >= inkLevel {
						ink += w
					}
				}
			}
			m[ty*mw+ox+tx] = total > 0 && ink/total >= 0.5
		}
	}
	return m
}

// holes counts enclosed background regions of at least 12 cells (digit counters,
// not rasterisation pinholes).
func holes(m *mask) int {
	var seen [mw * mh]bool
	n := 0
	queue := make([]int, 0, mw*mh)
	for start := range m {
		if seen[start] || m[start] {
			continue
		}
		queue = append(queue[:0], start)
		seen[start] = true
		edge := false
		for i := 0; i < len(queue); i++ {
			at := queue[i]
			x, y := at%mw, at/mw
			edge = edge || x == 0 || x == mw-1 || y == 0 || y == mh-1
			for _, nx := range [][2]int{{x - 1, y}, {x + 1, y}, {x, y - 1}, {x, y + 1}} {
				if nx[0] < 0 || nx[1] < 0 || nx[0] >= mw || nx[1] >= mh {
					continue
				}
				if q := nx[1]*mw + nx[0]; !seen[q] && !m[q] {
					seen[q] = true
					queue = append(queue, q)
				}
			}
		}
		if !edge && len(queue) >= 12 {
			n++
		}
	}
	return n
}

func iou(a, b *mask) float64 {
	both, any := 0, 0
	for i := range a {
		if a[i] && b[i] {
			both++
		}
		if a[i] || b[i] {
			any++
		}
	}
	if any == 0 {
		return 0
	}
	return float64(both) / float64(any)
}

func colInk(img *image.Gray, x, y0, y1 int) int {
	n := 0
	for y := y0; y <= y1; y++ {
		if img.Pix[y*img.Stride+x] >= inkLevel {
			n++
		}
	}
	return n
}

func abs(v int) int { return max(v, -v) }

// blobs are the 8-connected groups of bright pixels (to find text lines).
func blobs(img *image.Gray) []box {
	w, h := img.Rect.Dx(), img.Rect.Dy()
	seen := make([]bool, w*h)
	var out []box
	var stack []int
	for i := range seen {
		if seen[i] || img.Pix[i] < inkLevel {
			continue
		}
		b, n := box{w, h, 0, 0}, 0
		seen[i] = true
		stack = append(stack[:0], i)
		for len(stack) > 0 {
			p := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			x, y := p%w, p/w
			b.x0, b.x1, b.y0, b.y1, n = min(b.x0, x), max(b.x1, x), min(b.y0, y), max(b.y1, y), n+1
			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					nx, ny := x+dx, y+dy
					if nx < 0 || ny < 0 || nx >= w || ny >= h {
						continue
					}
					if q := ny*w + nx; !seen[q] && img.Pix[q] >= inkLevel {
						seen[q] = true
						stack = append(stack, q)
					}
				}
			}
		}
		if n >= 4 && n < 5000 {
			out = append(out, b)
		}
	}
	return out
}

func (c *clockReader) fits(pts float64, v int) bool {
	return c.hasLast && pts > c.lastPts && pts-c.lastPts <= 10 && math.Abs(float64(c.lastV-v)-(pts-c.lastPts)) <= 1.5
}
