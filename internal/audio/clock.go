package audio

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"io"
	"os/exec"
	"regexp"
	"strconv"
	"time"
)

// ClockRead is the round clock read off the stream's picture, and when that
// picture arrived.
type ClockRead struct {
	At    time.Time
	Round int
	Secs  int // the clock as shown, e.g. 73 for 01:13
}

// The HUD strip read: the top-centre of the picture, where CS2 broadcasts put the
// score, the round clock and the round number. Scaled to a fixed size for OCR.
const (
	stripW, stripH = 1536, 302
	stripFilter    = "crop=iw*0.4:ih*0.14:iw*0.3:0,format=gray,scale=1536:302,negate"
	warmup         = 5 * time.Second // frames this early can be the server's cached backlog
)

var (
	clockRe = regexp.MustCompile(`\b(\d{1,2}):(\d{2})\b`)
	roundRe = regexp.MustCompile(`(?i)round\s*(\d{1,2})\b`)
)

// HasOCR reports whether auto sync can run (it needs tesseract).
func HasOCR() bool { _, err := exec.LookPath("tesseract"); return err == nil }

// WatchClock reads src's round clock from its key frames until ctx ends; each
// successful read goes to read.
func WatchClock(ctx context.Context, src string, read func(ClockRead)) error {
	url, headers, err := resolve(ctx, src, "best[height<=1080]/best")
	if err != nil {
		return err
	}
	// key frames only: decoding every frame of a 1080p stream would cost a core
	cmd := exec.CommandContext(ctx, "ffmpeg", "-loglevel", "error", "-reconnect", "1", "-reconnect_streamed", "1",
		"-headers", headers, "-skip_frame", "nokey", "-i", url, "-an", "-vf", stripFilter, "-fps_mode", "passthrough",
		"-f", "rawvideo", "-pix_fmt", "gray", "pipe:1")
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	defer cmd.Wait()
	start := time.Now()
	frame := make([]byte, stripW*stripH)
	for {
		if _, err := io.ReadFull(out, frame); err != nil {
			return ctx.Err()
		}
		at := time.Now()
		if at.Sub(start) < warmup {
			continue
		}
		if r, ok := ocrStrip(ctx, frame); ok {
			r.At = at
			read(r)
		}
	}
}

// ocrStrip runs tesseract over one HUD strip.
func ocrStrip(ctx context.Context, frame []byte) (ClockRead, bool) {
	img := &image.Gray{Pix: frame, Stride: stripW, Rect: image.Rect(0, 0, stripW, stripH)}
	var buf bytes.Buffer
	if png.Encode(&buf, img) != nil {
		return ClockRead{}, false
	}
	cmd := exec.CommandContext(ctx, "tesseract", "stdin", "stdout", "--psm", "11")
	cmd.Stdin = &buf
	text, err := cmd.Output()
	if err != nil {
		return ClockRead{}, false
	}
	return parseHUD(string(text))
}

// parseHUD finds "01:13" and "Round 12" in the OCR text. Both are needed: the
// clock alone repeats every round.
func parseHUD(text string) (ClockRead, bool) {
	c, r := clockRe.FindStringSubmatch(text), roundRe.FindStringSubmatch(text)
	if c == nil || r == nil {
		return ClockRead{}, false
	}
	m, _ := strconv.Atoi(c[1])
	s, _ := strconv.Atoi(c[2])
	round, _ := strconv.Atoi(r[1])
	if s > 59 || m*60+s > 115 || round < 1 { // CS2 round clocks never pass 1:55
		return ClockRead{}, false
	}
	return ClockRead{Round: round, Secs: m*60 + s}, true
}
