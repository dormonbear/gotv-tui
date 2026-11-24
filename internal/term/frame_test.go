package term

import (
	"image"
	"strings"
	"testing"
)

func TestSixel(t *testing.T) {
	// 8x7: red 8x6 band with a transparent column 3, then one white row
	img := image.NewNRGBA(image.Rect(0, 0, 8, 7))
	for y := 0; y < 7; y++ {
		for x := 0; x < 8; x++ {
			c := []byte{255, 0, 0, 255}
			if y == 6 {
				c = []byte{255, 255, 255, 255}
			}
			if x == 3 && y < 6 {
				c[3] = 0
			}
			copy(img.Pix[img.PixOffset(x, y):], c)
		}
	}
	s := SixelImage(img)
	for _, want := range []string{
		"\x1bP0;1;0q\"1;1;8;7",         // transparent background, 8x7
		"#0;2;98;1;1", "#1;2;98;98;98", // palette: the image's own colours, most common first
		"#0~~~?!4~-", // red: 6 rows set (~), column 3 empty (?), a run of 4 compressed
		"#1!8@-",     // white: bottom row only, run of 8
		"\x1b\\",
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing %q in %q", want, s)
		}
	}
}

func TestSixelPaletteKeepsWarmGreys(t *testing.T) {
	// the radar's brown-grey floor must not drift towards another hue
	img := image.NewNRGBA(image.Rect(0, 0, 1, 6))
	for y := 0; y < 6; y++ {
		copy(img.Pix[img.PixOffset(0, y):], []byte{110, 95, 85, 255})
	}
	if s := SixelImage(img); !strings.Contains(s, "#0;2;42;36;32") { // ~107,92,82: 110,95,85 within one 5-bit step
		t.Fatalf("floor colour changed: %q", s)
	}
}

func TestITermImage(t *testing.T) {
	s := ITermImage([]byte("png"), 40, 12)
	if s != "\x1b]1337;File=inline=1;size=3;width=40;height=12;preserveAspectRatio=0:cG5n\x07" {
		t.Fatalf("%q", s)
	}
}
