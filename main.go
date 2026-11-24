// gotv-tui: live CS2 matches from CSTV (GOTV+) relays in the terminal.
package main

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/dormonbear/gotv-tui/internal/feeds"
	"github.com/dormonbear/gotv-tui/internal/ui"
	"github.com/dormonbear/gotv-tui/internal/update"
)

var version = "dev" // set by -ldflags "-X main.version=..."

const usage = `gotv-tui %s - live CS2 matches from CSTV relays

usage:
  gotv-tui                      pick a source from the list
  gotv-tui <relay url>          open a relay, e.g. https://cstv.fragnet.net/a
  gotv-tui 'playcast "<url>"'   same, pasted from a tournament page
  gotv-tui update               update the binary to the latest release
  gotv-tui --version
`

func main() {
	arg := strings.TrimSpace(strings.Join(os.Args[1:], " "))
	switch {
	case arg == "--version" || arg == "-v":
		fmt.Println(version)
		return
	case arg == "update":
		msg, err := update.Run(version)
		if err != nil {
			fmt.Fprintln(os.Stderr, "update failed:", err)
			os.Exit(1)
		}
		fmt.Println(msg)
		return
	case arg == "--help" || arg == "-h":
		fmt.Printf(usage, version)
		return
	case arg != "" && !strings.Contains(arg, "http://") && !strings.Contains(arg, "https://"):
		fmt.Printf(usage, version)
		os.Exit(1)
	}
	url := ""
	if arg != "" {
		url = feeds.NormalizeURL(arg)
	}
	if _, err := tea.NewProgram(ui.New(url), tea.WithAltScreen(), tea.WithMouseCellMotion()).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
