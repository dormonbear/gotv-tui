package ui

import (
	"regexp"
	"testing"

	"github.com/dormonbear/gotv-tui/internal/gotv"
)

var esc = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func plain(s string) string { return esc.ReplaceAllString(s, "") }

func TestRoundStatusAboveMap(t *testing.T) {
	st := &gotv.State{Map: "de_ancient", Round: 22, SecondsLeft: 78,
		CT: gotv.Team{Name: "Lynn Vision Gaming", Score: 9}, T: gotv.Team{Name: "Ninjas in Pyjamas", Score: 12},
		Bomb: &gotv.Bomb{State: gotv.BombDefusing, SecondsLeft: 31.2, DefuseLeft: 4.25}}
	if h := plain(roundStatus(st)); h != "R22 1:18   BOMB 32s · DEFUSING 4.2s " {
		t.Fatalf("%q", h)
	}
	if h := plain(header(st)); h != " Lynn Vision Gaming 9 : 12 Ninjas in Pyjamas  Ancient" {
		t.Fatalf("%q", h)
	}
	st.Over = true
	if h := plain(roundStatus(st)); h != "match over" {
		t.Fatalf("%q", h)
	}
}

func TestKillLine(t *testing.T) {
	e := gotv.Event{Kind: gotv.EvKill, Killer: &gotv.Who{Name: "stavn", Side: gotv.T}, Victim: &gotv.Who{Name: "z4KR", Side: gotv.CT},
		Assister: &gotv.Who{Name: "adamb", Side: gotv.T}, AssistFlash: true, Weapon: "AWP", Headshot: true}
	if got := plain(FormatEvent(e)); got != "stavn + adamb(flash) [AWP HS] z4KR" {
		t.Fatal(got)
	}
	e.Killer = e.Victim
	if got := plain(FormatEvent(e)); got != "z4KR died [AWP]" {
		t.Fatal(got)
	}
}
