package ui

import (
	"math"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// link is the relay connection state shown in the footer badge.
type link int

const (
	linkNone link = iota
	linkLive
	linkWaiting
	linkDown
	linkDemo
)

// stallAfter: connected but no snapshot for this long reads as stalled.
const stallAfter = 5 * time.Second

// linkOf reads the state from the relay client's status line.
func linkOf(status string) link {
	switch {
	case strings.HasPrefix(status, "connected"):
		return linkLive
	case strings.HasPrefix(status, "waiting"):
		return linkWaiting
	case strings.HasPrefix(status, "relay"), strings.Contains(status, "reconnecting"):
		return linkDown
	case strings.HasPrefix(status, "demo"), strings.HasPrefix(status, "playing demo"):
		return linkDemo
	}
	return linkNone
}

var spinFrames = []rune("⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏")

// linkBadge is the animated connection badge; it's redrawn on every tick.
func linkBadge(l link, last, now time.Time) string {
	ms := now.UnixMilli()
	style := func(fg string) lipgloss.Style { return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(fg)) }
	switch {
	case l == linkLive && !last.IsZero() && now.Sub(last) > stallAfter:
		return style("3").Render(" ◌ STALLED ")
	case l == linkLive:
		// a breathing dot: brightness follows a 1.6 s sine
		k := (1 + math.Sin(float64(ms)/1600*2*math.Pi)) / 2
		g := 120 + int(k*135)
		dot := lipgloss.NewStyle().Foreground(lipgloss.Color(rgbHex(40, g, 70))).Render("●")
		return " " + dot + style("2").Render(" LIVE ")
	case l == linkWaiting:
		return style("6").Render(" " + string(spinFrames[ms/100%int64(len(spinFrames))]) + " WAITING ")
	case l == linkDown:
		dot := "●"
		if ms/500%2 == 1 {
			dot = " "
		}
		return style("1").Render(" " + dot + " RECONNECTING ")
	case l == linkDemo:
		return style("5").Render(" ▶ DEMO ")
	}
	return ""
}

func rgbHex(r, g, b int) string {
	const hex = "0123456789abcdef"
	out := []byte{'#'}
	for _, v := range []int{r, g, b} {
		v = max(0, min(255, v))
		out = append(out, hex[v>>4], hex[v&15])
	}
	return string(out)
}
