package gotv

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/dormonbear/gotv-tui/internal/record"
)

// TestRelayEndToEnd plays a recorded CSTV relay (testdata/relay: ~15 s of a
// Major match on Cache) through Connect, the same path a live relay takes.
func TestRelayEndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("plays ~15 s of a recorded relay")
	}
	url, stop, err := record.Serve("testdata/relay")
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	var mu sync.Mutex
	var states []State
	var statuses []string
	go Connect(ctx, url, Handlers{
		State:  func(s State) { mu.Lock(); states = append(states, s); mu.Unlock() },
		Event:  func(Event) {},
		Status: func(s string) { mu.Lock(); statuses = append(statuses, s); mu.Unlock() },
	})
	for ctx.Err() == nil { // until a few seconds of frames have played
		mu.Lock()
		n := len(states)
		mu.Unlock()
		if n >= 48 {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	cancel()
	mu.Lock()
	defer mu.Unlock()
	if len(states) < 48 {
		t.Fatalf("%d frames (statuses %q)", len(states), statuses)
	}
	s := states[len(states)-1]
	if s.Map != "de_cache" || s.Round < 1 || s.CT.Name == "" || s.T.Name == "" {
		t.Fatalf("map %q round %d teams %q/%q", s.Map, s.Round, s.CT.Name, s.T.Name)
	}
	if len(s.Players) != 10 {
		t.Fatalf("%d players", len(s.Players))
	}
	first := states[0] // the first frame with everyone in it (the very first comes before the players)
	for _, st := range states {
		if len(st.Players) == 10 {
			first = st
			break
		}
	}
	moved := false
	for _, p := range s.Players {
		if p.Name == "" || p.Pos == nil || (p.Side != CT && p.Side != T) {
			t.Fatalf("player %+v", p)
		}
		for _, q := range first.Players {
			if q.Name == p.Name && q.Pos != nil && *q.Pos != *p.Pos {
				moved = true
			}
		}
	}
	if !moved {
		t.Fatal("nobody moved across the frames")
	}
}
