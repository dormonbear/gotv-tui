package update

import "testing"

func TestAssetName(t *testing.T) {
	for _, c := range [][3]string{
		{"darwin", "arm64", "gotv-tui-darwin-arm64"},
		{"linux", "amd64", "gotv-tui-linux-x64"},
		{"windows", "amd64", "gotv-tui-windows-x64.exe"},
		{"freebsd", "amd64", ""},
		{"linux", "386", ""},
	} {
		if got := AssetName(c[0], c[1]); got != c[2] {
			t.Errorf("%s/%s: %q", c[0], c[1], got)
		}
	}
}
