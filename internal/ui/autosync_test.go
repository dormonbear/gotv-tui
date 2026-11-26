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
			m.sound.auto.hist = append(m.sound.auto.hist, clockSample{at(ts), 5, 115 - ts})
		case ts >= 90:
			m.sound.auto.hist = append(m.sound.auto.hist, clockSample{at(ts), 6, 115 - (ts - 90)})
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
