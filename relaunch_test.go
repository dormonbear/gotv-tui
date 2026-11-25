package main

import (
	"strings"
	"testing"
)

func TestBetterTerminal(t *testing.T) {
	env := func(kv ...string) func(string) string {
		m := map[string]string{"HOME": "/Users/u"}
		for i := 0; i+1 < len(kv); i += 2 {
			m[kv[i]] = kv[i+1]
		}
		return func(k string) string { return m[k] }
	}
	only := func(paths ...string) func(string) bool {
		return func(p string) bool {
			for _, x := range paths {
				if p == x {
					return true
				}
			}
			return false
		}
	}
	cmd := []string{"/bin/gotv-tui", "https://cstv.fragnet.net/a"}
	line := func(goos string, e func(string) string, has func(string) bool) string {
		name, c := betterTerminal(goos, e, has, cmd)
		if c == nil {
			return ""
		}
		return name + ": " + strings.Join(c.Args, " ")
	}
	mac := env("TERM_PROGRAM", "Apple_Terminal")
	for _, tc := range []struct{ got, want string }{
		{line("darwin", mac, only("/Applications/Ghostty.app", "/Applications/iTerm.app")),
			"Ghostty: open -na /Applications/Ghostty.app --args -e /bin/gotv-tui https://cstv.fragnet.net/a --relaunched"},
		{line("darwin", mac, only("/Users/u/Applications/kitty.app")),
			"kitty: open -na /Users/u/Applications/kitty.app --args /bin/gotv-tui https://cstv.fragnet.net/a --relaunched"},
		{line("darwin", mac, only("/Applications/WezTerm.app")),
			"WezTerm: open -na /Applications/WezTerm.app --args start -- /bin/gotv-tui https://cstv.fragnet.net/a --relaunched"},
		{line("darwin", mac, only("/Applications/iTerm.app")),
			`iTerm2: osascript -e tell application "iTerm" to create window with default profile command "'/bin/gotv-tui' 'https://cstv.fragnet.net/a' '--relaunched'"`},
		{line("windows", env(), only("wt.exe")), "Windows Terminal: wt.exe /bin/gotv-tui https://cstv.fragnet.net/a --relaunched"},
		// stays put: nothing better installed, already in a capable terminal, over ssh, opted out, or Linux
		{line("darwin", mac, only()), ""},
		{line("darwin", env("TERM_PROGRAM", "Alacritty"), only("/Applications/Ghostty.app")), ""},
		{line("darwin", env("TERM_PROGRAM", "Apple_Terminal", "SSH_TTY", "/dev/ttys1"), only("/Applications/Ghostty.app")), ""},
		{line("darwin", env("TERM_PROGRAM", "Apple_Terminal", "GOTV_TUI_RELAUNCH", "0"), only("/Applications/Ghostty.app")), ""},
		{line("windows", env("WT_SESSION", "x"), only("wt.exe")), ""},
		{line("linux", env(), only("wt.exe")), ""},
	} {
		if tc.got != tc.want {
			t.Errorf("got  %q\nwant %q", tc.got, tc.want)
		}
	}
}

func TestWindowsTerminalArgsEscaped(t *testing.T) {
	_, c := betterTerminal("windows", func(string) string { return "" }, func(string) bool { return true },
		[]string{"gotv-tui", "https://x/a;new-tab cmd"})
	if got := c.Args[2]; got != `https://x/a\;new-tab cmd` {
		t.Fatalf("wt argument %q: an unescaped ; starts a new wt command", got)
	}
}

func TestWindowsTerminalArgs(t *testing.T) {
	has := func(p string) bool { return p == "wt.exe" }
	_, c := betterTerminal("windows", func(string) string { return "" }, has, []string{`C:\gotv\gotv-tui.exe`, "replay", `C:\my recs\`, "a;b"})
	got := strings.Join(c.Args[1:], "|")
	if want := `C:\gotv\gotv-tui.exe|replay|C:\my recs|a\;b|--relaunched`; got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}
