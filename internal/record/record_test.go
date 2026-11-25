package record

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRecordThenReplayPaced(t *testing.T) {
	relay := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "body of "+r.URL.Path)
	}))
	defer relay.Close()
	dir := t.TempDir()
	rt, err := Transport(dir, http.DefaultTransport)
	if err != nil {
		t.Fatal(err)
	}
	c := &http.Client{Transport: rt}
	get := func(base, p string) (int, string) {
		r, err := c.Get(base + p)
		if err != nil {
			t.Fatal(err)
		}
		defer r.Body.Close()
		b, _ := io.ReadAll(r.Body)
		return r.StatusCode, string(b)
	}
	for _, p := range []string{"/sync", "/7/start", "/9/full", "/9/delta"} {
		get(relay.URL, p)
	}
	time.Sleep(300 * time.Millisecond)
	get(relay.URL, "/10/delta") // arrived 0.3 s after the others
	get(relay.URL, "/robots.txt")

	url, stop, err := Serve(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	if code, body := get(url, "/sync"); code != 200 || body != "body of /sync" {
		t.Fatalf("sync: %d %q", code, body)
	}
	if code, _ := get(url, "/9/delta"); code != 200 {
		t.Fatal("first delta should be there at once")
	}
	if code, _ := get(url, "/10/delta"); code != 404 {
		t.Fatal("second delta served before its time")
	}
	if code, _ := get(url, "/robots.txt"); code != 404 {
		t.Fatal("served something that isn't a fragment")
	}
	time.Sleep(400 * time.Millisecond)
	if code, body := get(url, "/10/delta"); code != 200 || body != "body of /10/delta" {
		t.Fatalf("second delta: %d %q", code, body)
	}
	if code, _ := get(url, "/sync"); code != http.StatusMethodNotAllowed {
		t.Fatalf("after the last delta /sync should say waiting, got %d", code)
	}
}
