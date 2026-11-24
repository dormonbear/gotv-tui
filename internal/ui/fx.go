package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/dormonbear/gotv-tui/internal/gotv"
	"github.com/dormonbear/gotv-tui/internal/radar"
	"github.com/dormonbear/gotv-tui/internal/term"
)

const (
	roundBanner = 3 * time.Second
	matchBanner = 8 * time.Second
	typeIn      = 400 * time.Millisecond // the banner text types itself in
)

// fx are the short round-end effects: the C4 explosion and the winner banner.
type fx struct {
	bombAt    *gotv.Vec // where the bomb was last seen planted
	boomAt    time.Time
	boomPos   gotv.Vec
	banner    string
	side      gotv.Side
	bannerAt  time.Time
	bannerFor time.Duration
	over      bool // the match-over banner was shown
}

// onState tracks the planted bomb and spots the end of the match.
func (f *fx) onState(st *gotv.State, now time.Time) {
	if b := st.Bomb; b != nil && b.State != gotv.BombDropped {
		p := b.Pos
		f.bombAt = &p
	}
	if st.Over && !f.over {
		f.over = true
		win, name := gotv.CT, st.CT.Name
		if st.T.Score > st.CT.Score {
			win, name = gotv.T, st.T.Name
		}
		f.show(fmt.Sprintf("%s WIN THE MATCH  %d : %d", strings.ToUpper(name), max(st.CT.Score, st.T.Score), min(st.CT.Score, st.T.Score)), win, matchBanner, now)
	}
	if !st.Over {
		f.over = false
	}
}

// onEvent starts the explosion and the round banner.
func (f *fx) onEvent(e gotv.Event, st *gotv.State, now time.Time) {
	switch e.Kind {
	case gotv.EvExplode:
		if f.bombAt != nil {
			f.boomAt, f.boomPos = now, *f.bombAt
		}
	case gotv.EvRoundStart:
		f.bombAt = nil
	case gotv.EvRoundEnd:
		if e.Winner == "" || st == nil {
			return
		}
		name := st.CT.Name
		if e.Winner == gotv.T {
			name = st.T.Name
		}
		f.show(fmt.Sprintf("%s WIN  ·  %s", strings.ToUpper(name), e.Reason), e.Winner, roundBanner, now)
	}
}

func (f *fx) show(text string, side gotv.Side, d time.Duration, now time.Time) {
	f.banner, f.side, f.bannerAt, f.bannerFor = text, side, now, d
}

// boom is the explosion for the map renderers, if one is playing.
func (f *fx) boom(now time.Time) *radar.Boom {
	if age := now.Sub(f.boomAt).Seconds(); !f.boomAt.IsZero() && age < radar.BoomShow {
		return &radar.Boom{Pos: f.boomPos, Age: age}
	}
	return nil
}

// overlay writes the banner, if showing, across the middle row of block.
func (f *fx) overlay(block string, now time.Time) string {
	age := now.Sub(f.bannerAt)
	if f.banner == "" || age > f.bannerFor {
		return block
	}
	text := []rune(f.banner)
	n := len(text)
	if age < typeIn {
		n = int(float64(len(text)) * float64(age) / float64(typeIn))
	}
	// truecolour like the rest of the map, not lipgloss (which drops colour when it
	// can't detect the terminal's profile)
	label := term.Bg(sideColor(f.side)) + term.Fg(term.Black) + boldOn +
		"  " + string(text[:n]) + strings.Repeat(" ", len(text)-n) + "  " + reset
	lines := strings.Split(block, "\n")
	mid := len(lines) / 2
	w, lw := ansi.StringWidth(lines[mid]), ansi.StringWidth(label)
	if lw > w {
		return block
	}
	start := (w - lw) / 2
	lines[mid] = ansi.Truncate(lines[mid], start, "") + label + ansi.TruncateLeft(lines[mid], start+lw, "")
	return strings.Join(lines, "\n")
}
