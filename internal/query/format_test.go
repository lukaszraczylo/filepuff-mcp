package query

import (
	"strings"
	"testing"

	"github.com/lukaszraczylo/mcp-filepuff/pkg/protocol"
)

// makeResults builds N dummy MatchResults.
func makeResults(n int) []MatchResult {
	out := make([]MatchResult, n)
	for i := range out {
		out[i] = MatchResult{
			File: "file.go",
			Location: protocol.Location{
				Line:   i + 1,
				Column: 1,
			},
			Text: "func Foo() {}\nline2",
		}
	}
	return out
}

// TestFormatResultsVerboseDefault verifies verbose format includes code blocks (no preamble by default).
func TestFormatResultsVerboseDefault(t *testing.T) {
	results := makeResults(2)
	out := FormatResultsWithOptions(results, 0, "verbose", 0)
	// v2 default: no preamble
	if strings.Contains(out, "Found ") {
		t.Errorf("v2 default should NOT emit preamble, got:\n%s", out)
	}
	if !strings.Contains(out, "```") {
		t.Error("verbose mode should include code blocks")
	}
}

// TestFormatResultsVerbosePreamble verifies verbose=true restores the preamble.
func TestFormatResultsVerbosePreamble(t *testing.T) {
	results := makeResults(2)
	out := FormatResultsWithOptions(results, 0, "verbose", 0, true)
	if !strings.Contains(out, "Found 2 match(es):") {
		t.Errorf("expected preamble with verbose=true, got:\n%s", out)
	}
}

func TestFormatResultsCompact(t *testing.T) {
	results := makeResults(3)
	out := FormatResultsWithOptions(results, 0, "compact", 0)
	// v2 default: no preamble in compact mode
	// Should NOT have code blocks
	if strings.Contains(out, "```") {
		t.Error("compact mode should not have code blocks")
	}
	// Should have one line per match (beyond header)
	lines := strings.Split(strings.TrimSpace(out), "\n")
	// First two lines are "Found..." and blank, then 3 match lines
	matchLines := 0
	for _, l := range lines {
		if strings.Contains(l, "file.go:") {
			matchLines++
		}
	}
	if matchLines != 3 {
		t.Errorf("expected 3 match lines in compact mode, got %d\nOutput:\n%s", matchLines, out)
	}
}

func TestFormatResultsLocation(t *testing.T) {
	results := makeResults(3)
	out := FormatResultsWithOptions(results, 0, "location", 0)
	if strings.Contains(out, "```") {
		t.Error("location mode should not have code blocks")
	}
	// Should be file:line only
	for i := 1; i <= 3; i++ {
		expected := "file.go:" + itoa(i)
		if !strings.Contains(out, expected) {
			t.Errorf("location output missing %s", expected)
		}
	}
}

func TestFormatResultsMaxResults(t *testing.T) {
	results := makeResults(5)
	out := FormatResultsWithOptions(results, 3, "verbose", 0)
	// v2 default: no preamble — check that exactly 3 code blocks are present
	codeBlockCount := strings.Count(out, "```")
	if codeBlockCount != 6 { // 3 opening + 3 closing = 6
		t.Errorf("expected 3 matches (6 backtick markers), got %d in:\n%s", codeBlockCount, out)
	}
	if !strings.Contains(out, "[remaining: 2]") {
		t.Errorf("expected [remaining: 2] footer, got:\n%s", out)
	}
}

func TestFormatResultsOffset(t *testing.T) {
	results := makeResults(5)
	// Skip first 2, show all remaining
	out := FormatResultsWithOptions(results, 0, "verbose", 2)
	// offset=2 from 5 results → 3 results; check 3 code blocks
	codeBlockCount := strings.Count(out, "```")
	if codeBlockCount != 6 {
		t.Errorf("expected 3 matches (6 backtick markers) after offset=2, got %d in:\n%s", codeBlockCount, out)
	}
}

func TestFormatResultsOffsetBeyondEnd(t *testing.T) {
	results := makeResults(3)
	out := FormatResultsWithOptions(results, 0, "verbose", 10)
	if out != "No matches found." {
		t.Errorf("expected 'No matches found.' for offset beyond end, got: %s", out)
	}
}

func TestFormatResultsPaginationCursor(t *testing.T) {
	// Offset=2, maxResults=2, 5 total → show items 3&4, remaining=1
	results := makeResults(5)
	out := FormatResultsWithOptions(results, 2, "verbose", 2)
	// offset=2, maxResults=2 → items 3&4; check 2 code blocks
	codeBlockCount := strings.Count(out, "```")
	if codeBlockCount != 4 {
		t.Errorf("expected 2 matches (4 backtick markers), got %d in:\n%s", codeBlockCount, out)
	}
	if !strings.Contains(out, "[remaining: 1]") {
		t.Errorf("expected [remaining: 1], got:\n%s", out)
	}
}

func TestFormatResultsEmpty(t *testing.T) {
	out := FormatResultsWithOptions(nil, 0, "verbose", 0)
	if out != "No matches found." {
		t.Errorf("expected 'No matches found.', got: %s", out)
	}
}

func TestFormatResultsBackwardCompat(t *testing.T) {
	// FormatResults wrapper should produce same output as FormatResultsWithOptions with verbose=false (default).
	results := makeResults(2)
	a := FormatResults(results, 0)
	b := FormatResultsWithOptions(results, 0, "verbose", 0)
	if a != b {
		t.Error("FormatResults and FormatResultsWithOptions(verbose,0) should be identical")
	}
	// Both should have no preamble.
	if strings.Contains(a, "Found ") {
		t.Error("FormatResults should not emit preamble by default")
	}
}

func TestFirstLineOf(t *testing.T) {
	cases := []struct {
		input  string
		maxLen int
		want   string
	}{
		{"hello world", 20, "hello world"},
		{"line1\nline2", 20, "line1"},
		{"\n\nfoo", 20, "foo"},
		{"abcdefghij", 5, "abcd…"},
	}
	for _, c := range cases {
		got := firstLineOf(c.input, c.maxLen)
		if got != c.want {
			t.Errorf("firstLineOf(%q, %d) = %q, want %q", c.input, c.maxLen, got, c.want)
		}
	}
}

func itoa(n int) string {
	if n < 10 {
		return string(rune('0' + n))
	}
	return strings.TrimRight(strings.TrimRight(
		func() string {
			buf := make([]byte, 20)
			pos := 20
			for n > 0 {
				pos--
				buf[pos] = byte('0' + n%10)
				n /= 10
			}
			return string(buf[pos:])
		}(), ""), "")
}
