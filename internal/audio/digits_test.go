package audio

import (
	"bufio"
	"image"
	"image/draw"
	"image/png"
	"os"
	"strconv"
	"strings"
	"testing"
)

// testdata/clock: about a minute of a PGL Major broadcast's clock (a co-stream's
// key frames, cropped; all round 18), labelled where Tesseract could read the clock. The reader
// starts knowing no glyph of this font.
func TestReadsABroadcastClock(t *testing.T) {
	f, err := os.Open("testdata/clock/labels.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var c clockReader
	right, wrong, missed, rounds := 0, 0, 0, 0
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		name, label, _ := strings.Cut(sc.Text(), " ")
		n, _ := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(name, "f"), ".png"))
		img := grayPNG(t, "testdata/clock/"+name)
		v, ok := c.read(float64(n)/60, img)
		if r := c.readRound(img); ok && r != 0 && r != 18 { // all of it is round 18
			t.Errorf("%s: round %d", name, r)
		} else if r == 18 {
			rounds++
		}
		want, err := strconv.Atoi(label)
		switch {
		case err != nil: // Tesseract couldn't tell
		case !ok:
			missed++
		case v == want:
			right++
		case v-want == 1 || want-v == 1:
			wrong++ // a tick early or late: tolerated, the sync votes it away
		default:
			t.Errorf("%s: read %d, want %d", name, v, want)
		}
	}
	if right < 15 || wrong > 1 || missed > 2 || rounds < 25 {
		t.Fatalf("right %d, a second off %d, missed %d, round read %d times", right, wrong, missed, rounds)
	}
}

func grayPNG(t *testing.T, path string) *image.Gray {
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	g := image.NewGray(img.Bounds())
	draw.Draw(g, g.Bounds(), img, img.Bounds().Min, draw.Src)
	return g
}
