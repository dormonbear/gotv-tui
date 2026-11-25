// gotv-tui: live CS2 matches from CSTV (GOTV+) relays in the terminal.
package main

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/dormonbear/gotv-tui/internal/feeds"
	"github.com/dormonbear/gotv-tui/internal/term"
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
	args, relaunched := os.Args[1:], false
	for i, a := range args {
		if a == relaunchedFlag {
			args, relaunched = append(append([]string{}, args[:i]...), args[i+1:]...), true
			break
		}
	}
	arg := strings.TrimSpace(strings.Join(args, " "))
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
	opts := []tea.ProgramOption{tea.WithAltScreen(), tea.WithMouseCellMotion()}
	gfx := term.DetectGraphics()
	if gfx == term.Text && !relaunched {
		if name, ok := relaunch(args); ok {
			fmt.Printf("gotv-tui opened in %s (this terminal can't show the map as an image).\nSet GOTV_TUI_RELAUNCH=0 to stay here.\n", name)
			return
		}
	}
	var images *term.LockedFile // images share the TUI's output, one writer at a time
	if gfx != term.Text {
		images = term.NewLockedFile(os.Stdout)
		opts = append(opts, tea.WithOutput(images))
	}
	_, err := tea.NewProgram(ui.New(url, images, gfx), opts...).Run()
	if images != nil && gfx == term.Kitty {
		term.PixelMouse(images, false) // images were removed by the app on quit
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
