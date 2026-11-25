// Package playback re-times CSTV data. Fragments arrive once a second and their ~64
// ticks are parsed in a burst; samples are queued by tick and released at wall-clock
// pace, a buffer behind the newest tick, so the UI moves smoothly.
package playback

import "sync"

type item[T any] struct {
	tick int
	v    T
}

type Playback[S, E any] struct {
	mu      sync.Mutex
	tps     float64
	buffer  float64 // seconds behind the newest tick
	resync  float64 // fell this far behind (stall, catch-up burst): jump instead of crawling
	frames  []item[S]
	events  []item[E]
	play    float64
	wall    float64 // ms
	started bool
}

func New[S, E any](tps, bufferS, resyncS float64) *Playback[S, E] {
	return &Playback[S, E]{tps: tps, buffer: bufferS, resync: resyncS}
}

// Pending reports whether anything is still queued.
func (p *Playback[S, E]) Pending() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.frames)+len(p.events) > 0
}

func (p *Playback[S, E]) PushFrame(tick int, s S) {
	p.mu.Lock()
	p.frames = append(p.frames, item[S]{tick, s})
	p.mu.Unlock()
}

func (p *Playback[S, E]) PushEvent(tick int, e E) {
	p.mu.Lock()
	p.events = append(p.events, item[E]{tick, e})
	p.mu.Unlock()
}

// Step advances the playback clock to wall time nowMs and returns what became due.
func (p *Playback[S, E]) Step(nowMs float64) (frame *S, events []E) {
	p.mu.Lock()
	defer p.mu.Unlock()
	newest := -1
	if n := len(p.frames); n > 0 {
		newest = p.frames[n-1].tick
	}
	if n := len(p.events); n > 0 && p.events[n-1].tick > newest {
		newest = p.events[n-1].tick
	}
	if newest < 0 {
		return nil, nil
	}
	if !p.started || float64(newest)-p.play > p.tps*p.resync {
		p.play = float64(newest) - p.tps*p.buffer
		p.started = true
	} else {
		p.play = min(float64(newest), p.play+(nowMs-p.wall)/1000*p.tps)
	}
	p.wall = nowMs
	i := 0
	for ; i < len(p.frames) && float64(p.frames[i].tick) <= p.play; i++ {
		f := p.frames[i].v
		frame = &f
	}
	p.frames = p.frames[i:]
	j := 0
	for ; j < len(p.events) && float64(p.events[j].tick) <= p.play; j++ {
		events = append(events, p.events[j].v)
	}
	p.events = p.events[j:]
	return frame, events
}

// Soon is a queued event and how many seconds until it plays.
type Soon[E any] struct {
	In float64
	E  E
}

// Ahead peeks at what is queued but not yet played: events due within horizon
// seconds, and the latest frame inside that horizon. Call it after Step.
func (p *Playback[S, E]) Ahead(horizon float64) (events []Soon[E], future *S) {
	p.mu.Lock()
	defer p.mu.Unlock()
	until := p.play + horizon*p.tps
	for _, e := range p.events {
		if float64(e.tick) > until {
			break
		}
		events = append(events, Soon[E]{(float64(e.tick) - p.play) / p.tps, e.v})
	}
	for i := range p.frames {
		if float64(p.frames[i].tick) > until {
			break
		}
		f := p.frames[i].v
		future = &f
	}
	return events, future
}
