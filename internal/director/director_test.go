package director

import (
	"testing"
	"time"

	"github.com/dormonbear/gotv-tui/internal/gotv"
	"github.com/dormonbear/gotv-tui/internal/playback"
)

func pl(name string, side gotv.Side, x, y float64) gotv.Player {
	return gotv.Player{Name: name, Side: side, Alive: true, HP: 100, Pos: &gotv.Vec{X: x, Y: y}}
}

// two pairs: a close fight (a1 vs b1) and a farther one (a2 vs b2), plus loners.
func state() *gotv.State {
	return &gotv.State{Players: []gotv.Player{
		pl("a1", gotv.T, 0, 0), pl("b1", gotv.CT, 300, 0),
		pl("a2", gotv.T, 5000, 0), pl("b2", gotv.CT, 5000, 1200),
		pl("a3", gotv.T, -6000, -6000), pl("b3", gotv.CT, 6000, 6000),
	}}
}

func TestFramesTheClosestFight(t *testing.T) {
	d := New()
	s := d.Update(time.Unix(0, 0), state(), nil, nil)
	if s == nil || s.X != 150 || s.Y != 0 || s.Radius != minRadius {
		t.Fatalf("shot %+v, want centred between a1 and b1", s)
	}
}

func TestQuietRoundShowsTheWholeMap(t *testing.T) {
	st := &gotv.State{Players: []gotv.Player{pl("a", gotv.T, 0, 0), pl("b", gotv.CT, 4000, 4000)}}
	if s := New().Update(time.Unix(0, 0), st, nil, nil); s != nil {
		t.Fatalf("shot %+v, want whole map", s)
	}
	st.Freeze = true
	if s := New().Update(time.Unix(0, 0), state(), nil, nil); s == nil {
		t.Fatal("setup: expected a shot")
	}
	if s := New().Update(time.Unix(0, 0), st, nil, nil); s != nil {
		t.Fatal("freeze time shows the whole map")
	}
}

func TestShotHoldsForMinShotThenMoves(t *testing.T) {
	d := New()
	t0 := time.Unix(0, 0)
	st := state()
	d.Update(t0, st, nil, nil) // on a1 vs b1
	// a2 vs b2 get much closer: better than the current fight, but not by cutMargin
	st.Players[3].Pos = &gotv.Vec{X: 5000, Y: 100}
	if s := d.Update(t0.Add(2*time.Second), st, nil, nil); s.X != 150 {
		t.Fatalf("cut before minShot: %+v", s)
	}
	// the first fight ends (b1 dies): after minShot the camera moves on
	st.Players[1].Alive = false
	if s := d.Update(t0.Add(5*time.Second), st, nil, nil); s.X != 5000 {
		t.Fatalf("still on a dead fight after minShot: %+v", s)
	}
}

func TestImportantEventAheadCutsEarly(t *testing.T) {
	d := New()
	t0 := time.Unix(0, 0)
	st := state()
	d.Update(t0, st, nil, nil) // on a1 vs b1
	// a clutch kill is coming in 2 s on the other side of the map
	st.Players[1].Alive, st.Players[5].Alive = false, false // CT down to b2 (+ victim)
	soon := []playback.Soon[gotv.Event]{{In: 2, E: gotv.Event{Kind: gotv.EvKill,
		Killer: &gotv.Who{Name: "a2", Side: gotv.T}, Victim: &gotv.Who{Name: "b2", Side: gotv.CT}, To: &gotv.Vec{X: 5000, Y: 1200}}}}
	s := d.Update(t0.Add(time.Second), st, soon, nil)
	if s == nil || s.X != 5000 || s.Reason != "a2 → b2" {
		t.Fatalf("shot %+v, want the coming kill", s)
	}
}

func TestDamageHeatsAPlayerUp(t *testing.T) {
	d := New()
	t0 := time.Unix(0, 0)
	st := &gotv.State{Players: []gotv.Player{pl("a", gotv.T, 0, 0), pl("b", gotv.CT, 1400, 0)}}
	d.Update(t0, st, nil, nil)
	st.Players[0].HP = 30 // took 70 from b, far-ish away
	d.Update(t0.Add(100*time.Millisecond), st, nil, nil)
	if h := d.heat["a"]; h < 69 {
		t.Fatalf("heat %v", h)
	}
	d.Update(t0.Add(2100*time.Millisecond), st, nil, nil)
	if h := d.heat["a"]; h > 36 || h < 34 {
		t.Fatalf("heat after one half-life %v, want ~35", h)
	}
}

func TestFightsStayOnOneLevel(t *testing.T) {
	lb := -500.0
	st := &gotv.State{Players: []gotv.Player{pl("a", gotv.T, 0, 0), pl("b", gotv.CT, 200, 0)}}
	st.Players[1].Pos.Z = -700 // right below: not a fight
	if s := New().Update(time.Unix(0, 0), st, nil, &lb); s != nil {
		t.Fatalf("players on different levels framed as a fight: %+v", s)
	}
}
