package ui

import (
	"bytes"
	"fmt"
	"image"
	"regexp"
	"strings"
	"testing"

	"github.com/dormonbear/gotv-tui/internal/radar"
)

func dot() radar.Sprite {
	return radar.Sprite{Img: image.NewNRGBA(image.Rect(0, 0, 10, 10)), AX: 5, AY: 5}
}

var placeRe = regexp.MustCompile(`\x1b\[(\d+);(\d+)H\x1b_Ga=p,i=(\d+),p=(\d+),X=(\d+),Y=(\d+),x=(\d+),y=(\d+),w=(\d+),h=(\d+)`)

func TestSpriteLayerPlacesClipsAndRemoves(t *testing.T) {
	var s spriteLayer
	var out bytes.Buffer
	area := mapArea{x: 100, y: 10, cols: 20, rows: 5} // 13x29 px cells: 260x145 px
	area.uses = []radar.Use{
		{Key: "a", Make: dot, X: 40, Y: 70}, // top-left (35,65): cell (2,2) +(9,7)
		{Key: "a", Make: dot, X: 2, Y: 3},   // hangs off the top-left: clipped
	}
	s.frame(&out, []mapArea{area}, 13, 29)
	got := placeRe.FindAllStringSubmatch(out.String(), -1)
	if strings.Count(out.String(), "a=t") != 1 || len(got) != 2 {
		t.Fatalf("want 1 upload, 2 placements; got:\n%q", out.String())
	}
	if p := got[0]; p[1] != "13" || p[2] != "103" || p[5] != "9" || p[6] != "7" || p[9] != "10" {
		t.Fatalf("first placement %v, want row 13 col 103 offset (9,7)", p[1:])
	}
	if p := got[1]; p[1] != "11" || p[2] != "101" || p[7] != "3" || p[8] != "2" || p[9] != "7" || p[10] != "8" {
		t.Fatalf("clipped placement %v, want source (3,2) 7x8 at the map's corner", p[1:])
	}
	out.Reset()
	area.uses = area.uses[:1] // the second marker is gone
	s.frame(&out, []mapArea{area}, 13, 29)
	if strings.Contains(out.String(), "a=t") || !strings.Contains(out.String(), fmt.Sprintf("a=d,d=i,i=%d,p=2", spriteIDs)) {
		t.Fatalf("want no upload and the 2nd placement removed:\n%q", out.String())
	}
}
