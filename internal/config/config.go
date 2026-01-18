// Package config provides configuration management for the MCP file operations server.
package config

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	json "github.com/goccy/go-json"
)

// Config holds all configuration options for the MCP server.
type Config struct {
	Formatters       map[string]string `json:"formatters"`
	WorkspaceRoot    string            `json:"workspace_root"`
	LSPTimeout       time.Duration     `json:"lsp_timeout"`
	SearchTimeout    time.Duration     `json:"search_timeout"`
	MaxFileSize      int64             `json:"max_file_size"`
	MaxSearchResults int               `json:"max_search_results"`
	MaxEditSize      int64             `json:"max_edit_size"`
	EnableLSP        bool              `json:"enable_lsp"`
	FollowSymlinks   bool              `json:"follow_symlinks"`
	RespectGitignore bool              `json:"respect_gitignore"`
}

// Default values for configuration.
const (
	DefaultLSPTimeout       = 5 * time.Minute
	DefaultSearchTimeout    = 30 * time.Second
	DefaultMaxFileSize      = 10 * 1024 * 1024 // 10 MB
	DefaultMaxSearchResults = 1000
	DefaultMaxEditSize      = 100 * 1024 // 100 KB
)

// Default returns a Config with default values.
func Default() *Config {
	return &Config{
		WorkspaceRoot:    ".",
		LSPTimeout:       DefaultLSPTimeout,
		SearchTimeout:    DefaultSearchTimeout,
		MaxFileSize:      DefaultMaxFileSize,
		MaxSearchResults: DefaultMaxSearchResults,
		MaxEditSize:      DefaultMaxEditSize,
		EnableLSP:        true,
		Formatters:       make(map[string]string),
		FollowSymlinks:   true,
		RespectGitignore: true,
	}
}

// Load loads configuration from environment variables and optional config file.
// Priority: CLI flags > environment variables > config file > defaults.
func Load(workspaceRoot string) (*Config, error) {
	cfg := Default()

	// Set workspace root
	if workspaceRoot != "" {
		absPath, err := filepath.Abs(workspaceRoot)
		if err != nil {
			return nil, err
		}
		cfg.WorkspaceRoot = absPath
	} else if cwd, err := os.Getwd(); err == nil {
		cfg.WorkspaceRoot = cwd
	}

	// Try to load from config file in workspace root
	configPath := filepath.Join(cfg.WorkspaceRoot, ".mcp-filepuff.json")
	if data, err := os.ReadFile(configPath); err == nil {
		if err := json.Unmarshal(data, cfg); err != nil {
			return nil, err
		}
	}

	// Override from environment variables
	cfg.loadFromEnv()

	return cfg, nil
}

func (c *Config) loadFromEnv() {
	if v := os.Getenv("MCP_WORKSPACE_ROOT"); v != "" {
		if absPath, err := filepath.Abs(v); err == nil {
			c.WorkspaceRoot = absPath
		}
	}

	if v := os.Getenv("MCP_LSP_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			c.LSPTimeout = d
		}
	}

	if v := os.Getenv("MCP_SEARCH_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			c.SearchTimeout = d
		}
	}

	if v := os.Getenv("MCP_ENABLE_LSP"); v == "false" || v == "0" {
		c.EnableLSP = false
	}

	if v := os.Getenv("MCP_FOLLOW_SYMLINKS"); v == "false" || v == "0" {
		c.FollowSymlinks = false
	}

	if v := os.Getenv("MCP_RESPECT_GITIGNORE"); v == "false" || v == "0" {
		c.RespectGitignore = false
	}
}

// IsPathAllowed checks if a path is within the workspace root.
// It resolves symlinks to prevent path traversal attacks.
func (c *Config) IsPathAllowed(path string) bool {
	// Get absolute path of the target
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}

	// Get absolute path of workspace root
	absRoot, err := filepath.Abs(c.WorkspaceRoot)
	if err != nil {
		return false
	}

	// Always try to resolve workspace root symlinks for consistent comparison
	evalRoot, evalErr := filepath.EvalSymlinks(absRoot)
	if evalErr == nil {
		absRoot = evalRoot
	}

	// For the target path, try to resolve symlinks
	evalPath, evalErr := filepath.EvalSymlinks(absPath)
	if evalErr == nil {
		// File exists and was resolved
		absPath = evalPath
	} else {
		// File doesn't exist - resolve parent directories to match workspace root resolution
		// Walk up the tree until we find an existing directory
		dir := filepath.Dir(absPath)
		remaining := filepath.Base(absPath)

		for dir != "." && dir != "/" && dir != absPath {
			evalDir, evalErr := filepath.EvalSymlinks(dir)
			if evalErr == nil {
				// Found an existing directory, reconstruct the path
				absPath = filepath.Join(evalDir, remaining)
				break
			}
			// Move up one level
			newDir := filepath.Dir(dir)
			if newDir == dir {
				// Reached the root without finding an existing directory
				break
			}
			remaining = filepath.Join(filepath.Base(dir), remaining)
			dir = newDir
		}
	}

	// Compute relative path
	rel, err := filepath.Rel(absRoot, absPath)
	if err != nil {
		return false
	}

	// Check if the path is within workspace (doesn't start with ..)
	// This prevents both "../" attacks and symlink bypasses
	// Also reject empty relative path (which means it's the workspace root itself)
	return rel != "." && !strings.HasPrefix(rel, "..")
}
