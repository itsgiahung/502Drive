package tui

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Sanitize makes untrusted Drive names safe for a terminal. It mirrors
// internal/cli.Sanitize; the TUI cannot import the CLI without a cycle, so the
// rules are repeated here and tested against the same cases.
func Sanitize(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == utf8.RuneError, unicode.IsControl(r), r >= 0x202a && r <= 0x202e, r >= 0x2066 && r <= 0x2069, r == 0x200b, r == 0xfeff:
			if r < 0x100 {
				fmt.Fprintf(&b, "\\x%02x", r)
			} else {
				fmt.Fprintf(&b, "\\u%04x", r)
			}
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// cut truncates display text to n runes and marks the cut.
func cut(s string, n int) string {
	if n <= 3 {
		return s
	}
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n-1]) + "…"
}
