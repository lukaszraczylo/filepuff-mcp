package search

import (
	"strings"
	"testing"

	"github.com/lukaszraczylo/mcp-filepuff/internal/config"
	"log/slog"
	"os"
)

func newTestSearcher(t *testing.T) *Searcher {
	t.Helper()
	cfg := &config.Config{
		WorkspaceRoot: t.TempDir(),
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	// Create a Searcher directly without requiring rg
	return &Searcher{cfg: cfg, logger: logger, rgPath: "rg"}
}

func makeSearchResults(lines []int) *SearchResults {
	results := make([]Result, len(lines))
	for i, l := range lines {
		results[i] = Result{
			File:      "/workspace/foo.go",
			Line:      l,
			MatchText: "match at line " + itoa2(l),
		}
	}
	return &SearchResults{Results: results}
}

// itoa2 simple int to string for test
func itoa2(n int) string {
	if n == 0 {
		return "0"
	}
	digits := []byte{}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

// TestFormatResultsVerboseBackwardCompat verifies that Verbose=true restores the v1 preamble.
func TestFormatResultsVerboseBackwardCompat(t *testing.T) {
	s := newTestSearcher(t)
	sr := makeSearchResults([]int{1, 5, 10})
	// With Verbose=true, the preamble is emitted (v1 behaviour).
	out := s.FormatResultsWithOptions(sr, FormatOptions{Verbose: true})
	if !strings.Contains(out, "Found 3 matches in 1 files") {
		t.Errorf("expected header with Verbose=true, got:\n%s", out)
	}
	if !strings.Contains(out, "L1│") {
		t.Errorf("expected L1 match line, got:\n%s", out)
	}
}

// TestFormatResultsDefaultNoPreamble verifies the v2 default has no preamble.
func TestFormatResultsDefaultNoPreamble(t *testing.T) {
	s := newTestSearcher(t)
	sr := makeSearchResults([]int{1, 5, 10})
	out := s.FormatResults(sr)
	if strings.Contains(out, "Found ") {
		t.Errorf("v2 default should NOT emit preamble, got:\n%s", out)
	}
	if !strings.Contains(out, "L1│") {
		t.Errorf("expected L1 match line, got:\n%s", out)
	}
}

func TestFormatResultsClusterSingleLine(t *testing.T) {
	s := newTestSearcher(t)
	// Non-consecutive lines → each should appear separately
	sr := makeSearchResults([]int{1, 5, 10})
	out := s.FormatResultsWithOptions(sr, FormatOptions{Cluster: true})
	if !strings.Contains(out, "L1│") {
		t.Errorf("expected L1 cluster, got:\n%s", out)
	}
	if !strings.Contains(out, "L5│") {
		t.Errorf("expected L5 cluster, got:\n%s", out)
	}
	// Should NOT have "   │" context lines in cluster mode
	if strings.Contains(out, "   │") {
		t.Errorf("cluster mode should not have context-line decoration, got:\n%s", out)
	}
}

func TestFormatResultsClusterConsecutive(t *testing.T) {
	s := newTestSearcher(t)
	// Lines 3,4,5 are consecutive → should be clustered as L3-5
	sr := makeSearchResults([]int{3, 4, 5, 10})
	out := s.FormatResultsWithOptions(sr, FormatOptions{Cluster: true})
	if !strings.Contains(out, "L3-5│") {
		t.Errorf("expected L3-5 cluster, got:\n%s", out)
	}
	if !strings.Contains(out, "L10│") {
		t.Errorf("expected L10 separate cluster, got:\n%s", out)
	}
}

func TestFormatResultsClusterAdjacentMerge(t *testing.T) {
	s := newTestSearcher(t)
	// Lines 7 and 8 differ by 1 → merge
	sr := makeSearchResults([]int{7, 8})
	out := s.FormatResultsWithOptions(sr, FormatOptions{Cluster: true})
	if !strings.Contains(out, "L7-8│") {
		t.Errorf("expected L7-8 cluster, got:\n%s", out)
	}
}

func TestFormatResultsCursorLine(t *testing.T) {
	s := newTestSearcher(t)
	sr := makeSearchResults([]int{1})
	cursorText := "[cursor: abc123, remaining: 5]"
	out := s.FormatResultsWithOptions(sr, FormatOptions{CursorLine: cursorText})
	if !strings.Contains(out, cursorText) {
		t.Errorf("expected cursor footer in output, got:\n%s", out)
	}
}

func TestFormatResultsNoMatchesVerbose(t *testing.T) {
	s := newTestSearcher(t)
	sr := &SearchResults{Results: nil}
	out := s.FormatResults(sr)
	if out != "No matches found." {
		t.Errorf("expected 'No matches found.', got: %s", out)
	}
}
