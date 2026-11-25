package radar

import (
	"bytes"
	"image"
	"image/png"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/dormonbear/gotv-tui/internal/gotv"
)

var sgr = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func strip(s string) []rune { return []rune(sgr.ReplaceAllString(s, "")) }

func TestParseOverview(t *testing.T) {
	o := ParseOverview(`"de_nuke" { "pos_x" "-3453" "pos_y" "2887" "scale" "7"
    "verticalsections" { "default" { "AltitudeMax" "10000" "AltitudeMin" "-495" }
    "lower" { "AltitudeMax" "-495" "AltitudeMin" "-10000" } }
    "bombA_x" "0.58" "bombA_y" "0.48" "bombB_x" "0.58" "bombB_y" "0.58" }`)
	if o.PosX != -3453 || o.PosY != 2887 || o.Scale != 7 || o.LowerBelow == nil || *o.LowerBelow != -495 || len(o.Sites) != 2 || o.Sites[1] != (Site{"B", 0.58, 0.58}) {
		t.Fatalf("%+v", o)
	}
	if ParseOverview(`"pos_x" "1" "pos_y" "2" "scale" "4.4"`).LowerBelow != nil {
		t.Fatal("single level map has no lower split")
	}
	sites := []Site{{Name: "A"}, {Name: "B"}}
	if s := SitesOn("de_nuke", true, sites); len(s) != 1 || s[0].Name != "B" {
		t.Fatal(s)
	}
	if s := SitesOn("de_train", false, sites); len(s) != 2 {
		t.Fatal(s)
	}
}

func TestResample(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 2, 2)) // left column opaque red, right transparent
	img.Pix[0], img.Pix[3], img.Pix[8], img.Pix[11] = 255, 255, 255, 255
	if got := Resample(img, Box{0, 0, 1024, 1024}, 1, 1); got[0] != 255 || got[3] != 127 {
		t.Fatal(got)
	}
	if got := Resample(img, Box{512, 0, 512, 1024}, 1, 1); got[3] != 0 {
		t.Fatal(got)
	}
}

func TestBuildBaseKeepsThinWall(t *testing.T) {
	// 4x4 dots (2x1 cells) at SS=4; a 1px transparent column at x=7 splits two rooms,
	// its sides (x=6, x=8) fall in different cells
	r := &Raster{W: 4, H: 4, Pixels: make([]byte, 16*16*4)}
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			if x != 7 {
				copy(r.Pixels[(y*16+x)*4:], []byte{80, 100, 110, 255})
			}
		}
	}
	b := BuildBase(r, 1)
	if b.CW != 2 || b.dots[0]&0x8 == 0 || b.dots[1]&0x1 == 0 || b.bg[0] == nil || b.bg[1] == nil {
		t.Fatalf("CW=%d dots=%v bg=%v", b.CW, b.dots, b.bg)
	}
}

func TestBuildBaseViewEdgeIsNoWall(t *testing.T) {
	r := &Raster{W: 4, H: 4, Pixels: make([]byte, 16*16*4)} // all floor, cut by the view
	for i := 0; i < len(r.Pixels); i += 4 {
		copy(r.Pixels[i:], []byte{80, 100, 110, 255})
	}
	if b := BuildBase(r, 1); b.dots[0] != 0 || b.dots[1] != 0 {
		t.Fatalf("view edge drawn as a wall: %v", b.dots)
	}
}

func TestDrawPlacesPlayersPerLevel(t *testing.T) {
	const W, H = 20, 20
	r := &Raster{W: W, H: H, Pixels: make([]byte, W*SS*H*SS*4), Aspect: 2, ov: Overview{Scale: 1}, k: 1, ky: 1} // ToPx(x, y) = (x, -y)
	for i := range r.Pixels {
		r.Pixels[i] = 255
	}
	pl := func(name string, x, y, z float64) gotv.Player {
		return gotv.Player{Name: name, Side: gotv.T, Pos: &gotv.Vec{X: x, Y: -y, Z: z}, Alive: true}
	}
	st := &gotv.State{Players: []gotv.Player{pl("a", 7, 9, 0), pl("b", 1, 1, -600)}}
	lb := -495.0
	base := BuildBase(r, 1)
	o := Opts{Nums: map[string]int{"a": 2, "b": 3}, LowerBelow: &lb, Sites: []Site{{"A", 18.0 / 1024, 18.0 / 1024}}}
	rows := Draw(r, base, st, o)
	if len(rows) != 5 {
		t.Fatal(len(rows))
	}
	if c := strip(rows[2]); c[3] != '2' || c[4] != '→' { // x=7 -> col 3, y=9 -> row 2; yaw 0 faces east
		t.Fatalf("%q", string(c))
	}
	if strip(rows[0])[0] == '3' {
		t.Fatal("lower-level player drawn on upper map")
	}
	if strip(rows[4])[9] != 'A' {
		t.Fatalf("%q", string(strip(rows[4])))
	}
	o.Lower, o.Sites = true, nil
	lower := Draw(r, base, st, o)
	if strip(lower[0])[0] != '3' || strip(lower[2])[3] == '2' {
		t.Fatal("levels mixed up")
	}
}

