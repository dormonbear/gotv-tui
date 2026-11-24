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

var errNotFound = fmt.Errorf("not found")

func cached(name string) ([]byte, error) {
	home, _ := os.UserHomeDir()
	file := filepath.Join(home, ".cache", "gotv-tui", "radar", name)
	if b, err := os.ReadFile(file); err == nil {
		return b, nil
	}
	r, err := http.Get(src + "/" + name)
	if err != nil {
		return nil, err
	}
	defer r.Body.Close()
	if r.StatusCode == http.StatusNotFound {
		return nil, errNotFound
	}
	if r.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("radar %s: HTTP %s", name, r.Status)
	}
	b, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	_ = os.MkdirAll(filepath.Dir(file), 0o755)
	_ = os.WriteFile(file, b, 0o644)
	return b, nil
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

func Load(mapName string) (*Assets, error) {
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
	} else if err != errNotFound {
		return nil, err
	}
	return a, nil
}
