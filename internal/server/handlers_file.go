// Package server implements the MCP server for file operations.
package server

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	xxhash "github.com/cespare/xxhash/v2"
	"github.com/lukaszraczylo/mcp-filepuff/internal/parser"
	"github.com/lukaszraczylo/mcp-filepuff/internal/search"
	"github.com/lukaszraczylo/mcp-filepuff/pkg/errors"
	"github.com/lukaszraczylo/mcp-filepuff/pkg/protocol"
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

	output := s.searcher.FormatResults(results)
	return mcp.NewToolResultText(output), nil
}

// handleFileRead handles the file_read tool.
func (s *Server) handleFileRead(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	select {
	case s.readSem <- struct{}{}:
		defer func() { <-s.readSem }()
	case <-ctx.Done():
		return mcp.NewToolResultError("request cancelled"), nil
	}

	// Batch mode: paths[] takes precedence over path
	if paths := request.GetStringSlice("paths", nil); len(paths) > 0 {
		var output strings.Builder
		for i, p := range paths {
			if i > 0 {
				output.WriteString("\n")
			}
			result, err := s.readOneFile(ctx, request, p)
			if err != nil {
				output.WriteString(fmt.Sprintf("--- %s ---\n[error: %s]\n", p, errors.SanitizeError(err)))
				continue
			}
			output.WriteString(fmt.Sprintf("--- %s ---\n%s", p, result))
		}
		return mcp.NewToolResultText(output.String()), nil
	}

	path := request.GetString("path", "")
	if path == "" {
		return mcp.NewToolResultError("path or paths is required"), nil
	}

	result, err := s.readOneFile(ctx, request, path)
	if err != nil {
		return mcp.NewToolResultError(errors.SanitizeError(err)), nil
	}
	return mcp.NewToolResultText(result), nil
}

// readOneFile reads a single file applying all formatting options from the request.
func (s *Server) readOneFile(ctx context.Context, request mcp.CallToolRequest, path string) (string, error) {
	if !s.cfg.IsPathAllowed(path) {
		return "", fmt.Errorf("path is outside workspace root")
	}

	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("file not found: %s", path)
		}
		if os.IsPermission(err) {
			return "", fmt.Errorf("permission denied: %s", path)
		}
		s.logger.Warn("file stat error", "path", path, "error", err)
		return "", fmt.Errorf("error accessing file")
	}
	if info.Size() > s.cfg.MaxFileSize {
		return "", fmt.Errorf("file too large (%d bytes, max %d)", info.Size(), s.cfg.MaxFileSize)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsPermission(err) {
			return "", fmt.Errorf("permission denied: %s", path)
		}
		s.logger.Warn("file read error", "path", path, "error", err)
		return "", fmt.Errorf("error reading file")
	}

	// Compute etag from content hash
	etag := fmt.Sprintf("%016x", xxhash.Sum64(content))

	// Short-circuit if caller has the current version
	if prev := request.GetString("previous_etag", ""); prev != "" && prev == etag {
		return fmt.Sprintf("[unchanged, etag: %s]\n", etag), nil
	}

	// Parse request options
	includeAST := request.GetBool("include_ast", false)
	symbolsOnly := request.GetBool("symbols_only", false)
	symbolName := request.GetString("symbol_name", "")
	noLineNumbers := request.GetBool("no_line_numbers", false)
	lineInterval := request.GetInt("line_number_interval", 1)
	collapseBlank := request.GetBool("collapse_blank_lines", false)
	maxLines := request.GetInt("max_lines", 0)

	if lineInterval == 0 {
		noLineNumbers = true
	}
	if symbolsOnly && !includeAST {
		return "", fmt.Errorf("symbols_only requires include_ast=true")
	}

	lines := splitLines(string(content))
	lineStart := request.GetInt("line_start", 1)
	lineEnd := request.GetInt("line_end", len(lines))

	// Symbol-based line range: find the symbol and use its exact bounds
	if symbolName != "" {
		symbolKind := protocol.SymbolKind(request.GetString("symbol_kind", ""))
		start, end, found := s.resolveSymbolLines(ctx, path, content, symbolName, symbolKind)
		if !found {
			return "", fmt.Errorf("symbol %q not found in %s", symbolName, path)
		}
		lineStart = start
		lineEnd = end
	}

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

	if includeAST {
		if summary := s.generateASTSummary(ctx, path, content); summary != "" {
			output.WriteString(summary)
			if !symbolsOnly {
				output.WriteString("\n---\n\n")
			}
		}
	}

	if symbolsOnly {
		output.WriteString(fmt.Sprintf("[etag: %s]\n", etag))
		return output.String(), nil
	}

	writeLines(&output, lines, lineStart, lineEnd, maxLines, noLineNumbers, lineInterval, collapseBlank)

	output.WriteString(fmt.Sprintf("[etag: %s]\n", etag))
	return output.String(), nil
}

// resolveSymbolLines parses the AST and returns the line range of the named symbol.
// symbolKind optionally filters by kind (empty = any).
func (s *Server) resolveSymbolLines(ctx context.Context, path string, content []byte, symbolName string, symbolKind protocol.SymbolKind) (startLine, endLine int, found bool) {
	result, err := s.parser.Parse(ctx, path, content)
	if err != nil {
		return
	}
	return parser.FindSymbolRange(result.Tree, content, path, symbolName, symbolKind)
}

// writeLines writes the selected line range into output, applying all formatting options.
func writeLines(output *strings.Builder, lines []string, lineStart, lineEnd, maxLines int, noLineNumbers bool, lineInterval int, collapseBlank bool) {
	effectiveEnd := lineEnd
	truncatedCount := 0
	if maxLines > 0 && (lineEnd-lineStart+1) > maxLines {
		effectiveEnd = lineStart + maxLines - 1
		truncatedCount = lineEnd - effectiveEnd
	}

	prevBlank := false
	for i := lineStart - 1; i < effectiveEnd && i < len(lines); i++ {
		line := lines[i]
		isBlank := strings.TrimSpace(line) == ""
		if collapseBlank && isBlank && prevBlank {
			continue
		}
		prevBlank = isBlank

		lineNum := i + 1
		switch {
		case noLineNumbers:
			output.WriteString(line + "\n")
		case lineInterval <= 1 || lineNum%lineInterval == 0 || i == lineStart-1 || i == effectiveEnd-1:
			fmt.Fprintf(output, "%4d│ %s\n", lineNum, line)
		default:
			fmt.Fprintf(output, "    │ %s\n", line)
		}
	}

	if truncatedCount > 0 {
		fmt.Fprintf(output, "\n[... %d more lines omitted. Use line_start/line_end or increase max_lines to see more]\n", truncatedCount)
	}
}

// splitLines splits a string into lines.
// For large files (> 1MB), uses bufio.Scanner which is more memory efficient.
// For smaller files, uses simple string split which is faster.
func splitLines(s string) []string {
	const largeSizeThreshold = 1024 * 1024 // 1MB

	if len(s) > largeSizeThreshold {
		scanner := bufio.NewScanner(strings.NewReader(s))
		scanner.Buffer(make([]byte, 0, bufio.MaxScanTokenSize), 1024*1024)
		var lines []string
		for scanner.Scan() {
			lines = append(lines, scanner.Text())
		}
		if err := scanner.Err(); err != nil {
			return strings.Split(s, "\n")
		}
		if len(s) > 0 && s[len(s)-1] == '\n' {
			lines = append(lines, "")
		}
		return lines
	}

	return strings.Split(s, "\n")
}
