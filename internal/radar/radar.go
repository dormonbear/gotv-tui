// Package radar loads Valve radar images and maps world coordinates onto them.
package radar

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Valve radar PNGs + overview files dumped from the game. Fetched once and cached;
// not bundled (Valve assets).
const src = "https://raw.githubusercontent.com/2mlml/cs2-radar-images/master"

type Site struct {
	Name string
	X, Y float64 // normalized 0..1 radar coords
}

type Overview struct {
	PosX, PosY, Scale float64
	LowerBelow        *float64 // z under which a position is on the lower radar
	Sites             []Site
}

var (
	lowerRe = regexp.MustCompile(`"lower"[^{]*\{[^}]*"AltitudeMax"\s+"(-?[\d.]+)"`)
)

func num(txt, key string) (float64, bool) {
	m := regexp.MustCompile(`"` + key + `"\s+"(-?[\d.]+)"`).FindStringSubmatch(txt)
	if m == nil {
		return 0, false
	}
	v, err := strconv.ParseFloat(m[1], 64)
	return v, err == nil
}

func ParseOverview(txt string) Overview {
	var o Overview
	o.PosX, _ = num(txt, "pos_x")
	o.PosY, _ = num(txt, "pos_y")
	o.Scale, _ = num(txt, "scale")
	if m := lowerRe.FindStringSubmatch(txt); m != nil {
		if v, err := strconv.ParseFloat(m[1], 64); err == nil {
			o.LowerBelow = &v
		}
	}
	for _, n := range []string{"A", "B"} {
		x, ok1 := num(txt, "bomb"+n+"_x")
		y, ok2 := num(txt, "bomb"+n+"_y")
		if ok1 && ok2 {
			o.Sites = append(o.Sites, Site{n, x, y})
		}
	}
	return o
}

// Overview files don't say which level a bombsite is on; default is the main (upper) radar.
var lowerSites = map[string]string{"de_nuke": "B"}

func SitesOn(mapName string, lower bool, sites []Site) []Site {
	var out []Site
	for _, s := range sites {
		if (lowerSites[mapName] == s.Name) == lower {
			out = append(out, s)
		}
	}
	return out
}

type Assets struct {
	Map      string
	Overview Overview
	Upper    *image.NRGBA
	Lower    *image.NRGBA // nil for single-level maps
}

// ErrNotFound: no source has this map.
var ErrNotFound = fmt.Errorf("not found")

var client = &http.Client{Timeout: 20 * time.Second}

// valid rejects a truncated or wrong download (e.g. an HTML error page) so it is
// neither used nor cached.
func valid(name string, b []byte) bool {
	if strings.HasSuffix(name, ".png") { // full decode: a cut-off file can have a fine header
		_, err := png.Decode(bytes.NewReader(b))
		return err == nil
	}
	return ParseOverview(string(b)).Scale > 0
}

func cached(name string) ([]byte, error) {
	home, _ := os.UserHomeDir()
	file := filepath.Join(home, ".cache", "gotv-tui", "radar", name)
	if b, err := os.ReadFile(file); err == nil && valid(name, b) {
		return b, nil // a bad cached copy (older versions cached anything) is fetched again
	}
	var b []byte
	for _, url := range sources(name) {
		got, err := fetch(url)
		if err == ErrNotFound {
			continue
		}
		if err != nil {
			return nil, err
		}
		if !valid(name, got) {
			return nil, fmt.Errorf("radar %s: invalid download", name)
		}
		b = got
		break
	}
	if b == nil {
		return nil, ErrNotFound
	}
	_ = os.MkdirAll(filepath.Dir(file), 0o755)
	if tmp := file + ".tmp"; os.WriteFile(tmp, b, 0o644) == nil { // whole file or nothing
		_ = os.Rename(tmp, file)
	}
	return b, nil
}

// Fallbacks for maps the main source doesn't have yet (Cache, at the 2026
// Major): the overview from the game files as SteamTracking mirrors them, the
// radar image as MurkyYT/cs2-map-icons extracted it.
const (
	overviewSrc = "https://raw.githubusercontent.com/SteamTracking/GameTracking-CS2/master/game/csgo/pak01_dir/resource/overviews"
	imageSrc    = "https://raw.githubusercontent.com/MurkyYT/cs2-map-icons/main/images/radars"
)

// sources are the URLs to try, in order, for a cached file name.
func sources(name string) []string {
	out := []string{src + "/" + name}
	if strings.HasSuffix(name, ".txt") {
		return append(out, overviewSrc+"/"+name)
	}
	base := strings.TrimSuffix(name, ".png") // de_x or de_x_lower
	suffixes := []string{"_radar_psd.png", "_radar_tga.png", "_radar.png"}
	if strings.HasSuffix(base, "_lower") { // asked for every map: keep the misses few
		suffixes = suffixes[:1]
	}
	for _, suffix := range suffixes {
		out = append(out, imageSrc+"/"+base+suffix)
	}
	return out
}

func fetch(url string) ([]byte, error) {
	r, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer r.Body.Close()
	if r.StatusCode == http.StatusNotFound {
		return nil, ErrNotFound
	}
	if r.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("radar: HTTP %s from %s", r.Status, url)
	}
	return io.ReadAll(r.Body)
}

func decode(b []byte) (*image.NRGBA, error) {
	img, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	out := image.NewNRGBA(img.Bounds())
	draw.Draw(out, out.Bounds(), img, img.Bounds().Min, draw.Src)
	return out, nil
}

// mapNameRe is what a map name may look like: it becomes a file name and a URL.
var mapNameRe = regexp.MustCompile(`^[a-z0-9_]{1,40}$`)

func Load(mapName string) (*Assets, error) {
	if !mapNameRe.MatchString(mapName) {
		return nil, fmt.Errorf("unexpected map name %q", mapName)
	}
	txt, err := cached(mapName + ".txt")
	if err != nil {
		return nil, fmt.Errorf("radar %s: %w", mapName, err)
	}
	up, err := cached(mapName + ".png")
	if err != nil {
		return nil, fmt.Errorf("radar %s: %w", mapName, err)
	}
	a := &Assets{Map: mapName, Overview: ParseOverview(string(txt))}
	if a.Upper, err = decode(up); err != nil {
		return nil, err
	}
	if lo, err := cached(mapName + "_lower.png"); err == nil {
		if a.Lower, err = decode(lo); err != nil {
			return nil, err
		}
	} else if err != ErrNotFound {
		return nil, err
	}
	return a, nil
}
