//go:build unix

package term

import (
	"os"

	"golang.org/x/sys/unix"
)

// measuredAspect asks the tty for its pixel size (TIOCGWINSZ); terminals that don't
// report pixels leave it 0.
func measuredAspect() (float64, bool) {
	ws, err := unix.IoctlGetWinsize(int(os.Stdout.Fd()), unix.TIOCGWINSZ)
	if err != nil || ws.Xpixel == 0 || ws.Ypixel == 0 || ws.Row == 0 || ws.Col == 0 {
		return 0, false
	}
	return (float64(ws.Ypixel) / float64(ws.Row)) / (float64(ws.Xpixel) / float64(ws.Col)), true
}