func TestFacing(t *testing.T) {
	for _, c := range []struct {
		yaw    float64
		ch     string
		dx, dy int
	}{{0, "→", 1, 0}, {90, "↑", 0, -1}, {-135, "↙", -1, 1}, {359, "→", 1, 0}} {
		if ch, dx, dy := Facing(c.yaw); ch != c.ch || dx != c.dx || dy != c.dy {
			t.Errorf("yaw %v: %s %d %d", c.yaw, ch, dx, dy)
		}
	}
}

func TestRasterizeKeepsScreenProportions(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	box := Box{0, 0, 200, 100} // 2:1 on screen whatever the cell shape
	for _, aspect := range []float64{2, 2.16, 2.5} {
		r := Rasterize(img, Overview{Scale: 1}, box, 400, 400, aspect)
		w := float64(r.W) / 2          // cells wide (cell width = 1)
		h := float64(r.H) / 4 * aspect // cells tall, in cell widths
		if ratio := w / h; ratio < 1.95 || ratio > 2.05 {
			t.Errorf("aspect %v: %dx%d dots -> screen ratio %.3f", aspect, r.W, r.H, ratio)
		}
	}
}

func TestBoxView(t *testing.T) {
	b := Box{100, 200, 400, 200}
	if v := b.View(300, 300, 1); v != b {
		t.Fatalf("zoom 1 is the whole box: %+v", v)
	}
	if v := b.View(300, 300, 2); v != (Box{200, 250, 200, 100}) {
		t.Fatalf("zoom 2 around the centre: %+v", v)
	}
	if v := b.View(0, 0, 2); v != (Box{100, 200, 200, 100}) {
		t.Fatalf("clamped into the top-left corner: %+v", v)
	}
	if v := b.View(300, 300, 99); v.W != 100 {
		t.Fatalf("zoom capped at MaxZoom: %+v", v)
	}
}

func TestDrawLeavesMapCellsBlankOverAnImage(t *testing.T) {
	const W, H = 8, 8 // 4x2 cells
	r := &Raster{W: W, H: H, Pixels: make([]byte, W*SS*H*SS*4), Aspect: 2, ov: Overview{Scale: 1}, k: 1, ky: 1}
	for i := range r.Pixels {
		r.Pixels[i] = 255
	}
	st := &gotv.State{Players: []gotv.Player{{Name: "a", Side: gotv.T, Alive: true, Pos: &gotv.Vec{X: 5, Y: -1}}}}
	rows := Draw(r, BuildBase(r, 1), st, Opts{ImageUnder: true, Nums: map[string]int{"a": 2}})
	for y, row := range rows {
		if w := ansi.StringWidth(row); w != 4 {
			t.Fatalf("row %d is %d cells wide, want 4: %q", y, w, row)
		}
	}
	// x=5 -> col 2 is the player (opaque badge), col 3 its arrow; cols 0, 1 are blank
	// with no background so the image shows; row 1 is all image.
	if !strings.HasPrefix(rows[0], "  ") || !strings.Contains(rows[0], "48;2") || string(strip(rows[0])[2]) != "2" {
		t.Fatalf("row 0: %q", rows[0])
	}
	if rows[1] != "    " {
		t.Fatalf("row 1: %q", rows[1])
	}
}

