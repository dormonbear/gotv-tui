// gotv-tui: live CS2 matches from CSTV (GOTV+) relays in the terminal.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/dormonbear/gotv-tui/internal/feeds"
	"github.com/dormonbear/gotv-tui/internal/gotv"
	"github.com/dormonbear/gotv-tui/internal/record"
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
  gotv-tui record <url> [dir]   watch a relay and save it to dir (default gotv-<time>)
  gotv-tui replay <dir>         watch a saved recording, paced as it was broadcast
  gotv-tui demo <file.dem> [x]  play a demo file, at x times real time (default 1)
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
	var rec *record.Recorder
	demo := "" // a .dem file to play instead of a relay
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
	case len(args) >= 2 && (args[0] == "record" || args[0] == "replay"):
		if args[0] == "record" {
			dir := "gotv-" + time.Now().Format("20060102-150405")
			if len(args) > 2 {
				dir = args[2]
			}
			r, err := gotv.RecordTo(dir)
			if err != nil {
				fmt.Fprintln(os.Stderr, "record:", err)
				os.Exit(1)
			}
			rec = r
			fmt.Fprintln(os.Stderr, "recording to", dir)
			arg = args[1]
			break
		}
		url, stop, err := record.Serve(args[1])
		if err != nil {
			fmt.Fprintln(os.Stderr, "replay:", err)
			os.Exit(1)
		}
		defer stop()
		arg = url
	case (len(args) == 2 || len(args) == 3) && args[0] == "demo":
		abs, err := filepath.Abs(args[1])
		if err == nil {
			_, err = os.Stat(abs)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "demo:", err)
			os.Exit(1)
		}
		demo = ui.DemoPrefix + abs
		if len(args) == 3 {
			if x, err := strconv.ParseFloat(args[2], 64); err != nil || x <= 0 || x > 64 {
				fmt.Fprintln(os.Stderr, "demo: speed must be a number between 0 and 64")
				os.Exit(1)
			}
			demo += "@" + args[2]
		}
	case arg == "--help" || arg == "-h":
		fmt.Printf(usage, version)
		return
	case arg != "" && !strings.Contains(arg, "http://") && !strings.Contains(arg, "https://"):
		fmt.Printf(usage, version)
		os.Exit(1)
	}
	url := demo
	if url == "" && arg != "" {
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
	app := ui.New(url, images, gfx)
	app.Version = version
	_, err := tea.NewProgram(app, opts...).Run()
	if images != nil && gfx == term.Kitty {
		term.PixelMouse(images, false) // images were removed by the app on quit
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if rec != nil && rec.Err() != nil {
		fmt.Fprintln(os.Stderr, "warning: the recording is incomplete:", rec.Err())
	}
	if app.Restart { // updated from inside the app: carry on in the new binary
		if exe, err := os.Executable(); err == nil {
			err = syscall.Exec(exe, os.Args, os.Environ())
			fmt.Fprintln(os.Stderr, "updated; restart gotv-tui to use the new version:", err)
		}
	}
}
