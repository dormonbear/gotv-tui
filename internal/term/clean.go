package term

import "strings"

// Clean makes text from a relay safe to print: control characters (escape
// sequences could drive the terminal, e.g. write the clipboard) become spaces,
// it is one line, and at most max runes long.
func Clean(s string, max int) string {
	var b strings.Builder
	n := 0
	for _, r := range s {
		if n == max {
			break
		}
		if r < 0x20 || r == 0x7f || (r >= 0x80 && r < 0xa0) {
			r = ' '
		}
		b.WriteRune(r)
		n++
	}
	return strings.TrimSpace(b.String())
}