func TestNamesWhereTheyFitNumbersElsewhere(t *testing.T) {
	const W, H = 60, 16 // 30x4 cells
	r := &Raster{W: W, H: H, Pixels: make([]byte, W*SS*H*SS*4), Aspect: 2, ov: Overview{Scale: 1}, k: 1, ky: 1}
	p := func(name string, x, row float64) gotv.Player {
		return gotv.Player{Name: name, Side: gotv.T, Alive: true, Yaw: 180, Pos: &gotv.Vec{X: x * 2, Y: -row * 4}}
	}
	st := &gotv.State{Players: []gotv.Player{
		p("s1n", 2, 0),     // room to the right: named
		p("JBa", 10, 2),    // ...
		p("Lake", 12, 2),   // only one cell gap after JBa's label would be needed: JBa falls back
		p("Swisher", 2, 3), // fits
	}}
	rows := Draw(r, BuildBase(r, 1), st, Opts{Names: true, Nums: map[string]int{"s1n": 1, "jba": 2, "lake": 3, "swisher": 4}})
	got := func(y int) string { return strings.TrimRight(string(strip(rows[y])), " ") }
	// text map labels are 2 cells (about square), not the whole name
	if !strings.HasSuffix(got(0), "s1") || !strings.HasSuffix(got(3), "Sw") {
		t.Fatalf("named rows: %q / %q", got(0), got(3))
	}
	if row := got(2); strings.Contains(row, "JB") || !strings.Contains(row, "2") || !strings.Contains(row, "La") {
		t.Fatalf("crowded row: %q (JBa should fall back to its number, Lake gets its label)", row)
	}
	st.Players[0].Name = "Swift" // reads "Sw" like Swisher: both keep numbers
	rows = Draw(r, BuildBase(r, 1), st, Opts{Names: true, Nums: map[string]int{"swift": 1, "swisher": 4}})
	if strings.Contains(got(0), "Sw") || strings.Contains(got(3), "Sw") || !strings.Contains(got(0), "1") || !strings.Contains(got(3), "4") {
		t.Fatalf("ambiguous labels: %q / %q", got(0), got(3))
	}
	if rows := Draw(r, BuildBase(r, 1), st, Opts{Nums: map[string]int{"s1n": 1}}); strings.Contains(string(strip(rows[0])), "s1n") {
		t.Fatal("names shown with Names off")
	}
}

func TestBoxFill(t *testing.T) {
	tall := Box{300, 100, 400, 800}
	if f := tall.Fill(1); f != (Box{224, 100, 800, 800}) {
		t.Fatalf("tall map in a square area: %+v", f) // grows right, shifts left to stay inside
	}
	if f := tall.Fill(0.25); f != (Box{300, 0, 400, 1024}) {
		t.Fatalf("tall map in a taller area: %+v", f) // capped at the whole radar height
	}
	if f := (Box{100, 100, 400, 400}).Fill(1); f != (Box{100, 100, 400, 400}) {
		t.Fatalf("same shape: %+v", f)
	}
}

func TestDetailLinesAppearWhenZoomedIn(t *testing.T) {
	// one cell of floor: left half a lighter shade than the right (a height step)
	r := &Raster{W: 2, H: 4, Pixels: make([]byte, 8*16*4)}
	for y := 0; y < 16; y++ {
		for x := 0; x < 8; x++ {
			v := byte(120)
			if x >= 4 {
				v = 70 // 50 luminance darker
			}
			copy(r.Pixels[(y*8+x)*4:], []byte{v, v, v, 255})
		}
	}
	if b := BuildBase(r, 1); b.dots[0] != 0 {
		t.Fatalf("whole map: shading step drawn (%08b)", b.dots[0])
	}
	b := BuildBase(r, 3)                            // threshold 30
	if b.dots[0]&0x09 == 0 || b.dots[0]&0x36 == 0 { // dots on both sides of the step
		t.Fatalf("zoomed in: no detail line (%08b)", b.dots[0])
	}
	if out := BuildBase(&Raster{W: 2, H: 4, Pixels: make([]byte, 8*16*4)}, 3); out.dots[0] != 0 {
		t.Fatal("empty radar has no lines")
	}
}

