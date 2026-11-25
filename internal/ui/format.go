package ui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/dormonbear/gotv-tui/internal/gotv"
	"github.com/dormonbear/gotv-tui/internal/term"
)

const (
	dimOn  = "\x1b[2m"
	boldOn = "\x1b[1m"
	reset  = term.Reset
)

func dim(s string) string  { return dimOn + s + reset }
func bold(s string) string { return boldOn + s + reset }

func sideColor(s gotv.Side) term.RGB {
	if s == gotv.CT {
		return term.CT
	}
	return term.T
}

func who(w *gotv.Who) string {
	if w == nil {
		return "?"
	}
	return term.Fg(sideColor(w.Side)) + w.Name + reset
}

func orQ(s string) string {
	if s == "" {
		return "?"
	}
	return s
}

// FormatEvent renders one game-log line.
func FormatEvent(e gotv.Event) string {
	switch e.Kind {
	case gotv.EvKill:
		if e.Killer == nil || e.Killer.Name == e.Victim.Name {
			return who(e.Victim) + " " + dim("died ["+e.Weapon+"]")
		}
		s := who(e.Killer)
		if e.Assister != nil {
			s += " + " + who(e.Assister)
			if e.AssistFlash {
				s += dim("(flash)")
			}
		}
		tags := []string{e.Weapon}
		for _, t := range []struct {
			on   bool
			name string
		}{{e.Headshot, "HS"}, {e.Wallbang, "wall"}, {e.ThroughSmoke, "smoke"}, {e.NoScope, "noscope"}, {e.Blind, "blind"}} {
			if t.on {
				tags = append(tags, t.name)
			}
		}
		return s + " " + dim("["+strings.Join(tags, " ")+"]") + " " + who(e.Victim)
	case gotv.EvRoundStart:
		return bold(fmt.Sprintf("── Round %d started", e.Round))
	case gotv.EvRoundEnd:
		w := "?"
		if e.Winner != "" {
			w = term.Fg(sideColor(e.Winner)) + string(e.Winner) + reset + boldOn
		}
		return bold("── Round over - " + w + " " + e.Reason)
	case gotv.EvPlant:
		return term.Fg(term.T) + orQ(e.Player) + reset + " planted the bomb"
	case gotv.EvDefuse:
		return term.Fg(term.CT) + orQ(e.Player) + reset + " defused the bomb"
	case gotv.EvExplode:
		return bold("Bomb exploded")
	}
	return ""
}

// PlayerNumbers is a stable 1..5 numbering per side, shared by table badges and map dots.
func PlayerNumbers(players []gotv.Player) map[string]int {
	m := map[string]int{}
	for _, side := range []gotv.Side{gotv.CT, gotv.T} {
		var names []string
		for _, p := range players {
			if p.Side == side {
				names = append(names, strings.ToLower(p.Name))
			}
		}
		sort.Strings(names)
		for i, n := range names {
			m[n] = i + 1
		}
	}
	return m
}

var nadeGlyph = map[gotv.Nade]string{
	gotv.HE:    "\x1b[38;2;235;80;70mH",
	gotv.Flash: "\x1b[38;2;240;240;240mF",
	gotv.Smoke: "\x1b[38;2;160;165;175mS",
	gotv.Fire:  "\x1b[38;2;240;130;40mM",
	gotv.Decoy: "\x1b[38;2;120;200;120mD",
}

func nadeText(ns []gotv.Nade) string {
	if len(ns) == 0 {
		return ""
	}
	s := boldOn
	for _, n := range ns {
		s += nadeGlyph[n]
	}
	return s + reset
}

// Header columns line up with the rows below; the team title fills the first
// titleW (compactTitleW) cells.
const (
	titleW        = 17
	compactTitleW = 14
	compactWidth  = 44 // a compact row: number, name, hp, money, K/D, weapon
)

var (
	columns        = fmt.Sprintf("%-14s %6s   K  D  A%6s  KIT %-11sUTIL", "HP", "$", "ADR", "WEAPON")
	compactColumns = fmt.Sprintf(" %4s %6s%3s%3s WEAPON", "HP", "$", "K", "D")
	// the stats view (s): match totals instead of the live state
	statsColumns        = fmt.Sprintf("%3s%3s%3s %4s %5s %4s %4s %4s %4s %3s  %s", "K", "D", "A", "+/-", "ADR", "HS%", "KAS", "KPR", "UD", "EF", "3K 4K 5K")
	compactStatsColumns = fmt.Sprintf("%3s%3s %4s %5s %4s %4s", "K", "D", "+/-", "ADR", "HS%", "KAS")
)

// pct is part/whole as a whole percentage, "-" when there is nothing to divide by.
func pct(part, whole int) string {
	if whole <= 0 {
		return "-"
	}
	return fmt.Sprintf("%d%%", part*100/whole)
}

// statsRow is the stats view of a player after their badge.
func statsRow(p gotv.Player, compact bool) string {
	s := p.Stats
	hs, kas := pct(s.HSKills, p.Kills), pct(s.KASRounds, s.Rounds)
	if compact {
		return fmt.Sprintf("%s %3d%3d %+4d %5.0f %4s %4s", pad(p.Name, 10), p.Kills, p.Deaths, p.Kills-p.Deaths, p.ADR, hs, kas)
	}
	multi := func(n int) string {
		if n == 0 {
			return dim(" · ")
		}
		return fmt.Sprintf("%2d ", n)
	}
	return fmt.Sprintf("%s %3d%3d%3d %+4d %5.0f %4s %4s %4.2f %4d %3d  %s%s%s", pad(p.Name, 12), p.Kills, p.Deaths, p.Assists,
		p.Kills-p.Deaths, p.ADR, hs, kas, float64(p.Kills)/float64(max(1, s.Rounds)), s.UtilDamage, s.Flashed,
		multi(s.Multi[0]), multi(s.Multi[1]), multi(s.Multi[2]))
}

