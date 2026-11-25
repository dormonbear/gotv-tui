package ui

import (
	"fmt"
	"strings"

	"github.com/dormonbear/gotv-tui/internal/gotv"
	"github.com/dormonbear/gotv-tui/internal/term"
)

// roundResult is one finished round, as seen live.
type roundResult struct {
	n      int
	winner gotv.Side
	reason string
}

// recordRound keeps the result of round n (a replayed round end replaces it).
func (m *matchModel) recordRound(n int, e gotv.Event) {
	if n <= 0 {
		return
	}
	for i := range m.rounds {
		if m.rounds[i].n == n {
			m.rounds[i] = roundResult{n, e.Winner, e.Reason}
			return
		}
	}
	m.rounds = append(m.rounds, roundResult{n, e.Winner, e.Reason})
}

var reasonGlyph = map[string]string{
	"Enemy eliminated": "✕",
	"Bomb exploded":    "✱",
	"Bomb defused":     "✓",
	"Time ran out":     "~",
	"Surrender":        "s",
}

// historyLine is one glyph per round, coloured by the winning side: ✕ elimination,
// ✱ bomb, ✓ defuse, ~ time. Outcomes come from the game rules (known) and from round
// ends seen live (rs, which also cover overtime past the rules' 30 rounds); a round
// with neither shows as "-". A gap marks
// halftime (after 12) and each overtime half (every 3 after 24).
func historyLine(known []gotv.Outcome, rs []roundResult, current, width int) string {
	byN := map[int]roundResult{}
	for i, o := range known { // from the game rules: covers rounds before we connected
		if o.Winner != "" {
			byN[i+1] = roundResult{i + 1, o.Winner, o.Reason}
		}
	}
	last := current - 1
	for _, r := range rs {
		byN[r.n] = r
		last = max(last, r.n)
	}
	if last < 1 {
		return ""
	}
	var cells []string // one per round, plus a gap after each half
	for n := 1; n <= last; n++ {
		if r, ok := byN[n]; ok {
			g := reasonGlyph[r.reason]
			if g == "" {
				g = "•"
			}
			cells = append(cells, term.Fg(sideColor(r.winner))+g+reset)
		} else {
			cells = append(cells, dim("-"))
		}
		if n == 12 || (n > 24 && (n-24)%3 == 0) {
			cells = append(cells, " ")
		}
	}
	const label = " rounds "
	if room := width - len(label); len(cells) > room { // keep the newest rounds
		cells = cells[len(cells)-max(room, 0):]
	}
	return dim(label) + strings.Join(cells, "")
}

// buyType classifies a team's equipment value the way HLTV does.
func buyType(equip, round int) string {
	switch {
	case round == 1 || round == 13:
		return "PISTOL"
	case equip < 5000:
		return "ECO"
	case equip < 10000:
		return "SEMI-ECO"
	case equip < 20000:
		return "SEMI-BUY"
	}
	return "FULL BUY"
}

func money(v int) string {
	if v >= 10000 {
		return fmt.Sprintf("$%.1fk", float64(v)/1000)
	}
	return fmt.Sprintf("$%d", v)
}

// econLine is a team's money in the bank, what it carries, and its buy.
func econLine(side gotv.Side, players []gotv.Player, round int) string {
	bank, equip := 0, 0
	for _, p := range players {
		if p.Side == side {
			bank += p.Money
			equip += p.Equip
		}
	}
	return dim(fmt.Sprintf("     BANK %s · EQUIP %s · ", money(bank), money(equip))) + buyType(equip, round)
}
