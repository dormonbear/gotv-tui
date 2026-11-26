package ui

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/dormonbear/gotv-tui/internal/audio"
	"github.com/dormonbear/gotv-tui/internal/gotv"
)

// autoSync keeps the audio lined up on its own: it reads the round clock off the
// stream's picture and finds when our data showed the same second. A clock value
// recurs every round, so each read fits several offsets; the true one is the offset
// that reads across different rounds all agree on.
type autoSync struct {
	on     bool
	cancel context.CancelFunc
	hist   []clockSample     // when the undelayed data showed each round time
	reads  []audio.ClockRead // recent stream reads
	run    int               // the data clock's current unbroken countdown
}

type clockSample struct {
	t     time.Time // when the data showed it, before any sync delay
	round int       // which unbroken countdown: a round's live time, or its freeze time
	left  float64
}

type clockReadMsg audio.ClockRead

const (
	syncKeep   = 8 * time.Minute // a stream can lag the relay by minutes, either way
	syncWindow = 1.0             // seconds of spread allowed among agreeing reads
	syncRounds = 2               // rounds the agreeing reads must span
	syncReads  = 4
)

func (a *autoSync) stop() {
	if a.cancel != nil {
		a.cancel()
	}
	*a = autoSync{on: a.on}
}

// startAutoSync starts reading src's clock, if auto sync is on.
func (m *matchModel) startAutoSync(src string) {
	a := &m.sound.auto
	a.stop()
	if !a.on {
		return
	}
	ctx, cancel := context.WithCancel(m.ctx)
	a.cancel = cancel
	send := m.send
	go func() {
		err := audio.WatchClock(ctx, src, func(r audio.ClockRead) { send(clockReadMsg(r), false) })
		if err != nil && ctx.Err() == nil {
			send(soundStatusMsg("auto sync: "+err.Error()), false)
		}
	}()
}

// recordClock notes when the data (without our delay) showed this round time.
func (m *matchModel) recordClock(s *gotv.State, now time.Time) {
	a := &m.sound.auto
	if a.cancel == nil || s.SecondsLeft < 0 || s.Paused {
		return
	}
	t := now.Add(-time.Duration(m.sound.delay() * float64(time.Second)))
	if n := len(a.hist); n > 0 {
		p := a.hist[n-1]
		if t.Sub(p.t) < 250*time.Millisecond {
			return
		}
		// The relay's round number moves mid-round, so rounds are told apart by
		// the clock itself: within one it runs down with time, without jumps.
		dt := t.Sub(p.t).Seconds()
		if math.Abs(p.left-dt-s.SecondsLeft) > 2 || dt > 5 {
			a.run++
		}
	}
	a.hist = append(a.hist, clockSample{t, a.run, s.SecondsLeft})
	for len(a.hist) > 0 && t.Sub(a.hist[0].t) > syncKeep {
		a.hist = a.hist[1:]
	}
}

func (m *matchModel) onClockRead(r audio.ClockRead) {
	a := &m.sound.auto
	if a.cancel == nil {
		return
	}
	a.reads = append(a.reads, r)
	for len(a.reads) > 0 && r.At.Sub(a.reads[0].At) > syncKeep {
		a.reads = a.reads[1:]
	}
	if off, ok := a.vote(); ok && math.Abs(off-m.sound.offset) >= 0.3 {
		m.sound.offset = off
		m.sound.apply()
		m.status = fmt.Sprintf("auto sync: offset %+.1fs", off)
	}
}

type syncCand struct {
	off         float64
	read, round int
}

// candidates: offsets (audio time - data time) at which the data showed r's second.
func (a *autoSync) candidates(r audio.ClockRead) []syncCand {
	target := float64(r.Secs) + 0.5 // the HUD shows a whole second; aim at its middle
	var out []syncCand
	for i := 1; i < len(a.hist); i++ {
		p, q := a.hist[i-1], a.hist[i]
		if p.round != q.round || p.left < target || q.left >= target || p.left-q.left > 3 {
			continue
		}
		f := (p.left - target) / (p.left - q.left)
		t := p.t.Add(time.Duration(f * float64(q.t.Sub(p.t))))
		out = append(out, syncCand{off: r.At.Sub(t).Seconds(), round: p.round})
	}
	return out
}

// vote finds the offset that reads from at least two countdowns agree on, and that
// no other offset matches as well.
func (a *autoSync) vote() (float64, bool) {
	var all []syncCand
	for i, r := range a.reads {
		for _, c := range a.candidates(r) {
			c.read = i
			all = append(all, c)
		}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].off < all[j].off })
	type win struct{ lo, hi, rounds, reads int }
	var wins []win
	for lo, hi := 0, 0; lo < len(all); lo++ {
		for hi < len(all) && all[hi].off-all[lo].off <= syncWindow {
			hi++
		}
		rounds, reads := map[int]bool{}, map[int]bool{}
		for _, c := range all[lo:hi] {
			rounds[c.round], reads[c.read] = true, true
		}
		wins = append(wins, win{lo, hi, len(rounds), len(reads)})
	}
	better := func(x, y win) bool { return x.rounds > y.rounds || (x.rounds == y.rounds && x.reads > y.reads) }
	var best, rival win
	for _, w := range wins {
		if better(w, best) {
			best = w
		}
	}
	for _, w := range wins { // the best of the offsets clearly apart from it
		if math.Abs(all[w.lo].off-all[best.lo].off) > 2*syncWindow && better(w, rival) {
			rival = w
		}
	}
	if best.rounds < syncRounds || best.reads < syncReads || best.rounds <= rival.rounds {
		return 0, false
	}
	offs := make([]float64, 0, best.hi-best.lo)
	for _, c := range all[best.lo:best.hi] {
		offs = append(offs, c.off)
	}
	return math.Round(offs[len(offs)/2]*10) / 10, true
}