func title(side gotv.Side, t gotv.Team, w int) string {
	tail := fmt.Sprintf(" %d", t.Score)
	room := w - 1 - len(tail) - 4 // keep one space before "HP"
	name := t.Name
	if ansi.StringWidth(name) > room {
		name = strings.TrimRight(ansi.Truncate(name, room-1, ""), " ") + "…"
	}
	return ansi.Truncate(fmt.Sprintf(" %-2s %s%s%s", side, name, tail, strings.Repeat(" ", w)), w, "")
}

func gunName(g string) string {
	if len(g) > 10 {
		return g[:10]
	}
	return g
}

// hpBar is a 14-cell bar with the hp number inside its right end.
func hpBar(hp int) string {
	const w = 14
	n := min(w, max(0, (hp*w+50)/100))
	bg := "\x1b[41;30m" // red
	if hp > 60 {
		bg = "\x1b[42;30m"
	} else if hp > 25 {
		bg = "\x1b[43;30m"
	}
	text := []rune(fmt.Sprintf("%*d ", w-1, hp))
	return bg + string(text[:n]) + "\x1b[48;2;58;62;70;38;2;230;230;230m" + string(text[n:]) + reset
}

func pad(s string, w int) string { // pad/cut a plain string to exactly w cells
	return ansi.Truncate(s+strings.Repeat(" ", w), w, "")
}

// TeamTable renders the title row and one row per player of side.
// The followed player's row (focus, lowercase) is framed; see framed. compact keeps
// only number, name, hp, money, K/D and weapon, for narrow screens.
func TeamTable(side gotv.Side, t gotv.Team, players []gotv.Player, nums map[string]int, focus string, compact, stats bool) []string {
	c := sideColor(side)
	head := map[[2]bool]string{{false, false}: columns, {true, false}: compactColumns, {false, true}: statsColumns, {true, true}: compactStatsColumns}[[2]bool{compact, stats}]
	tw := titleW
	if compact {
		tw = compactTitleW
	}
	out := []string{term.Fg(c) + boldOn + title(side, t, tw) + reset + dim(head)}
	num := func(p gotv.Player) int {
		if n, ok := nums[strings.ToLower(p.Name)]; ok {
			return n
		}
		return 9
	}
	for _, p := range teamRows(side, players, nums) {
		badge := " " + " " + " "
		if n := num(p); n < 9 {
			badge = fmt.Sprintf(" %d ", n)
		}
		focused := strings.ToLower(p.Name) == focus
		if focused { // a white bar in the badge's left padding marks the row
			badge = term.Fg(term.White) + "▌" + term.Fg(term.Black) + badge[1:]
		}
		badge = term.Bg(c) + term.Fg(term.Black) + badge + reset
		kit := " "
		if p.Helmet {
			kit = "H"
		} else if p.Armor > 0 {
			kit = "A"
		}
		if p.Defuser {
			kit += "K"
		} else {
			kit += " "
		}
		if p.HasBomb {
			kit += "\x1b[31mB" + reset
		} else {
			kit += " "
		}
		hp := dim("dead          ")
		gear := ""
		if p.Alive {
			hp = hpBar(p.HP)
			gear = fmt.Sprintf("%-10s ", gunName(p.Gun)) + nadeText(p.Nades)
		}
		row := fmt.Sprintf("%s %s %s %6s %3d%3d%3d %5.0f  %s %s", badge, pad(p.Name, 12), hp, fmt.Sprintf("$%d", p.Money), p.Kills, p.Deaths, p.Assists, p.ADR, kit, gear)
		width := rowWidth
		if compact {
			hp, gun := dim("dead"), ""
			if p.Alive {
				hp, gun = fmt.Sprintf("%4d", p.HP), gunName(p.Gun)
			}
			if p.HasBomb {
				gun += " \x1b[31mB" + reset
			}
			row = fmt.Sprintf("%s %s %s %6s %3d%3d %s", badge, pad(p.Name, 10), hp, fmt.Sprintf("$%d", p.Money), p.Kills, p.Deaths, gun)
			width = compactWidth
		}
		if stats { // totals: a dead player's numbers matter as much as a live one's
			row = badge + " " + statsRow(p, compact)
		} else if !p.Alive {
			row = dimOn + row + reset
		}
		if focused {
			row = framed(row, width)
		}
		out = append(out, row)
	}
	return out
}

// teamRows is side's players in scoreboard order (by number).
func teamRows(side gotv.Side, players []gotv.Player, nums map[string]int) []gotv.Player {
	var rows []gotv.Player
	for _, p := range players {
		if p.Side == side {
			rows = append(rows, p)
		}
	}
	num := func(p gotv.Player) int {
		if n, ok := nums[strings.ToLower(p.Name)]; ok {
			return n
		}
		return 9
	}
	sort.SliceStable(rows, func(i, j int) bool { return num(rows[i]) < num(rows[j]) })
	return rows
}

// rowWidth is how wide a framed scoreboard row is drawn (the table's width).
const rowWidth = 76

// frame: highlighted background with a white over- and underline, so the row reads
// as boxed without extra lines (terminals without overline still get the rest).
var frameOn = term.Bg(term.RGB{44, 54, 74}) + "\x1b[53;4;58;2;235;235;235m"

// framed boxes a scoreboard row: the frame style across the whole table width, kept
// on through the row's own colour resets.
func framed(row string, width int) string {
	row = strings.ReplaceAll(row, reset, reset+frameOn)
	if w := ansi.StringWidth(row); w < width {
		row += strings.Repeat(" ", width-w)
	}
	return frameOn + row + reset
}
