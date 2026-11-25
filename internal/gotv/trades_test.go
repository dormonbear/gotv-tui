package gotv

import (
	"testing"
	"time"

	"github.com/markus-wa/demoinfocs-golang/v6/pkg/demoinfocs/common"
	"github.com/markus-wa/demoinfocs-golang/v6/pkg/demoinfocs/events"
)

func TestTrades(t *testing.T) {
	ct := func(n string) *common.Player { return &common.Player{Name: n, Team: common.TeamCounterTerrorists} }
	tt := func(n string) *common.Player { return &common.Player{Name: n, Team: common.TeamTerrorists} }
	tr := trades{}
	var d []death
	d = tr.record(d, events.Kill{Killer: tt("k"), Victim: ct("a")}, 3, 10*time.Second) // a dies to k
	d = tr.record(d, events.Kill{Killer: ct("b"), Victim: tt("k")}, 3, 13*time.Second) // b kills k 3 s later: a traded
	d = tr.record(d, events.Kill{Killer: tt("z"), Victim: ct("c")}, 3, 20*time.Second) // c dies to z
	tr.record(d, events.Kill{Killer: ct("b"), Victim: tt("z")}, 3, 26*time.Second)     // 6 s later: too late
	if !tr["a"][3] {
		t.Fatal("a was traded within 5 s")
	}
	if tr["c"][3] {
		t.Fatal("c's killer died 6 s later: not a trade")
	}
}
