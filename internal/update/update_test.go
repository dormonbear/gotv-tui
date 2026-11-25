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

func TestVerify(t *testing.T) {
	data := []byte("binary")
	sum := "9a3a45d01531a20e89ac6ae10b0b0beb0492acd7216a368aa062d1a5fecaf9cd" // sha256("binary")
	ok := []byte("deadbeef  other.gz\n" + sum + "  gotv-tui-linux-x64.gz\n")
	if err := verify(data, ok, "gotv-tui-linux-x64.gz"); err != nil {
		t.Fatal(err)
	}
	if verify([]byte("tampered"), ok, "gotv-tui-linux-x64.gz") == nil {
		t.Fatal("accepted a mismatching download")
	}
	if verify(data, ok, "gotv-tui-darwin-arm64.gz") == nil {
		t.Fatal("accepted an asset missing from SHA256SUMS")
	}
}

func TestNewer(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{{"0.17.0", "0.16.9", true}, {"0.10.0", "0.9.3", true}, {"v1.0.0", "0.99.0", true}, {"0.17.0", "0.17.0", false}, {"0.16.0", "0.17.0", false}} {
		if got := Newer(c.a, c.b); got != c.want {
			t.Errorf("Newer(%s, %s) = %v", c.a, c.b, got)
		}
	}
}
