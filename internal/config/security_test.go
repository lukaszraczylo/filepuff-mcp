package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestIsPathAllowed_SymlinkSecurity tests the symlink security fix.
func TestIsPathAllowed_SymlinkSecurity(t *testing.T) {
	// Create a temporary workspace
	tmpDir := t.TempDir()
	workspace := filepath.Join(tmpDir, "workspace")
	outside := filepath.Join(tmpDir, "outside")

	if err := os.MkdirAll(workspace, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outside, 0700); err != nil {
		t.Fatal(err)
	}

	// Create a file outside the workspace
	outsideFile := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(outsideFile, []byte("secret data"), 0600); err != nil {
		t.Fatal(err)
	}

	cfg := &Config{
		WorkspaceRoot: workspace,
	}

	tests := []struct {
		setup    func() string
		name     string
		expected bool
	}{
		{
			name: "regular file inside workspace",
			setup: func() string {
				return filepath.Join(workspace, "file.txt")
			},
			expected: true,
		},
		{
			name: "file with parent directory traversal",
			setup: func() string {
				return filepath.Join(workspace, "../outside/secret.txt")
			},
			expected: false,
		},
		{
			name: "symlink pointing outside workspace",
			setup: func() string {
				symlink := filepath.Join(workspace, "link.txt")
				_ = os.Symlink(outsideFile, symlink)
				return symlink
			},
			expected: false,
		},
		{
			name: "symlink pointing inside workspace",
			setup: func() string {
				inside := filepath.Join(workspace, "inside.txt")
				_ = os.WriteFile(inside, []byte("ok"), 0600)
				symlink := filepath.Join(workspace, "link_inside.txt")
				_ = os.Symlink(inside, symlink)
				return symlink
			},
			expected: true,
		},
		{
			name: "dotfile inside workspace",
			setup: func() string {
				return filepath.Join(workspace, ".gitignore")
			},
			expected: true,
		},
		{
			name: "hidden directory inside workspace",
			setup: func() string {
				return filepath.Join(workspace, ".git/config")
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := tt.setup()
			result := cfg.IsPathAllowed(path)
			if result != tt.expected {
				t.Errorf("IsPathAllowed(%q) = %v, want %v", path, result, tt.expected)
			}
		})
	}
}

// TestIsPathAllowed_BasicCases tests basic path validation.
func TestIsPathAllowed_BasicCases(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &Config{
		WorkspaceRoot: tmpDir,
	}

	tests := []struct {
		name     string
		path     string
		expected bool
	}{
		{
			name:     "path inside workspace",
			path:     filepath.Join(tmpDir, "file.txt"),
			expected: true,
		},
		{
			name:     "path outside workspace",
			path:     "/etc/passwd",
			expected: false,
		},
		{
			name:     "parent directory reference",
			path:     filepath.Join(tmpDir, "../../../etc/passwd"),
			expected: false,
		},
		{
			name:     "workspace root itself",
			path:     tmpDir,
			expected: true, // Workspace root is a valid, allowed path (needed for ast_query)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cfg.IsPathAllowed(tt.path)
			if result != tt.expected {
				t.Errorf("IsPathAllowed(%q) = %v, want %v", tt.path, result, tt.expected)
			}
		})
	}
}
