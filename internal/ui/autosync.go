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
	locked bool              // the offset has been set from the stream at least once
	floor  bool              // the HUD rounds its clock down (default: up), as the freeze time's end tells
}

type clockSample struct {
	t     time.Time // when the data showed it, before any sync delay
	round int       // which unbroken countdown: a round's live time, or its freeze time
	id    int       // the round by the score (rounds played + 1), as the HUD numbers it
	left  float64
	live  bool // the round's own clock, not the freeze time or a pause before it
}

type clockReadMsg audio.ClockRead

const (
	syncKeep       = 8 * time.Minute // a stream can lag the relay by minutes, either way
	syncWindow     = 1.0             // seconds of spread allowed among agreeing reads
	syncRounds     = 2               // rounds the agreeing reads must span
	syncReads      = 4
	syncKnownReads = 3 // reads that named their round: fewer will do
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
	tl := &audio.Timeline{} // the clock reads and the sound share one notion of stream time
	if m.sound.player != nil {
		m.sound.player.SetTimeline(tl)
	}
	go func() {
		err := audio.WatchClock(ctx, src, tl, func(r audio.ClockRead) { send(clockReadMsg(r), false) })
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
	a.hist = append(a.hist, clockSample{t, a.run, s.CT.Score + s.T.Score + 1, s.SecondsLeft, !s.Freeze})
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
	off, ok := a.vote()
	if ok && !a.locked {
		a.locked = true
		m.status = fmt.Sprintf("auto sync: locked, offset %+.1fs", off)
	}
	if ok && math.Abs(off-m.sound.offset) >= 0.3 {
		m.setOffset(off)
		m.status = fmt.Sprintf("auto sync: offset %+.1fs", off)
	}
}

type syncCand struct {
	off         float64
	read, round int
	known       bool // matched on the read's round number too
	tick        bool // from the moment the clock ticked: good to a frame
}

// roundTrust: below this many seconds the HUD may still show the last round's
// number over the next round's freeze time, so the number isn't used.
const roundTrust = 25

// candidates: offsets (audio time - data time) at which the data showed r's second.
func (a *autoSync) candidates(r audio.ClockRead) []syncCand {
	// The HUD shows whole seconds. Rounding up, it turns to Secs as the time left
	// reaches Secs; rounding down, at Secs+1. A tick read is that exact moment;
	// any other read is somewhere in the second, so aim at its middle.
	target := float64(r.Secs)
	if a.floor {
		target++
	}
	if !r.Tick {
		target -= 0.5
	}
	known := r.Round > 0 && r.Secs >= roundTrust
	var out []syncCand
	for i := 1; i < len(a.hist); i++ {
		p, q := a.hist[i-1], a.hist[i]
		if p.round != q.round || p.left < target || q.left >= target || p.left-q.left > 3 {
			continue
		}
		if known && (p.id != r.Round || q.id != r.Round) {
			continue
		}
		// a long clock on the stream is the round's own: never match it to a freeze
		// time or a timeout counting through the same seconds
		if r.Secs >= roundTrust && !(p.live && q.live) {
			continue
		}
		f := (p.left - target) / (p.left - q.left)
		t := p.t.Add(time.Duration(f * float64(q.t.Sub(p.t))))
		out = append(out, syncCand{off: r.At.Sub(t).Seconds(), round: p.round, known: known, tick: r.Tick})
	}
	return out
}

// vote finds the offset that reads agree on, and that no other offset matches as
// well: reads that named their round settle it alone; otherwise reads from at least
// two countdowns must agree.
func (a *autoSync) voteOnce() (float64, bool) {
	named := false // the stream's round number can be read: trust only reads that name it
	for _, r := range a.reads {
		named = named || r.Round > 0
	}
	var all []syncCand
	for i, r := range a.reads {
		if named && (r.Round == 0 || r.Secs < roundTrust) {
			continue // short clocks also fit freeze times; leave them to HUDs without a round number
		}
		for _, c := range a.candidates(r) {
			c.read = i
			all = append(all, c)
		}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].off < all[j].off })
	type win struct {
		lo, hi, rounds, reads int
		known                 bool
	}
	var wins []win
	for lo, hi := 0, 0; lo < len(all); lo++ {
		for hi < len(all) && all[hi].off-all[lo].off <= syncWindow {
			hi++
		}
		rounds, reads, known := map[int]bool{}, map[int]bool{}, true
		for _, c := range all[lo:hi] {
			rounds[c.round], reads[c.read], known = true, true, known && c.known
		}
		wins = append(wins, win{lo, hi, len(rounds), len(reads), known})
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
	settled := best.known && best.reads >= syncKnownReads && best.reads > rival.reads ||
		best.rounds >= syncRounds && best.rounds > rival.rounds
	if !settled || !best.known && best.reads < syncReads {
		return 0, false
	}
	var offs, ticks []float64 // the ticks, good to a frame, decide if there are enough
	for _, c := range all[best.lo:best.hi] {
		offs = append(offs, c.off)
		if c.tick {
			ticks = append(ticks, c.off)
		}
	}
	if len(ticks) >= syncKnownReads {
		offs = ticks
	}
	sort.Float64s(offs)
	return math.Round(offs[len(offs)/2]*10) / 10, true
}

