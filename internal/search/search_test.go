package search

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/lukaszraczylo/mcp-filepuff/internal/config"
)

// parseOutput must not attach one file's pending before-context to a match in a
// different file (cross-file leak guard in the JSON stream parser).
func TestParseOutputCrossFileContextNotLeaked(t *testing.T) {
	s := newTestSearcher(t)
	var buf bytes.Buffer
	// fileA emits a before-context with no following match in fileA; then fileB
	// has a match. fileA's pending context must NOT attach to fileB's match.
	buf.WriteString(`{"type":"context","data":{"path":{"text":"fileA.go"},"lines":{"text":"orphan a ctx\n"},"line_number":1}}` + "\n")
	buf.WriteString(`{"type":"match","data":{"path":{"text":"fileB.go"},"lines":{"text":"hit B\n"},"line_number":5,"submatches":[{"match":{"text":"hit"},"start":0,"end":3}]}}` + "\n")

	res, err := s.parseOutput(&buf, 0)
	if err != nil {
		t.Fatalf("parseOutput: %v", err)
	}
	if len(res.Results) != 1 {
		t.Fatalf("want 1 result, got %d", len(res.Results))
	}
	if res.Results[0].File != "fileB.go" {
		t.Errorf("result file = %q, want fileB.go", res.Results[0].File)
	}
	if len(res.Results[0].Context.Before) != 0 {
		t.Errorf("fileA context leaked into fileB match: %v", res.Results[0].Context.Before)
	}
}

// parseOutput must attach a same-file before-context to the following match.
func TestParseOutputSameFileBeforeContext(t *testing.T) {
	s := newTestSearcher(t)
	var buf bytes.Buffer
	buf.WriteString(`{"type":"context","data":{"path":{"text":"f.go"},"lines":{"text":"before line\n"},"line_number":4}}` + "\n")
	buf.WriteString(`{"type":"match","data":{"path":{"text":"f.go"},"lines":{"text":"the match\n"},"line_number":5,"submatches":[{"match":{"text":"match"},"start":4,"end":9}]}}` + "\n")

	res, err := s.parseOutput(&buf, 0)
	if err != nil {
		t.Fatalf("parseOutput: %v", err)
	}
	if len(res.Results) != 1 {
		t.Fatalf("want 1 result, got %d", len(res.Results))
	}
	got := res.Results[0].Context.Before
	if len(got) != 1 || got[0] != "before line" {
		t.Errorf("same-file before-context not attached: %v", got)
	}
}

// truncateLine must back off to a rune boundary so multibyte characters are
// never split into invalid UTF-8.
func TestTruncateLineRuneSafe(t *testing.T) {
	s := strings.Repeat("é", 200) // 2 bytes each → 400 bytes
	out := truncateLine(s, 50)
	if !utf8.ValidString(out) {
		t.Errorf("truncated output is not valid UTF-8: %q", out)
	}
	if !strings.HasSuffix(out, "...") {
		t.Errorf("expected ellipsis suffix, got %q", out)
	}
	if got := truncateLine("hi", 50); got != "hi" {
		t.Errorf("short string should be unchanged, got %q", got)
	}
}

func TestNew(t *testing.T) {
	cfg := config.Default()
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	searcher, err := New(cfg, logger)
	if err != nil {
		// ripgrep might not be installed
		if strings.Contains(err.Error(), "not found") {
			t.Skip("ripgrep not installed, skipping test")
		}
		t.Fatalf("New failed: %v", err)
	}

	if searcher == nil {
		t.Fatal("expected non-nil searcher")
	}
}

