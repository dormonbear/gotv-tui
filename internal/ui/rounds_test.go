package ui

import (
	"strings"
	"testing"

	"github.com/dormonbear/gotv-tui/internal/gotv"
)

func TestHistoryLine(t *testing.T) {
	m := &matchModel{}
	m.recordRound(13, gotv.Event{Winner: gotv.T, Reason: "Bomb exploded"})
	m.recordRound(14, gotv.Event{Winner: gotv.CT, Reason: "Enemy eliminated"})
	m.recordRound(14, gotv.Event{Winner: gotv.CT, Reason: "Bomb defused"}) // replaced, not added
	got := plain(historyLine(m.rounds, 15, 80))
	// rounds 1-12 happened before we connected; a gap at halftime
	if want := " rounds ------------ ✱✓"; got != want {
		t.Fatalf("%q, want %q", got, want)
	}
	if historyLine(nil, 1, 80) != "" {
		t.Fatal("nothing to show in round 1")
	}
}

func TestEconomy(t *testing.T) {
	for _, c := range []struct {
		equip, round int
		want         string
	}{{800, 1, "pistol"}, {3000, 2, "eco"}, {7000, 3, "semi-eco"}, {15000, 4, "semi-buy"}, {24000, 5, "full buy"}, {900, 13, "pistol"}} {
		if got := buyType(c.equip, c.round); got != c.want {
			t.Errorf("buyType(%d, %d) = %s, want %s", c.equip, c.round, got, c.want)
		}
	}
	ps := []gotv.Player{{Side: gotv.CT, Money: 4000, Equip: 5200}, {Side: gotv.CT, Money: 7350, Equip: 5100}, {Side: gotv.T, Money: 99999, Equip: 99999}}
	if got := plain(econLine(gotv.CT, ps, 5)); !strings.Contains(got, "bank $11.3k · equipment $10.3k · semi-buy") {
		t.Fatalf("%q", got)
	}
}

func TestRoundsResetOnANewMatch(t *testing.T) {
	m := testMatch(false)
	m.st = &gotv.State{Map: "de_test", Round: 5}
	m.recordRound(4, gotv.Event{Winner: gotv.T, Reason: "Bomb exploded"})
	m.update(stateMsg(gotv.State{Map: "de_test", Round: 5}))
	if len(m.rounds) != 1 {
		t.Fatal("same match lost its history")
	}
	m.update(stateMsg(gotv.State{Map: "de_inferno", Round: 1}))
	if len(m.rounds) != 0 {
		t.Fatal("kept the previous match's rounds")
	}
}
