package util

import "unicode/utf8"

// RuneBoundary returns the largest index j <= i such that s[:j] is valid UTF-8
// (j falls on a rune start, never inside a multibyte sequence). i is clamped to
// [0, len(s)]. Callers slicing s[:i] for truncation should slice s[:RuneBoundary(s, i)]
// instead so a multibyte character is never split into an invalid encoding.
func RuneBoundary(s string, i int) int {
	if i <= 0 {
		return 0
	}
	if i >= len(s) {
		return len(s)
	}
	for i > 0 && !utf8.RuneStart(s[i]) {
		i--
	}
	return i
}
