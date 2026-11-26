package audio

import "testing"

func TestParseHUD(t *testing.T) {
	// real tesseract output from a PGL Major frame
	r, ok := parseHUD("A 2a Jl  wy' =  01:13  FAZE  Round 12  OO  <  wW ||  LL")
	if !ok || r.Round != 12 || r.Secs != 73 {
		t.Fatalf("got %+v %v", r, ok)
	}
	for _, bad := range []string{"01:13 only the clock", "Round 12 only", "Round 3 02:30"} {
		if _, ok := parseHUD(bad); ok {
			t.Errorf("%q: should not read", bad)
		}
	}
}
