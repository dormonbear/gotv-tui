package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"

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
// ✱ bomb, ✓ defuse, ~ time. Rounds before we connected show as "-". A gap marks
// halftime (after 12) and each overtime half (every 3 after 24).
func historyLine(rs []roundResult, current, width int) string {
	byN := map[int]roundResult{}
	last := current - 1
	for _, r := range rs {
		byN[r.n] = r
		last = max(last, r.n)
	}
	if last < 1 {
		return ""
	}
	var b strings.Builder
	b.WriteString(dim(" rounds "))
	for n := 1; n <= last; n++ {
		if r, ok := byN[n]; ok {
			g := reasonGlyph[r.reason]
			if g == "" {
				g = "•"
			}
			b.WriteString(term.Fg(sideColor(r.winner)) + g + reset)
		} else {
			b.WriteString(dim("-"))
		}
		if n == 12 || (n > 24 && (n-24)%3 == 0) {
			b.WriteString(" ")
		}
	}
	return ansi.Truncate(b.String(), width, "")
}

// buyType classifies a team's equipment value the way HLTV does.
func buyType(equip, round int) string {
	switch {
	case round == 1 || round == 13:
		return "pistol"
	case equip < 5000:
		return "eco"
	case equip < 10000:
		return "semi-eco"
	case equip < 20000:
		return "semi-buy"
	}
	return "full buy"
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
	return dim(fmt.Sprintf("     bank %s · equipment %s · ", money(bank), money(equip))) + buyType(equip, round)
}
