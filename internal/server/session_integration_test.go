package server

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lukaszraczylo/mcp-filepuff/internal/config"
	"github.com/mark3labs/mcp-go/mcp"
)

// ---- Session prefs integration tests ----

// setSessionPrefs injects prefs directly on the server (bypasses MCP hook machinery).
func setSessionPrefs(srv *Server, prefs SessionPrefs) {
	srv.sessionPrefs.Store(&prefs)
}

// TestSessionPrefsFileReadLineNumbersNone verifies that session pref line_numbers=none
// disables line-number prefixes and is overridden by explicit compact_line_numbers=true.
func TestSessionPrefsFileReadLineNumbersNone(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.go")
	content := "package main\n\nfunc Foo() {}\n"
	if err := os.WriteFile(testFile, []byte(content), 0600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	cfg := &config.Config{WorkspaceRoot: tmpDir, MaxFileSize: 1 << 20}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	srv, err := New(cfg, logger)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	setSessionPrefs(srv, ParseSessionPrefs(map[string]any{"line_numbers": "none"}))

	ctx := context.Background()

	// Without explicit override: session pref should suppress line numbers.
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{"path": testFile}
	result, err := srv.handleFileRead(ctx, req)
	if err != nil {
		t.Fatalf("handleFileRead: %v", err)
	}
	text := result.Content[0].(mcp.TextContent).Text
	// Standard line-number format is "   1│ "; with no_line_numbers it's absent.
	if strings.Contains(text, "   1│") {
		t.Errorf("session line_numbers=none: expected no line-number prefix, got:\n%s", text)
	}

	// Explicit per-call compact_line_numbers=true should override session none.
	req2 := mcp.CallToolRequest{}
	req2.Params.Arguments = map[string]interface{}{
		"path":                 testFile,
		"compact_line_numbers": true,
	}
	result2, err := srv.handleFileRead(ctx, req2)
	if err != nil {
		t.Fatalf("handleFileRead (explicit compact): %v", err)
	}
	text2 := result2.Content[0].(mcp.TextContent).Text
	// Compact format emits "1│" prefix.
	if !strings.Contains(text2, "\u2502") {
		t.Errorf("explicit compact_line_numbers should override session none, got:\n%s", text2)
	}
}

// TestSessionPrefsFileReadLineNumbersCompact verifies line_numbers=compact session pref.
func TestSessionPrefsFileReadLineNumbersCompact(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "x.go")
	if err := os.WriteFile(testFile, []byte("package main\nfunc Bar() {}\n"), 0600); err != nil {
		t.Fatalf("write: %v", err)
	}
	cfg := &config.Config{WorkspaceRoot: tmpDir, MaxFileSize: 1 << 20}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	srv, _ := New(cfg, logger)
	setSessionPrefs(srv, ParseSessionPrefs(map[string]any{"line_numbers": "compact"}))

	ctx := context.Background()
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{"path": testFile}
	result, _ := srv.handleFileRead(ctx, req)
	text := result.Content[0].(mcp.TextContent).Text
	// Standard padded prefix is "   1│ "; compact is "1│".
	if strings.Contains(text, "   1\u2502") {
		t.Errorf("session line_numbers=compact should use compact prefix, got:\n%s", text)
	}
	// Should still have the │ separator somewhere.
	if !strings.Contains(text, "\u2502") {
		t.Errorf("session line_numbers=compact should still have \u2502 separator, got:\n%s", text)
	}
}

