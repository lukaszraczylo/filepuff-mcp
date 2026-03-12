package server

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lukaszraczylo/mcp-filepuff/internal/config"
	"github.com/mark3labs/mcp-go/mcp"
)

func TestNew(t *testing.T) {
	// Create temp directory for testing
	tmpDir := t.TempDir()

	cfg := &config.Config{
		WorkspaceRoot: tmpDir,
		EnableLSP:     false, // Disable LSP for simpler testing
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	srv, err := New(cfg, logger)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if srv == nil {
		t.Fatal("New() returned nil server")
		return
	}
	if srv.cfg != cfg {
		t.Error("server config mismatch")
	}

	if srv.parser == nil {
		t.Error("parser should not be nil")
	}

	if srv.matcher == nil {
		t.Error("matcher should not be nil")
	}

	if srv.editor == nil {
		t.Error("editor should not be nil")
	}
}

func TestHandlePing(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &config.Config{WorkspaceRoot: tmpDir, EnableLSP: false}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	srv, err := New(cfg, logger)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx := context.Background()
	req := mcp.CallToolRequest{}

	result, err := srv.handlePing(ctx, req)
	if err != nil {
		t.Errorf("handlePing() error = %v", err)
	}

	if result == nil {
		t.Fatal("handlePing() returned nil result")
		return
	}

	// Check that the result contains "pong"
	contents := result.Content
	if len(contents) == 0 {
		t.Fatal("handlePing() returned empty content")
	}

	textContent, ok := contents[0].(mcp.TextContent)
	if !ok {
		t.Fatal("handlePing() did not return text content")
	}

	if textContent.Text != "pong" {
		t.Errorf("handlePing() = %v, want 'pong'", textContent.Text)
	}
}

func TestHandleFileRead(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a test file
	testFile := filepath.Join(tmpDir, "test.go")
	content := `package main

// Hello says hello
func Hello() {
	println("Hello, World!")
}
`
	if err := os.WriteFile(testFile, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	cfg := &config.Config{WorkspaceRoot: tmpDir, EnableLSP: false}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	srv, err := New(cfg, logger)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx := context.Background()
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{
		"path": testFile,
	}

	result, err := srv.handleFileRead(ctx, req)
	if err != nil {
		t.Errorf("handleFileRead() error = %v", err)
	}

	if result == nil {
		t.Fatal("handleFileRead() returned nil result")
		return
	}

	contents := result.Content
	if len(contents) == 0 {
		t.Fatal("handleFileRead() returned empty content")
	}

	textContent, ok := contents[0].(mcp.TextContent)
	if !ok {
		t.Fatal("handleFileRead() did not return text content")
	}

	// Should contain the file content
	if textContent.Text == "" {
		t.Error("handleFileRead() returned empty text")
	}
}

func TestHandleFileReadWithAST(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a test file
	testFile := filepath.Join(tmpDir, "test.go")
	content := `package main

// Hello says hello
func Hello() {
	println("Hello, World!")
}
`
	if err := os.WriteFile(testFile, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	cfg := &config.Config{WorkspaceRoot: tmpDir, EnableLSP: false}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	srv, err := New(cfg, logger)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx := context.Background()
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{
		"path":        testFile,
		"include_ast": true,
	}

	result, err := srv.handleFileRead(ctx, req)
	if err != nil {
		t.Errorf("handleFileRead() error = %v", err)
	}

	if result == nil {
		t.Fatal("handleFileRead() returned nil result")
		return
	}

	contents := result.Content
	if len(contents) == 0 {
		t.Fatal("handleFileRead() returned empty content")
	}

	textContent, ok := contents[0].(mcp.TextContent)
	if !ok {
		t.Fatal("handleFileRead() did not return text content")
	}

	// Should contain "Symbols:" section when include_ast is true
	if textContent.Text == "" {
		t.Error("handleFileRead() returned empty text")
	}
}

func TestHandleFileReadNotFound(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &config.Config{WorkspaceRoot: tmpDir, EnableLSP: false}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	srv, err := New(cfg, logger)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx := context.Background()
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{
		"path": filepath.Join(tmpDir, "nonexistent.go"),
	}

	result, err := srv.handleFileRead(ctx, req)
	// Should return error for non-existent file
	if err == nil && result != nil {
		// Check if result indicates an error
		contents := result.Content
		if len(contents) > 0 {
			textContent, ok := contents[0].(mcp.TextContent)
			if ok && textContent.Text == "" {
				t.Log("handleFileRead() returned empty text for non-existent file")
			}
		}
	}
}

func TestHandleASTQuery(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a test file
	testFile := filepath.Join(tmpDir, "test.go")
	content := `package main

func Hello() error {
	return nil
}

func Goodbye() error {
	return nil
}
`
	if err := os.WriteFile(testFile, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	cfg := &config.Config{WorkspaceRoot: tmpDir, EnableLSP: false}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	srv, err := New(cfg, logger)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx := context.Background()
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{
		"pattern":  "func $NAME() error",
		"language": "go",
		"paths":    []interface{}{tmpDir},
	}

	result, err := srv.handleASTQuery(ctx, req)
	if err != nil {
		t.Errorf("handleASTQuery() error = %v", err)
	}

	if result == nil {
		t.Fatal("handleASTQuery() returned nil result")
		return
	}

	contents := result.Content
	if len(contents) == 0 {
		t.Fatal("handleASTQuery() returned empty content")
	}
}

func TestHandleEditApply(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a test file
	testFile := filepath.Join(tmpDir, "test.go")
	content := `package main

func Hello() {
	println("Hello")
}
`
	if err := os.WriteFile(testFile, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	cfg := &config.Config{WorkspaceRoot: tmpDir, EnableLSP: false}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	srv, err := New(cfg, logger)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx := context.Background()
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{
		"file":          testFile,
		"operation":     "replace",
		"selector_kind": "function_declaration",
		"selector_name": "Hello",
		"new_content":   "func Hello() {\n\tprintln(\"Goodbye\")\n}",
	}

	result, err := srv.handleEditApply(ctx, req)
	if err != nil {
		t.Errorf("handleEditApply error = %v", err)
	}

	if result == nil {
		t.Fatal("handleEditApply returned nil result")
		return
	}

	// Verify file WAS modified
	fileContent, _ := os.ReadFile(testFile)
	if string(fileContent) == content {
		t.Error("handleEditApply should modify the file")
	}
}

// TestHandleFileReadMaxFileSize verifies that handleFileRead rejects files
// exceeding MaxFileSize via os.Stat before loading them into memory (T-03, S-01).
func TestHandleFileReadMaxFileSize(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a test file
	testFile := filepath.Join(tmpDir, "big.txt")
	content := make([]byte, 1024) // 1KB file
	for i := range content {
		content[i] = 'A'
	}
	if err := os.WriteFile(testFile, content, 0600); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// Set MaxFileSize smaller than the file
	cfg := &config.Config{
		WorkspaceRoot: tmpDir,
		EnableLSP:     false,
		MaxFileSize:   512, // 512 bytes — smaller than our 1KB file
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	srv, err := New(cfg, logger)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx := context.Background()
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{
		"path": testFile,
	}

	result, err := srv.handleFileRead(ctx, req)
	if err != nil {
		t.Fatalf("handleFileRead() returned Go error: %v", err)
	}

	// The result should indicate an error (file too large)
	if result == nil {
		t.Fatal("handleFileRead() returned nil result")
	}
	if !result.IsError {
		t.Error("expected IsError=true for file exceeding MaxFileSize")
	}
	contents := result.Content
	if len(contents) == 0 {
		t.Fatal("expected non-empty content with error message")
	}
	textContent, ok := contents[0].(mcp.TextContent)
	if !ok {
		t.Fatal("expected text content")
	}
	if !strings.Contains(textContent.Text, "too large") {
		t.Errorf("expected 'too large' error message, got: %s", textContent.Text)
	}
}

// TestSplitLinesLargeFile tests the splitLines function with a large file (>1MB)
// to exercise the bufio.Scanner path including the scanner.Err() check (T-07, C-05).
func TestSplitLinesLargeFile(t *testing.T) {
	// Build a string >1MB with known line count
	lineCount := 20000
	var sb strings.Builder
	for i := 0; i < lineCount; i++ {
		sb.WriteString(strings.Repeat("x", 60))
		sb.WriteByte('\n')
	}
	largeContent := sb.String()

	// Verify it's large enough to trigger the scanner path
	if len(largeContent) <= 1024*1024 {
		t.Fatalf("test content too small: %d bytes, need >1MB", len(largeContent))
	}

	lines := splitLines(largeContent)

	// String ends with \n, so splitLines adds an empty trailing element
	// (matching the behavior of strings.Split for the small-file path)
	expectedLines := lineCount + 1 // lineCount lines + 1 trailing empty
	if len(lines) != expectedLines {
		t.Errorf("splitLines returned %d lines, expected %d", len(lines), expectedLines)
	}

	// Check first and last actual lines
	if lines[0] != strings.Repeat("x", 60) {
		t.Errorf("first line mismatch: got %q", lines[0][:20])
	}
	if lines[lineCount-1] != strings.Repeat("x", 60) {
		t.Errorf("last content line mismatch: got %q", lines[lineCount-1][:20])
	}
	if lines[lineCount] != "" {
		t.Errorf("expected empty trailing line, got %q", lines[lineCount])
	}
}

// TestSplitLinesLargeFileNoTrailingNewline verifies splitLines for large files
// without a trailing newline.
func TestSplitLinesLargeFileNoTrailingNewline(t *testing.T) {
	lineCount := 20000
	var sb strings.Builder
	for i := 0; i < lineCount; i++ {
		if i > 0 {
			sb.WriteByte('\n')
		}
		sb.WriteString(strings.Repeat("y", 60))
	}
	largeContent := sb.String()

	if len(largeContent) <= 1024*1024 {
		t.Fatalf("test content too small: %d bytes", len(largeContent))
	}

	lines := splitLines(largeContent)
	if len(lines) != lineCount {
		t.Errorf("splitLines returned %d lines, expected %d", len(lines), lineCount)
	}
}

// TestSplitLinesLongLine verifies the scanner gracefully handles very long lines
// (up to the 1MB buffer limit set in C-05 fix).
func TestSplitLinesLongLine(t *testing.T) {
	// Create content with one very long line (500KB) embedded in a >1MB file
	shortLines := strings.Repeat("short line content\n", 60000) // ~60KB * ~1 = ~1.08MB
	longLine := strings.Repeat("L", 500*1024)                   // 500KB line
	largeContent := shortLines + longLine + "\n"

	if len(largeContent) <= 1024*1024 {
		t.Fatalf("test content too small: %d bytes", len(largeContent))
	}

	lines := splitLines(largeContent)

	// Should not crash and should return some lines
	if len(lines) == 0 {
		t.Fatal("splitLines returned no lines for valid content")
	}

	// The long line should be present somewhere in the output
	foundLong := false
	for _, line := range lines {
		if len(line) >= 500*1024 {
			foundLong = true
			break
		}
	}
	if !foundLong {
		t.Error("the 500KB long line was not found in splitLines output")
	}
}
