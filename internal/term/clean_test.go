package term

import "testing"

func TestClean(t *testing.T) {
	for in, want := range map[string]string{
		"NAVI":                         "NAVI",
		"evil\x1b]52;c;aGk=\x07name":   "evil ]52;c;aGk= name",
		"two\nlines\r":                 "two lines",
		"c1\u009b31mred":               "c1 31mred",
		"verylongteamnamethatgoeson!!": "verylongteamnamethatgoeson!!"[:20],
	} {
		if got := Clean(in, 20); got != want {
			t.Errorf("Clean(%q) = %q, want %q", in, got, want)
		}
	}
}
