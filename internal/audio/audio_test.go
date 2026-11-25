package audio

import "testing"

func TestSource(t *testing.T) {
	for in, want := range map[string]string{
		"https://live.bilibili.com/6": "https://live.bilibili.com/6",
		"bili 6":                      "https://live.bilibili.com/6",
		"Twitch pgl":                  "https://www.twitch.tv/pgl",
		"douyin:12345":                "https://live.douyin.com/12345",
	} {
		if got, err := Source(in); err != nil || got != want {
			t.Errorf("Source(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "6657", "foo 1", "bili 6; rm -rf"} {
		if _, err := Source(bad); err == nil {
			t.Errorf("Source(%q) accepted", bad)
		}
	}
}
