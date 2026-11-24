package playback

import (
	"reflect"
	"testing"
)

func TestBurstIsReleasedSmoothly(t *testing.T) {
	pb := New[int, string](64, 1.5, 5)
	for tick := 0; tick <= 256; tick += 4 { // one burst = 4 s of game time
		pb.PushFrame(tick, tick)
	}
	pb.PushEvent(170, "kill")
	step := func(now float64, wantFrame int, wantEvents []string) {
		t.Helper()
		f, ev := pb.Step(now)
		got := -1
		if f != nil {
			got = *f
		}
		if got != wantFrame || !reflect.DeepEqual(ev, wantEvents) {
			t.Fatalf("Step(%v) = %d %v, want %d %v", now, got, ev, wantFrame, wantEvents)
		}
	}
	step(0, 160, nil) // starts 1.5 s (96 ticks) behind newest
	step(62.5, 164, nil)
	step(125, 168, nil)
	step(187.5, 172, []string{"kill"})
	step(187.5, -1, nil) // nothing new due
	pb.PushFrame(1000, 1000) // far ahead: jump to 904 and flush the stale queue
	step(250, 256, nil)
	pb.PushFrame(1004, 1004)
	step(312.5, -1, nil) // play ~908: 1000 not due yet
}