func TestKillLineIsThinAndHeadshotMarked(t *testing.T) {
	const W, H = 60, 16 // 30x4 cells; 1 world unit = 1 braille dot here
	r := &Raster{W: W, H: H, Pixels: make([]byte, W*SS*H*SS*4), Aspect: 2, ov: Overview{Scale: 1}, k: 1, ky: 1}
	now := time.Now()
	k := KillMark{From: &gotv.Vec{X: 0, Y: 0}, To: gotv.Vec{X: 44, Y: -14}, Headshot: true, At: now.Add(-time.Second)}
	rows := Draw(r, BuildBase(r, 1), &gotv.State{}, Opts{Kills: []KillMark{k}, Now: now})
	cols := map[int]int{} // cells per column holding line dots
	for y, row := range rows {
		for x, c := range []rune(string(strip(row))) {
			if c > 0x2800 && c <= 0x28ff {
				cols[x]++
				if y > 3 {
					t.Fatal("line outside its rows")
				}
			}
		}
	}
	for x := 1; x <= 20; x++ {
		if cols[x] == 0 || cols[x] > 2 {
			t.Fatalf("column %d has %d line cells (gap or staircase): %v", x, cols[x], cols)
		}
	}
	if !strings.Contains(string(strip(rows[3])), "✸HS") {
		t.Fatalf("no headshot mark: %q", string(strip(rows[3])))
	}
	k.At = now.Add(-2 * time.Second) // headshot mark gone after hsShow
	if rows := Draw(r, BuildBase(r, 1), &gotv.State{}, Opts{Kills: []KillMark{k}, Now: now}); strings.Contains(string(strip(rows[3])), "HS") {
		t.Fatal("headshot mark still shown")
	}
}

func TestValidRejectsBadDownloads(t *testing.T) {
	var png1 bytes.Buffer
	_ = png.Encode(&png1, image.NewNRGBA(image.Rect(0, 0, 1, 1)))
	if !valid("de_nuke.png", png1.Bytes()) || valid("de_nuke.png", []byte("<html>portal</html>")) || valid("de_nuke.png", png1.Bytes()[:10]) {
		t.Fatal("png validation")
	}
	if !valid("de_nuke.txt", []byte(`"de_nuke" { "pos_x" "-3453" "pos_y" "2887" "scale" "7" }`)) || valid("de_nuke.txt", []byte("<html>")) {
		t.Fatal("overview validation")
	}
}

func TestLoadRejectsOddMapNames(t *testing.T) {
	for _, n := range []string{"../../.ssh/id_rsa", "de_nuke/../x", "", "DE NUKE"} {
		if _, err := Load(n); err == nil {
			t.Errorf("Load(%q) accepted", n)
		}
	}
}

func TestWideNamesKeepTheirNumber(t *testing.T) {
	const W, H = 60, 16
	r := &Raster{W: W, H: H, Pixels: make([]byte, W*SS*H*SS*4), Aspect: 2, ov: Overview{Scale: 1}, k: 1, ky: 1}
	st := &gotv.State{Players: []gotv.Player{{Name: "あなたの王様", Side: gotv.T, Alive: true, Yaw: 180, Pos: &gotv.Vec{X: 4, Y: 0}}}}
	rows := Draw(r, BuildBase(r, 1), st, Opts{Names: true, Nums: map[string]int{"あなたの王様": 4}})
	if got := string(strip(rows[0])); strings.Contains(got, "あ") || !strings.Contains(got, "4") {
		t.Fatalf("%q: a wide name should show its number", got)
	}
}

func TestTouchingBadgesAlternate(t *testing.T) {
	const W, H = 20, 20
	r := &Raster{W: W, H: H, Pixels: make([]byte, W*SS*H*SS*4), Aspect: 2, ov: Overview{Scale: 1}, k: 1, ky: 1}
	pl := func(name string, x float64) gotv.Player {
		return gotv.Player{Name: name, Side: gotv.CT, Pos: &gotv.Vec{X: x, Y: -9}, Alive: true, Yaw: 90}
	}
	st := &gotv.State{Players: []gotv.Player{pl("a", 6), pl("b", 8)}} // cells 3 and 4: "12" without help
	rows := Draw(r, BuildBase(r, 1), st, Opts{Nums: map[string]int{"a": 1, "b": 2}})
	bgs := regexp.MustCompile(`48;2;(\d+;\d+;\d+)m(?:\x1b\[[0-9;]*m)*[12]`).FindAllStringSubmatch(rows[2], -1)
	if len(bgs) != 2 || bgs[0][1] == bgs[1][1] {
		t.Fatalf("touching badges should differ in shade: %q", rows[2])
	}
}
