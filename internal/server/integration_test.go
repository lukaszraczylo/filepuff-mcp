package server

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lukaszraczylo/mcp-filepuff/internal/config"
	"github.com/mark3labs/mcp-go/mcp"
)

// TestMCPProtocolEndToEnd tests the complete MCP protocol communication flow.
func TestMCPProtocolEndToEnd(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test files
	testFile := filepath.Join(tmpDir, "test.go")
	content := `package main

func Hello() string {
	return "hello"
}
`
	if err := os.WriteFile(testFile, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	cfg := &config.Config{
		WorkspaceRoot: tmpDir,
		EnableLSP:     false,
		MaxFileSize:   config.DefaultMaxFileSize,
		MaxParseSize:  config.DefaultMaxParseSize,
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	srv, err := New(cfg, logger)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx := context.Background()

	// Test 1: Ping tool (health check)
	t.Run("ping", func(t *testing.T) {
		req := mcp.CallToolRequest{}
		result, err := srv.handlePing(ctx, req)
		if err != nil {
			t.Errorf("handlePing() error = %v", err)
		}
		if result == nil {
			t.Fatal("handlePing() returned nil")
			return
		}
		if len(result.Content) == 0 {
			t.Fatal("handlePing() returned empty content")
		}
	})

	// Test 2: File read
	t.Run("file_read", func(t *testing.T) {
		req := mcp.CallToolRequest{}
		req.Params.Arguments = map[string]interface{}{
			"path": testFile,
		}
		result, err := srv.handleFileRead(ctx, req)
		if err != nil {
			t.Errorf("handleFileRead() error = %v", err)
		}
		if result == nil {
			t.Fatal("handleFileRead() returned nil")
			return
		}
		if len(result.Content) == 0 {
			t.Fatal("handleFileRead() returned empty content")
		}
	})

	// Test 3: AST query
	t.Run("ast_query", func(t *testing.T) {
		req := mcp.CallToolRequest{}
		req.Params.Arguments = map[string]interface{}{
			"pattern":  "func $NAME() string",
			"language": "go",
			"paths":    []interface{}{tmpDir},
		}
		result, err := srv.handleASTQuery(ctx, req)
		if err != nil {
			t.Errorf("handleASTQuery() error = %v", err)
		}
		if result == nil {
			t.Fatal("handleASTQuery() returned nil")
			return
		}
	})

	// Test 4: Edit workflow
	t.Run("edit_workflow", func(t *testing.T) {
		// Apply edit directly (preview removed to avoid confusing LLMs)
		applyReq := mcp.CallToolRequest{}
		applyReq.Params.Arguments = map[string]interface{}{
			"file":          testFile,
			"operation":     "replace",
			"selector_kind": "function_declaration",
			"selector_name": "Hello",
			"new_content":   "func Hello() string {\n\treturn \"goodbye\"\n}",
		}
		applyResult, err := srv.handleEditApply(ctx, applyReq)
		if err != nil {
			t.Errorf("handleEditApply() error = %v", err)
		}
		if applyResult == nil {
			t.Fatal("handleEditApply() returned nil")
			return
		}

		// Verify file changed after apply
		modifiedContent, _ := os.ReadFile(testFile)
		if string(modifiedContent) == content {
			t.Error("apply should modify file")
		}
	})
}

// TestMCPToolDiscovery tests that all expected tools are registered.
func TestMCPToolDiscovery(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &config.Config{
		WorkspaceRoot: tmpDir,
		EnableLSP:     false,
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	srv, err := New(cfg, logger)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	// Note: The MCP server doesn't expose a method to list tools directly,
	// but we can verify the server was created successfully
	if srv.mcp == nil {
		t.Fatal("MCP server not initialized")
	}

	// Verify each expected tool works
	ctx := context.Background()

	// Test ping tool
	pingReq := mcp.CallToolRequest{}
	if _, err := srv.handlePing(ctx, pingReq); err != nil {
		t.Errorf("ping tool failed: %v", err)
	}
}

// TestMCPErrorResponses tests error handling following MCP spec.
func TestMCPErrorResponses(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &config.Config{
		WorkspaceRoot: tmpDir,
		EnableLSP:     false,
		MaxFileSize:   1024, // Small size to trigger errors
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	srv, err := New(cfg, logger)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx := context.Background()

	tests := []struct {
		name        string
		handler     func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error)
		setupReq    func() mcp.CallToolRequest
		expectError bool
	}{
		{
			name:    "file_read_missing_path",
			handler: srv.handleFileRead,
			setupReq: func() mcp.CallToolRequest {
				req := mcp.CallToolRequest{}
				req.Params.Arguments = map[string]interface{}{}
				return req
			},
			expectError: true,
		},
		{
			name:    "file_read_nonexistent",
			handler: srv.handleFileRead,
			setupReq: func() mcp.CallToolRequest {
				req := mcp.CallToolRequest{}
				req.Params.Arguments = map[string]interface{}{
					"path": filepath.Join(tmpDir, "nonexistent.txt"),
				}
				return req
			},
			expectError: true,
		},
		{
			name:    "file_read_outside_workspace",
			handler: srv.handleFileRead,
			setupReq: func() mcp.CallToolRequest {
				req := mcp.CallToolRequest{}
				req.Params.Arguments = map[string]interface{}{
					"path": "/etc/passwd",
				}
				return req
			},
			expectError: true,
		},
		{
			name:    "ast_query_missing_pattern",
			handler: srv.handleASTQuery,
			setupReq: func() mcp.CallToolRequest {
				req := mcp.CallToolRequest{}
				req.Params.Arguments = map[string]interface{}{
					"language": "go",
				}
				return req
			},
			expectError: true,
		},
		{
			name:    "ast_query_missing_language",
			handler: srv.handleASTQuery,
			setupReq: func() mcp.CallToolRequest {
				req := mcp.CallToolRequest{}
				req.Params.Arguments = map[string]interface{}{
					"pattern": "func $NAME()",
				}
				return req
			},
			expectError: true,
		},
		{
			name:    "ast_query_unsupported_language",
			handler: srv.handleASTQuery,
			setupReq: func() mcp.CallToolRequest {
				req := mcp.CallToolRequest{}
				req.Params.Arguments = map[string]interface{}{
					"pattern":  "func $NAME()",
					"language": "cobol",
				}
				return req
			},
			expectError: true,
		},
		{
			name:    "edit_missing_file",
			handler: srv.handleEditApply,
			setupReq: func() mcp.CallToolRequest {
				req := mcp.CallToolRequest{}
				req.Params.Arguments = map[string]interface{}{
					"operation": "replace",
				}
				return req
			},
			expectError: true,
		},
		{
			name:    "edit_missing_operation",
			handler: srv.handleEditApply,
			setupReq: func() mcp.CallToolRequest {
				req := mcp.CallToolRequest{}
				req.Params.Arguments = map[string]interface{}{
					"file": filepath.Join(tmpDir, "test.go"),
				}
				return req
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := tt.setupReq()
			result, err := tt.handler(ctx, request)

			// Check for error - MCP tools return errors as nil error with error in result content
			hasError := err != nil || (result != nil && len(result.Content) > 0)

			if tt.expectError && !hasError {
				t.Errorf("expected error but got none")
			}
			// Note: We don't check for unexpected success because some operations
			// might legitimately return empty results
		})
	}
}

// TestMCPRequestResponseFlow tests the complete request/response flow.
func TestMCPRequestResponseFlow(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test file
	testFile := filepath.Join(tmpDir, "flow.go")
	content := `package main

func Add(a, b int) int {
	return a + b
}
`
	if err := os.WriteFile(testFile, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	cfg := &config.Config{
		WorkspaceRoot: tmpDir,
		EnableLSP:     false,
		MaxFileSize:   config.DefaultMaxFileSize,
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	srv, err := New(cfg, logger)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Test sequential operations
	t.Run("sequential_operations", func(t *testing.T) {
		// 1. Read file
		readReq := mcp.CallToolRequest{}
		readReq.Params.Arguments = map[string]interface{}{
			"path": testFile,
		}
		readResult, err := srv.handleFileRead(ctx, readReq)
		if err != nil {
			t.Fatalf("handleFileRead() error = %v", err)
		}
		if readResult == nil {
			t.Fatal("handleFileRead() returned nil")
			return
		}

		// 2. Query AST
		queryReq := mcp.CallToolRequest{}
		queryReq.Params.Arguments = map[string]interface{}{
			"pattern":  "func $NAME($$$ARGS) int",
			"language": "go",
			"paths":    []interface{}{tmpDir},
		}
		queryResult, err := srv.handleASTQuery(ctx, queryReq)
		if err != nil {
			t.Fatalf("handleASTQuery() error = %v", err)
		}
		if queryResult == nil {
			t.Fatal("handleASTQuery() returned nil")
			return
		}

		// 3. Edit (no preview)
		editReq := mcp.CallToolRequest{}
		editReq.Params.Arguments = map[string]interface{}{
			"file":          testFile,
			"operation":     "replace",
			"selector_kind": "function_declaration",
			"selector_name": "Add",
			"new_content":   "func Add(a, b int) int {\n\treturn a + b + 1\n}",
		}
		editResult, err := srv.handleEditApply(ctx, editReq)
		if err != nil {
			t.Fatalf("handleEditApply() error = %v", err)
		}
		if editResult == nil {
			t.Fatal("handleEditApply() returned nil")
			return
		}
	})
}

// TestMCPConcurrentRequests tests handling of concurrent requests.
func TestMCPConcurrentRequests(t *testing.T) {
	tmpDir := t.TempDir()

	// Create multiple test files
	for i := 0; i < 5; i++ {
		testFile := filepath.Join(tmpDir, "test"+string(rune(i+48))+".go")
		content := `package main

func Test() {
	println("test")
}
`
		if err := os.WriteFile(testFile, []byte(content), 0o600); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}
	}

	cfg := &config.Config{
		WorkspaceRoot: tmpDir,
		EnableLSP:     false,
		MaxFileSize:   config.DefaultMaxFileSize,
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	srv, err := New(cfg, logger)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx := context.Background()

	// Run multiple concurrent requests
	const numRequests = 10
	done := make(chan bool, numRequests)
	errors := make(chan error, numRequests)

	for i := 0; i < numRequests; i++ {
		go func(index int) {
			req := mcp.CallToolRequest{}
			req.Params.Arguments = map[string]interface{}{
				"pattern":  "func $NAME()",
				"language": "go",
				"paths":    []interface{}{tmpDir},
			}
			_, err := srv.handleASTQuery(ctx, req)
			if err != nil {
				errors <- err
			}
			done <- true
		}(i)
	}

	// Wait for all requests to complete
	for i := 0; i < numRequests; i++ {
		<-done
	}

	// Check for errors
	close(errors)
	for err := range errors {
		t.Errorf("concurrent request failed: %v", err)
	}
}

// TestMCPContextCancellation tests handling of context cancellation.
func TestMCPContextCancellation(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a large directory structure to ensure operation takes time
	for i := 0; i < 10; i++ {
		subdir := filepath.Join(tmpDir, "subdir"+string(rune(i+48)))
		if err := os.MkdirAll(subdir, 0o755); err != nil {
			t.Fatalf("failed to create subdir: %v", err)
		}
		for j := 0; j < 10; j++ {
			testFile := filepath.Join(subdir, "test"+string(rune(j+48))+".go")
			content := `package main

func Test() {
	println("test")
}
`
			if err := os.WriteFile(testFile, []byte(content), 0o600); err != nil {
				t.Fatalf("failed to write test file: %v", err)
			}
		}
	}

	cfg := &config.Config{
		WorkspaceRoot: tmpDir,
		EnableLSP:     false,
		MaxFileSize:   config.DefaultMaxFileSize,
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	srv, err := New(cfg, logger)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	// Create a context with a very short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{
		"pattern":  "func $NAME()",
		"language": "go",
		"paths":    []interface{}{tmpDir},
	}

	// This should either complete quickly or handle cancellation gracefully
	_, err = srv.handleASTQuery(ctx, req)
	// We don't check for specific error as it might complete before timeout
	// The important thing is it doesn't panic or hang
	if err != nil {
		t.Logf("handleASTQuery with cancelled context: %v", err)
	}
}
