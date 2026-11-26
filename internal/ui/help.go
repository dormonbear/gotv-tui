package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var helpKeys = [][2]string{
	{"q · ctrl+c", "quit"},
	{"b · esc", "back to the sources"},
	{"s", "scoreboard: live state / match stats"},
	{"1-0", "follow a player: 1-5 CT, 6-0 T (badge number); again to stop"},
	{"d · click the switch", "auto director on / off"},
	{"click a player / row", "follow them; click again or elsewhere to stop"},
	{"wheel · + -", "zoom"},
	{"drag · ← ↑ ↓ → · hjkl", "pan"},
	{"0", "whole map"},
	{"v", "audio: pick Huya / Twitch / Bilibili / URL, then the room (↑/↓: past rooms)"},
	{"a", "auto sync on/off (reads the stream's round clock)"},
	{"t", "tap sync: press as a kill is called"},
	{"[ ] · { }", "audio offset ±1 s · ±0.1 s"},
	{"u", "update, when a new version is out"},
	{"?", "this help; any key closes it"},
}

// helpBox is the key help, a framed two-column table.
func helpBox() string {
	w := 0
	for _, k := range helpKeys {
		w = max(w, lipgloss.Width(k[0]))
	}
	var rows []string
	for _, k := range helpKeys {
		rows = append(rows, bold(k[0])+strings.Repeat(" ", w-lipgloss.Width(k[0])+3)+k[1])
	}
	return panel.Render(bold("KEYS") + "\n\n" + strings.Join(rows, "\n"))
}
