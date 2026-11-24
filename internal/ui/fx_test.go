package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/dormonbear/gotv-tui/internal/gotv"
)

func TestExplosionStartsAtThePlantedBomb(t *testing.T) {
	var f fx
	t0 := time.Unix(100, 0)
	f.onEvent(gotv.Event{Kind: gotv.EvExplode}, nil, t0)
	if f.boom(t0) != nil {
		t.Fatal("no bomb seen planted: no explosion")
	}
	f.onState(&gotv.State{Bomb: &gotv.Bomb{Pos: gotv.Vec{X: 10, Y: 20}, State: gotv.BombPlanted}}, t0)
	f.onEvent(gotv.Event{Kind: gotv.EvExplode}, nil, t0)
	if b := f.boom(t0.Add(500 * time.Millisecond)); b == nil || b.Pos.X != 10 || b.Age != 0.5 {
		t.Fatalf("explosion %+v", b)
	}
	if f.boom(t0.Add(2*time.Second)) != nil {
		t.Fatal("explosion should be over after 1.5 s")
	}
}

func TestWinnerBanner(t *testing.T) {
	var f fx
	t0 := time.Unix(100, 0)
	st := &gotv.State{CT: gotv.Team{Name: "100 Thieves", Score: 7}, T: gotv.Team{Name: "Imperial", Score: 5}}
	f.onEvent(gotv.Event{Kind: gotv.EvRoundEnd, Winner: gotv.CT, Reason: "Bomb defused"}, st, t0)
	block := strings.Repeat(strings.Repeat(" ", 60)+"\n", 4) + strings.Repeat(" ", 60)
	mid := func(at time.Duration) string { return strings.Split(f.overlay(block, t0.Add(at)), "\n")[2] }
	if got := plain(mid(2 * time.Second)); !strings.Contains(got, "100 THIEVES WIN  ·  Bomb defused") || ansi.StringWidth(mid(2*time.Second)) != 60 {
		t.Fatalf("banner row %q", got)
	}
	if got := plain(mid(100 * time.Millisecond)); strings.Contains(got, "defused") || !strings.Contains(got, "100") {
		t.Fatalf("banner should still be typing in: %q", got)
	}
	if got := plain(mid(4 * time.Second)); strings.TrimSpace(got) != "" {
		t.Fatalf("banner should be gone: %q", got)
	}
	st.Over = true
	f.onState(st, t0)
	if !strings.Contains(plain(mid(time.Second)), "100 THIEVES WIN THE MATCH  7 : 5") {
		t.Fatalf("match banner: %q", plain(mid(time.Second)))
	}
	f.banner = ""
	f.onState(st, t0)
	if f.banner != "" {
		t.Fatal("match banner should show once")
	}
}
