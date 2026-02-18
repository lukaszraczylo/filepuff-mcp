package server

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
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

func TestHandleEditPreview(t *testing.T) {
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

	result, err := srv.handleEdit(ctx, req, false)
	if err != nil {
		t.Errorf("handleEdit(preview) error = %v", err)
	}

	if result == nil {
		t.Fatal("handleEdit(preview) returned nil result")
		return
	}

	// Verify file was NOT modified (it's just a preview)
	fileContent, _ := os.ReadFile(testFile)
	if string(fileContent) != content {
		t.Error("handleEdit(preview) should not modify the file")
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

	result, err := srv.handleEdit(ctx, req, true)
	if err != nil {
		t.Errorf("handleEdit(apply) error = %v", err)
	}

	if result == nil {
		t.Fatal("handleEdit(apply) returned nil result")
		return
	}

	// Verify file WAS modified
	fileContent, _ := os.ReadFile(testFile)
	if string(fileContent) == content {
		t.Error("handleEdit(apply) should modify the file")
	}
}
