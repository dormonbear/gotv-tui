// Package audio plays a live stream's sound next to the TUI: yt-dlp fetches the
// stream, ffplay plays it without a window, and a delay buffer between them holds
// the audio back when it runs ahead of the match data.
package audio

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// rooms turns "<platform> <room>" shortcuts into stream page URLs.
var rooms = map[string]string{
	"huya": "https://www.huya.com/", "bili": "https://live.bilibili.com/", "bilibili": "https://live.bilibili.com/",
	"twitch": "https://www.twitch.tv/", "youtube": "https://www.youtube.com/@",
}

var roomRe = regexp.MustCompile(`^\s*([a-z]+)[\s:/]+([A-Za-z0-9_]+)\s*$`)

// Source normalises what the user typed: a URL, or "huya 6657", "bili 6", "twitch name".
func Source(in string) (string, error) {
	in = strings.TrimSpace(in)
	if strings.HasPrefix(in, "http://") || strings.HasPrefix(in, "https://") {
		return in, nil
	}
	if m := roomRe.FindStringSubmatch(strings.ToLower(in)); m != nil {
		if base, ok := rooms[m[1]]; ok {
			return base + m[2], nil
		}
	}
	return "", fmt.Errorf("not a stream: %q (a URL, or e.g. huya 6657, bili 6, twitch name)", in)
}

// Player is one stream being played. Delay is how long the audio is held back.
type Player struct {
	src    string
	status func(string)
	mu     sync.Mutex
	delay  atomic.Int64       // ns; read by the pump without the lock
	stop   context.CancelFunc // the running pipeline
	done   chan struct{}
	closed bool
}

// Start plays src's audio. status gets progress and errors (it may be called from
// other goroutines).
func Start(src string, status func(string)) (*Player, error) {
	for _, tool := range []string{"yt-dlp", "ffmpeg", "ffplay"} {
		if _, err := exec.LookPath(tool); err != nil {
			return nil, fmt.Errorf("%s not found: install yt-dlp and ffmpeg (e.g. brew install yt-dlp ffmpeg)", tool)
		}
	}
	p := &Player{src: src, status: status}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.spawn()
	return p, nil
}

// spawn starts yt-dlp | delay buffer | ffplay (p.mu held).
func (p *Player) spawn() {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	p.stop, p.done = cancel, done
	p.status("audio: finding the stream...")
	go func() {
		defer close(done)
		url, headers, err := resolve(ctx, p.src)
		if err != nil {
			if ctx.Err() == nil {
				p.status("audio: " + err.Error())
			}
			return
		}
		// ffmpeg keeps just the sound, as MPEG-TS: easy to pass through a pipe
		fetch := exec.CommandContext(ctx, "ffmpeg", "-loglevel", "error", "-reconnect", "1", "-reconnect_streamed", "1",
			"-reconnect_delay_max", "5", "-headers", headers, "-i", url, "-vn", "-c:a", "copy", "-f", "mpegts", "pipe:1")
		play := exec.CommandContext(ctx, "ffplay", "-nodisp", "-nostats", "-loglevel", "error", "-i", "pipe:0")
		var fetchErr, playErr strings.Builder
		fetch.Stderr = &limited{w: &fetchErr, n: 2000}
		play.Stderr = &limited{w: &playErr, n: 2000} // e.g. no audio device: shown if it stops
		out, err1 := fetch.StdoutPipe()
		in, err2 := play.StdinPipe()
		if err := errors.Join(err1, err2); err != nil {
			p.status("audio: " + err.Error())
			return
		}
		if err := fetch.Start(); err != nil {
			p.status("audio: " + err.Error())
			return
		}
		if err := play.Start(); err != nil {
			fetch.Wait()
			p.status("audio: " + err.Error())
			return
		}
		p.pump(ctx, out, in, p.status)
		in.Close()
		fetch.Wait()
		play.Wait()
		if ctx.Err() == nil { // ended by itself: say why
			msg := firstLine(strings.TrimSpace(playErr.String() + "\n" + fetchErr.String()))
			if msg == "" {
				msg = "stream ended"
			}
			p.status("audio stopped: " + msg)
		}
	}()
}

