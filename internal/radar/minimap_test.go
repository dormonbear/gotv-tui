package radar

import (
	"strings"
	"testing"
	"time"

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

func TestFloorRaised(t *testing.T) {
	f := Floor{}
	f.Observe([]gotv.Player{{Alive: true, Pos: &gotv.Vec{X: 10, Y: 10, Z: 0}}, {Alive: false, Pos: &gotv.Vec{X: 500, Y: 500, Z: -300}}})
	if f.Raised(gotv.Vec{X: 30, Y: 40, Z: 40}) {
		t.Fatal("stairs-height is not raised")
	}
	if !f.Raised(gotv.Vec{X: 30, Y: 40, Z: 70}) {
		t.Fatal("on a crate above learnt ground should be raised")
	}
	if f.Raised(gotv.Vec{X: 500, Y: 500, Z: 0}) {
		t.Fatal("dead players don't teach the floor; unknown ground is never raised")
	}
}

func TestLabelsHoldSteady(t *testing.T) {
	cell := func(v gotv.Vec) (int, int) { return int(v.X), int(v.Y) }
	here := func(gotv.Vec) bool { return true }
	now := time.Now()
	o := Opts{Names: true, Labels: LabelMemory{}, Nums: map[string]int{"alpha": 1, "bravo": 2}}
	alpha := func(bravoX float64, after time.Duration) string {
		o.Now = now.Add(after)
		ps := []gotv.Player{
			{Name: "alpha", Alive: true, Pos: &gotv.Vec{X: 10, Y: 5}},
			{Name: "bravo", Alive: true, Pos: &gotv.Vec{X: bravoX, Y: 5}},
		}
		return playerLabels(ps, o, here, cell, 80, 4)["alpha"]
	}
	if alpha(30, 0) == "1" {
		t.Fatal("room for the name")
	}
	// bravo stops right after alpha's name (no gap left): within the hold the name stays
	if alpha(16, 100*time.Millisecond) == "1" {
		t.Fatal("name dropped at once when the gap closed")
	}
	if alpha(16, 2*time.Second) != "1" {
		t.Fatal("after the hold, a name with no gap goes back to the number")
	}
	// bravo leaves again: the number holds for a moment instead of popping back
	if alpha(30, 2100*time.Millisecond) != "1" {
		t.Fatal("name popped back within the hold")
	}
	if alpha(30, 3*time.Second) == "1" {
		t.Fatal("after the hold the name returns")
	}
}

func TestFloorUnder(t *testing.T) {
	f := Floor{}
	f.Observe([]gotv.Player{{Alive: true, Pos: &gotv.Vec{X: 10, Y: 10, Z: 200}}}) // on the bridge
	f.Observe([]gotv.Player{{Alive: true, Pos: &gotv.Vec{X: 20, Y: 20, Z: 0}}})   // walking under it
	if !f.Under(gotv.Vec{X: 30, Y: 30, Z: 0}) || f.Raised(gotv.Vec{X: 30, Y: 30, Z: 0}) {
		t.Fatal("under the bridge: ▾, not ▴")
	}
	if f.Under(gotv.Vec{X: 30, Y: 30, Z: 100}) {
		t.Fatal("less than a storey below isn't under")
	}
}

func TestRadarSources(t *testing.T) {
	if s := sources("de_cache.txt"); len(s) != 2 || !strings.Contains(s[1], "GameTracking-CS2") {
		t.Fatalf("overview fallback: %v", s)
	}
	if s := sources("de_cache.png"); len(s) != 4 || !strings.HasSuffix(s[1], "de_cache_radar_psd.png") {
		t.Fatalf("image fallbacks: %v", s)
	}
	if s := sources("de_cache_lower.png"); len(s) != 2 {
		t.Fatalf("lower level: one fallback only, got %v", s)
	}
}
