package audio

import (
	"bufio"
	"context"
	"image"
	"io"
	"math"
	"os/exec"
	"regexp"
	"runtime"
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
	// Tick: At is the moment the clock turned to Secs (to a frame), not just a
	// moment it showed Secs. Jump: that tick was the freeze time ending (the
	// clock jumped up to the round's start), a moment the HUD's rounding can't blur.
	Tick, Jump bool
}

// The HUD strip read: the top-centre of the picture, where CS2 broadcasts put the
// score and the round clock, scaled to a fixed size.
const (
	stripW, stripH = 1536, 302
	clockFPS       = 20
	clockWarmup    = 8 * time.Second
	stripFilter    = "fps=20,crop=iw*0.4:ih*0.14:iw*0.3:0,format=gray,scale=1536:302,showinfo"
)

var ptsRe = regexp.MustCompile(`pts_time:\s*(-?[0-9.]+)`)

// hwaccel decodes the video in hardware where ffmpeg has it (every frame is
// decoded now, to catch the clock's ticks).
var hwaccel = func() string {
	if runtime.GOOS == "darwin" {
		return "videotoolbox"
	}
	return ""
}()

// WatchClock reads src's round clock until ctx ends; each successful read goes to
// read. Every frame is decoded (in hardware where there is one) at clockFPS, so
// the moment the clock ticks is caught to a frame; the digits are read on ticks
// and once a second besides.
func WatchClock(ctx context.Context, src string, tl *Timeline, read func(ClockRead)) error {
	url, headers, err := resolve(ctx, src, "best[height<=1080]/best")
	if err != nil {
		return err
	}
	args := []string{"-nostats", "-loglevel", "info", "-reconnect", "1", "-reconnect_streamed", "1", "-headers", headers}
	if hwaccel != "" {
		args = append(args, "-hwaccel", hwaccel)
	}
	args = append(args, "-copyts", "-i", url, "-an", "-vf", stripFilter, "-fps_mode", "passthrough",
		"-f", "rawvideo", "-pix_fmt", "gray", "pipe:1")
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
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
	var ticks tickDetector
	began := time.Now()
	last, lastPts := -1, 0.0 // the last value read, for spotting the freeze time's end
	frame := make([]byte, stripW*stripH)
	for n := 0; ; n++ {
		if _, err := io.ReadFull(out, frame); err != nil {
			return ctx.Err()
		}
		p, ok := <-pts
		if !ok {
			return ctx.Err()
		}
		at := tl.observe(p, time.Now())
		settled := time.Since(began) > clockWarmup
		img := &image.Gray{Pix: frame, Stride: stripW, Rect: image.Rect(0, 0, stripW, stripH)}
		r := ClockRead{At: at}
		if reader.lay != nil {
			t, tick := ticks.push(img, reader.lay.box(img), p)
			if !tick && n%clockFPS != 0 {
				continue // between ticks the digits can't have changed
			}
			if tick {
				r.At, _ = tl.Local(t)
				r.Tick = true
			}
		} else if n%(clockFPS/4) != 0 { // still finding the clock: 4 tries a second
			continue
		}
		secs, ok := reader.read(p, img)
		if !ok {
			continue
		}
		r.Secs, r.Round = secs, reader.readRound(img)
		r.Jump = r.Tick && secs >= 100 && last >= 0 && last <= 20 && p-lastPts < 5
		// as in Fy1Sync, a tick counts only if the clock went down by exactly one
		// (or jumped up at the freeze time's end); other pixel changes are not ticks
		r.Tick = r.Tick && (r.Jump || secs == last-1 && p-lastPts < 2.5)
		last, lastPts = secs, p
		if settled { // the first seconds arrive in a burst: their timing is no good
			read(r)
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
