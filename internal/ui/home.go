package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/dormonbear/gotv-tui/internal/feeds"
	"github.com/dormonbear/gotv-tui/internal/term"
)

// logo is "GOTV TUI" in block letters.
var logo = []string{
	" ██████╗  ██████╗ ████████╗██╗   ██╗    ████████╗██╗   ██╗██╗",
	"██╔════╝ ██╔═══██╗╚══██╔══╝██║   ██║    ╚══██╔══╝██║   ██║██║",
	"██║  ███╗██║   ██║   ██║   ██║   ██║       ██║   ██║   ██║██║",
	"██║   ██║██║   ██║   ██║   ╚██╗ ██╔╝       ██║   ██║   ██║██║",
	"╚██████╔╝╚██████╔╝   ██║    ╚████╔╝        ██║   ╚██████╔╝██║",
	" ╚═════╝  ╚═════╝    ╚═╝     ╚═══╝         ╚═╝    ╚═════╝ ╚═╝",
}

const tagline = "live Counter-Strike 2 from CSTV relays"

// homeHeader is the logo, shaded from CT blue to T gold, and the tagline, centred in w.
func homeHeader(w int) []string {
	center := func(s string) string { return lipgloss.PlaceHorizontal(max(w, 1), lipgloss.Center, s) }
	if w < ansi.StringWidth(logo[0])+2 {
		return []string{center(term.Fg(term.CT) + boldOn + "GOTV" + reset + " " + term.Fg(term.T) + boldOn + "TUI" + reset), center(dim(tagline))}
	}
	var out []string
	for i, l := range logo {
		c := term.Mix(term.CT, term.T, float64(i)/float64(len(logo)-1))
		out = append(out, center(term.Fg(c)+l+reset))
	}
	return append(out, center(dim(tagline)))
}

var panel = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("8")).Padding(0, 1)

// titled draws body in a rounded box of outer width w with title set in its top edge.
func titled(title string, body []string, w int) []string {
	box := strings.Split(panel.Width(w-2).Render(strings.Join(body, "\n")), "\n")
	if len(box) > 0 && title != "" {
		top := []rune(ansi.Strip(box[0]))
		t := " " + title + " "
		if tw := ansi.StringWidth(ansi.Strip(t)); tw+4 < len(top) {
			box[0] = dim(string(top[:2])) + t + dim(string(top[2+tw:]))
		}
	}
	return box
}

// sourceRows are the list's rows in aligned columns, each exactly inner wide.
func (m *listModel) sourceRows(inner int) []string {
	if len(m.feeds) == 0 {
		return []string{dim("No sources yet. Press a to add one.")}
	}
	nameW, stateW := 0, 0
	states := make([]string, len(m.feeds))
	for i, f := range m.feeds {
		nameW = max(nameW, ansi.StringWidth(f.Name))
		states[i] = m.stateText(f)
		stateW = max(stateW, ansi.StringWidth(ansi.Strip(states[i])))
	}
	var rows []string
	for i, f := range m.feeds {
		row := " " + pad(f.Name, nameW) + "  " + states[i] + strings.Repeat(" ", stateW-ansi.StringWidth(ansi.Strip(states[i]))) + "  " + dim(f.URL)
		row = ansi.Truncate(row, inner, "…")
		if i == m.sel && m.form == nil {
			row = inverse.Render(pad(ansi.Strip(row), inner))
		}
		rows = append(rows, row)
	}
	return rows
}

// stateText is a source's coloured status (with who is playing, once known).
func (m *listModel) stateText(f feeds.Feed) string {
	s, ok := m.status[f.URL]
	if !ok {
		return m.spin.View() + lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Render(" probing")
	}
	text := string(s.State) + " · " + feeds.MapName(s.Map)
	switch s.State {
	case feeds.Offline:
		text = "offline"
	case feeds.Waiting:
		text = "waiting for broadcast"
	}
	return lipgloss.NewStyle().Foreground(stateColor[s.State]).Render(text)
}

