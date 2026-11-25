package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// relaunchedFlag marks a child we started, so it never relaunches again.
const relaunchedFlag = "--relaunched"

// betterTerminal is how to reopen gotv-tui in a terminal that shows the map as an
// image, when it was opened (e.g. by a double-click) in the system default one,
// which can't: Terminal.app on macOS, the old console on Windows. nil = stay.
func betterTerminal(goos string, getenv func(string) string, has func(string) bool, cmd []string) (name string, c *exec.Cmd) {
	if getenv("GOTV_TUI_RELAUNCH") == "0" || getenv("GOTV_TUI_GRAPHICS") != "" ||
		getenv("SSH_CONNECTION") != "" || getenv("SSH_TTY") != "" {
		return "", nil
	}
	cmd = append(cmd, relaunchedFlag)
	switch goos {
	case "darwin":
		if getenv("TERM_PROGRAM") != "Apple_Terminal" {
			return "", nil
		}
		home := getenv("HOME")
		app := func(n string) string {
			for _, dir := range []string{"/Applications", filepath.Join(home, "Applications")} {
				if p := filepath.Join(dir, n+".app"); has(p) {
					return p
				}
			}
			return ""
		}
		if p := app("Ghostty"); p != "" {
			return "Ghostty", exec.Command("open", append([]string{"-na", p, "--args", "-e"}, cmd...)...)
		}
		if p := app("kitty"); p != "" {
			return "kitty", exec.Command("open", append([]string{"-na", p, "--args"}, cmd...)...)
		}
		if p := app("WezTerm"); p != "" {
			return "WezTerm", exec.Command("open", append([]string{"-na", p, "--args", "start", "--"}, cmd...)...)
		}
		if app("iTerm") != "" {
			q := make([]string, len(cmd))
			for i, a := range cmd {
				q[i] = "'" + strings.ReplaceAll(a, "'", `'\''`) + "'"
			}
			line := strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(strings.Join(q, " "))
			return "iTerm2", exec.Command("osascript", "-e", `tell application "iTerm" to create window with default profile command "`+line+`"`)
		}
	case "windows":
		if getenv("WT_SESSION") != "" || !has("wt.exe") { // already in Windows Terminal, or none
			return "", nil
		}
		esc := make([]string, len(cmd))
		for i, a := range cmd { // wt.exe splits its command line on ";" even inside an argument
			esc[i] = strings.ReplaceAll(a, ";", `\;`)
		}
		return "Windows Terminal", exec.Command("wt.exe", esc...)
	}
	return "", nil
}

// relaunch reopens gotv-tui in a better terminal; true if it did (the caller exits).
func relaunch(args []string) (string, bool) {
	exe, err := os.Executable()
	if err != nil {
		return "", false
	}
	has := func(p string) bool {
		if p == "wt.exe" {
			_, err := exec.LookPath(p)
			return err == nil
		}
		_, err := os.Stat(p)
		return err == nil
	}
	name, c := betterTerminal(runtime.GOOS, os.Getenv, has, append([]string{exe}, args...))
	if c == nil || c.Run() != nil {
		return "", false // couldn't start it: carry on here, with the text map
	}
	return name, true
}
