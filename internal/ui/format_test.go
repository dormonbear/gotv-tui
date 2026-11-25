package ui

import (
	"regexp"
	"strings"
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
	if h := plain(roundStatus(st)); h != "MATCH OVER" {
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

func TestStatsView(t *testing.T) {
	p := gotv.Player{Name: "donk", Side: gotv.T, Kills: 24, Deaths: 12, Assists: 3, ADR: 112.4,
		Stats: gotv.Stats{Rounds: 20, HSKills: 12, KASRounds: 16, UtilDamage: 140, Flashed: 7, Multi: [3]int{3, 1, 0}}}
	nums := map[string]int{"donk": 1}
	rows := TeamTable(gotv.T, gotv.Team{Name: "Spirit"}, []gotv.Player{p}, nums, "", false, true)
	head, row := plain(rows[0]), plain(rows[1])
	for _, want := range []string{"+12", "112", "50%", "80%", "1.20", "140", "7", " 3  1 "} {
		if !strings.Contains(row, want) {
			t.Errorf("stats row %q lacks %q", row, want)
		}
	}
	// each number sits under its column title
	for col, val := range map[string]string{"HS%": "50%", "KAS": "80%", "KPR": "1.20"} {
		if strings.Index(head, col)+len(col) != strings.Index(row, val)+len(val) {
			t.Errorf("%s not right-aligned with %s:\n%s\n%s", val, col, head, row)
		}
	}
	compact := plain(TeamTable(gotv.T, gotv.Team{Name: "Spirit"}, []gotv.Player{p}, nums, "", true, true)[1])
	if !strings.Contains(compact, "50%") || len([]rune(compact)) > compactWidth {
		t.Errorf("compact stats row %q (%d wide)", compact, len([]rune(compact)))
	}
}
