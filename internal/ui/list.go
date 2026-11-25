package ui

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/dormonbear/gotv-tui/internal/feeds"
	"github.com/dormonbear/gotv-tui/internal/gotv"
)

type probeMsg feeds.Status

// autoMsg re-probes the list; its loop only continues while the list gets messages,
// so it stops by itself when a match is open (a new list starts a new loop).
type autoMsg struct{ gen int }

const autoEvery = 30 * time.Second

func autoTick(gen int) tea.Cmd {
	return tea.Tick(autoEvery, func(time.Time) tea.Msg { return autoMsg{gen} })
}

// resume re-probes on coming back from a match and starts a fresh refresh loop
// (the old one ended, or ends at its next tick, being of an older generation).
func (m *listModel) resume() tea.Cmd {
	m.gen++
	return tea.Batch(m.refresh(), autoTick(m.gen))
}

type peekMsg struct {
	url string
	s   *gotv.Summary
}

type form struct {
	index     int // -1 = new
	name, url textinput.Model
	onURL     bool
	err       string
}

type listModel struct {
	file          string
	feeds         []feeds.Feed
	status        map[string]feeds.Status
	match         map[string]*gotv.Summary // who is playing, once peeked
	sel           int
	rowY          int // screen row of the first source, set by view
	form          *form
	confirmDelete bool
	saveErr       string
	spin          spinner.Model
	gen           int // refresh loop generation
	w, h          int
}

var stateColor = map[feeds.State]lipgloss.Color{feeds.Live: "2", feeds.Stale: "3", feeds.Waiting: "6", feeds.Offline: "8"}

func newList(file string) (*listModel, tea.Cmd) {
	m := &listModel{file: file, feeds: feeds.Load(file), status: map[string]feeds.Status{}, match: map[string]*gotv.Summary{},
		spin: spinner.New(spinner.WithSpinner(spinner.Dot), spinner.WithStyle(lipgloss.NewStyle().Foreground(lipgloss.Color("6"))))}
	return m, tea.Batch(m.refresh(), autoTick(m.gen))
}

// refresh re-probes every source; also restarts the spinner, whose tick loop stops
// while the match screen has the messages.
func (m *listModel) refresh() tea.Cmd { return tea.Batch(m.spin.Tick, m.probe(m.feeds...)) }

// probe clears the rows first so they show a spinner until their answer arrives.
func (m *listModel) probe(list ...feeds.Feed) tea.Cmd {
	for _, f := range list {
		delete(m.status, f.URL)
		delete(m.match, f.URL)
	}
	return quietProbe(list...)
}

// quietProbe re-checks sources in place: rows keep what they show until the answer.
func quietProbe(list ...feeds.Feed) tea.Cmd {
	var cmds []tea.Cmd
	for _, f := range list {
		cmds = append(cmds, func() tea.Msg { return probeMsg(feeds.Probe(f)) })
	}
	return tea.Batch(cmds...)
}

func (m *listModel) commit(next []feeds.Feed) {
	m.feeds = next
	m.saveErr = ""
	if err := feeds.Save(m.file, next); err != nil {
		m.saveErr = "could not save sources: " + err.Error()
	}
	m.sel = max(0, min(m.sel, len(next)-1))
}

func newInput(placeholder, value string) textinput.Model {
	t := textinput.New()
	t.Placeholder = placeholder
	t.SetValue(value)
	t.Prompt = ""
	t.CharLimit = 300
	return t
}

func (m *listModel) openForm(index int, f feeds.Feed) tea.Cmd {
	m.form = &form{index: index, name: newInput("", f.Name), url: newInput(`https://... or playcast "https://..."`, f.URL)}
	return m.form.name.Focus()
}

// editing reports whether keys belong to a text field.
func (m *listModel) editing() bool { return m.form != nil }

