package term

import (
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
)

// Kitty graphics: an image is uploaded once and then placed with a source rectangle
// under the cell backgrounds, so zooming and panning only send a tiny placement and
// the TUI's text (markers) draws over it.

// KittyGraphics reports whether to draw images: GOTV_TUI_GRAPHICS=kitty|off forces
// it; otherwise only terminals whose kitty graphics we rely on (kitty, Ghostty).
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

// LockedFile serialises the TUI's frames and image uploads on one terminal. It is
// still a terminal file (Fd, Read, Close pass through), so the TUI keeps reading the
// window size and resize events from it.
type LockedFile struct {
	mu sync.Mutex
	f  *os.File
}

func NewLockedFile(f *os.File) *LockedFile { return &LockedFile{f: f} }

func (l *LockedFile) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.f.Write(p)
}

func (l *LockedFile) Read(p []byte) (int, error) { return l.f.Read(p) }
func (l *LockedFile) Close() error               { return l.f.Close() }
func (l *LockedFile) Fd() uintptr                { return l.f.Fd() }

// UploadImage stores png in the terminal as image id (replacing it), without showing it.
func UploadImage(w io.Writer, id uint32, png []byte) error {
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
			fmt.Fprintf(&b, "a=t,f=100,q=2,i=%d,", id)
		}
		fmt.Fprintf(&b, "m=%d;%s\x1b\\", more, data[i:end])
	}
	_, err := io.WriteString(w, b.String()) // one write: never split by a TUI frame
	return err
}

// Rect is a source rectangle in image pixels.
type Rect struct{ X, Y, W, H int }

// belowBackgrounds is kitty's z-index for images under cell backgrounds: blank
// cells show the image, cells with a background colour (map markers) cover it.
const belowBackgrounds = -1073741825

// PlaceImage shows src of image id in cols x rows cells with the top-left at screen
// cell (x, y), 0-based, replacing the image's earlier placement. The cursor is saved
// and restored so the TUI's own cursor bookkeeping is unaffected.
func PlaceImage(w io.Writer, id uint32, x, y, cols, rows int, src Rect) error {
	_, err := fmt.Fprintf(w, "\x1b7\x1b[%d;%dH\x1b_Ga=p,i=%d,p=1,x=%d,y=%d,w=%d,h=%d,c=%d,r=%d,z=%d,C=1,q=2\x1b\\\x1b8",
		y+1, x+1, id, src.X, src.Y, src.W, src.H, cols, rows, belowBackgrounds)
	return err
}

// HideImage removes the placements of image id but keeps it stored.
func HideImage(w io.Writer, id uint32) { fmt.Fprintf(w, "\x1b_Ga=d,d=i,i=%d,q=2\x1b\\", id) }

// PixelMouse switches mouse reports to pixel coordinates (SGR-pixel, mode 1016), so
// dragging the image map can move it by single pixels instead of whole cells.
func PixelMouse(w io.Writer, on bool) {
	if on {
		io.WriteString(w, "\x1b[?1016h")
	} else {
		io.WriteString(w, "\x1b[?1016l")
	}
}

// DeleteImages frees every image this program uploaded.
func DeleteImages(w io.Writer) { io.WriteString(w, "\x1b_Ga=d,d=A,q=2\x1b\\") }
