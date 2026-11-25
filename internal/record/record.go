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
	"strings"
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

// Recorder saves the fragments of one relay: the first one it sees.
type Recorder struct {
	rt    http.RoundTripper
	dir   string
	start time.Time
	mu    sync.Mutex
	base  string // the relay being recorded; others (after going back to the list) are not
	seen  map[string]bool
	err   error // first write failure: the recording is incomplete
}

// Transport wraps rt so every fragment of the relay it fetches first is also saved
// under dir, which must not already hold a recording.
func Transport(dir string, rt http.RoundTripper) (*Recorder, error) {
	if _, err := os.Stat(filepath.Join(dir, indexFile)); err == nil {
		return nil, fmt.Errorf("%s already holds a recording; pick a new directory", dir)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &Recorder{rt: rt, dir: dir, start: time.Now(), seen: map[string]bool{}}, nil
}

// Err reports a fragment that couldn't be saved.
func (r *Recorder) Err() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.err
}

func (r *Recorder) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := r.rt.RoundTrip(req)
	m := pathRe.FindStringSubmatchIndex(req.URL.Path)
	if err != nil || m == nil || resp.StatusCode != http.StatusOK {
		return resp, err
	}
	base := req.URL.Scheme + "://" + req.URL.Host + req.URL.Path[:m[0]]
	r.mu.Lock()
	if r.base == "" {
		r.base = base
	}
	same := r.base == base
	r.mu.Unlock()
	if !same {
		return resp, nil
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return nil, err
	}
	resp.Body = io.NopCloser(bytes.NewReader(body))
	r.save(req.URL.Path[m[2]:m[3]], body)
	return resp, nil
}

// save keeps the first copy of each path (later /sync answers are the relay
// moving on, which the replay recreates by itself).
func (r *Recorder) save(path string, body []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.seen[path] {
		return
	}
	line, _ := json.Marshal(entry{path, time.Since(r.start).Milliseconds()})
	file := filepath.Join(r.dir, filepath.FromSlash(path))
	err := os.MkdirAll(filepath.Dir(file), 0o755)
	if err == nil {
		err = os.WriteFile(file, body, 0o644)
	}
	if err == nil {
		var f *os.File
		if f, err = os.OpenFile(filepath.Join(r.dir, indexFile), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); err == nil {
			_, err = f.Write(append(line, '\n'))
			if cerr := f.Close(); err == nil {
				err = cerr
			}
		}
	}
	if err != nil {
		if r.err == nil {
			r.err = err
		}
		return // not marked seen: a later fetch of the same path can still save it
	}
	r.seen[path] = true
}

// Serve plays a recording back as a relay on 127.0.0.1 and returns its URL.
// Deltas become available at the pace they were recorded (a request for one that
// isn't due yet waits for it); after the last one /sync answers "waiting" (405), as
// a relay between matches does.
func Serve(dir string) (string, func(), error) {
	root, err := os.OpenRoot(dir) // files are only ever opened inside the recording
	if err != nil {
		return "", nil, err
	}
	f, err := root.Open(indexFile)
	if err != nil {
		root.Close()
		return "", nil, fmt.Errorf("not a recording (no %s): %w", indexFile, err)
	}
	at := map[string]int64{}
	var base int64 = -1 // when the recording's first /full arrived: replay time zero
	var last string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var e entry
		if json.Unmarshal(sc.Bytes(), &e) != nil || !exactRe.MatchString(e.Path) {
			continue
		}
		at[e.Path] = e.At
		kind := e.Path[strings.LastIndexByte(e.Path, '/')+1:]
		if base < 0 && kind == "full" {
			base, last = e.At, e.Path // a recording without deltas ends after its /full
		}
		if kind == "delta" {
			last = e.Path
		}
	}
	f.Close()
	if _, ok := at["sync"]; !ok || base < 0 {
		root.Close()
		return "", nil, errors.New("recording has no /sync or /full: nothing to replay")
	}
	syncBody, err := readFile(root, "sync")
	if err != nil {
		root.Close()
		return "", nil, err
	}
	// the stored paths are relative to the relay root: a token redirect would point
	// the reader elsewhere
	var meta map[string]any
	if json.Unmarshal(syncBody, &meta) == nil {
		delete(meta, "token_redirect")
		syncBody, _ = json.Marshal(meta)
	}
	var mu sync.Mutex
	var started time.Time
	done := false
	h := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		path := strings.TrimPrefix(req.URL.Path, "/")
		mu.Lock()
		if started.IsZero() {
			started = time.Now()
		}
		start, finished := started, done
		mu.Unlock()
		t, ok := at[path]
		switch {
		case path == "sync" && finished:
			http.Error(w, "replay over", http.StatusMethodNotAllowed)
			return
		case path == "sync":
			w.Header().Set("Content-Type", "application/json")
			w.Write(syncBody)
			return
		case !ok:
			http.NotFound(w, req)
			return
		}
		if due := start.Add(time.Duration(t-base) * time.Millisecond); time.Now().Before(due) {
			select { // recorded later: hold the request until then, as the relay did
			case <-time.After(time.Until(due)):
			case <-req.Context().Done():
				return
			}
		}
		b, err := readFile(root, path)
		if err != nil {
			http.NotFound(w, req)
			return
		}
		if path == last {
			mu.Lock()
			done = true
			mu.Unlock()
		}
		w.Write(b)
	})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		root.Close()
		return "", nil, err
	}
	srv := &http.Server{Handler: h}
	go srv.Serve(ln)
	return "http://" + ln.Addr().String(), func() { srv.Close(); root.Close() }, nil
}

// exactRe is a fragment path as stored in the index.
var exactRe = regexp.MustCompile(`^(sync|\d+/(?:start|full|delta))$`)

// readFile reads a regular file inside root (no symlink games, no escaping it).
func readFile(root *os.Root, name string) ([]byte, error) {
	f, err := root.Open(filepath.FromSlash(name))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if st, err := f.Stat(); err != nil || !st.Mode().IsRegular() {
		return nil, errors.New("not a regular file")
	}
	return io.ReadAll(io.LimitReader(f, 64<<20))
}