// update returns the url to open, if any.
func (m *listModel) update(msg tea.Msg) (string, tea.Cmd) {
	switch msg := msg.(type) {
	case autoMsg:
		if msg.gen != m.gen {
			return "", nil
		}
		return "", tea.Batch(quietProbe(m.feeds...), autoTick(m.gen))
	case probeMsg:
		prev, had := m.status[msg.URL]
		m.status[msg.URL] = feeds.Status(msg)
		if had && (prev.State != msg.State || prev.Map != msg.Map) {
			delete(m.match, msg.URL) // another match now: don't show the old teams
		}
		// read who is playing: live score changes, a finished match only once
		if msg.State == feeds.Live || (msg.State == feeds.Stale && m.match[msg.URL] == nil) {
			url := msg.URL
			return "", func() tea.Msg {
				s, _ := gotv.Peek(context.Background(), url)
				return peekMsg{url, s}
			}
		}
		return "", nil
	case peekMsg:
		if msg.s != nil {
			if _, probed := m.status[msg.url]; probed { // not re-probed meanwhile
				m.match[msg.url] = msg.s
			}
		}
		return "", nil
	case tea.MouseMsg:
		return m.mouse(msg), nil
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return "", cmd
	case tea.KeyMsg:
		if m.form != nil {
			return "", m.updateForm(msg)
		}
		if m.confirmDelete {
			if msg.String() == "y" {
				m.commit(slices.Delete(slices.Clone(m.feeds), m.sel, m.sel+1))
			}
			m.confirmDelete = false
			return "", nil
		}
		has := m.sel < len(m.feeds)
		switch msg.String() {
		case "up", "k":
			m.sel = max(0, m.sel-1)
		case "down", "j":
			m.sel = max(0, min(len(m.feeds)-1, m.sel+1))
		case "enter":
			if has {
				return m.feeds[m.sel].URL, nil
			}
		case "r":
			return "", m.probe(m.feeds...)
		case "a":
			return "", m.openForm(-1, feeds.Feed{})
		case "e":
			if has {
				return "", m.openForm(m.sel, m.feeds[m.sel])
			}
		case "d":
			m.confirmDelete = has
		}
	}
	return "", nil
}

func (m *listModel) updateForm(k tea.KeyMsg) tea.Cmd {
	f := m.form
	switch k.String() {
	case "esc":
		m.form = nil
		return nil
	case "tab", "shift+tab":
		return f.focus(!f.onURL)
	case "enter":
		if !f.onURL {
			return f.focus(true)
		}
		url := feeds.NormalizeURL(f.url.Value())
		if !feeds.IsRelayURL(url) {
			f.err = "URL must start with http(s)://"
			return nil
		}
		feed := feeds.Feed{Name: strings.TrimSpace(f.name.Value()), URL: url}
		if feed.Name == "" {
			feed.Name = url
		}
		next := slices.Clone(m.feeds)
		if f.index < 0 {
			next = append(next, feed)
			m.sel = len(next) - 1
		} else {
			next[f.index] = feed
		}
		m.commit(next)
		m.form = nil
		return m.probe(feed)
	}
	var cmd tea.Cmd
	if f.onURL {
		f.url, cmd = f.url.Update(k)
		f.err = ""
	} else {
		f.name, cmd = f.name.Update(k)
	}
	return cmd
}

func (f *form) focus(url bool) tea.Cmd {
	f.onURL = url
	if url {
		f.name.Blur()
		return f.url.Focus()
	}
	f.url.Blur()
	return f.name.Focus()
}

var (
	inverse = lipgloss.NewStyle().Reverse(true)
	box     = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1).MarginTop(1)
	red     = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	yellow  = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
)

// matchText is " · CT 11 : 5 T · R17" (or "· ended ..."), "" until peeked.
func matchText(s *gotv.Summary) string {
	if s == nil {
		return ""
	}
	round := fmt.Sprintf(" · R%d", s.Round)
	if s.Over {
		round = " · ended"
	}
	return fmt.Sprintf(" · %s %d : %d %s%s", s.CT.Name, s.CT.Score, s.T.Score, s.T.Name, round)
}

// mouse: click a source to select it, click the selected one to watch it; the
// wheel moves the selection.
func (m *listModel) mouse(e tea.MouseMsg) string {
	if m.form != nil || len(m.feeds) == 0 {
		return ""
	}
	switch {
	case e.Button == tea.MouseButtonWheelUp:
		m.sel = max(0, m.sel-1)
	case e.Button == tea.MouseButtonWheelDown:
		m.sel = min(len(m.feeds)-1, m.sel+1)
	case e.Button == tea.MouseButtonLeft && e.Action == tea.MouseActionPress:
		i := e.Y - m.rowY
		if i < 0 || i >= len(m.feeds) {
			return ""
		}
		m.confirmDelete = false
		if i == m.sel {
			return m.feeds[i].URL
		}
		m.sel = i
	}
	return ""
}