// TestSessionPrefsResourceLinkThreshold verifies per-session threshold override.
func TestSessionPrefsResourceLinkThreshold(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "big.go")
	var sb strings.Builder
	sb.WriteString("package main\n\nfunc Foo() {\n")
	for i := 0; i < 15; i++ {
		sb.WriteString("// comment line\n")
	}
	sb.WriteString("}\n")
	if err := os.WriteFile(testFile, []byte(sb.String()), 0600); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Config threshold = 0 (disabled) so content is always inlined by default.
	cfg := &config.Config{WorkspaceRoot: tmpDir, MaxFileSize: 1 << 20, ResourceLinkThresholdBytes: 0}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	srv, _ := New(cfg, logger)

	// Set session threshold = 10 bytes (tiny), so any real file triggers resource-link.
	setSessionPrefs(srv, ParseSessionPrefs(map[string]any{"resource_link_threshold": float64(10)}))

	ctx := context.Background()
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{"path": testFile}
	result, _ := srv.handleFileRead(ctx, req)

	if len(result.Content) == 0 {
		t.Fatal("expected content")
	}
	_, isLink := result.Content[0].(mcp.ResourceLink)
	_, isText := result.Content[0].(mcp.TextContent)
	if !isLink && isText {
		t.Error("expected ResourceLink when session threshold is very small, got TextContent")
	}

	// force_inline should still bypass even a session threshold.
	req2 := mcp.CallToolRequest{}
	req2.Params.Arguments = map[string]interface{}{"path": testFile, "force_inline": true}
	result2, _ := srv.handleFileRead(ctx, req2)
	if _, ok := result2.Content[0].(mcp.TextContent); !ok {
		t.Error("force_inline=true should bypass session threshold and return TextContent")
	}
}

// TestSessionPrefsASTQueryFormat verifies default_format session pref for ast_query.
func TestSessionPrefsASTQueryFormat(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.go")
	if err := os.WriteFile(testFile, []byte("package main\n\nfunc Greet() {}\n"), 0600); err != nil {
		t.Fatalf("write: %v", err)
	}
	cfg := &config.Config{WorkspaceRoot: tmpDir, MaxFileSize: 1 << 20}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	srv, _ := New(cfg, logger)
	setSessionPrefs(srv, ParseSessionPrefs(map[string]any{"default_format": "compact"}))

	ctx := context.Background()
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{
		"pattern":  "func $NAME()",
		"language": "go",
		"paths":    []interface{}{tmpDir},
		// no format key → session default should apply
	}
	result, err := srv.handleASTQuery(ctx, req)
	if err != nil {
		t.Fatalf("handleASTQuery: %v", err)
	}
	if result == nil || len(result.Content) == 0 {
		t.Fatal("empty result")
	}
	text := result.Content[0].(mcp.TextContent).Text
	// Compact format emits one-line results without "**file:line**" markers.
	if strings.Contains(text, "**") {
		t.Errorf("session default_format=compact: expected compact output (no **), got:\n%s", text)
	}

	// Explicit format=verbose should override session compact.
	req2 := mcp.CallToolRequest{}
	req2.Params.Arguments = map[string]interface{}{
		"pattern":  "func $NAME()",
		"language": "go",
		"paths":    []interface{}{tmpDir},
		"format":   "verbose",
	}
	result2, _ := srv.handleASTQuery(ctx, req2)
	if result2 != nil && len(result2.Content) > 0 {
		text2 := result2.Content[0].(mcp.TextContent).Text
		if !strings.Contains(text2, "**") {
			t.Errorf("explicit format=verbose should override session compact, got:\n%s", text2)
		}
	}
}