func TestBuildArgs(t *testing.T) {
	cfg := config.Default()
	cfg.WorkspaceRoot = "/test/workspace"
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	// Create searcher without checking for rg binary
	s := &Searcher{
		cfg:    cfg,
		logger: logger,
		rgPath: "/usr/bin/rg",
	}

	tests := []struct {
		name        string
		req         *Request
		expected    []string
		notExpected []string // T-06: verify absence of unexpected flags
	}{
		{
			name: "basic search",
			req: &Request{
				Pattern:      "test",
				ContextLines: 2,
				Regex:        true,
			},
			expected:    []string{"--json", "--context=2", "--", "test", "."},
			notExpected: []string{"--ignore-case", "--fixed-strings", "--max-total-count=0"},
		},
		{
			name: "ignore case",
			req: &Request{
				Pattern:    "test",
				IgnoreCase: true,
				Regex:      true,
			},
			expected:    []string{"--json", "--ignore-case", "--", "test", "."},
			notExpected: []string{"--fixed-strings"},
		},
		{
			name: "fixed strings",
			req: &Request{
				Pattern: "test",
				Regex:   false,
			},
			expected:    []string{"--json", "--fixed-strings", "--", "test", "."},
			notExpected: []string{"--ignore-case"},
		},
		{
			name: "with file types",
			req: &Request{
				Pattern:   "test",
				FileTypes: []string{"go", "ts"},
				Regex:     true,
			},
			expected:    []string{"--json", "--type", "go", "--type", "ts", "--", "test", "."},
			notExpected: []string{"--ignore-case", "--fixed-strings"},
		},
		{
			name: "with max results",
			req: &Request{
				Pattern:    "test",
				MaxResults: 10,
				Regex:      true,
			},
			expected:    []string{"--json", "--", "test", "."},
			notExpected: []string{"--ignore-case", "--fixed-strings", "--max-total-count=10", "--max-count=10"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := s.buildArgs(tt.req)

			// Check that all expected args are present
			for _, exp := range tt.expected {
				found := false
				for _, arg := range args {
					if arg == exp {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected arg %q not found in %v", exp, args)
				}
			}

			// T-06: Check that unexpected args are absent
			for _, notExp := range tt.notExpected {
				for _, arg := range args {
					if arg == notExp {
						t.Errorf("unexpected arg %q found in %v", notExp, args)
						break
					}
				}
			}
		})
	}
}

func TestFormatResults(t *testing.T) {
	cfg := config.Default()
	cfg.WorkspaceRoot = "/test/workspace"
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	s := &Searcher{
		cfg:    cfg,
		logger: logger,
		rgPath: "/usr/bin/rg",
	}

	tests := []struct {
		name     string
		results  *SearchResults
		contains []string
	}{
		{
			name: "empty results",
			results: &SearchResults{
				Results: []Result{},
			},
			contains: []string{"No matches found"},
		},
		{
			name: "single result",
			results: &SearchResults{
				Results: []Result{
					{
						File:      "test.go",
						Line:      10,
						Column:    5,
						MatchText: "func TestSomething()",
					},
				},
				Total: 1,
			},
			contains: []string{"test.go", "L10", "TestSomething"},
		},
		{
			name: "truncated results",
			results: &SearchResults{
				Results: []Result{
					{
						File:      "test.go",
						Line:      10,
						MatchText: "match",
					},
				},
				Truncated: true,
				Total:     100,
			},
			contains: []string{"truncated", "100"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := s.FormatResults(tt.results)

			for _, exp := range tt.contains {
				if !strings.Contains(output, exp) {
					t.Errorf("expected output to contain %q, got:\n%s", exp, output)
				}
			}
		})
	}
}

func TestParseOutput(t *testing.T) {
	cfg := config.Default()
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	s := &Searcher{
		cfg:    cfg,
		logger: logger,
		rgPath: "/usr/bin/rg",
	}

	// Sample ripgrep JSON output
	jsonOutput := `{"type":"begin","data":{"path":{"text":"test.go"}}}
{"type":"match","data":{"path":{"text":"test.go"},"lines":{"text":"func TestSomething() {\n"},"line_number":10,"absolute_offset":100,"submatches":[{"match":{"text":"Test"},"start":5,"end":9}]}}
{"type":"end","data":{"path":{"text":"test.go"},"stats":{"bytes_searched":1000}}}
{"type":"summary","data":{"elapsed_total":{"secs":0,"nanos":1000000},"stats":{"searches":1,"searches_with_match":1,"bytes_searched":1000,"bytes_printed":100,"matched_lines":1,"matches":1}}}
`
	buf := bytes.NewBufferString(jsonOutput)

	results, err := s.parseOutput(buf, 0)
	if err != nil {
		t.Fatalf("parseOutput failed: %v", err)
	}

	if len(results.Results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results.Results))
	}

	if results.Results[0].File != "test.go" {
		t.Errorf("expected file 'test.go', got %q", results.Results[0].File)
	}

	if results.Results[0].Line != 10 {
		t.Errorf("expected line 10, got %d", results.Results[0].Line)
	}

	if results.Results[0].Column != 6 { // 1-indexed
		t.Errorf("expected column 6, got %d", results.Results[0].Column)
	}
}

func TestTruncateLine(t *testing.T) {
	tests := []struct {
		input    string
		expected string
		maxLen   int
	}{
		{
			input:    "short",
			maxLen:   10,
			expected: "short",
		},
		{
			input:    "this is a very long line that should be truncated",
			maxLen:   20,
			expected: "this is a very lo...",
		},
		{
			input:    "exact",
			maxLen:   5,
			expected: "exact",
		},
	}

	for _, tt := range tests {
		result := truncateLine(tt.input, tt.maxLen)
		if result != tt.expected {
			t.Errorf("truncateLine(%q, %d) = %q, want %q", tt.input, tt.maxLen, result, tt.expected)
		}
	}
}

func TestSearchIntegration(t *testing.T) {
	// Create a temporary directory with test files
	tmpDir, err := os.MkdirTemp("", "mcp-filepuff-search-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })

	// Create test files
	testFile := filepath.Join(tmpDir, "test.go")
	content := `package main

func main() {
	println("Hello, World!")
}
`
	err = os.WriteFile(testFile, []byte(content), 0o600)
	if err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	cfg := config.Default()
	cfg.WorkspaceRoot = tmpDir
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	searcher, err := New(cfg, logger)
	if err != nil {
		t.Skip("ripgrep not installed, skipping integration test")
	}

	ctx := context.Background()
	req := &Request{
		Pattern:      "Hello",
		ContextLines: 1,
		Regex:        false,
	}

	results, err := searcher.Search(ctx, req)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(results.Results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results.Results))
	}

	if len(results.Results) > 0 && !strings.Contains(results.Results[0].MatchText, "Hello") {
		t.Errorf("expected match to contain 'Hello', got %q", results.Results[0].MatchText)
	}
}

func TestSearchEmptyPattern(t *testing.T) {
	cfg := config.Default()
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	s := &Searcher{
		cfg:    cfg,
		logger: logger,
		rgPath: "/usr/bin/rg",
	}

	ctx := context.Background()
	req := &Request{
		Pattern: "",
	}

	_, err := s.Search(ctx, req)
	if err == nil {
		t.Error("expected error for empty pattern")
	}
}
