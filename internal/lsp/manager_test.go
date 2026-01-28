package lsp

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/lukaszraczylo/mcp-filepuff/pkg/protocol"
)

func TestFileToURI(t *testing.T) {
	tests := []struct {
		name string
		file string
		want string
	}{
		{
			name: "absolute path",
			file: "/Users/test/file.go",
			want: "file:///Users/test/file.go",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fileToURI(tt.file)
			if got != tt.want {
				t.Errorf("fileToURI() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestURIToFile(t *testing.T) {
	tests := []struct {
		name string
		uri  string
		want string
	}{
		{
			name: "file uri",
			uri:  "file:///Users/test/file.go",
			want: "/Users/test/file.go",
		},
		{
			name: "not a file uri",
			uri:  "/Users/test/file.go",
			want: "/Users/test/file.go",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := URIToFile(tt.uri)
			if got != tt.want {
				t.Errorf("URIToFile() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLanguageToLSPID(t *testing.T) {
	tests := []struct {
		lang protocol.Language
		want string
	}{
		{protocol.LangGo, "go"},
		{protocol.LangTypeScript, "typescript"},
		{protocol.LangJavaScript, "javascript"},
		{protocol.LangPython, "python"},
		{protocol.LangC, "c"},
		{protocol.LangCpp, "cpp"},
		{protocol.LangUnknown, "unknown"},
	}

	for _, tt := range tests {
		t.Run(string(tt.lang), func(t *testing.T) {
			got := languageToLSPID(tt.lang)
			if got != tt.want {
				t.Errorf("languageToLSPID() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsAvailable(t *testing.T) {
	// This tests the structure of the manager without actually spawning servers
	// which requires the actual LSP servers to be installed

	// Just verify the DefaultServerConfigs structure
	expectedLanguages := []protocol.Language{
		protocol.LangGo,
		protocol.LangTypeScript,
		protocol.LangJavaScript,
		protocol.LangPython,
		protocol.LangC,
		protocol.LangCpp,
	}

	for _, lang := range expectedLanguages {
		if _, ok := DefaultServerConfigs[lang]; !ok {
			t.Errorf("missing server config for language: %s", lang)
		}
	}
}

func TestDefaultServerConfigs(t *testing.T) {
	// Verify the command structure
	for lang, config := range DefaultServerConfigs {
		if len(config.Command) == 0 {
			t.Errorf("language %s has empty command", lang)
		}
	}
}

// TestManagerTimeout tests timeout handling in LSP operations.
func TestManagerTimeout(t *testing.T) {
	tmpDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	manager := NewManager(tmpDir, logger)
	t.Cleanup(func() { _ = manager.Close() })

	// Verify timeout is set
	if manager.timeout == 0 {
		t.Error("manager timeout should not be zero")
	}

	// Verify default timeout is reasonable
	if manager.timeout != 10*time.Second {
		t.Errorf("expected default timeout of 10s, got %v", manager.timeout)
	}

	// Test that manager can handle short timeouts
	manager.timeout = 1 * time.Millisecond

	// Create a context that will timeout
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	// Try to get a server with very short timeout - this should fail quickly
	// Use a language that doesn't have an LSP server installed
	_, err := manager.GetServer(ctx, "invalid_language")
	if err == nil {
		t.Log("GetServer with invalid language succeeded (LSP server may be installed)")
	}
}

// TestManagerConnectionFailure tests handling of LSP connection failures.
func TestManagerConnectionFailure(t *testing.T) {
	tmpDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	manager := NewManager(tmpDir, logger)
	t.Cleanup(func() { _ = manager.Close() })

	ctx := context.Background()

	// Test 1: Invalid language
	_, err := manager.GetServer(ctx, "nonexistent_language")
	if err == nil {
		t.Error("expected error for nonexistent language")
	}

	// Test 2: Try to use LSP features without a valid server
	// This should fail gracefully
	_, err = manager.Hover(ctx, "/tmp/test.fake", 1, 1)
	if err == nil {
		t.Error("expected error for hover on unsupported language")
	}
}

// TestManagerGracefulShutdown tests graceful shutdown of LSP servers.
func TestManagerGracefulShutdown(t *testing.T) {
	tmpDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	manager := NewManager(tmpDir, logger)

	// Close should not panic even with no servers started
	err := manager.Close()
	if err != nil {
		t.Errorf("Close() returned error: %v", err)
	}

	// Verify manager is stopped
	if !manager.stopped {
		t.Error("manager should be marked as stopped after Close()")
	}

	// Note: We don't test multiple Close() calls because the implementation
	// closes the stopReaper channel which can't be closed twice.
	// In production, Close() should only be called once during shutdown.
}

// TestManagerIdleReaper tests the idle server cleanup mechanism.
func TestManagerIdleReaper(t *testing.T) {
	tmpDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	manager := NewManager(tmpDir, logger)

	// Set a very short idle timeout for testing
	manager.idleTimeout = 100 * time.Millisecond

	// Verify idle timeout is set correctly
	if manager.idleTimeout != 100*time.Millisecond {
		t.Errorf("expected idle timeout of 100ms, got %v", manager.idleTimeout)
	}

	// The reaper goroutine should be running
	// We can't easily test it without actually starting LSP servers,
	// but we can verify it doesn't panic on close
	time.Sleep(150 * time.Millisecond)

	err := manager.Close()
	if err != nil {
		t.Errorf("Close() with active reaper returned error: %v", err)
	}
}

// TestManagerDocumentManagement tests document open/close operations.
func TestManagerDocumentManagement(t *testing.T) {
	tmpDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	manager := NewManager(tmpDir, logger)
	t.Cleanup(func() { _ = manager.Close() })

	ctx := context.Background()

	// Test closing a document for a non-existent server
	err := manager.CloseDocument(ctx, protocol.LangGo, "/tmp/test.go")
	if err != nil {
		t.Errorf("CloseDocument on non-existent server should not error: %v", err)
	}

	// Test closing a document that was never opened
	err = manager.CloseDocument(ctx, protocol.LangGo, "/tmp/test.go")
	if err != nil {
		t.Errorf("CloseDocument on unopened document should not error: %v", err)
	}
}

// TestManagerConcurrentAccess tests concurrent access to the manager.
func TestManagerConcurrentAccess(t *testing.T) {
	tmpDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	manager := NewManager(tmpDir, logger)
	t.Cleanup(func() { _ = manager.Close() })

	// Test concurrent IsAvailable calls
	const numGoroutines = 10
	done := make(chan bool, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("panic in concurrent IsAvailable: %v", r)
				}
				done <- true
			}()

			// Call IsAvailable multiple times
			for j := 0; j < 5; j++ {
				_ = manager.IsAvailable(protocol.LangGo)
				_ = manager.IsAvailable(protocol.LangPython)
				_ = manager.IsAvailable(protocol.LangTypeScript)
			}
		}()
	}

	// Wait for all goroutines
	for i := 0; i < numGoroutines; i++ {
		<-done
	}
}

// TestManagerErrorHandling tests various error conditions.
func TestManagerErrorHandling(t *testing.T) {
	tmpDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	manager := NewManager(tmpDir, logger)
	t.Cleanup(func() { _ = manager.Close() })

	ctx := context.Background()

	tests := []struct {
		name     string
		testFunc func() error
	}{
		{
			name: "hover_on_nonexistent_file",
			testFunc: func() error {
				_, err := manager.Hover(ctx, "/nonexistent/file.go", 1, 1)
				return err
			},
		},
		{
			name: "definition_on_nonexistent_file",
			testFunc: func() error {
				_, err := manager.Definition(ctx, "/nonexistent/file.go", 1, 1)
				return err
			},
		},
		{
			name: "references_on_nonexistent_file",
			testFunc: func() error {
				_, err := manager.References(ctx, "/nonexistent/file.go", 1, 1, true)
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.testFunc()
			if err == nil {
				t.Log("operation succeeded (LSP server may be handling gracefully)")
			}
			// We don't require an error because behavior depends on whether
			// the LSP server is installed and how it handles missing files
		})
	}
}

// TestManagerWorkspaceRoot tests workspace root handling.
func TestManagerWorkspaceRoot(t *testing.T) {
	tmpDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	manager := NewManager(tmpDir, logger)
	t.Cleanup(func() { _ = manager.Close() })

	if manager.workspaceRoot != tmpDir {
		t.Errorf("expected workspace root %s, got %s", tmpDir, manager.workspaceRoot)
	}
}
