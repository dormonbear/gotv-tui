package radar

import (
	"image"
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/dormonbear/gotv-tui/internal/gotv"
	"github.com/dormonbear/gotv-tui/internal/term"
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

func TestBuildBaseIsTheImageInHalfBlocks(t *testing.T) {
	// 2x4 dots = one cell at SS=4: 8x16 px. Top half red, bottom half empty
	// (transparent), except the bottom-right quarter which is the near-black
	// backdrop of lower-level radars (also empty).
	r := &Raster{W: 2, H: 4, Pixels: make([]byte, 8*16*4)}
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			copy(r.Pixels[(y*8+x)*4:], []byte{200, 40, 40, 255})
		}
	}
	for y := 12; y < 16; y++ {
		for x := 4; x < 8; x++ {
			copy(r.Pixels[(y*8+x)*4:], []byte{8, 8, 8, 255})
		}
	}
	b := BuildBase(r)
	if b.CW != 1 || b.CH != 1 || b.top[0] == nil || *b.top[0] != term.Scale(term.RGB{200, 40, 40}, mapAlpha) || b.bot[0] != nil {
		t.Fatalf("top=%v bot=%v", b.top[0], b.bot[0])
	}
	row := Draw(r, b, nil, Opts{})[0]
	if !strings.Contains(row, "▀") || strings.Contains(row, "48;2") {
		t.Fatalf("want an upper half block with no background: %q", row)
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
	base := BuildBase(r)
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
	rows := Draw(r, BuildBase(r), st, Opts{ImageUnder: true, Nums: map[string]int{"a": 2}})
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
