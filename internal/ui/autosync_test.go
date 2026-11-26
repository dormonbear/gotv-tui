package ui

import (
	"context"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	xansi "github.com/charmbracelet/x/ansi"

	"github.com/dormonbear/gotv-tui/internal/audio"
	"github.com/dormonbear/gotv-tui/internal/gotv"
)

// The data shows two rounds counting down; the stream shows the same clock 42 s
// later. Reads from one round alone fit every round, so nothing moves until reads
// from both rounds agree.
func TestAutoSyncFindsOffset(t *testing.T) {
	m := &matchModel{}
	_, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.sound.auto = autoSync{on: true, cancel: cancel}
	t0 := time.Unix(1000, 0)
	at := func(s float64) time.Time { return t0.Add(time.Duration(s * float64(time.Second))) }
	// round 5: 115 -> 40 s left over data time 0..75; round 6: 115 -> 0 over 90..205
	for i := 0; i <= 820; i++ {
		ts := float64(i) * 0.25
		switch {
		case ts <= 75:
			m.sound.auto.hist = append(m.sound.auto.hist, clockSample{t: at(ts), round: 5, id: 5, left: 115 - ts, live: true})
		case ts >= 90:
			m.sound.auto.hist = append(m.sound.auto.hist, clockSample{t: at(ts), round: 6, id: 6, left: 115 - (ts - 90), live: true})
		}
	}
	read := func(dataT float64, round int) { // what the stream shows 42 s after the data
		left := 115 - dataT
		if round == 6 {
			left = 115 - (dataT - 90)
		}
		m.onClockRead(audio.ClockRead{At: at(dataT + 42 - 0.5 + (left - float64(int(left)))), Secs: int(left)})
	}
	for _, d := range []float64{10, 20, 30, 40} {
		read(d, 5)
	}
	if m.sound.offset != 0 {
		t.Fatalf("one round's reads must not move it, got %v", m.sound.offset)
	}
	for _, d := range []float64{100, 110} {
		read(d, 6)
	}
	if math.Abs(m.sound.offset-42) > 0.15 {
		t.Fatalf("offset = %v, want 42", m.sound.offset)
	}
}

func TestManualKeyTurnsAutoOff(t *testing.T) {
	m := &matchModel{}
	m.sound.player = nil
	m.sound.auto.on = true
	m.sound.manual()
	if m.sound.auto.on {
		t.Fatal("manual adjust must turn auto sync off")
	}
}

func TestSoundBadge(t *testing.T) {
	m := &matchModel{}
	if m.soundBadge() != "" {
		t.Fatal("no badge without audio")
	}
	m.sound.player, m.sound.src, m.sound.offset = &audio.Player{}, "huya 123321", -2.5
	_, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.sound.auto.cancel = cancel
	b := xansi.Strip(m.soundBadge())
	if !strings.Contains(b, "HUYA 123321") || !strings.Contains(b, "-2.5s") || !strings.Contains(b, "AUTO") {
		t.Fatalf("badge %q", b)
	}
}

// The relay's round number can change mid-countdown; only the clock jumping
// starts a new countdown.
func TestAutoSyncRoundsFollowTheClock(t *testing.T) {
	m := &matchModel{}
	_, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.sound.auto = autoSync{on: true, cancel: cancel}
	t0 := time.Unix(1000, 0)
	for i, st := range []gotv.State{
		{Round: 21, SecondsLeft: 80}, {Round: 21, SecondsLeft: 79}, {Round: 22, SecondsLeft: 78}, // number moves, clock runs on
		{Round: 22, SecondsLeft: 15}, // freeze time: the clock jumped
		{Round: 22, SecondsLeft: 14},
	} {
		m.recordClock(&st, t0.Add(time.Duration(i)*time.Second))
	}
	var runs []int
	for _, h := range m.sound.auto.hist {
		runs = append(runs, h.round)
	}
	if fmt.Sprint(runs) != "[0 0 0 1 1]" {
		t.Fatalf("runs %v", runs)
	}
}

