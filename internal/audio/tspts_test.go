package audio

import (
	"bytes"
	"os/exec"
	"testing"
)

// A real MPEG-TS from ffmpeg, cut into odd-sized pieces: the audio PTS comes out
// right, even across piece boundaries.
func TestTSPTS(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("no ffmpeg")
	}
	ts, err := exec.Command("ffmpeg", "-loglevel", "error", "-f", "lavfi", "-i", "sine=duration=2", "-c:a", "aac",
		"-output_ts_offset", "1000", "-muxdelay", "0", "-muxpreload", "0", "-f", "mpegts", "pipe:1").Output()
	if err != nil {
		t.Fatal(err)
	}
	var r tsPTS
	var pts []float64
	for b := bytes.NewReader(ts); b.Len() > 0; {
		piece := make([]byte, 1000)
		n, _ := b.Read(piece)
		if v, ok := r.feed(piece[:n]); ok {
			pts = append(pts, v)
		}
	}
	if len(pts) < 2 || pts[0] < 999.9 || pts[0] > 1000.1 || pts[len(pts)-1] <= pts[0] || pts[len(pts)-1] > 1002.1 {
		t.Fatalf("pts %v", pts)
	}
}
