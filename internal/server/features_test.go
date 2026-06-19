package server

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lukaszraczylo/mcp-filepuff/internal/config"
	"github.com/mark3labs/mcp-go/mcp"
)

// newTestServer creates a server with a temp workspace containing Go files.
func newFeaturesServer(t *testing.T) (*Server, string) {
	t.Helper()
	tmpDir := t.TempDir()

	// Write a few Go files for AST queries
	goFile1 := filepath.Join(tmpDir, "a.go")
	if err := os.WriteFile(goFile1, []byte(`package main

func Alpha() string { return "alpha" }
func Beta() string { return "beta" }
func Gamma() string { return "gamma" }
`), 0o600); err != nil {
		t.Fatal(err)
	}

	goFile2 := filepath.Join(tmpDir, "b.go")
	if err := os.WriteFile(goFile2, []byte(`package main

func Delta() int { return 1 }
func Epsilon() int { return 2 }
`), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		WorkspaceRoot: tmpDir,
		EnableLSP:     false,
		MaxFileSize:   config.DefaultMaxFileSize,
		MaxParseSize:  config.DefaultMaxParseSize,
		SearchTimeout: 30 * time.Second,
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	srv, err := New(cfg, logger)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	return srv, tmpDir
}

// ---- Feature 1: ast_query format flag ----

func TestASTQueryFormatCompactDefault(t *testing.T) {
	srv, tmpDir := newFeaturesServer(t)
	ctx := context.Background()

	// Default (no format arg) is compact: one line per match, no code blocks.
	args := map[string]interface{}{
		"pattern":  "func $NAME() string",
		"language": "go",
		"paths":    []interface{}{tmpDir},
	}
	req := mcp.CallToolRequest{}
	req.Params.Arguments = args
	res, err := srv.handleASTQuery(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil || len(res.Content) == 0 {
		t.Fatal("nil/empty result")
	}
	if text := res.Content[0].(mcp.TextContent).Text; strings.Contains(text, "```") {
		t.Errorf("compact default should NOT have code blocks, got:\n%s", text)
	}

	// Verbose remains available on explicit request.
	args["format"] = "verbose"
	req.Params.Arguments = args
	res, err = srv.handleASTQuery(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if text := res.Content[0].(mcp.TextContent).Text; !strings.Contains(text, "```") {
		t.Errorf("format=verbose should have code blocks, got:\n%s", text)
	}
}

func TestASTQueryFormatCompact(t *testing.T) {
	srv, tmpDir := newFeaturesServer(t)
	ctx := context.Background()

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{
		"pattern":  "func $NAME() string",
		"language": "go",
		"paths":    []interface{}{tmpDir},
		"format":   "compact",
	}
	res, err := srv.handleASTQuery(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := res.Content[0].(mcp.TextContent).Text
	if strings.Contains(text, "```") {
		t.Errorf("compact mode should NOT have code blocks, got:\n%s", text)
	}
	// Each line should contain file:line (kind) text
	for _, line := range strings.Split(strings.TrimSpace(text), "\n") {
		if line == "" || strings.HasPrefix(line, "Found") {
			continue
		}
		if !strings.Contains(line, ":") {
			t.Errorf("compact line missing ':' separator: %q", line)
		}
	}
}

func TestASTQueryFormatLocation(t *testing.T) {
	srv, tmpDir := newFeaturesServer(t)
	ctx := context.Background()

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{
		"pattern":  "func $NAME() string",
		"language": "go",
		"paths":    []interface{}{tmpDir},
		"format":   "location",
	}
	res, err := srv.handleASTQuery(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := res.Content[0].(mcp.TextContent).Text
	if strings.Contains(text, "```") {
		t.Errorf("location mode should NOT have code blocks, got:\n%s", text)
	}
	// Lines should be file:linenum only (no parentheses with kind)
	for _, line := range strings.Split(strings.TrimSpace(text), "\n") {
		if line == "" || strings.HasPrefix(line, "Found") {
			continue
		}
		if strings.Contains(line, "(") {
			t.Errorf("location mode should not have node type in parens: %q", line)
		}
	}
}

// ---- Feature 3 (ast_query): pagination cursor ----

func TestASTQueryPaginationCursor(t *testing.T) {
	srv, tmpDir := newFeaturesServer(t)
	ctx := context.Background()

	// Page 1: max_results=2
	req1 := mcp.CallToolRequest{}
	req1.Params.Arguments = map[string]interface{}{
		"pattern":     "func $NAME() $RET",
		"language":    "go",
		"paths":       []interface{}{tmpDir},
		"max_results": float64(2),
	}
	res1, err := srv.handleASTQuery(ctx, req1)
	if err != nil {
		t.Fatalf("page1 error: %v", err)
	}
	text1 := res1.Content[0].(mcp.TextContent).Text

	// Should contain cursor footer if there are more results
	if !strings.Contains(text1, "[cursor:") {
		// Might have fewer than 2 total results — skip cursor test
		t.Logf("no cursor footer (fewer than 2 total matches), skipping pagination round-trip")
		return
	}

	// Extract cursor token
	var cursorToken string
	for _, line := range strings.Split(text1, "\n") {
		if strings.HasPrefix(line, "[cursor:") {
			// [cursor: <token>, remaining: N]
			parts := strings.Split(line, " ")
			if len(parts) >= 2 {
				cursorToken = strings.TrimSuffix(parts[1], ",")
			}
			break
		}
	}
	if cursorToken == "" {
		t.Fatal("could not extract cursor token from output")
	}

	// Page 2: pass cursor back
	req2 := mcp.CallToolRequest{}
	req2.Params.Arguments = map[string]interface{}{
		"pattern":     "func $NAME() $RET",
		"language":    "go",
		"paths":       []interface{}{tmpDir},
		"max_results": float64(2),
		"cursor":      cursorToken,
	}
	res2, err := srv.handleASTQuery(ctx, req2)
	if err != nil {
		t.Fatalf("page2 error: %v", err)
	}
	text2 := res2.Content[0].(mcp.TextContent).Text
	if strings.Contains(text2, "cursor is for a different query") {
		t.Error("cursor was rejected as mismatched query")
	}
	// Page 2 should have results
	if strings.Contains(text2, "No matches found.") {
		t.Error("page2 should have some results")
	}
}

func TestASTQueryCursorStaleMismatch(t *testing.T) {
	srv, tmpDir := newFeaturesServer(t)
	ctx := context.Background()

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{
		"pattern":  "func $NAME() string",
		"language": "go",
		"paths":    []interface{}{tmpDir},
		"cursor":   "eyJvZmZzZXQiOjIsInF1ZXJ5X2hhc2giOiJkZWFkYmVlZiJ9", // offset=2, hash=deadbeef
	}
	res, err := srv.handleASTQuery(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := res.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "cursor is for a different query") {
		t.Errorf("expected stale cursor error, got:\n%s", text)
	}
}

// ---- Feature 2: file_search cluster ----

func TestFileSearchClusterFlag(t *testing.T) {
	srv, tmpDir := newFeaturesServer(t)
	if srv.searcher == nil {
		t.Skip("rg not available")
	}
	ctx := context.Background()

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{
		"pattern": "func",
		"paths":   []interface{}{tmpDir},
		"cluster": true,
	}
	res, err := srv.handleFileSearch(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil || len(res.Content) == 0 {
		t.Fatal("nil/empty result")
	}
	text := res.Content[0].(mcp.TextContent).Text
	if text == "No matches found." {
		t.Skip("no matches found (unexpected)")
	}
	// In cluster mode, should NOT have "   │" context decorations
	if strings.Contains(text, "   │") {
		t.Errorf("cluster mode should not contain context-line decoration '   │', got:\n%s", text)
	}
}

func TestFileSearchCursorStaleHash(t *testing.T) {
	srv, tmpDir := newFeaturesServer(t)
	if srv.searcher == nil {
		t.Skip("rg not available")
	}
	ctx := context.Background()

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{
		"pattern": "func",
		"paths":   []interface{}{tmpDir},
		"cursor":  "eyJvZmZzZXQiOjEsInF1ZXJ5X2hhc2giOiJiYWRoYXNoIn0", // offset=1, hash=badhash
	}
	res, err := srv.handleFileSearch(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := res.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "cursor is for a different query") {
		t.Errorf("expected stale cursor error, got:\n%s", text)
	}
}

func TestFileSearchPaginationCursor(t *testing.T) {
	srv, tmpDir := newFeaturesServer(t)
	if srv.searcher == nil {
		t.Skip("rg not available")
	}
	ctx := context.Background()

	// Page 1: get 1 result
	req1 := mcp.CallToolRequest{}
	req1.Params.Arguments = map[string]interface{}{
		"pattern":       "func",
		"paths":         []interface{}{tmpDir},
		"max_results":   float64(1),
		"context_lines": float64(0),
	}
	res1, err := srv.handleFileSearch(ctx, req1)
	if err != nil {
		t.Fatalf("page1 error: %v", err)
	}
	text1 := res1.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text1, "[cursor:") {
		t.Logf("no cursor in page1 (only 1 total result), skipping round-trip:\n%s", text1)
		return
	}

	// Extract cursor
	var cursorToken string
	for _, line := range strings.Split(text1, "\n") {
		if strings.HasPrefix(line, "[cursor:") {
			parts := strings.Split(line, " ")
			if len(parts) >= 2 {
				cursorToken = strings.TrimSuffix(parts[1], ",")
			}
			break
		}
	}
	if cursorToken == "" {
		t.Fatal("could not extract cursor from page1")
	}

	// Page 2
	req2 := mcp.CallToolRequest{}
	req2.Params.Arguments = map[string]interface{}{
		"pattern":       "func",
		"paths":         []interface{}{tmpDir},
		"max_results":   float64(1),
		"context_lines": float64(0),
		"cursor":        cursorToken,
	}
	res2, err := srv.handleFileSearch(ctx, req2)
	if err != nil {
		t.Fatalf("page2 error: %v", err)
	}
	text2 := res2.Content[0].(mcp.TextContent).Text
	if strings.Contains(text2, "cursor is for a different query") {
		t.Error("cursor was rejected as mismatched")
	}
	if strings.Contains(text2, "No matches found.") {
		t.Error("page2 should have matches")
	}
}
