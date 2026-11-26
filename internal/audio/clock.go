package audio

import (
	"bufio"
	"context"
	"image"
	"io"
	"math"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"sync"
	"time"
)

// ClockRead is the round clock read off the stream's picture, and when that
// picture played (local time).
type ClockRead struct {
	At    time.Time
	Secs  int // the clock as shown, e.g. 73 for 01:13
	Round int // "Round N" under the clock, 0 if not read
}

// The HUD strip read: the top-centre of the picture, where CS2 broadcasts put the
// score and the round clock, scaled to a fixed size.
const (
	stripW, stripH = 1536, 302
	stripFilter    = "crop=iw*0.4:ih*0.14:iw*0.3:0,format=gray,scale=1536:302,showinfo"
)

var ptsRe = regexp.MustCompile(`pts_time:\s*(-?[0-9.]+)`)

// WatchClock reads src's round clock from its key frames until ctx ends; each
// successful read goes to read.
func WatchClock(ctx context.Context, src string, tl *Timeline, read func(ClockRead)) error {
	url, headers, err := resolve(ctx, src, "best[height<=1080]/best")
	if err != nil {
		return err
	}
	// key frames only: decoding every frame of a 1080p stream would cost a core
	cmd := exec.CommandContext(ctx, "ffmpeg", "-nostats", "-loglevel", "info", "-reconnect", "1", "-reconnect_streamed", "1",
		"-headers", headers, "-skip_frame", "nokey", "-copyts", "-i", url, "-an", "-vf", stripFilter, "-fps_mode", "passthrough",
		"-f", "rawvideo", "-pix_fmt", "gray", "pipe:1")
	out, err1 := cmd.StdoutPipe()
	logs, err2 := cmd.StderrPipe()
	if err1 != nil || err2 != nil {
		return err1
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	defer cmd.Wait()
	pts := make(chan float64, 64) // showinfo logs each frame before it is written
	go func() {
		defer close(pts)
		r := bufio.NewReader(logs) // not a Scanner: showinfo dumps SEI payloads on very long lines
		for {
			line, err := r.ReadString('\n')
			if m := ptsRe.FindStringSubmatch(line); m != nil {
				if v, err := strconv.ParseFloat(m[1], 64); err == nil {
					pts <- v
				}
			}
			if err != nil {
				return
			}
		}
	}()
	var reader clockReader
	frame := make([]byte, stripW*stripH)
	for {
		if _, err := io.ReadFull(out, frame); err != nil {
			return ctx.Err()
		}
		p, ok := <-pts
		if !ok {
			return ctx.Err()
		}
		at := tl.observe(p, time.Now())
		img := &image.Gray{Pix: frame, Stride: stripW, Rect: image.Rect(0, 0, stripW, stripH)}
		if secs, ok := reader.read(p, img); ok {
			read(ClockRead{At: at, Secs: secs, Round: reader.readRound(img)})
		}
	}
}

// Timeline maps the stream's own time (pts) to local time: arrival minus pts,
// steadied by the median of recent key frames (network bursts only make frames
// late). The clock reader and the audio share it, so the sound is timed on the
// same footing as the clock reads, whatever each pipeline's own lag.
type Timeline struct {
	mu   sync.Mutex
	offs []float64
}

// observe records a frame of stream time pts arriving at arrived and returns
// its local time.
func (c *Timeline) observe(pts float64, arrived time.Time) time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	at := float64(arrived.UnixNano())/1e9 - pts
	if n := len(c.offs); n > 0 && math.Abs(at-c.offs[n-1]) > 30 { // the stream restarted its clock
		c.offs = nil
	}
	c.offs = append(c.offs, at)
	if len(c.offs) > 31 {
		c.offs = c.offs[1:]
	}
	return c.at(pts)
}

// Local is when stream time pts plays locally; false until a frame was seen.
func (c *Timeline) Local(pts float64) (time.Time, bool) {
	if c == nil {
		return time.Time{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.offs) == 0 {
		return time.Time{}, false
	}
	return c.at(pts), true
}

func (c *Timeline) at(pts float64) time.Time {
	s := append([]float64(nil), c.offs...)
	sort.Float64s(s)
	return time.Unix(0, int64((s[len(s)/2]+pts)*1e9))
}
