package record

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func get(t *testing.T, c *http.Client, url string) (int, string) {
	t.Helper()
	r, err := c.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Body.Close()
	b, _ := io.ReadAll(r.Body)
	return r.StatusCode, string(b)
}

func relay(t *testing.T) *httptest.Server {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/sync") {
			io.WriteString(w, `{"fragment":9,"token_redirect":"s123t456"}`)
			return
		}
		io.WriteString(w, "body of "+r.URL.Path)
	}))
	t.Cleanup(s.Close)
	return s
}

func TestRecordThenReplayPaced(t *testing.T) {
	a, b := relay(t), relay(t)
	dir := t.TempDir()
	rec, err := Transport(dir, http.DefaultTransport)
	if err != nil {
		t.Fatal(err)
	}
	c := &http.Client{Transport: rec}
	for _, p := range []string{"/sync", "/7/start", "/9/full", "/9/delta"} {
		get(t, c, a.URL+p)
	}
	get(t, c, b.URL+"/10/delta") // another relay (after going back to the list): not recorded
	time.Sleep(300 * time.Millisecond)
	get(t, c, a.URL+"/10/delta") // arrived 0.3 s after the others
	if rec.Err() != nil {
		t.Fatal(rec.Err())
	}
	if _, err := Transport(dir, http.DefaultTransport); err == nil {
		t.Fatal("recorded over an existing recording")
	}

	url, stop, err := Serve(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	srv := http.DefaultClient
	if code, body := get(t, srv, url+"/sync"); code != 200 || strings.Contains(body, "token_redirect") {
		t.Fatalf("sync: %d %q (a token redirect would send the reader off the recording)", code, body)
	}
	if code, _ := get(t, srv, url+"/9/delta"); code != 200 {
		t.Fatal("first delta should be there at once")
	}
	t0 := time.Now()
	if code, body := get(t, srv, url+"/10/delta"); code != 200 || body != "body of /10/delta" {
		t.Fatalf("second delta: %d %q", code, body)
	}
	if d := time.Since(t0); d < 150*time.Millisecond {
		t.Fatalf("second delta served after %v, before its recorded time", d)
	}
	if code, _ := get(t, srv, url+"/robots.txt"); code != 404 {
		t.Fatal("served something that isn't a fragment")
	}
	if code, _ := get(t, srv, url+"/sync"); code != http.StatusMethodNotAllowed {
		t.Fatalf("after the last delta /sync should say waiting, got %d", code)
	}
}

func TestReplayWaitIsCancellable(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "9"), 0o755)
	os.WriteFile(filepath.Join(dir, "sync"), []byte(`{}`), 0o644)
	os.WriteFile(filepath.Join(dir, "9", "full"), []byte("f"), 0o644)
	os.WriteFile(filepath.Join(dir, "9", "delta"), []byte("d"), 0o644)
	os.WriteFile(filepath.Join(dir, indexFile), []byte(`{"path":"sync","at":0}
{"path":"9/full","at":0}
{"path":"9/delta","at":60000}
`), 0o644)
	url, stop, err := Serve(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", url+"/9/delta", nil)
	if _, err := http.DefaultClient.Do(req); err == nil {
		t.Fatal("a delta due in a minute was served at once")
	}
}

func TestReplayServesOnlyRegularFilesInside(t *testing.T) {
	dir := t.TempDir()
	secret := filepath.Join(t.TempDir(), "secret")
	os.WriteFile(secret, []byte("private"), 0o644)
	os.MkdirAll(filepath.Join(dir, "9"), 0o755)
	os.WriteFile(filepath.Join(dir, "sync"), []byte(`{}`), 0o644)
	if err := os.Symlink(secret, filepath.Join(dir, "9", "full")); err != nil {
		t.Skip("no symlinks here:", err)
	}
	os.WriteFile(filepath.Join(dir, indexFile), []byte("{\"path\":\"sync\",\"at\":0}\n{\"path\":\"9/full\",\"at\":0}\n"), 0o644)
	url, stop, err := Serve(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	if code, body := get(t, http.DefaultClient, url+"/9/full"); code == 200 || strings.Contains(body, "private") {
		t.Fatalf("served a symlink out of the recording: %d %q", code, body)
	}
}
