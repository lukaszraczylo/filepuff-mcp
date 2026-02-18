// Package server implements the MCP server for file operations.
package server

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/lukaszraczylo/mcp-filepuff/internal/search"
	"github.com/lukaszraczylo/mcp-filepuff/pkg/errors"
	"github.com/mark3labs/mcp-go/mcp"
)

// handleFileSearch handles the file_search tool.
func (s *Server) handleFileSearch(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	start := time.Now()
	defer func() {
		s.logger.Debug("file_search completed",
			"duration_ms", time.Since(start).Milliseconds(),
		)
	}()

	if s.searcher == nil {
		return mcp.NewToolResultError("ripgrep (rg) is not available. Please install it: https://github.com/BurntSushi/ripgrep#installation"), nil
	}

	// Parse request arguments using SDK helpers
	pattern, err := request.RequireString("pattern")
	if err != nil {
		return mcp.NewToolResultError("pattern is required"), nil
	}

	req := &search.Request{
		Pattern:      pattern,
		Paths:        request.GetStringSlice("paths", nil),
		FileTypes:    request.GetStringSlice("file_types", nil),
		IgnoreCase:   request.GetBool("ignore_case", false),
		Regex:        request.GetBool("regex", true),
		ContextLines: request.GetInt("context_lines", 2),
		MaxResults:   request.GetInt("max_results", 0),
	}

	// Execute search
	results, err := s.searcher.Search(ctx, req)
	if err != nil {
		s.logger.Warn("search error", "error", err)
		return mcp.NewToolResultError(fmt.Sprintf("search error: %s", errors.SanitizeError(err))), nil
	}

	s.logger.Info("search completed",
		"pattern", pattern,
		"results_count", len(results.Results),
		"truncated", results.Truncated,
	)

	// Format results
	output := s.searcher.FormatResults(results)
	return mcp.NewToolResultText(output), nil
}

// handleFileRead handles the file_read tool.
func (s *Server) handleFileRead(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Acquire semaphore to limit concurrent reads (prevents memory exhaustion)
	select {
	case s.readSem <- struct{}{}:
		defer func() { <-s.readSem }()
	case <-ctx.Done():
		return mcp.NewToolResultError("request cancelled"), nil
	}

	path, err := request.RequireString("path")
	if err != nil {
		return mcp.NewToolResultError("path is required"), nil
	}

	// Validate path is within workspace
	if !s.cfg.IsPathAllowed(path) {
		return mcp.NewToolResultError("path is outside workspace root"), nil
	}

	// Read file
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return mcp.NewToolResultError(fmt.Sprintf("file not found: %s", path)), nil
		}
		if os.IsPermission(err) {
			return mcp.NewToolResultError(fmt.Sprintf("permission denied: %s", path)), nil
		}
		s.logger.Warn("file read error", "path", path, "error", err)
		return mcp.NewToolResultError("error reading file"), nil
	}

	// Check file size
	if int64(len(content)) > s.cfg.MaxFileSize {
		return mcp.NewToolResultError(fmt.Sprintf("file too large (%d bytes, max %d)", len(content), s.cfg.MaxFileSize)), nil
	}

	// Handle line range
	lines := splitLines(string(content))
	lineStart := request.GetInt("line_start", 1)
	lineEnd := request.GetInt("line_end", len(lines))

	// Clamp to valid range
	if lineStart < 1 {
		lineStart = 1
	}
	if lineEnd > len(lines) {
		lineEnd = len(lines)
	}
	if lineStart > lineEnd {
		lineStart = lineEnd
	}

	var output strings.Builder

	// Include AST summary if requested
	includeAST := request.GetBool("include_ast", false)
	symbolsOnly := request.GetBool("symbols_only", false)
	maxLines := request.GetInt("max_lines", 0)

	// Validate symbols_only requires include_ast
	if symbolsOnly && !includeAST {
		return mcp.NewToolResultError("symbols_only requires include_ast=true"), nil
	}

	if includeAST {
		astSummary := s.generateASTSummary(ctx, path, content)
		if astSummary != "" {
			output.WriteString(astSummary)
			if !symbolsOnly {
				output.WriteString("\n---\n\n")
			}
		}
	}

	// Skip file content if symbols_only mode
	if !symbolsOnly {
		// Apply max_lines limit if specified
		effectiveEnd := lineEnd
		if maxLines > 0 && (lineEnd-lineStart+1) > maxLines {
			effectiveEnd = lineStart + maxLines - 1
			if effectiveEnd < lineEnd {
				// Add note that output was truncated
				defer func() {
					output.WriteString(fmt.Sprintf("\n[... %d more lines omitted for token efficiency. Use line_start/line_end or increase max_lines to see more]\n", lineEnd-effectiveEnd))
				}()
			}
		}

		// Extract requested lines
		for i := lineStart - 1; i < effectiveEnd && i < len(lines); i++ {
			output.WriteString(fmt.Sprintf("%4d│ %s\n", i+1, lines[i]))
		}
	}

	return mcp.NewToolResultText(output.String()), nil
}

// splitLines splits a string into lines.
// For large files (> 1MB), uses bufio.Scanner which is more memory efficient.
// For smaller files, uses simple string split which is faster.
func splitLines(s string) []string {
	const largeSizeThreshold = 1024 * 1024 // 1MB

	if len(s) > largeSizeThreshold {
		// Use scanner for large files
		scanner := bufio.NewScanner(strings.NewReader(s))
		var lines []string
		for scanner.Scan() {
			lines = append(lines, scanner.Text())
		}
		// Handle potential error and add empty line if string ended with newline
		if len(s) > 0 && s[len(s)-1] == '\n' {
			lines = append(lines, "")
		}
		return lines
	}

	// Use optimized stdlib implementation for smaller files (2-3x faster than manual loop)
	return strings.Split(s, "\n")
}
