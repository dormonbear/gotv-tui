package radar

import (
	"testing"
	"time"

	"github.com/dormonbear/gotv-tui/internal/gotv"
)

func overlayScene() (*Raster, *gotv.State) {
	r := &Raster{W: 176, H: 72, Aspect: 2.2, ov: Overview{Scale: 1}, k: 0.2, ky: 0.18}
	var ps []gotv.Player
	for i := 0; i < 10; i++ {
		side := gotv.T
		if i%2 == 0 {
			side = gotv.CT
		}
		ps = append(ps, gotv.Player{Name: "player" + string(rune('a'+i)), Side: side, Alive: i != 3, Yaw: float64(i * 36), Pos: &gotv.Vec{X: float64(100 + i*70), Y: -float64(100 + i*30)}})
	}
	st := &gotv.State{Players: ps, Utility: []gotv.Utility{{Kind: gotv.USmoke, Pos: gotv.Vec{X: 300, Y: -200}}, {Kind: gotv.UFire, Pos: gotv.Vec{X: 600, Y: -300}}}}
	return r, st
}

func TestSpritesSitWhereTheMapPutsThem(t *testing.T) {
	r, st := overlayScene()
	uses := Sprites(r, 13, 29, st, Opts{Nums: map[string]int{"playera": 1}})
	px, py := r.ToPx(st.Players[0].Pos.X, st.Players[0].Pos.Y)
	found := false
	for _, u := range uses {
		if u.Key == "player:CT:1:false" {
			found = true
			if u.X != px*13/2 || u.Y != py*29/4 {
				t.Fatalf("player at (%.1f,%.1f), want (%.1f,%.1f)", u.X, u.Y, px*13/2, py*29/4)
			}
			s := u.Make()
			if a := s.Img.NRGBAAt(s.AX, s.AY).A; a != 255 {
				t.Fatalf("anchor of the player sprite is not on the dot (alpha %d)", a)
			}
		}
	}
	if !found {
		t.Fatal("no sprite for player 1")
	}
	// same scene, moved players: same looks, so nothing new to upload
	keys := map[string]bool{}
	for _, u := range uses {
		keys[u.Key] = true
	}
	for i := range st.Players {
		st.Players[i].Pos.X += 3
	}
	for _, u := range Sprites(r, 13, 29, st, Opts{Nums: map[string]int{"playera": 1}}) {
		if !keys[u.Key] {
			t.Fatalf("moving changed a sprite's look: %s", u.Key)
		}
	}
}

// A frame where nothing new appears only computes placements.
func BenchmarkSpritesFrame(b *testing.B) {
	r, st := overlayScene()
	o := Opts{Names: true, Nums: map[string]int{}, Now: time.Now()}
	for i := 0; i < b.N; i++ {
		Sprites(r, 26, 58, st, o)
	}
}

// Drawing every sprite of a scene from scratch (happens once per new look).
func BenchmarkSpritesMakeAll(b *testing.B) {
	r, st := overlayScene()
	uses := Sprites(r, 26, 58, st, Opts{Names: true, Nums: map[string]int{}, Now: time.Now()})
	for i := 0; i < b.N; i++ {
		for _, u := range uses {
			u.Make()
		}
	}
}
