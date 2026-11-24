package term

import (
	"os"
	"strings"
)

// Graphics is how the terminal can show the radar as an image.
type Graphics int

const (
	Text  Graphics = iota // no images: braille map
	Kitty                 // kitty graphics: upload once, place every frame (kitty, Ghostty)
	ITerm                 // iTerm2 inline images: whole frame as PNG (iTerm2, WezTerm)
	Sixel                 // sixel: whole frame, 256 colours (Windows Terminal, foot, Konsole)
)

// DetectGraphics picks the image protocol: GOTV_TUI_GRAPHICS=kitty|iterm|sixel|off
// forces one; otherwise only terminals known to support it get images. (VS Code
// needs terminal.integrated.enableImages, which can't be detected: opt in.)
func DetectGraphics() Graphics {
	switch os.Getenv("GOTV_TUI_GRAPHICS") {
	case "kitty", "1", "on":
		return Kitty
	case "iterm":
		return ITerm
	case "sixel":
		return Sixel
	case "off", "0", "text":
		return Text
	}
	prog, t := os.Getenv("TERM_PROGRAM"), os.Getenv("TERM")
	switch {
	case prog == "ghostty" || t == "xterm-ghostty" || t == "xterm-kitty" || os.Getenv("KITTY_WINDOW_ID") != "":
		return Kitty
	case prog == "iTerm.app" || prog == "WezTerm":
		return ITerm
	case os.Getenv("WT_SESSION") != "" || strings.HasPrefix(t, "foot") || os.Getenv("KONSOLE_VERSION") != "":
		return Sixel
	}
	return Text
}
