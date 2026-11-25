package radar

import (
	"testing"

	"github.com/dormonbear/gotv-tui/internal/gotv"
	"github.com/dormonbear/gotv-tui/internal/term"
)

func TestBlastFx(t *testing.T) {
	he, fl := Blast{Nade: gotv.HE}, Blast{Nade: gotv.Flash}
	r0, a0, c, ok := he.fx(0.05)
	r1, a1, _, _ := he.fx(0.4)
	if !ok || c != boomC || r1 <= r0 || a1 >= a0 {
		t.Fatalf("HE should grow and fade: %v %v -> %v %v", r0, a0, r1, a1)
	}
	if _, _, _, ok := he.fx(0.6); ok {
		t.Fatal("HE flash should be over after 0.55 s")
	}
	if _, _, c, ok := fl.fx(0.1); !ok || c != term.White {
		t.Fatal("flashbang pops white")
	}
	if blinded(red, 0) != red || blinded(red, 3) != term.White {
		t.Fatal("blindness whitens the marker, fully while deeply flashed")
	}
}
