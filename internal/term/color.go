// Package term holds the truecolor ANSI helpers shared by the map and the UI.
package term

import "fmt"

type RGB [3]uint8

var (
	CT    = RGB{90, 170, 255}
	T     = RGB{240, 176, 48}
	Black = RGB{10, 10, 10}
	White = RGB{255, 255, 255}
)

const Reset = "\x1b[0m"

func Fg(c RGB) string { return fmt.Sprintf("\x1b[38;2;%d;%d;%dm", c[0], c[1], c[2]) }
func Bg(c RGB) string { return fmt.Sprintf("\x1b[48;2;%d;%d;%dm", c[0], c[1], c[2]) }

func Mix(a, b RGB, t float64) RGB {
	var o RGB
	for i := range o {
		o[i] = uint8(float64(a[i])*(1-t) + float64(b[i])*t + 0.5)
	}
	return o
}

func Scale(c RGB, k float64) RGB {
	var o RGB
	for i := range o {
		o[i] = uint8(min(255, float64(c[i])*k+0.5))
	}
	return o
}
