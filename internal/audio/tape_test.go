package audio

import (
	"context"
	"testing"
	"time"
)

// The tape keeps what arrived, finds a place by arrival time (a longer delay
// replays from further back), waits for what hasn't arrived, and drops what is
// older than it keeps.
func TestTape(t *testing.T) {
	tp := newTape()
	t0 := time.Unix(1000, 0)
	for i := 0; i < 5; i++ {
		tp.add(chunk{at: t0.Add(time.Duration(i) * time.Second), b: []byte{byte(i)}})
	}
	if got := tp.seek(t0.Add(2500*time.Millisecond), func(c chunk) time.Time { return c.at }); got != 3 {
		t.Fatalf("seek 2.5 s: %d, want 3", got)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if c, ok := tp.wait(ctx, 1); !ok || c.b[0] != 1 {
		t.Fatalf("chunk 1: %v %v", c, ok)
	}
	go func() {
		time.Sleep(20 * time.Millisecond)
		tp.add(chunk{at: t0.Add(5 * time.Second), b: []byte{5}})
	}()
	if c, ok := tp.wait(ctx, 5); !ok || c.b[0] != 5 {
		t.Fatalf("waited chunk 5: %v %v", c, ok)
	}
	tp.add(chunk{at: t0.Add(tapeKeep + 3*time.Second), b: []byte{6}}) // chunks 0-2 now too old
	if c, ok := tp.wait(ctx, 0); !ok || c.b[0] != 3 {
		t.Fatalf("a dropped chunk should skip to the oldest kept, got %v", c)
	}
	short, stop := context.WithCancel(context.Background())
	stop()
	if _, ok := tp.wait(short, 99); ok {
		t.Fatal("wait should give up when its context ends")
	}
}
