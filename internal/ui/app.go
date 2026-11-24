// Package ui is the Bubble Tea front end: a source list and a match screen.
package ui

import (
	"io"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/dormonbear/gotv-tui/internal/feeds"
	"github.com/dormonbear/gotv-tui/internal/term"
)

type App struct {
	list  *listModel
	match *matchModel
	w, h  int
	out   io.Writer // kitty image uploads; nil draws maps in half blocks
}

// New opens url directly when given, else starts on the source list. With out set,
// maps are uploaded there as kitty images (it must be the program's output writer).
func New(url string, out io.Writer) *App { return &App{w: 100, h: 40, match: pending(url), out: out} }

func pending(url string) *matchModel {
	if url == "" {
		return nil
	}
	return &matchModel{url: url} // started in Init once the size is known
}

func (a *App) Init() tea.Cmd {
	if a.match != nil {
		m, cmd := newMatch(a.match.url, a.w, a.h)
		m.out = a.out
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
		if a.match != nil {
			switch k {
			case "q":
				return a, a.quit()
			case "b": // back to the source list
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
			m.out = a.out
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
	return tea.Quit
}

func (a *App) View() string {
	if a.match != nil {
		return a.match.view()
	}
	if a.list != nil {
		return a.list.view()
	}
	return ""
}
