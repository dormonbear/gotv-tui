package ui

import (
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/dormonbear/gotv-tui/internal/gotv"
)

var esc = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func plain(s string) string { return esc.ReplaceAllString(s, "") }

func TestHeaderKeepsCountdownsWhenCut(t *testing.T) {
	st := &gotv.State{Map: "de_ancient", Round: 22, SecondsLeft: 78,
		CT: gotv.Team{Name: "Lynn Vision Gaming", Score: 9}, T: gotv.Team{Name: "Ninjas in Pyjamas", Score: 12},
		Bomb: &gotv.Bomb{State: gotv.BombDefusing, SecondsLeft: 31.2, DefuseLeft: 4.25}}
	h := plain(ansi.Truncate(header(st), 40, ""))
	if !strings.HasPrefix(h, " 1:18   BOMB 32s · DEFUSING 4.2s   Lynn") {
		t.Fatalf("%q", h)
	}
	st.Over, st.Freeze = true, true
	if h := plain(header(st)); !strings.HasPrefix(h, " match over  Lynn") {
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
