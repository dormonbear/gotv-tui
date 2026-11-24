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

func TestFollowedPlayerRowIsFramed(t *testing.T) {
	ps := []gotv.Player{{Name: "s1n", Side: gotv.T, Alive: true, HP: 100}, {Name: "JBa", Side: gotv.T, Alive: false}}
	nums := PlayerNumbers(ps)
	rows := TeamTable(gotv.T, gotv.Team{Name: "M80"}, ps, nums, "jba", false)
	plainRow := plain(rows[1]) // JBa sorts first
	if !strings.Contains(rows[1], "\x1b[53;4") || !strings.HasPrefix(plainRow, "▌1 ") {
		t.Fatalf("followed row not framed: %q", rows[1])
	}
	if w := ansi.StringWidth(rows[1]); w != rowWidth {
		t.Fatalf("framed row is %d wide, want the table's %d", w, rowWidth)
	}
	if strings.Contains(rows[2], "\x1b[53") || !strings.HasPrefix(plain(rows[2]), " 2 ") {
		t.Fatalf("other row changed: %q", rows[2])
	}
	// columns stay aligned: the name starts at the same place on both rows
	col := func(s, sub string) int { return ansi.StringWidth(s[:strings.Index(s, sub)]) }
	if col(plain(rows[1]), "JBa") != col(plain(rows[2]), "s1n") {
		t.Fatalf("misaligned:\n%q\n%q", plain(rows[1]), plain(rows[2]))
	}
}
