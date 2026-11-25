package feeds

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestNormalizeURL(t *testing.T) {
	if got := NormalizeURL(` playcast "https://cstv.fragnet.net/a/" `); got != "https://cstv.fragnet.net/a" {
		t.Fatalf("got %q", got)
	}
	if !IsRelayURL(`playcast "https://x.y/a"`) || IsRelayURL("cstv.fragnet.net/a") {
		t.Fatal("IsRelayURL")
	}
}

func TestLoadSaveFallback(t *testing.T) {
	file := filepath.Join(t.TempDir(), "feeds.json")
	if !reflect.DeepEqual(Load(file), Presets) {
		t.Fatal("missing file should give presets")
	}
	mine := []Feed{{Name: "Mine", URL: "https://r.example/x"}}
	if err := Save(file, mine); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(Load(file), mine) {
		t.Fatal("round trip")
	}
	os.WriteFile(file, []byte("{broken"), 0o644)
	if !reflect.DeepEqual(Load(file), Presets) {
		t.Fatal("broken file should give presets")
	}
}

func TestStateOf(t *testing.T) {
	tick, fresh, old := 1, 0.2, 367.0
	if StateOf(&Sync{Tick: &tick, RcvAge: &fresh}) != Live || StateOf(&Sync{Tick: &tick, RcvAge: &old}) != Stale || StateOf(nil) != Offline {
		t.Fatal("StateOf")
	}
}

func TestMapName(t *testing.T) {
	for api, want := range map[string]string{"de_dust2": "Dust II", "de_nuke": "Nuke", "cs_office": "Office", "de_brand_new": "Brand new", "weird": "Weird", "": ""} {
		if got := MapName(api); got != want {
			t.Errorf("MapName(%q) = %q, want %q", api, got, want)
		}
	}
}
