// Package record saves a CSTV relay's fragments while watching and serves them
// back later as a local relay, paced as they arrived: a replay is indistinguishable
// from the live broadcast, so it needs no special path in the parser or the UI.
package record

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"
)

// fragment paths a CSTV reader asks for: /sync, /<n>/start, /<n>/full, /<n>/delta
var pathRe = regexp.MustCompile(`/(sync|\d+/(?:start|full|delta))$`)

// entry is one saved response; At is ms since the recording started.
type entry struct {
	Path string `json:"path"`
	At   int64  `json:"at"`
}

const indexFile = "index.jsonl"

type recorder struct {
	rt    http.RoundTripper
	dir   string
	start time.Time
	mu    sync.Mutex
	seen  map[string]bool
}

// Transport wraps rt so every relay fragment it fetches is also saved under dir.
func Transport(dir string, rt http.RoundTripper) (http.RoundTripper, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &recorder{rt: rt, dir: dir, start: time.Now(), seen: map[string]bool{}}, nil
}

func (r *recorder) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := r.rt.RoundTrip(req)
	m := pathRe.FindStringSubmatch(req.URL.Path)
	if err != nil || m == nil || resp.StatusCode != http.StatusOK {
		return resp, err
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return nil, err
	}
	resp.Body = io.NopCloser(bytes.NewReader(body))
	r.save(m[1], body)
	return resp, nil
}

// save keeps the first copy of each path (later /sync answers are the relay
// moving on, which the replay recreates by itself).
func (r *recorder) save(path string, body []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.seen[path] {
		return
	}
	r.seen[path] = true
	file := filepath.Join(r.dir, filepath.FromSlash(path))
	if os.MkdirAll(filepath.Dir(file), 0o755) != nil || os.WriteFile(file, body, 0o644) != nil {
		return
	}
	line, _ := json.Marshal(entry{path, time.Since(r.start).Milliseconds()})
	f, err := os.OpenFile(filepath.Join(r.dir, indexFile), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	f.Write(append(line, '\n'))
	f.Close()
}

// Serve plays a recording back as a relay on 127.0.0.1 and returns its URL.
// Deltas become available at the pace they were recorded; after the last one /sync
// answers "waiting" (405), as a relay between matches does.
func Serve(dir string) (string, func(), error) {
	f, err := os.Open(filepath.Join(dir, indexFile))
	if err != nil {
		return "", nil, fmt.Errorf("not a recording (no %s): %w", indexFile, err)
	}
	at := map[string]int64{}
	var base int64 = -1 // when the recording's first /full arrived: replay time zero
	var last string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var e entry
		if json.Unmarshal(sc.Bytes(), &e) != nil || !pathRe.MatchString("/"+e.Path) {
			continue
		}
		at[e.Path] = e.At
		if base < 0 && filepath.Base(e.Path) == "full" {
			base = e.At
		}
		if filepath.Base(e.Path) == "delta" {
			last = e.Path
		}
	}
	f.Close()
	if _, ok := at["sync"]; !ok || base < 0 {
		return "", nil, errors.New("recording has no /sync or /full: nothing to replay")
	}
	var mu sync.Mutex
	var started time.Time
	done := false
	h := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		path := req.URL.Path[1:]
		mu.Lock()
		if started.IsZero() {
			started = time.Now()
		}
		elapsed := time.Since(started).Milliseconds()
		finished := done
		mu.Unlock()
		t, ok := at[path]
		switch {
		case path == "sync" && finished:
			http.Error(w, "replay over", http.StatusMethodNotAllowed)
			return
		case !ok, filepath.Base(path) == "delta" && t-base > elapsed:
			http.NotFound(w, req) // not recorded, or not "arrived" yet: the reader retries
			return
		}
		if path == last {
			mu.Lock()
			done = true
			mu.Unlock()
		}
		http.ServeFile(w, req, filepath.Join(dir, filepath.FromSlash(path)))
	})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", nil, err
	}
	srv := &http.Server{Handler: h}
	go srv.Serve(ln)
	return "http://" + ln.Addr().String(), func() { srv.Close() }, nil
}
