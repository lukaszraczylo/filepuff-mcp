package util

import (
	"testing"
	"unicode/utf8"
)

func TestRuneBoundary(t *testing.T) {
	// "café" — 'é' is 2 bytes (0xC3 0xA9), occupying indices 3,4. len == 5.
	const cafe = "café"
	cases := []struct {
		name string
		s    string
		i    int
		want int
	}{
		{"negative clamps to zero", "abc", -1, 0},
		{"zero", "abc", 0, 0},
		{"beyond length clamps to len", "abc", 99, 3},
		{"ascii exact boundary", "abcdef", 4, 4},
		{"inside multibyte backs off", cafe, 4, 3}, // index 4 is the 2nd byte of 'é'
		{"on multibyte start stays", cafe, 3, 3},   // index 3 is the 1st byte of 'é'
		{"end of multibyte string", cafe, 5, 5},    // len == 5, clamped path
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := RuneBoundary(c.s, c.i)
			if got != c.want {
				t.Fatalf("RuneBoundary(%q, %d) = %d, want %d", c.s, c.i, got, c.want)
			}
			// Invariant: the prefix is always valid UTF-8.
			if !utf8.ValidString(c.s[:got]) {
				t.Fatalf("RuneBoundary(%q, %d) sliced invalid UTF-8: %q", c.s, c.i, c.s[:got])
			}
		})
	}
}
