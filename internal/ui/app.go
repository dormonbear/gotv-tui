// Package ui is the Bubble Tea front end: a source list and a match screen.
package ui

import (
	"runtime"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/dormonbear/gotv-tui/internal/feeds"
	"github.com/dormonbear/gotv-tui/internal/term"
	"github.com/dormonbear/gotv-tui/internal/update"
)

type App struct {
	list  *listModel
	match *matchModel
	w, h  int
	out   *term.LockedFile // image output; nil draws maps as text
	gfx   term.Graphics

	Version  string // running version; "dev" skips the update check
	latest   string // newer release, once the check found one
	updating bool
	notice   string // update result
	Restart  bool   // updated in place: main starts the new binary
}

type latestMsg string

type updatedMsg struct {
	msg string
	err error
}

func checkUpdate(v string) tea.Cmd {
	return func() tea.Msg {
		l, _ := update.Latest(v) // offline or rate-limited: just no notice
		return latestMsg(l)
	}
}

// updateHint is the one-line update notice shown on both screens.
func (a *App) updateHint() string {
	switch {
	case a.updating:
		return "updating to v" + a.latest + "..."
	case a.notice != "":
		return a.notice
	case a.latest != "":
		return "v" + a.latest + " available · press u to update"
	}
	return ""
}

// New opens url directly when given, else starts on the source list. With out set
// (it must be the program's output), maps are shown as images with protocol gfx.
func New(url string, out *term.LockedFile, gfx term.Graphics) *App {
	return &App{w: 100, h: 40, match: pending(url), out: out, gfx: gfx}
}

func pending(url string) *matchModel {
	if url == "" {
		return nil
	}
	return &matchModel{url: url} // started in Init once the size is known
}

func (a *App) Init() tea.Cmd {
	return tea.Batch(a.start(), checkUpdate(a.Version))
}

func (a *App) start() tea.Cmd {
	if a.out != nil && a.gfx == term.Kitty {
		term.PixelMouse(a.out, true) // matchModel.pointer reads pixels with kitty graphics
	}
	if a.match != nil {
		m, cmd := newMatch(a.match.url, a.w, a.h)
		m.out, m.gfx = a.out, a.gfx
		a.match = m
		return cmd
	}
	var cmd tea.Cmd
	a.list, cmd = newList(feeds.File())
	return cmd
}

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.w, a.h = msg.Width, msg.Height
		if a.match != nil {
			a.match.w, a.match.h = a.w, a.h
			a.match.aspect = term.CellAspect() // font or zoom may have changed
		}
		if a.list != nil {
			a.list.w = a.w
		}
		return a, nil
	case latestMsg:
		a.latest = string(msg)
		return a, nil
	case updatedMsg:
		a.updating = false
		if msg.err != nil {
			a.notice = "update failed: " + msg.err.Error()
			return a, nil
		}
		if runtime.GOOS == "windows" { // a running .exe can't be replaced and restarted in place
			a.notice, a.latest = msg.msg+" Restart gotv-tui to use it.", ""
			return a, nil
		}
		a.Restart = true
		return a, a.quit()
	case tea.MouseMsg:
		if a.match != nil {
			return a, a.match.update(msg)
		}
		return a, nil
	case tea.KeyMsg:
		k := msg.String()
		if k == "ctrl+c" {
			return a, a.quit()
		}
		if k == "u" && a.latest != "" && !a.updating && (a.match != nil || !a.list.editing()) {
			a.updating, a.notice = true, ""
			v := a.Version
			return a, func() tea.Msg {
				msg, err := update.Run(v)
				return updatedMsg{msg, err}
			}
		}
		if a.match != nil {
			switch k {
			case "q":
				return a, a.quit()
			case "b", "esc": // back to the source list
				a.match.hideImages()
				a.match.close()
				a.match = nil
				if a.list == nil {
					var cmd tea.Cmd
					a.list, cmd = newList(feeds.File())
					a.list.w = a.w
					return a, cmd
				}
				return a, a.list.refresh() // states change while a match is open
			}
			return a, a.match.update(msg)
		}
		if k == "q" && !a.list.editing() {
			return a, a.quit()
		}
	}
	if a.match != nil {
		return a, a.match.update(msg)
	}
	if a.list != nil {
		url, cmd := a.list.update(msg)
		if url != "" {
			m, start := newMatch(url, a.w, a.h)
			m.out, m.gfx = a.out, a.gfx
			a.match = m
			return a, tea.Batch(cmd, start)
		}
		return a, cmd
	}
	return a, nil
}

func (a *App) quit() tea.Cmd {
	if a.match != nil {
		a.match.close()
	}
	// images go now, while the TUI's screen is still up: afterwards they would be
	// drawn over (iTerm2/Sixel) or deleted from (kitty) the user's shell screen
	if a.out != nil {
		a.out.SetOverlay(nil)
		if a.gfx == term.Kitty {
			term.DeleteImages(a.out)
		}
	}
	return tea.Quit
}

func (a *App) View() string {
	hint := a.updateHint()
	if a.match != nil {
		a.match.notice = hint
		return a.match.view()
	}
	if a.list != nil {
		v := a.list.view()
		if hint != "" {
			v += "\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Render(hint)
		}
		return v
	}
	return ""
}
