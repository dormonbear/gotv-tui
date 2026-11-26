package audio

import (
	"context"
	"sync"
	"time"
)

// tapeKeep is how much received sound is kept: the furthest the audio can be
// held back, and the furthest a longer delay can replay from.
const tapeKeep = 10 * time.Minute

type chunk struct {
	at time.Time
	b  []byte
}

// tape is the stream's sound as it arrived, kept for tapeKeep. Chunks are
// numbered from the start, so a reader's place survives old ones being dropped.
type tape struct {
	mu     sync.Mutex
	base   int // number of chunks[0]
	chunks []chunk
	more   chan struct{} // closed when a chunk arrives
}

func newTape() *tape { return &tape{more: make(chan struct{})} }

func (t *tape) add(at time.Time, b []byte) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.chunks = append(t.chunks, chunk{at, b})
	for len(t.chunks) > 0 && at.Sub(t.chunks[0].at) > tapeKeep {
		t.chunks = t.chunks[1:]
		t.base++
	}
	close(t.more)
	t.more = make(chan struct{})
}

// seek is the number of the first chunk that arrived at or after at.
func (t *tape) seek(at time.Time) int {
	t.mu.Lock()
	defer t.mu.Unlock()
	for i, c := range t.chunks {
		if !c.at.Before(at) {
			return t.base + i
		}
	}
	return t.base + len(t.chunks)
}

// wait returns chunk seq, waiting for it to arrive; false once ctx ends. A chunk
// already dropped is skipped to the oldest kept.
func (t *tape) wait(ctx context.Context, seq int) (chunk, bool) {
	for {
		t.mu.Lock()
		if seq < t.base {
			seq = t.base
		}
		if i := seq - t.base; i < len(t.chunks) {
			c := t.chunks[i]
			t.mu.Unlock()
			return c, true
		}
		more := t.more
		t.mu.Unlock()
		select {
		case <-more:
		case <-ctx.Done():
			return chunk{}, false
		}
	}
}
