package gotv

import (
	"time"

	"github.com/markus-wa/demoinfocs-golang/v6/pkg/demoinfocs/common"
	"github.com/markus-wa/demoinfocs-golang/v6/pkg/demoinfocs/events"
)

// tradeWindow: a death is traded when a teammate kills the killer this soon after.
const tradeWindow = 5 * time.Second

// trades are the rounds (0-based) in which each player died and was traded, the
// T in KAST. Only kills seen since connecting count.
type trades map[string]map[int]bool

type death struct {
	victim, killer string
	side           common.Team
	at             time.Duration
}

// record notes a kill and marks any death it trades; it returns this round's deaths.
func (t trades) record(deaths []death, e events.Kill, round int, now time.Duration) []death {
	if e.Victim == nil {
		return deaths
	}
	if e.Killer != nil && e.Killer.Team != e.Victim.Team {
		for _, d := range deaths {
			if d.killer == e.Victim.Name && d.side == e.Killer.Team && now-d.at <= tradeWindow {
				if t[d.victim] == nil {
					t[d.victim] = map[int]bool{}
				}
				t[d.victim][round] = true
			}
		}
	}
	if e.Killer == nil {
		return deaths
	}
	return append(deaths, death{e.Victim.Name, e.Killer.Name, e.Victim.Team, now})
}