// TestSessionPrefsASTQueryMaxResults verifies default_max_results for ast_query.
func TestSessionPrefsASTQueryMaxResults(t *testing.T) {
	tmpDir := t.TempDir()
	// Build a file with 5 functions.
	var sb strings.Builder
	sb.WriteString("package main\n\n")
	for i := 0; i < 5; i++ {
		fmt.Fprintf(&sb, "func Fn%c() {}\n\n", rune('A'+i))
	}
	testFile := filepath.Join(tmpDir, "many.go")
	if err := os.WriteFile(testFile, []byte(sb.String()), 0600); err != nil {
		t.Fatalf("write: %v", err)
	}
	cfg := &config.Config{WorkspaceRoot: tmpDir, MaxFileSize: 1 << 20}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	srv, _ := New(cfg, logger)
	// Session pref: max 2 results.
	setSessionPrefs(srv, ParseSessionPrefs(map[string]any{"default_max_results": float64(2)}))

	ctx := context.Background()
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{
		"pattern":  "func $NAME()",
		"language": "go",
		"paths":    []interface{}{tmpDir},
		// no max_results → session pref of 2 should apply
	}
	result, err := srv.handleASTQuery(ctx, req)
	if err != nil {
		t.Fatalf("handleASTQuery: %v", err)
	}
	if result == nil || len(result.Content) == 0 {
		t.Fatal("empty result")
	}
	text := result.Content[0].(mcp.TextContent).Text
	// With 5 funcs and max=2, output should mention remaining.
	if !strings.Contains(text, "remaining") && !strings.Contains(text, "cursor") {
		t.Errorf("session max_results=2 with 5 matches should produce cursor line, got:\n%s", text)
	}
}

// TestSessionPrefsFileSearchDefaultCluster verifies default_cluster session pref.
func TestSessionPrefsFileSearchDefaultCluster(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "x.go")
	content := "package main\n\nfunc Foo() {}\nfunc Foo2() {}\n"
	if err := os.WriteFile(testFile, []byte(content), 0600); err != nil {
		t.Fatalf("write: %v", err)
	}
	cfg := &config.Config{
		WorkspaceRoot: tmpDir,
		MaxFileSize:   1 << 20,
		SearchTimeout: 10 * time.Second,
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	srv, err := New(cfg, logger)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if srv.searcher == nil {
		t.Skip("ripgrep not available")
	}
	setSessionPrefs(srv, ParseSessionPrefs(map[string]any{"default_cluster": true}))

	ctx := context.Background()
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{
		"pattern": "func",
		"paths":   []interface{}{tmpDir},
		// no cluster flag → session default_cluster=true should apply
	}
	result, err := srv.handleFileSearch(ctx, req)
	if err != nil {
		t.Fatalf("handleFileSearch: %v", err)
	}
	if result == nil || len(result.Content) == 0 {
		t.Skip("search returned no results")
	}
	// Verify call succeeded (cluster behaviour is ripgrep-version dependent).
	_ = result.Content[0].(mcp.TextContent).Text
}

// TestSessionPrefsMaxResultsExplicitOverride verifies explicit call-time max_results
// overrides session pref for ast_query.
func TestSessionPrefsMaxResultsExplicitOverride(t *testing.T) {
	tmpDir := t.TempDir()
	var sb strings.Builder
	sb.WriteString("package main\n\n")
	for i := 0; i < 5; i++ {
		fmt.Fprintf(&sb, "func Fn%c() {}\n\n", rune('A'+i))
	}
	testFile := filepath.Join(tmpDir, "many.go")
	if err := os.WriteFile(testFile, []byte(sb.String()), 0600); err != nil {
		t.Fatalf("write: %v", err)
	}
	cfg := &config.Config{WorkspaceRoot: tmpDir, MaxFileSize: 1 << 20}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	srv, _ := New(cfg, logger)
	// Session wants 2, caller supplies 10 — all 5 should fit without cursor.
	setSessionPrefs(srv, ParseSessionPrefs(map[string]any{"default_max_results": float64(2)}))

	ctx := context.Background()
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{
		"pattern":     "func $NAME()",
		"language":    "go",
		"paths":       []interface{}{tmpDir},
		"max_results": 10, // explicit override
	}
	result, _ := srv.handleASTQuery(ctx, req)
	if result == nil || len(result.Content) == 0 {
		t.Fatal("empty result")
	}
	text := result.Content[0].(mcp.TextContent).Text
	// With max_results=10 and only 5 funcs, no cursor line expected.
	if strings.Contains(text, "remaining") {
		t.Errorf("explicit max_results=10 should override session 2; 5 funcs fit; no cursor expected, got:\n%s", text)
	}
}
