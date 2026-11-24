package ui

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/dormonbear/gotv-tui/internal/gotv"
	"github.com/dormonbear/gotv-tui/internal/term"
)

func TestWholeFramesAreOverlays(t *testing.T) {
	for _, g := range []term.Graphics{term.ITerm, term.Sixel} {
		f, err := os.CreateTemp(t.TempDir(), "out")
		if err != nil {
			t.Fatal(err)
		}
		m := testMatch(false)
		m.out, m.gfx = term.NewLockedFile(f), g
		m.st = &gotv.State{Players: []gotv.Player{{Name: "a", Side: gotv.T, Alive: true, Pos: &gotv.Vec{X: 500, Y: -500}}}}
		screen := m.view()
		if strings.ContainsAny(plain(screen), "⣿⠿") {
			t.Fatal("the text map should be blank under whole-frame images")
		}
		m.out.Write([]byte("frame")) // a TUI frame: the overlay must follow it
		b, _ := os.ReadFile(f.Name())
		want := map[term.Graphics]string{term.ITerm: "\x1b]1337;File=inline=1;", term.Sixel: "\x1bP0;1;0q"}[g]
		pos := "\x1b7\x1b[" // At(): cursor saved and moved to the map's corner
		if n := strings.Count(string(b), want); n != 2 || !strings.Contains(string(b), pos) || !strings.Contains(string(b), "frame"+pos) {
			t.Fatalf("gfx %d: want the image now and again right after the frame, got %d", g, n)
		}
		m.lastFrame = time.Time{}
		m.hideImages()
		m.out.Write([]byte("list"))
		if b2, _ := os.ReadFile(f.Name()); strings.HasSuffix(string(b2), "\x1b8") {
			t.Fatal("after leaving the match, frames must not be redrawn")
		}
	}
}
