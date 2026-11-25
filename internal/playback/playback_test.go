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
	step(187.5, -1, nil)     // nothing new due
	pb.PushFrame(1000, 1000) // far ahead: jump to 904 and flush the stale queue
	step(250, 256, nil)
	pb.PushFrame(1004, 1004)
	step(312.5, -1, nil) // play ~908: 1000 not due yet
}

func TestAheadPeeksAtTheQueue(t *testing.T) {
	pb := New[int, string](64, 4, 8)
	for tick := 0; tick <= 512; tick += 4 {
		pb.PushFrame(tick, tick)
	}
	pb.PushEvent(300, "kill")
	pb.PushEvent(500, "plant")
	pb.Step(0) // play = 512 - 256 = 256
	ev, f := pb.Ahead(1)
	if len(ev) != 1 || ev[0].E != "kill" || ev[0].In != 44.0/64 || f == nil || *f != 320 {
		t.Fatalf("Ahead(1) = %+v %v", ev, *f)
	}
	if ev, _ := pb.Ahead(4); len(ev) != 2 {
		t.Fatalf("Ahead(4) = %+v", ev)
	}
}

func TestPendingUntilPlayedOut(t *testing.T) {
	p := New[int, string](64, 4, 8)
	for i := 0; i <= 640; i += 4 { // a burst, then the stream ends
		p.PushFrame(i, i)
	}
	p.PushEvent(630, "last kill")
	var got []string
	now := 0.0
	for i := 0; i < 200 && p.Pending(); i++ {
		_, evs := p.Step(now)
		got = append(got, evs...)
		now += 62.5
	}
	if p.Pending() || len(got) != 1 {
		t.Fatalf("not played out: pending=%v events=%v", p.Pending(), got)
	}
}
