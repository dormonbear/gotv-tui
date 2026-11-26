package ui

import (
	"context"
	"math"
	"strings"
	"testing"
	"time"

	xansi "github.com/charmbracelet/x/ansi"

	"github.com/dormonbear/gotv-tui/internal/audio"
)

// The data shows round 5 counting down from 100 s; the stream shows the same
// clock 42 s later. Two agreeing reads must set the offset to +42.
func TestAutoSyncFindsOffset(t *testing.T) {
	m := &matchModel{}
	_, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.sound.auto = autoSync{on: true, cancel: cancel}
	t0 := time.Unix(1000, 0)
	for i := 0; i <= 200; i++ { // 4 samples a second for 50 s
		at := t0.Add(time.Duration(i) * 250 * time.Millisecond)
		m.sound.auto.hist = append(m.sound.auto.hist, clockSample{at, 5, 100 - float64(i)*0.25})
	}
	// the HUD shows 90 from left=91 to 90, i.e. data time t0+9..10 s; mid 9.5 s
	m.onClockRead(audio.ClockRead{At: t0.Add(51500 * time.Millisecond), Round: 5, Secs: 90})
	if m.sound.offset != 0 {
		t.Fatalf("one read must not move it, got %v", m.sound.offset)
	}
	m.onClockRead(audio.ClockRead{At: t0.Add(61500 * time.Millisecond), Round: 5, Secs: 80})
	if math.Abs(m.sound.offset-42) > 0.05 {
		t.Fatalf("offset = %v, want 42", m.sound.offset)
	}
	// a read from another round, or not reached yet, is held, not used
	m.onClockRead(audio.ClockRead{At: t0.Add(70 * time.Second), Round: 6, Secs: 80})
	if len(m.sound.auto.reads) != 1 || m.sound.offset != 42 {
		t.Fatalf("reads %d offset %v", len(m.sound.auto.reads), m.sound.offset)
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