// streamNow is the stream's round and clock as heard now (the audio is held back
// by audioDelay): the last read that has played, run on to now.
func (a *autoSync) streamNow(now time.Time, audioDelay time.Duration) (round, secs int, ok bool) {
	for i := len(a.reads) - 1; i >= 0; i-- {
		r := a.reads[i]
		heard := r.At.Add(audioDelay)
		if heard.After(now) {
			continue
		}
		gone := now.Sub(heard)
		if gone > 6*time.Second || r.Secs-int(gone.Seconds()) < 0 {
			return 0, 0, false
		}
		return r.Round, r.Secs - int(gone.Seconds()), true
	}
	return 0, 0, false
}

// lockTarget is the stream moment the data has to reach before auto sync can lock
// (the last of the first reads that named their round), when the data hasn't
// reached it yet; nil otherwise.
func (a *autoSync) lockTarget() *audio.ClockRead {
	n := 0
	for i := range a.reads {
		r := &a.reads[i]
		if r.Round == 0 || r.Secs < roundTrust {
			continue
		}
		if len(a.candidates(*r)) > 0 {
			return nil // the data has reached the stream's reads: locking is up to the vote
		}
		if n++; n == syncKnownReads {
			return r
		}
	}
	return nil
}

// vote settles the offset. Once the stream's freeze time has been seen ending
// (the clock jumping up to the round's start), that moment, which no rounding
// blurs, also decides whether the HUD rounds up or down.
func (a *autoSync) vote() (float64, bool) {
	jump, ok := a.jumpOffset()
	if !ok {
		return a.voteOnce()
	}
	up, okUp := a.withFloor(false)
	down, okDown := a.withFloor(true)
	switch {
	case okUp && okDown:
		a.floor = math.Abs(down-jump) < math.Abs(up-jump)
	case okDown:
		a.floor = true
	case okUp:
		a.floor = false
	default:
		return 0, false
	}
	if a.floor {
		return down, true
	}
	return up, true
}

func (a *autoSync) withFloor(floor bool) (float64, bool) {
	keep := a.floor
	a.floor = floor
	defer func() { a.floor = keep }()
	return a.voteOnce()
}

// roundLen is a competitive round's clock at its start (1:55).
const roundLen = 115

// jumpOffset is the offset given by the freeze times' ends seen on the stream:
// each against the moment the data's round of that number started.
func (a *autoSync) jumpOffset() (float64, bool) {
	var offs []float64
	for _, r := range a.reads {
		if !r.Jump || r.Round == 0 {
			continue
		}
		for i := 1; i < len(a.hist); i++ {
			p, q := a.hist[i-1], a.hist[i]
			if p.live || !q.live || q.id != r.Round || q.left < roundLen-3 {
				continue
			}
			start := q.t.Add(-time.Duration((roundLen - q.left) * float64(time.Second)))
			offs = append(offs, r.At.Sub(start).Seconds())
		}
	}
	if len(offs) == 0 {
		return 0, false
	}
	sort.Float64s(offs)
	return offs[len(offs)/2], true
}
