// Package update replaces the running binary with the latest GitHub release.
package update

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

var client = &http.Client{Timeout: 2 * time.Minute} // a stalled download fails instead of hanging

const repo = "dormonbear/gotv-tui"

// AssetName is this platform's release asset, e.g. gotv-tui-darwin-arm64, gotv-tui-windows-x64.exe.
func AssetName(goos, goarch string) string {
	arch := map[string]string{"amd64": "x64", "arm64": "arm64"}[goarch]
	if arch == "" || (goos != "darwin" && goos != "linux" && goos != "windows") {
		return ""
	}
	name := "gotv-tui-" + goos + "-" + arch
	if goos == "windows" {
		name += ".exe"
	}
	return name
}

func get(url string) (*http.Response, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "gotv-tui")
	req.Header.Set("Accept", "application/vnd.github+json")
	r, err := client.Do(req)
	if err == nil && r.StatusCode != http.StatusOK {
		r.Body.Close()
		return nil, fmt.Errorf("%s: HTTP %s", url, r.Status)
	}
	return r, err
}

// Run downloads the latest release over the running executable.
func Run(current string) (string, error) {
	if current == "dev" {
		return "Built from source: update with git pull && go build.", nil
	}
	asset := AssetName(runtime.GOOS, runtime.GOARCH)
	if asset == "" {
		return "No prebuilt binary for " + runtime.GOOS + "-" + runtime.GOARCH + ".", nil
	}
	r, err := get("https://api.github.com/repos/" + repo + "/releases/latest")
	if err != nil {
		return "", err
	}
	var rel struct {
		Tag    string `json:"tag_name"`
		Assets []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
	}
	err = json.NewDecoder(r.Body).Decode(&rel)
	r.Body.Close()
	if err != nil {
		return "", err
	}
	latest := strings.TrimPrefix(rel.Tag, "v")
	if latest == strings.TrimPrefix(current, "v") {
		return "Already up to date (v" + latest + ").", nil
	}
	url, sums := "", ""
	for _, a := range rel.Assets {
		switch a.Name {
		case asset + ".gz":
			url = a.URL
		case "SHA256SUMS":
			sums = a.URL
		}
	}
	if url == "" || sums == "" {
		return "", fmt.Errorf("release %s has no %s.gz or SHA256SUMS", rel.Tag, asset)
	}
	gz, err := fetch(url)
	if err != nil {
		return "", err
	}
	list, err := fetch(sums)
	if err != nil {
		return "", err
	}
	if err := verify(gz, list, asset+".gz"); err != nil {
		return "", err
	}
	target, err := os.Executable()
	if err != nil {
		return "", err
	}
	// a private temp file beside the target: concurrent updates can't share it, and
	// the final rename stays on one filesystem
	f, err := os.CreateTemp(filepath.Dir(target), ".gotv-tui-update-*")
	if err != nil {
		return "", err
	}
	tmp := f.Name()
	defer os.Remove(tmp) // no-op once renamed into place
	if err := writeGunzip(f, bytes.NewReader(gz)); err != nil {
		return "", err
	}
	// a running .exe can't be overwritten on Windows, but it can be renamed away
	if runtime.GOOS == "windows" {
		os.Remove(target + ".old")
		if err := os.Rename(target, target+".old"); err != nil {
			return "", err
		}
	}
	if err := os.Rename(tmp, target); err != nil {
		if runtime.GOOS == "windows" {
			os.Rename(target+".old", target) // put the working binary back
		}
		return "", err
	}
	return fmt.Sprintf("Updated v%s -> v%s.", strings.TrimPrefix(current, "v"), latest), nil
}

func writeGunzip(f *os.File, r io.Reader) error {
	z, err := gzip.NewReader(r)
	if err != nil {
		f.Close()
		return err
	}
	if err := f.Chmod(0o755); err != nil {
		f.Close()
		return err
	}
	if _, err := io.Copy(f, z); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func fetch(url string) ([]byte, error) {
	r, err := get(url)
	if err != nil {
		return nil, err
	}
	defer r.Body.Close()
	return io.ReadAll(r.Body)
}

// verify checks data against its line ("<sha256>  <name>") in a SHA256SUMS file.
func verify(data, sums []byte, name string) error {
	got := sha256.Sum256(data)
	for _, line := range strings.Split(string(sums), "\n") {
		if f := strings.Fields(line); len(f) == 2 && strings.TrimPrefix(f[1], "*") == name {
			if f[0] != hex.EncodeToString(got[:]) {
				return fmt.Errorf("%s: checksum mismatch, not installing it", name)
			}
			return nil
		}
	}
	return fmt.Errorf("%s: not listed in SHA256SUMS, not installing it", name)
}