// details describes the selected source.
func (m *listModel) details() []string {
	if m.sel >= len(m.feeds) {
		return nil
	}
	f := m.feeds[m.sel]
	line := func(k, v string) string { return dim(fmt.Sprintf("%-7s", k)) + v }
	out := []string{line("name", bold(f.Name)), line("relay", f.URL)}
	if s, ok := m.status[f.URL]; ok {
		out = append(out, line("status", m.stateText(f)))
		if p := m.match[f.URL]; p != nil {
			round := fmt.Sprintf("round %d", p.Round)
			if p.Over {
				round = "match over"
			}
			ct := term.Fg(term.CT) + boldOn + fmt.Sprintf("%s %d", p.CT.Name, p.CT.Score) + reset
			t := term.Fg(term.T) + boldOn + fmt.Sprintf("%d %s", p.T.Score, p.T.Name) + reset
			out = append(out, line("match", ct+" : "+t+"  "+dim(round)))
		} else if s.State == feeds.Live || s.State == feeds.Stale {
			out = append(out, line("match", dim("reading teams...")))
		}
		if s.State == feeds.Live {
			out = append(out, "", lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Render("enter to watch"))
		}
	}
	return out
}

// view is the full-screen home: logo, the sources and the selected one's details,
// any form or prompt, and the keys pinned to the bottom with status on the right.
func (m *listModel) view(status string) string {
	w, h := max(m.w, 30), max(m.h, 12)
	title := "Sources"
	for _, f := range m.feeds {
		if _, ok := m.status[f.URL]; !ok {
			title += " " + m.spin.View() + lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Render(" refreshing")
			break
		}
	}
	lines := titled(title, m.sourceRows(w-4), w)
	if f := m.form; f != nil {
		head := "Edit source"
		if f.index < 0 {
			head = "Add source"
		}
		cur := func(on bool) string {
			if on {
				return "› "
			}
			return "  "
		}
		body := []string{cur(!f.onURL) + "Name: " + f.name.View(), cur(f.onURL) + "URL:  " + f.url.View()}
		if f.err != "" {
			body = append(body, red.Render(f.err))
		}
		body = append(body, dim("enter next/save · tab switch field · esc cancel"))
		lines = append(lines, titled(head, body, w)...)
	} else if d := m.details(); d != nil {
		lines = append(lines, titled("Selected", d, w)...)
	}
	if m.confirmDelete && m.sel < len(m.feeds) {
		lines = append(lines, yellow.Render(fmt.Sprintf(" Delete %q? y to confirm, any other key cancels", m.feeds[m.sel].Name)))
	}
	if m.saveErr != "" {
		lines = append(lines, red.Render(" "+m.saveErr))
	}
	keys := dim(" ↑/↓ select · enter watch · a add · e edit · d delete · r refresh · q quit")
	if m.form != nil {
		keys = ""
	}
	footer := []string{dim(" sources: " + m.file)}
	if gap := w - ansi.StringWidth(keys) - ansi.StringWidth(status) - 1; gap >= 2 {
		footer = append(footer, keys+strings.Repeat(" ", gap)+status)
	} else { // narrow: status on its own line, then the keys
		footer = append(footer, " "+status, keys)
	}
	// the big logo only when it fits with everything else; else a one-line title
	top := homeHeader(w)
	if len(top)+1+len(lines)+len(footer) > h {
		top = homeHeader(0)[:1]
	}
	lines = append(append(top, ""), lines...)
	for len(lines)+len(footer) < h { // pin the footer to the bottom
		lines = append(lines, "")
	}
	if over := len(lines) + len(footer) - h; over > 0 { // still too short: drop from the top
		lines = lines[min(over, len(lines)):]
	}
	for i := range lines {
		lines[i] = ansi.Truncate(lines[i], w, "")
	}
	return strings.Join(append(lines, footer...), "\n")
}
