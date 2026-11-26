package ui

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/dormonbear/gotv-tui/internal/audio"
	"github.com/dormonbear/gotv-tui/internal/gotv"
)

// autoSync keeps the audio lined up on its own: it reads the round clock off the
// stream's picture and finds when our data showed the same round and second.
type autoSync struct {
	on      bool
	cancel  context.CancelFunc
	hist    []clockSample     // when the undelayed data showed each round time
	reads   []audio.ClockRead // stream reads the data hasn't reached yet
	last    float64           // the previous estimate, to check the next against
	hasLast bool
}

type clockSample struct {
	t     time.Time // when the data showed it, before any sync delay
	round int
	left  float64
}

type clockReadMsg audio.ClockRead

const (
	syncKeep  = 8 * time.Minute // a stream can lag the relay by minutes, either way
	syncAgree = 1.0             // seconds two estimates must agree within to be used
)

var hasOCR = audio.HasOCR

func (a *autoSync) stop() {
	if a.cancel != nil {
		a.cancel()
	}
	*a = autoSync{on: a.on}
}

// startAutoSync starts reading src's clock, if auto sync is on and can run.
func (m *matchModel) startAutoSync(src string) {
	a := &m.sound.auto
	a.stop()
	if !a.on {
		return
	}
	if !hasOCR() {
		m.status = "auto sync needs tesseract (brew install tesseract); sync by hand with t [ ]"
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
	if n := len(a.hist); n > 0 && t.Sub(a.hist[n-1].t) < 250*time.Millisecond {
		return
	}
	a.hist = append(a.hist, clockSample{t, s.Round, s.SecondsLeft})
	for len(a.hist) > 0 && t.Sub(a.hist[0].t) > syncKeep {
		a.hist = a.hist[1:]
	}
	if len(a.reads) > 0 {
		m.matchReads(now)
	}
}

func (m *matchModel) onClockRead(r audio.ClockRead) {
	a := &m.sound.auto
	if a.cancel == nil {
		return
	}
	a.reads = append(a.reads, r)
	m.matchReads(r.At)
}

// matchReads turns every stream read the data has caught up with into an estimate.
func (m *matchModel) matchReads(now time.Time) {
	a := &m.sound.auto
	kept := a.reads[:0]
	for _, r := range a.reads {
		cands := a.candidates(r)
		switch {
		case len(cands) > 0:
			m.feedSync(a.pick(cands))
		case now.Sub(r.At) < syncKeep:
			kept = append(kept, r)
		}
	}
	a.reads = kept
}

// candidates: offsets (audio time - data time) at which the data showed r's round
// and second. A short clock can match both the freeze time and the round's end.
func (a *autoSync) candidates(r audio.ClockRead) []float64 {
	target := float64(r.Secs) + 0.5 // the HUD shows a whole second; aim at its middle
	var out []float64
	for i := 1; i < len(a.hist); i++ {
		p, q := a.hist[i-1], a.hist[i]
		if p.round != r.Round || q.round != r.Round || p.left < target || q.left >= target || p.left-q.left > 3 {
			continue
		}
		f := (p.left - target) / (p.left - q.left)
		t := p.t.Add(time.Duration(f * float64(q.t.Sub(p.t))))
		out = append(out, r.At.Sub(t).Seconds())
	}
	return out
}

func (a *autoSync) pick(cands []float64) float64 {
	best := cands[len(cands)-1] // latest: the live round, not its freeze time
	if a.hasLast {
		for _, c := range cands {
			if math.Abs(c-a.last) < math.Abs(best-a.last) {
				best = c
			}
		}
	}
	return best
}

// feedSync applies an estimate once the one before it agrees.
func (m *matchModel) feedSync(est float64) {
	a := &m.sound.auto
	prev, had := a.last, a.hasLast
	a.last, a.hasLast = est, true
	if !had || math.Abs(est-prev) > syncAgree {
		return
	}
	agreed := math.Round((est+prev)/2*10) / 10
	if math.Abs(agreed-m.sound.offset) < 0.3 {
		return
	}
	m.sound.offset = agreed
	m.sound.apply()
	m.status = fmt.Sprintf("auto sync: offset %+.1fs", agreed)
}
