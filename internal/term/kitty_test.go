package term

import (
	"os"
	"testing"

	xterm "github.com/charmbracelet/x/term"
)

// The TUI only reads the window size from an output that is a terminal file.
func TestLockedFileIsATerminalFile(t *testing.T) {
	var _ xterm.File = NewLockedFile(os.Stdout)
}
