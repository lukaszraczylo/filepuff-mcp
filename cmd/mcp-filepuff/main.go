// Package main is the entry point for the MCP file operations server.
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"

	"github.com/lukaszraczylo/mcp-filepuff/internal/config"
	"github.com/lukaszraczylo/mcp-filepuff/internal/server"
)

func main() {
	// Parse command line flags
	var (
		workspaceRoot = flag.String("workspace", "", "Workspace root directory (default: current directory)")
		logLevel      = flag.String("log-level", "info", "Log level (debug, info, warn, error)")
		logFile       = flag.String("log-file", "", "Log file path (default: stderr)")
	)
	flag.Parse()

	// Set up logging
	logger := setupLogger(*logLevel, *logFile)

	// Load configuration
	cfg, err := config.Load(*workspaceRoot)
	if err != nil {
		logger.Error("failed to load configuration", "error", err)
		os.Exit(1)
	}

	logger.Info("configuration loaded",
		"workspace_root", cfg.WorkspaceRoot,
		"lsp_enabled", cfg.EnableLSP,
	)

	// Create and run server
	srv, err := server.New(cfg, logger)
	if err != nil {
		logger.Error("failed to create server", "error", err)
		os.Exit(1)
	}

	ctx := context.Background()
	if err := srv.Run(ctx); err != nil {
		logger.Error("server error", "error", err)
		os.Exit(1)
	}
}

func setupLogger(level string, logFile string) *slog.Logger {
	var logLevel slog.Level
	switch level {
	case "debug":
		logLevel = slog.LevelDebug
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{
		Level: logLevel,
	}

	var handler slog.Handler
	var logFileErr error
	if logFile != "" {
		f, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
		if err != nil {
			logFileErr = err
			// Fallback to stderr
			handler = slog.NewJSONHandler(os.Stderr, opts)
		} else {
			handler = slog.NewJSONHandler(f, opts)
		}
	} else {
		// Use stderr for MCP servers (stdout is for protocol messages)
		handler = slog.NewJSONHandler(os.Stderr, opts)
	}

	logger := slog.New(handler)

	// Warn if log file couldn't be opened
	if logFileErr != nil {
		logger.Warn("failed to open log file, using stderr",
			"file", logFile,
			"error", logFileErr,
		)
	}

	return logger
}