// Reads that name their round settle the offset within that round alone, and the
// stream's clock as heard now is shown next to the data's.
func TestAutoSyncWithRoundNumbers(t *testing.T) {
	m := &matchModel{}
	_, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.sound.auto = autoSync{on: true, cancel: cancel}
	t0 := time.Unix(1000, 0)
	at := func(s float64) time.Time { return t0.Add(time.Duration(s * float64(time.Second))) }
	for r := 0; r < 3; r++ { // rounds 7, 8, 9 each counting 115 -> 40 over 75 s, 90 s apart
		for i := 0; i <= 300; i++ {
			ts := float64(r*90) + float64(i)*0.25
			m.sound.auto.hist = append(m.sound.auto.hist, clockSample{t: at(ts), round: r, id: 7 + r, left: 115 - float64(i)*0.25, live: true})
		}
	}
	// round 8 seen on the stream 30 s later than the data showed it
	for _, d := range []float64{10, 14, 18, 22} {
		left := 115 - d
		m.onClockRead(audio.ClockRead{At: at(90 + d + 30 - 0.5 + (left - float64(int(left)))), Secs: int(left), Round: 8})
	}
	if math.Abs(m.sound.offset-30) > 0.15 || !m.sound.auto.locked {
		t.Fatalf("offset %v locked %v", m.sound.offset, m.sound.auto.locked)
	}
	r, secs, ok := m.sound.auto.streamNow(at(90+22+30+2), 0)
	if !ok || r != 8 || secs != 115-22-2 {
		t.Fatalf("stream now R%d %d %v", r, secs, ok)
	}
}

// While the stream is ahead, the badge says which moment the radar must reach.
func TestLockTargetWhileStreamAhead(t *testing.T) {
	a := &autoSync{}
	t0 := time.Unix(1000, 0)
	for i := 0; i < 40; i++ { // data: round 13, 76 s left and counting
		a.hist = append(a.hist, clockSample{t: t0.Add(time.Duration(i) * 250 * time.Millisecond), round: 0, id: 13, left: 86 - float64(i)*0.25, live: true})
	}
	for _, s := range []int{55, 54, 52, 51} { // stream: round 14
		a.reads = append(a.reads, audio.ClockRead{At: t0, Secs: s, Round: 14})
	}
	if r := a.lockTarget(); r == nil || r.Round != 14 || r.Secs != 52 {
		t.Fatalf("lock target %+v, want R14 0:52", r)
	}
	a.reads = []audio.ClockRead{{At: t0, Secs: 80, Round: 13}} // data has passed this one
	if a.lockTarget() != nil {
		t.Fatal("once the data has reached the reads there is nothing to wait for")
	}
}

// The bug behind a wrong lock: the stream's round clock (R16 0:39, 0:36, 0:35)
// matched the data's freeze/timeout countdown of the same round. Long stream
// clocks must only match the round's own clock.
func TestNoLockOnAFreezeCountdown(t *testing.T) {
	m := &matchModel{}
	_, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.sound.auto = autoSync{on: true, cancel: cancel}
	t0 := time.Unix(1000, 0)
	for i := 0; i <= 240; i++ { // data: round 16, a 60 s timeout before the round starts
		m.sound.auto.hist = append(m.sound.auto.hist, clockSample{t: t0.Add(time.Duration(i) * 250 * time.Millisecond), id: 16, left: 60 - float64(i)*0.25})
	}
	for _, s := range []int{39, 36, 35, 32} {
		m.onClockRead(audio.ClockRead{At: t0.Add(time.Duration(70-s) * time.Second), Secs: s, Round: 16})
	}
	if m.sound.auto.locked || m.sound.offset != 0 {
		t.Fatalf("locked on a freeze countdown: offset %v", m.sound.offset)
	}
}

// When the stream's round number is readable, short clocks (which also fit the
// data's freeze times) never vote: they once locked a stream that was ahead onto
// a positive offset.
func TestShortClocksDontVoteWhenRoundsAreRead(t *testing.T) {
	m := &matchModel{}
	_, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.sound.auto = autoSync{on: true, cancel: cancel}
	t0 := time.Unix(1000, 0)
	at := func(s float64) time.Time { return t0.Add(time.Duration(s * float64(time.Second))) }
	for r := 0; r < 2; r++ { // two freeze times (20 s) in the data, 100 s apart
		for i := 0; i <= 80; i++ {
			ts := float64(r*100) + float64(i)*0.25
			m.sound.auto.hist = append(m.sound.auto.hist, clockSample{t: at(ts), round: r, id: 20 + r, left: 20 - float64(i)*0.25})
		}
	}
	m.sound.auto.reads = append(m.sound.auto.reads, audio.ClockRead{At: at(300), Secs: 70, Round: 22}) // the round number is read
	for r := 0; r < 2; r++ {                                                                           // the stream's round ends (short clocks) fit those freezes 30 s later
		for _, s := range []int{12, 9, 6} {
			m.onClockRead(audio.ClockRead{At: at(float64(r*100) + 20 - float64(s) - 0.5 + 30), Secs: s})
		}
	}
	if m.sound.auto.locked {
		t.Fatalf("locked on short clocks: offset %v", m.sound.offset)
	}
}
