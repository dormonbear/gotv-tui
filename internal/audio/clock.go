package audio

import (
	"bufio"
	"context"
	"image"
	"io"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"time"
)

// ClockRead is the round clock read off the stream's picture, and when that
// picture played (local time).
type ClockRead struct {
	At   time.Time
	Secs int // the clock as shown, e.g. 73 for 01:13
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
func WatchClock(ctx context.Context, src string, read func(ClockRead)) error {
	url, headers, err := resolve(ctx, src, "best[height<=1080]/best")
	if err != nil {
		return err
	}
	// key frames only: decoding every frame of a 1080p stream would cost a core
	cmd := exec.CommandContext(ctx, "ffmpeg", "-nostats", "-loglevel", "info", "-reconnect", "1", "-reconnect_streamed", "1",
		"-headers", headers, "-skip_frame", "nokey", "-i", url, "-an", "-vf", stripFilter, "-fps_mode", "passthrough",
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
	var clock streamClock
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
		at := clock.local(p, time.Now())
		img := &image.Gray{Pix: frame, Stride: stripW, Rect: image.Rect(0, 0, stripW, stripH)}
		if secs, ok := reader.read(p, img); ok {
			read(ClockRead{At: at, Secs: secs})
		}
	}
}

// streamClock maps stream time to local time: arrival minus pts, steadied by
// taking the median of recent frames (network bursts only ever make frames late).
type streamClock struct{ offs []float64 }

func (c *streamClock) local(pts float64, arrived time.Time) time.Time {
	at := float64(arrived.UnixNano())/1e9 - pts
	if n := len(c.offs); n > 0 && abs(int(at-c.offs[n-1])) > 30 { // the stream restarted its clock
		c.offs = nil
	}
	c.offs = append(c.offs, at)
	if len(c.offs) > 31 {
		c.offs = c.offs[1:]
	}
	s := append([]float64(nil), c.offs...)
	sort.Float64s(s)
	base := s[len(s)/2]
	return time.Unix(0, int64((base+pts)*1e9))
}
