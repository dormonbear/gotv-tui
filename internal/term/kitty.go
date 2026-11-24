package term

import (
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
)

// Kitty graphics with Unicode placeholders: an image is sent once, then cells printed
// as U+10EEEE (+ row/column diacritics, image id in the foreground colour) show it.
// Placeholders are ordinary text, so the TUI can redraw and overlay them freely.

// KittyGraphics reports whether to draw images: GOTV_TUI_GRAPHICS=kitty|off forces
// it; otherwise only terminals known to support Unicode placeholders (kitty, Ghostty).
func KittyGraphics() bool {
	switch os.Getenv("GOTV_TUI_GRAPHICS") {
	case "kitty", "1", "on":
		return true
	case "off", "0", "text":
		return false
	}
	return os.Getenv("TERM_PROGRAM") == "ghostty" || os.Getenv("TERM") == "xterm-ghostty" ||
		os.Getenv("TERM") == "xterm-kitty" || os.Getenv("KITTY_WINDOW_ID") != ""
}

// LockedWriter serialises the TUI's frames and image uploads on one terminal.
type LockedWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func NewLockedWriter(w io.Writer) *LockedWriter { return &LockedWriter{w: w} }

func (l *LockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}

// SendImage uploads png as image id with a virtual placement of cols x rows cells,
// replacing any earlier image with that id (placeholders on screen update in place).
func SendImage(w io.Writer, id uint32, png []byte, cols, rows int) error {
	data := base64.StdEncoding.EncodeToString(png)
	var b strings.Builder
	for i := 0; i < len(data); i += 4096 {
		end := min(i+4096, len(data))
		more := 0
		if end < len(data) {
			more = 1
		}
		b.WriteString("\x1b_G")
		if i == 0 {
			fmt.Fprintf(&b, "a=T,U=1,f=100,q=2,i=%d,c=%d,r=%d,", id, cols, rows)
		}
		fmt.Fprintf(&b, "m=%d;%s\x1b\\", more, data[i:end])
	}
	_, err := io.WriteString(w, b.String()) // one write: never split by a TUI frame
	return err
}

// DeleteImages frees every image this program uploaded.
func DeleteImages(w io.Writer) { io.WriteString(w, "\x1b_Ga=d,d=A,q=2\x1b\\") }

// ImageColor is the foreground colour that tells placeholders which image to show.
func ImageColor(id uint32) string { return Fg(RGB{uint8(id >> 16), uint8(id >> 8), uint8(id)}) }

// Placeholder is the cell at row, col of an image (both < len(diacritics)).
func Placeholder(row, col int) string {
	return string([]rune{0x10EEEE, diacritics[min(row, len(diacritics)-1)], diacritics[min(col, len(diacritics)-1)]})
}
