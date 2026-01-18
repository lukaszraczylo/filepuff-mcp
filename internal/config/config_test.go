package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefault(t *testing.T) {
	cfg := Default()

	if cfg.WorkspaceRoot != "." {
		t.Errorf("expected default workspace root '.', got %q", cfg.WorkspaceRoot)
	}
	if cfg.LSPTimeout != DefaultLSPTimeout {
		t.Errorf("expected default LSP timeout %v, got %v", DefaultLSPTimeout, cfg.LSPTimeout)
	}
	if cfg.SearchTimeout != DefaultSearchTimeout {
		t.Errorf("expected default search timeout %v, got %v", DefaultSearchTimeout, cfg.SearchTimeout)
	}
	if cfg.MaxFileSize != DefaultMaxFileSize {
		t.Errorf("expected default max file size %d, got %d", DefaultMaxFileSize, cfg.MaxFileSize)
	}
	if cfg.MaxSearchResults != DefaultMaxSearchResults {
		t.Errorf("expected default max search results %d, got %d", DefaultMaxSearchResults, cfg.MaxSearchResults)
	}
	if cfg.MaxEditSize != DefaultMaxEditSize {
		t.Errorf("expected default max edit size %d, got %d", DefaultMaxEditSize, cfg.MaxEditSize)
	}
	if !cfg.EnableLSP {
		t.Error("expected EnableLSP to be true by default")
	}
	if !cfg.FollowSymlinks {
		t.Error("expected FollowSymlinks to be true by default")
	}
	if !cfg.RespectGitignore {
		t.Error("expected RespectGitignore to be true by default")
	}
}

func TestLoad(t *testing.T) {
	// Create a temporary directory for workspace
	tmpDir, err := os.MkdirTemp("", "mcp-filepuff-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg, err := Load(tmpDir)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	absPath, _ := filepath.Abs(tmpDir)
	if cfg.WorkspaceRoot != absPath {
		t.Errorf("expected workspace root %q, got %q", absPath, cfg.WorkspaceRoot)
	}
}

func TestLoadFromEnv(t *testing.T) {
	// Save original env values
	origLSPTimeout := os.Getenv("MCP_LSP_TIMEOUT")
	origSearchTimeout := os.Getenv("MCP_SEARCH_TIMEOUT")
	origEnableLSP := os.Getenv("MCP_ENABLE_LSP")
	origFollowSymlinks := os.Getenv("MCP_FOLLOW_SYMLINKS")
	origRespectGitignore := os.Getenv("MCP_RESPECT_GITIGNORE")

	// Restore env after test
	t.Cleanup(func() {
		_ = os.Setenv("MCP_LSP_TIMEOUT", origLSPTimeout)
		_ = os.Setenv("MCP_SEARCH_TIMEOUT", origSearchTimeout)
		_ = os.Setenv("MCP_ENABLE_LSP", origEnableLSP)
		_ = os.Setenv("MCP_FOLLOW_SYMLINKS", origFollowSymlinks)
		_ = os.Setenv("MCP_RESPECT_GITIGNORE", origRespectGitignore)
	})

	// Set test env values
	_ = os.Setenv("MCP_LSP_TIMEOUT", "10m")
	_ = os.Setenv("MCP_SEARCH_TIMEOUT", "1m")
	_ = os.Setenv("MCP_ENABLE_LSP", "false")
	_ = os.Setenv("MCP_FOLLOW_SYMLINKS", "0")
	_ = os.Setenv("MCP_RESPECT_GITIGNORE", "false")

	cfg := Default()
	cfg.loadFromEnv()

	if cfg.LSPTimeout != 10*time.Minute {
		t.Errorf("expected LSP timeout 10m, got %v", cfg.LSPTimeout)
	}
	if cfg.SearchTimeout != 1*time.Minute {
		t.Errorf("expected search timeout 1m, got %v", cfg.SearchTimeout)
	}
	if cfg.EnableLSP {
		t.Error("expected EnableLSP to be false")
	}
	if cfg.FollowSymlinks {
		t.Error("expected FollowSymlinks to be false")
	}
	if cfg.RespectGitignore {
		t.Error("expected RespectGitignore to be false")
	}
}

func TestIsPathAllowed(t *testing.T) {
	// Create a temporary directory
	tmpDir, err := os.MkdirTemp("", "mcp-filepuff-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := Default()
	cfg.WorkspaceRoot = tmpDir

	tests := []struct {
		name    string
		path    string
		allowed bool
	}{
		{
			name:    "file in workspace",
			path:    filepath.Join(tmpDir, "test.go"),
			allowed: true,
		},
		{
			name:    "nested file in workspace",
			path:    filepath.Join(tmpDir, "subdir", "test.go"),
			allowed: true,
		},
		{
			name:    "path outside workspace",
			path:    "/etc/passwd",
			allowed: false,
		},
		{
			name:    "relative path traversal",
			path:    filepath.Join(tmpDir, "..", "outside.txt"),
			allowed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cfg.IsPathAllowed(tt.path)
			if result != tt.allowed {
				t.Errorf("IsPathAllowed(%q) = %v, want %v", tt.path, result, tt.allowed)
			}
		})
	}
}

func TestLoadWithConfigFile(t *testing.T) {
	// Create a temporary directory
	tmpDir, err := os.MkdirTemp("", "mcp-filepuff-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Write config file
	configPath := filepath.Join(tmpDir, ".mcp-filepuff.json")
	configContent := `{
		"enable_lsp": false,
		"follow_symlinks": false
	}`
	err = os.WriteFile(configPath, []byte(configContent), 0600)
	if err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	var cfg *Config
	cfg, err = Load(tmpDir)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.EnableLSP {
		t.Error("expected EnableLSP to be false from config file")
	}
	if cfg.FollowSymlinks {
		t.Error("expected FollowSymlinks to be false from config file")
	}
}
