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
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })

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
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })

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
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })

	// Write config file
	configPath := filepath.Join(tmpDir, ".mcp-filepuff.json")
	configContent := `{
		"enable_lsp": false,
		"follow_symlinks": false
	}`
	err = os.WriteFile(configPath, []byte(configContent), 0o600)
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

// A config file requesting excessive size/count limits must be clamped so it
// cannot force the server into unbounded memory use.
func TestLoadClampsExcessiveLimits(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".mcp-filepuff.json")
	configContent := `{
		"max_file_size": 999999999999,
		"max_parse_size": 999999999999,
		"max_edit_size": 999999999999,
		"max_search_results": 999999999
	}`
	if err := os.WriteFile(configPath, []byte(configContent), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(tmpDir)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	const maxSize int64 = 100 * 1024 * 1024
	if cfg.MaxFileSize > maxSize || cfg.MaxParseSize > maxSize || cfg.MaxEditSize > maxSize {
		t.Errorf("size limits not clamped: file=%d parse=%d edit=%d", cfg.MaxFileSize, cfg.MaxParseSize, cfg.MaxEditSize)
	}
	if cfg.MaxSearchResults > 100000 {
		t.Errorf("MaxSearchResults not clamped: %d", cfg.MaxSearchResults)
	}
}

// TestValidate tests the Validate method with various inputs.
func TestValidate(t *testing.T) {
	tests := []struct {
		name      string
		cfg       *Config
		expectErr bool
		errMsg    string
	}{
		{
			name:      "valid_config",
			cfg:       Default(),
			expectErr: false,
		},
		{
			name: "invalid_max_file_size",
			cfg: &Config{
				WorkspaceRoot: ".",
				MaxFileSize:   -1,
				MaxParseSize:  DefaultMaxParseSize,
				LSPTimeout:    DefaultLSPTimeout,
			},
			expectErr: true,
			errMsg:    "max_file_size must be positive",
		},
		{
			name: "zero_max_file_size",
			cfg: &Config{
				WorkspaceRoot: ".",
				MaxFileSize:   0,
				MaxParseSize:  DefaultMaxParseSize,
				LSPTimeout:    DefaultLSPTimeout,
			},
			expectErr: true,
			errMsg:    "max_file_size must be positive",
		},
		{
			name: "invalid_max_parse_size",
			cfg: &Config{
				WorkspaceRoot: ".",
				MaxFileSize:   DefaultMaxFileSize,
				MaxParseSize:  -1,
				LSPTimeout:    DefaultLSPTimeout,
			},
			expectErr: true,
			errMsg:    "max_parse_size must be positive",
		},
		{
			name: "zero_max_parse_size",
			cfg: &Config{
				WorkspaceRoot: ".",
				MaxFileSize:   DefaultMaxFileSize,
				MaxParseSize:  0,
				LSPTimeout:    DefaultLSPTimeout,
			},
			expectErr: true,
			errMsg:    "max_parse_size must be positive",
		},
		{
			name: "invalid_lsp_timeout",
			cfg: &Config{
				WorkspaceRoot: ".",
				MaxFileSize:   DefaultMaxFileSize,
				MaxParseSize:  DefaultMaxParseSize,
				LSPTimeout:    -1 * time.Second,
			},
			expectErr: true,
			errMsg:    "lsp_timeout must be positive",
		},
		{
			name: "zero_lsp_timeout",
			cfg: &Config{
				WorkspaceRoot: ".",
				MaxFileSize:   DefaultMaxFileSize,
				MaxParseSize:  DefaultMaxParseSize,
				LSPTimeout:    0,
			},
			expectErr: true,
			errMsg:    "lsp_timeout must be positive",
		},
		{
			name: "nonexistent_workspace",
			cfg: &Config{
				WorkspaceRoot: "/nonexistent/path/that/does/not/exist",
				MaxFileSize:   DefaultMaxFileSize,
				MaxParseSize:  DefaultMaxParseSize,
				LSPTimeout:    DefaultLSPTimeout,
			},
			expectErr: true,
			errMsg:    "workspace_root does not exist",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if tt.expectErr {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errMsg)
				} else if !contains(err.Error(), tt.errMsg) {
					t.Errorf("expected error containing %q, got %q", tt.errMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

// TestValidateWithFile tests validation with an actual file as workspace root.
func TestValidateWithFile(t *testing.T) {
	// Create a temporary file
	tmpFile, err := os.CreateTemp("", "test-file-*.txt")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	_ = tmpFile.Close()
	t.Cleanup(func() { _ = os.Remove(tmpFile.Name()) })

	cfg := &Config{
		WorkspaceRoot: tmpFile.Name(),
		MaxFileSize:   DefaultMaxFileSize,
		MaxParseSize:  DefaultMaxParseSize,
		LSPTimeout:    DefaultLSPTimeout,
	}

	err = cfg.Validate()
	if err == nil {
		t.Error("expected error when workspace_root is a file, got nil")
	} else if !contains(err.Error(), "is not a directory") {
		t.Errorf("expected error about not being a directory, got: %v", err)
	}
}

// TestLoadEnvironmentPrecedence tests environment variable precedence.
func TestLoadEnvironmentPrecedence(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "mcp-filepuff-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })

	// Write a config file with specific values
	configPath := filepath.Join(tmpDir, ".mcp-filepuff.json")
	configContent := `{
		"enable_lsp": false,
		"follow_symlinks": false,
		"lsp_timeout": 60000000000
	}`
	if err := os.WriteFile(configPath, []byte(configContent), 0o600); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	// Save and restore environment variables
	origEnableLSP := os.Getenv("MCP_ENABLE_LSP")
	origLSPTimeout := os.Getenv("MCP_LSP_TIMEOUT")
	t.Cleanup(func() {
		_ = os.Setenv("MCP_ENABLE_LSP", origEnableLSP)
		_ = os.Setenv("MCP_LSP_TIMEOUT", origLSPTimeout)
	})

	// Set environment variables that should override config file
	_ = os.Setenv("MCP_ENABLE_LSP", "false")
	_ = os.Setenv("MCP_LSP_TIMEOUT", "2m")

	cfg, err := Load(tmpDir)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Environment variable should override config file
	if cfg.LSPTimeout != 2*time.Minute {
		t.Errorf("expected LSP timeout 2m from env, got %v", cfg.LSPTimeout)
	}

	if cfg.EnableLSP {
		t.Error("expected EnableLSP to be false from env")
	}

	// Value from config file (not overridden by env)
	if cfg.FollowSymlinks {
		t.Error("expected FollowSymlinks to be false from config file")
	}
}

// TestIsPathAllowedEdgeCases tests edge cases in path validation.
func TestIsPathAllowedEdgeCases(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "mcp-filepuff-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })

	cfg := Default()
	cfg.WorkspaceRoot = tmpDir

	tests := []struct {
		name    string
		path    string
		allowed bool
		desc    string
	}{
		{
			name:    "workspace_root_itself",
			path:    tmpDir,
			allowed: true,
			desc:    "workspace root itself should be allowed",
		},
		{
			name:    "dot_relative",
			path:    ".",
			allowed: false,
			desc:    "current directory should not be allowed",
		},
		{
			name:    "empty_path",
			path:    "",
			allowed: false,
			desc:    "empty path should not be allowed",
		},
		{
			name:    "path_with_double_dots",
			path:    filepath.Join(tmpDir, "..", filepath.Base(tmpDir), "file.txt"),
			allowed: true,
			desc:    "path with .. that resolves back inside workspace should be allowed",
		},
		{
			name:    "deeply_nested_valid",
			path:    filepath.Join(tmpDir, "a", "b", "c", "file.txt"),
			allowed: true,
			desc:    "deeply nested path should be allowed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cfg.IsPathAllowed(tt.path)
			if result != tt.allowed {
				t.Errorf("%s: IsPathAllowed(%q) = %v, want %v", tt.desc, tt.path, result, tt.allowed)
			}
		})
	}
}

// TestIsPathAllowedWithSymlinks tests path validation with symbolic links.
func TestIsPathAllowedWithSymlinks(t *testing.T) {
	// Create temporary directories
	tmpDir, err := os.MkdirTemp("", "mcp-filepuff-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })

	realDir := filepath.Join(tmpDir, "real")
	if err := os.MkdirAll(realDir, 0o755); err != nil {
		t.Fatalf("failed to create real dir: %v", err)
	}

	// Create a symlink inside workspace
	symlinkPath := filepath.Join(tmpDir, "link")
	if err := os.Symlink(realDir, symlinkPath); err != nil {
		t.Skip("symlink creation not supported on this system")
	}

	cfg := Default()
	cfg.WorkspaceRoot = tmpDir

	// File accessed through symlink should be allowed
	fileViaSymlink := filepath.Join(symlinkPath, "test.txt")
	if !cfg.IsPathAllowed(fileViaSymlink) {
		t.Error("file accessed through symlink inside workspace should be allowed")
	}

	// Direct access should also work
	fileDirect := filepath.Join(realDir, "test.txt")
	if !cfg.IsPathAllowed(fileDirect) {
		t.Error("file accessed directly should be allowed")
	}
}

// TestLoadDefaultValues tests that default values are properly set.
func TestLoadDefaultValues(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "mcp-filepuff-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })

	// Clear any environment variables that might affect defaults
	origVars := []struct{ key, val string }{
		{"MCP_ENABLE_LSP", os.Getenv("MCP_ENABLE_LSP")},
		{"MCP_FOLLOW_SYMLINKS", os.Getenv("MCP_FOLLOW_SYMLINKS")},
		{"MCP_RESPECT_GITIGNORE", os.Getenv("MCP_RESPECT_GITIGNORE")},
	}
	t.Cleanup(func() {
		for _, v := range origVars {
			_ = os.Setenv(v.key, v.val)
		}
	})
	for _, v := range origVars {
		_ = os.Unsetenv(v.key)
	}

	cfg, err := Load(tmpDir)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Verify all default values
	if cfg.LSPTimeout != DefaultLSPTimeout {
		t.Errorf("expected LSPTimeout %v, got %v", DefaultLSPTimeout, cfg.LSPTimeout)
	}
	if cfg.SearchTimeout != DefaultSearchTimeout {
		t.Errorf("expected SearchTimeout %v, got %v", DefaultSearchTimeout, cfg.SearchTimeout)
	}
	if cfg.MaxFileSize != DefaultMaxFileSize {
		t.Errorf("expected MaxFileSize %d, got %d", DefaultMaxFileSize, cfg.MaxFileSize)
	}
	if cfg.MaxParseSize != DefaultMaxParseSize {
		t.Errorf("expected MaxParseSize %d, got %d", DefaultMaxParseSize, cfg.MaxParseSize)
	}
	if cfg.MaxSearchResults != DefaultMaxSearchResults {
		t.Errorf("expected MaxSearchResults %d, got %d", DefaultMaxSearchResults, cfg.MaxSearchResults)
	}
	if cfg.MaxEditSize != DefaultMaxEditSize {
		t.Errorf("expected MaxEditSize %d, got %d", DefaultMaxEditSize, cfg.MaxEditSize)
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
	if cfg.Formatters == nil {
		t.Error("expected Formatters map to be initialized")
	}
}

// TestConfigFileLoadingErrors tests error handling during config file loading.
func TestConfigFileLoadingErrors(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "mcp-filepuff-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })

	// Write invalid JSON
	configPath := filepath.Join(tmpDir, ".mcp-filepuff.json")
	invalidJSON := `{"enable_lsp": invalid_value}`
	if err := os.WriteFile(configPath, []byte(invalidJSON), 0o600); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	_, err = Load(tmpDir)
	if err == nil {
		t.Error("expected error when loading invalid JSON config file")
	}
}

// TestIsPathAllowed_SymlinkOutsideWorkspace verifies that symlinks pointing
// outside the workspace are rejected (T-01).
func TestIsPathAllowed_SymlinkOutsideWorkspace(t *testing.T) {
	// Create two separate temp dirs: one as workspace, one as outside target
	workspace, err := os.MkdirTemp("", "mcp-workspace-*")
	if err != nil {
		t.Fatalf("failed to create workspace dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(workspace) })

	outside, err := os.MkdirTemp("", "mcp-outside-*")
	if err != nil {
		t.Fatalf("failed to create outside dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(outside) })

	// Create a file outside the workspace
	outsideFile := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(outsideFile, []byte("secret"), 0o600); err != nil {
		t.Fatalf("failed to write outside file: %v", err)
	}

	// Create a symlink inside the workspace pointing outside
	symlinkPath := filepath.Join(workspace, "escape-link")
	if err := os.Symlink(outsideFile, symlinkPath); err != nil {
		t.Skip("symlink creation not supported on this system")
	}

	cfg := Default()
	cfg.WorkspaceRoot = workspace

	// The symlink resolves to a file outside workspace — must be rejected
	if cfg.IsPathAllowed(symlinkPath) {
		t.Error("symlink pointing outside workspace should NOT be allowed")
	}

	// Direct access to the outside file should also be rejected
	if cfg.IsPathAllowed(outsideFile) {
		t.Error("file outside workspace should NOT be allowed")
	}

	// File inside workspace should still be allowed
	insideFile := filepath.Join(workspace, "safe.txt")
	if !cfg.IsPathAllowed(insideFile) {
		t.Error("file inside workspace should be allowed")
	}

	// Workspace root itself should be allowed (C-08 fix)
	if !cfg.IsPathAllowed(workspace) {
		t.Error("workspace root itself should be allowed")
	}
}

// Helper function to check if a string contains a substring.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && containsHelper(s, substr)))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