// resolve asks yt-dlp for the stream's media URL and the headers it must be
// fetched with.
func resolve(ctx context.Context, src string) (string, string, error) {
	var stderr strings.Builder
	cmd := exec.CommandContext(ctx, "yt-dlp", "-q", "--no-warnings", "-f", "bestaudio/best", "-j", src)
	cmd.Stderr = &limited{w: &stderr, n: 2000}
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if i := strings.LastIndex(msg, "ERROR:"); i >= 0 {
			msg = msg[i+len("ERROR:"):]
		}
		return "", "", fmt.Errorf("can't open the stream: %s", firstLine(strings.TrimSpace(msg)))
	}
	var info struct {
		URL     string            `json:"url"`
		Headers map[string]string `json:"http_headers"`
	}
	if err := json.Unmarshal(out, &info); err != nil || info.URL == "" {
		return "", "", errors.New("can't open the stream: no media URL")
	}
	if strings.HasSuffix(strings.ToLower(hostOf(info.URL)), ".huya.com") {
		signed, err := huyaSign(info.URL, time.Now())
		if err != nil {
			return "", "", err
		}
		info.URL = signed
	}
	var h strings.Builder
	for k, v := range info.Headers {
		if strings.ContainsAny(k+v, "\r\n") {
			continue
		}
		h.WriteString(k + ": " + v + "\r\n")
	}
	return info.URL, h.String(), nil
}

// pump copies the stream to the player, each chunk held until delay after it arrived.
func (p *Player) pump(ctx context.Context, r io.Reader, w io.Writer, status func(string)) {
	type chunk struct {
		at time.Time
		b  []byte
	}
	queue := make(chan chunk, 4096) // ~ minutes of audio at typical chunk sizes
	go func() {
		defer close(queue)
		first := true
		for {
			buf := make([]byte, 32<<10)
			n, err := r.Read(buf)
			if n > 0 {
				if first {
					status("audio: playing")
					first = false
				}
				select {
				case queue <- chunk{time.Now(), buf[:n]}:
				case <-ctx.Done():
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()
	for c := range queue {
		if wait := time.Until(c.at.Add(p.Delay())); wait > 0 {
			select {
			case <-time.After(wait):
			case <-ctx.Done():
				return
			}
		}
		if _, err := w.Write(c.b); err != nil {
			return
		}
	}
}

// SetDelay holds the audio back by d (>= 0). Raising it just holds the sound for
// the difference. Lowering it can't hurry what the player already has, so the
// stream is reopened at its live edge and held by the new, shorter delay.
func (p *Player) SetDelay(d time.Duration) {
	d = max(d, 0)
	p.mu.Lock()
	defer p.mu.Unlock()
	old := time.Duration(p.delay.Swap(int64(d)))
	if d < old && !p.closed {
		p.restart()
	}
}

func (p *Player) Delay() time.Duration { return time.Duration(p.delay.Load()) }

// restart replaces the pipeline (p.mu held).
func (p *Player) restart() {
	p.stop()
	waitFor(p.done)
	p.spawn()
}

// Stop ends playback.
func (p *Player) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed = true
	p.stop()
	waitFor(p.done)
}

func waitFor(done chan struct{}) {
	select {
	case <-done:
	case <-time.After(3 * time.Second):
	}
}

type limited struct {
	w io.Writer
	n int
}

func (l *limited) Write(b []byte) (int, error) {
	if l.n > 0 {
		k := min(len(b), l.n)
		l.w.Write(b[:k])
		l.n -= k
	}
	return len(b), nil
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return s
}

func hostOf(raw string) string {
	if u, err := url.Parse(raw); err == nil {
		return u.Hostname()
	}
	return ""
}
