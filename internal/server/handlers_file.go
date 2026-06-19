// Package server implements the MCP server for file operations.
package server

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	xxhash "github.com/cespare/xxhash/v2"
	"github.com/lukaszraczylo/mcp-filepuff/internal/cursor"
	"github.com/lukaszraczylo/mcp-filepuff/internal/parser"
	"github.com/lukaszraczylo/mcp-filepuff/internal/search"
	"github.com/lukaszraczylo/mcp-filepuff/pkg/errors"
	"github.com/lukaszraczylo/mcp-filepuff/pkg/fuzzy"
	"github.com/lukaszraczylo/mcp-filepuff/pkg/protocol"
	"github.com/mark3labs/mcp-go/mcp"
)

// Symbol-suggestion tuning for not-found symbol_name reads.
const (
	// maxSymbolSuggestionDistance is the max fuzzy edit distance for a suggestion.
	maxSymbolSuggestionDistance = 3
	// maxSymbolSuggestions caps how many "did you mean" names are listed.
	maxSymbolSuggestions = 3
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

	paths := request.GetStringSlice("paths", nil)
	fileTypes := request.GetStringSlice("file_types", nil)
	ignoreCase := request.GetBool("ignore_case", false)
	regex := request.GetBool("regex", true)
	contextLines := request.GetInt("context_lines", 2)

	// Consult session prefs for max_results and cluster when not explicitly supplied.
	prefs := s.sessionPrefs.Load()
	var prefsMaxResults int
	var prefsCluster *bool
	if prefs != nil {
		prefsMaxResults = prefs.DefaultMaxResults
		prefsCluster = prefs.DefaultCluster
	}
	// Default to the configured cap (not 0/unbounded) so a broad pattern can't
	// flood memory and the context window; an explicit max_results still wins,
	// and the cursor footer lets the caller page through the rest.
	maxResults := effectiveInt(request, "max_results", prefsMaxResults, s.cfg.MaxSearchResults)
	cluster := effectiveBool(request, "cluster", prefsCluster, false)

	cursorStr := request.GetString("cursor", "")

	// Compute query hash for cursor validation.
	queryHash := cursor.HashParams(map[string]string{
		"pattern":       pattern,
		"paths":         strings.Join(paths, ","),
		"file_types":    strings.Join(fileTypes, ","),
		"ignore_case":   strconv.FormatBool(ignoreCase),
		"regex":         strconv.FormatBool(regex),
		"context_lines": strconv.Itoa(contextLines),
	})

	// Resolve cursor offset.
	offset := 0
	if cursorStr != "" {
		off, hash, decErr := cursor.Decode(cursorStr)
		if decErr != nil {
			return mcp.NewToolResultError(fmt.Sprintf("invalid cursor: %s", decErr)), nil
		}
		if hash != queryHash {
			return mcp.NewToolResultError("cursor is for a different query, re-run without cursor"), nil
		}
		offset = off
	}

	// When paginating with a cursor, fetch all results (no rg-level cap) so we
	// can apply the offset in-process. Without a cursor, let rg cap at maxResults.
	rgMaxResults := maxResults
	if offset > 0 {
		rgMaxResults = 0 // fetch all, apply cap after skipping
	}

	req := &search.Request{
		Pattern:       pattern,
		Paths:         paths,
		FileTypes:     fileTypes,
		IgnoreCase:    ignoreCase,
		Regex:         regex,
		ContextLines:  contextLines,
		MaxResults:    rgMaxResults,
		IncludeHidden: request.GetBool("include_hidden", false),
	}

	results, err := s.searcher.Search(ctx, req)
	if err != nil {
		s.logger.Warn("search error", "error", err)
		return mcp.NewToolResultError(fmt.Sprintf("search error: %s", errors.SanitizeError(err))), nil
	}

	// Apply cursor offset.
	if offset > 0 && offset < len(results.Results) {
		results.Results = results.Results[offset:]
		results.Truncated = false // will re-evaluate below
	} else if offset > 0 {
		results.Results = nil
		results.Truncated = false
	}

	// Apply in-process max_results cap and compute cursor footer.
	var cursorLine string
	switch {
	case maxResults > 0 && len(results.Results) > maxResults:
		// Cursor pages (offset>0) fetch all results, so cap here and emit a cursor.
		remaining := len(results.Results) - maxResults
		results.Results = results.Results[:maxResults]
		results.Truncated = true
		nextOffset := offset + maxResults
		nextCursor := cursor.Encode(nextOffset, queryHash)
		cursorLine = fmt.Sprintf("[cursor: %s, remaining: %d]", nextCursor, remaining)
	case offset == 0 && maxResults > 0 && results.Truncated:
		// First page: ripgrep capped us at exactly maxResults in parseOutput, but
		// more matches exist. Emit a cursor so the caller can page instead of being
		// told "truncated" with no way to continue. remaining derives from rg's
		// total match count (a submatch count — an upper-bound proxy for rows).
		nextCursor := cursor.Encode(maxResults, queryHash)
		remaining := max(results.Total-maxResults, 1)
		cursorLine = fmt.Sprintf("[cursor: %s, remaining: %d]", nextCursor, remaining)
	}

	s.logger.Info("search completed",
		"pattern", pattern,
		"results_count", len(results.Results),
		"truncated", results.Truncated,
	)

	verbose := request.GetBool("verbose", false)
	opts := search.FormatOptions{
		Cluster:    cluster,
		CursorLine: cursorLine,
		Verbose:    verbose,
	}
	output := s.searcher.FormatResultsWithOptions(results, opts)
	return mcp.NewToolResultText(output), nil
}

// effectiveInt returns the per-call value if the key is explicitly present in the
// request arguments, otherwise falls back to sessionDefault (if > 0), then builtIn.
func effectiveInt(request mcp.CallToolRequest, key string, sessionDefault, builtIn int) int {
	if _, explicit := request.GetArguments()[key]; explicit {
		return request.GetInt(key, builtIn)
	}
	if sessionDefault > 0 {
		return sessionDefault
	}
	return builtIn
}

// effectiveBool returns the per-call value if the key is explicitly present in the
// request arguments, otherwise falls back to sessionDefault (if non-nil), then builtIn.
func effectiveBool(request mcp.CallToolRequest, key string, sessionDefault *bool, builtIn bool) bool {
	if _, explicit := request.GetArguments()[key]; explicit {
		return request.GetBool(key, builtIn)
	}
	if sessionDefault != nil {
		return *sessionDefault
	}
	return builtIn
}

// handleFileRead handles the file_read tool.
func (s *Server) handleFileRead(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	select {
	case s.readSem <- struct{}{}:
		defer func() { <-s.readSem }()
	case <-ctx.Done():
		return mcp.NewToolResultError("request cancelled"), nil
	}

	// Batch mode: paths[] takes precedence over path.
	// NOTE: batch reads are always inlined — mixing dedup + resource_links is
	// too complex and the savings are unclear for multi-file calls.
	if paths := request.GetStringSlice("paths", nil); len(paths) > 0 {
		var output strings.Builder
		// Dedup: track etag -> first path that produced it.
		seenEtag := make(map[string]string) // etag -> first path
		for i, p := range paths {
			if i > 0 {
				output.WriteString("\n")
			}
			result, err := s.readOneFile(ctx, request, p)
			if err != nil {
				fmt.Fprintf(&output, "--- %s ---\n[error: %s]\n", p, errors.SanitizeError(err))
				continue
			}
			// Extract etag from result footer for dedup check.
			etag := extractEtag(result)
			if etag != "" {
				if firstPath, seen := seenEtag[etag]; seen {
					// Duplicate content: emit pointer instead of full content.
					fmt.Fprintf(&output, "--- %s ---\n[duplicate of %s, etag: %s]\n", p, firstPath, etag)
					continue
				}
				seenEtag[etag] = p
			}
			fmt.Fprintf(&output, "--- %s ---\n%s", p, result)
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

	// Resource-link threshold check: for single-file reads, if the result
	// exceeds the configured threshold, return a ResourceLink instead of
	// inlining the content. The client can fetch the resource on demand.
	// Bypassed when:
	//   - force_inline=true
	//   - max_inline_bytes is set and result fits within it
	//   - threshold is 0 (disabled)
	//   - result is already small (skeleton/symbols_only/line-range paths
	//     produce small output; threshold is on result bytes, not file bytes)
	// Determine resource-link threshold: session pref overrides cfg, per-call overrides session.
	threshold := s.cfg.ResourceLinkThresholdBytes
	if sp := s.sessionPrefs.Load(); sp != nil && sp.ResourceLinkThreshold > 0 {
		threshold = sp.ResourceLinkThreshold
	}
	forceInline := request.GetBool("force_inline", false)
	maxInlineBytes := request.GetInt("max_inline_bytes", 0)
	if maxInlineBytes > 0 {
		threshold = maxInlineBytes
	}

	if !forceInline && threshold > 0 && len(result) > threshold {
		etag := extractEtag(result)
		uri := buildReadResourceURI(path, etag)
		lineCount := strings.Count(result, "\n")
		desc := fmt.Sprintf("etag=%s, size=%d bytes, lines=%d", etag, len(result), lineCount)
		mimeType := detectMIMEType(path)
		link := mcp.NewResourceLink(uri, path, desc, mimeType)
		return &mcp.CallToolResult{
			Content: []mcp.Content{link},
		}, nil
	}

	return mcp.NewToolResultText(result), nil
}

// buildReadResourceURI constructs the filepuff://read URI for a file + etag pair.
func buildReadResourceURI(path, etag string) string {
	if etag == "" {
		return "filepuff://read/" + path
	}
	return "filepuff://read/" + path + "?etag=" + etag
}

// detectMIMEType returns a best-effort MIME type for the given file path.
func detectMIMEType(path string) string {
	ext := strings.ToLower(path)
	switch {
	case strings.HasSuffix(ext, ".go"):
		return "text/x-go"
	case strings.HasSuffix(ext, ".ts"), strings.HasSuffix(ext, ".tsx"):
		return "text/typescript"
	case strings.HasSuffix(ext, ".js"), strings.HasSuffix(ext, ".jsx"):
		return "text/javascript"
	case strings.HasSuffix(ext, ".py"):
		return "text/x-python"
	case strings.HasSuffix(ext, ".rs"):
		return "text/x-rust"
	case strings.HasSuffix(ext, ".md"):
		return "text/markdown"
	case strings.HasSuffix(ext, ".json"):
		return "application/json"
	case strings.HasSuffix(ext, ".yaml"), strings.HasSuffix(ext, ".yml"):
		return "text/yaml"
	case strings.HasSuffix(ext, ".toml"):
		return "text/toml"
	case strings.HasSuffix(ext, ".html"), strings.HasSuffix(ext, ".htm"):
		return "text/html"
	case strings.HasSuffix(ext, ".css"):
		return "text/css"
	case strings.HasSuffix(ext, ".sh"):
		return "text/x-sh"
	case strings.HasSuffix(ext, ".c"), strings.HasSuffix(ext, ".h"):
		return "text/x-c"
	case strings.HasSuffix(ext, ".cpp"), strings.HasSuffix(ext, ".cc"), strings.HasSuffix(ext, ".cxx"):
		return "text/x-c++"
	default:
		return "text/plain"
	}
}

// lineNumberOpts holds resolved line-numbering preferences for readOneFile.
type lineNumberOpts struct {
	noLineNumbers   bool
	compactLineNums bool
	lineInterval    int
}

// resolveLineNumberOpts resolves per-call vs session-pref line-number options.
func (s *Server) resolveLineNumberOpts(request mcp.CallToolRequest) lineNumberOpts {
	opts := lineNumberOpts{
		noLineNumbers:   request.GetBool("no_line_numbers", false),
		lineInterval:    request.GetInt("line_number_interval", 1),
		compactLineNums: request.GetBool("compact_line_numbers", false),
	}

	// Apply session line_numbers pref when no explicit per-call override was supplied.
	if sp := s.sessionPrefs.Load(); sp != nil && sp.LineNumbers != "" {
		_, hasNoLN := request.GetArguments()["no_line_numbers"]
		_, hasCompact := request.GetArguments()["compact_line_numbers"]
		_, hasInterval := request.GetArguments()["line_number_interval"]
		if !hasNoLN && !hasCompact && !hasInterval {
			switch sp.LineNumbers {
			case "none":
				opts.noLineNumbers = true
				opts.compactLineNums = false
			case "compact":
				opts.noLineNumbers = false
				opts.compactLineNums = true
			case "full":
				opts.noLineNumbers = false
				opts.compactLineNums = false
			}
		}
	}

	if opts.lineInterval == 0 {
		opts.noLineNumbers = true
	}
	return opts
}

// applyStrip applies strip flags to the selected line range and returns the
// possibly-rewritten lines, new bounds, and a stripped-footer annotation.
func applyStrip(lines []string, lineStart, lineEnd int, stripFlags []parser.StripFlag, path string) (newLines []string, newStart, newEnd int, footer string) {
	if len(stripFlags) == 0 {
		return lines, lineStart, lineEnd, ""
	}
	selectedContent := strings.Join(lines[lineStart-1:lineEnd], "\n")
	lang := protocol.DetectLanguage(path)
	stripped := parser.StripContent(selectedContent, stripFlags, lang)
	if len(stripped.Stripped) > 0 {
		names := make([]string, len(stripped.Stripped))
		for i, f := range stripped.Stripped {
			names[i] = string(f)
		}
		footer = "[stripped: " + strings.Join(names, ", ") + "]\n"
	}
	newLines = splitLines(stripped.Content)
	return newLines, 1, len(newLines), footer
}

// loadFileForRead performs workspace, stat, size, and read checks for a path.
func (s *Server) loadFileForRead(path string) ([]byte, error) {
	if !s.cfg.IsPathAllowed(path) {
		return nil, fmt.Errorf("path is outside workspace root")
	}

	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("file not found: %s", path)
		}
		if os.IsPermission(err) {
			return nil, fmt.Errorf("permission denied: %s", path)
		}
		s.logger.Warn("file stat error", "path", path, "error", err)
		return nil, fmt.Errorf("error accessing file")
	}
	if info.Size() > s.cfg.MaxFileSize {
		return nil, fmt.Errorf("file too large (%d bytes, max %d)", info.Size(), s.cfg.MaxFileSize)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsPermission(err) {
			return nil, fmt.Errorf("permission denied: %s", path)
		}
		s.logger.Warn("file read error", "path", path, "error", err)
		return nil, fmt.Errorf("error reading file")
	}
	return content, nil
}

// readOneFile reads a single file applying all formatting options from the request.
func (s *Server) readOneFile(ctx context.Context, request mcp.CallToolRequest, path string) (string, error) {
	content, err := s.loadFileForRead(path)
	if err != nil {
		return "", err
	}

	// Feature 3: short etag — 8 hex chars (32-bit).
	// Accept previous_etag by prefix match so old 16-char etags keep working.
	fullHash := fmt.Sprintf("%016x", xxhash.Sum64(content))
	etag := fullHash[:8]

	if prev := request.GetString("previous_etag", ""); prev != "" {
		// Match: exact 8-char match, old client sent full 16-char etag, or new client sent 8-char prefix of old.
		if prev == etag || strings.HasPrefix(fullHash, prev) || strings.HasPrefix(prev, etag) {
			return fmt.Sprintf("[unchanged, etag: %s]\n", etag), nil
		}
	}

	// Parse request options
	includeAST := request.GetBool("include_ast", false)
	symbolsOnly := request.GetBool("symbols_only", false)
	symbolName := request.GetString("symbol_name", "")
	collapseBlank := request.GetBool("collapse_blank_lines", false)
	maxLines := request.GetInt("max_lines", 0)

	lnOpts := s.resolveLineNumberOpts(request)

	// Feature 1: mode flag — "full" (default) | "skeleton" | "symbols_only".
	// symbols_only mode is an alias for include_ast+symbols_only.
	mode := request.GetString("mode", "full")
	if mode == "symbols_only" {
		symbolsOnly = true
		includeAST = true
	}

	// Feature 2: strip — remove selected content classes before line-numbering.
	stripRaw := request.GetStringSlice("strip", nil)
	var stripFlags []parser.StripFlag
	for _, sf := range stripRaw {
		stripFlags = append(stripFlags, parser.StripFlag(sf))
	}

	if symbolsOnly && !includeAST {
		return "", fmt.Errorf("symbols_only requires include_ast=true")
	}

	lines := splitLines(string(content))
	lineStart := request.GetInt("line_start", 1)
	lineEnd := request.GetInt("line_end", len(lines))

	// Symbol-based line range: find the symbol and use its exact bounds.
	if symbolName != "" {
		symbolKind := protocol.SymbolKind(request.GetString("symbol_kind", ""))
		candidates := s.resolveSymbolCandidates(ctx, path, content, symbolName, symbolKind)
		switch len(candidates) {
		case 0:
			return "", fmt.Errorf("symbol %q not found in %s%s", symbolName, path, s.nearestSymbolSuggestion(ctx, path, content, symbolName))
		case 1:
			lineStart = candidates[0].StartLine
			lineEnd = candidates[0].EndLine
		default:
			// Ambiguous: returning the first match risks reading the wrong
			// symbol. List candidates so the caller can disambiguate.
			return "", fmt.Errorf("symbol %q is ambiguous in %s (%d matches): %s — disambiguate with symbol_kind, or read a specific one with line_start/line_end",
				symbolName, path, len(candidates), formatSymbolCandidates(candidates))
		}
	}

	// Clamp to valid range.
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

	// Feature 1: skeleton mode — replace function bodies with { ... }.
	if mode == "skeleton" {
		skText, _, skErr := parser.SkeletonFile(ctx, s.parser, path, content)
		if skErr != nil {
			// Fall back to full mode on parse error.
			s.logger.Warn("skeleton mode failed, falling back to full", "path", path, "error", skErr)
		} else {
			output.WriteString(skText)
			fmt.Fprintf(&output, "[etag: %s]\n", etag)
			return output.String(), nil
		}
	}

	if includeAST {
		if summary := s.generateASTSummary(ctx, path, content); summary != "" {
			output.WriteString(summary)
			if !symbolsOnly {
				output.WriteString("\n---\n\n")
			}
		}
	}

	if symbolsOnly {
		fmt.Fprintf(&output, "[etag: %s]\n", etag)
		return output.String(), nil
	}

	// Feature 2: apply strip AFTER line-range selection, BEFORE line numbering.
	lines, lineStart, lineEnd, strippedFooter := applyStrip(lines, lineStart, lineEnd, stripFlags, path)

	writeLines(&output, lines, lineStart, lineEnd, maxLines, lnOpts.noLineNumbers, lnOpts.lineInterval, collapseBlank, lnOpts.compactLineNums)

	if strippedFooter != "" {
		output.WriteString(strippedFooter)
	}
	fmt.Fprintf(&output, "[etag: %s]\n", etag)
	return output.String(), nil
}

// resolveSymbolCandidates parses the AST and returns every symbol matching the
// name (and optional kind). Zero results means not found; more than one means an
// ambiguous lookup. Returns nil on parse failure (treated as not found).
func (s *Server) resolveSymbolCandidates(ctx context.Context, path string, content []byte, symbolName string, symbolKind protocol.SymbolKind) []parser.SymbolCandidate {
	result, err := s.parser.Parse(ctx, path, content)
	if err != nil {
		return nil
	}
	return parser.FindSymbolCandidates(result.Tree, content, path, symbolName, symbolKind)
}

// nearestSymbolSuggestion returns a " — did you mean: a, b, c?" hint listing the
// closest symbol names in the file to name, or "" if none are reasonably close.
// The re-parse is cheap: the tree is served from the parser cache.
func (s *Server) nearestSymbolSuggestion(ctx context.Context, path string, content []byte, name string) string {
	result, err := s.parser.Parse(ctx, path, content)
	if err != nil {
		return ""
	}
	syms := parser.ExtractSymbols(result.Tree, content, result.Language, path)
	if len(syms) == 0 {
		return ""
	}
	names := make([]string, 0, len(syms))
	seen := make(map[string]bool, len(syms))
	for _, sym := range syms {
		if !seen[sym.Name] {
			seen[sym.Name] = true
			names = append(names, sym.Name)
		}
	}
	matches := fuzzy.New(maxSymbolSuggestionDistance).Match(name, names)
	if len(matches) == 0 {
		return ""
	}
	top := matches
	if len(top) > maxSymbolSuggestions {
		top = top[:maxSymbolSuggestions]
	}
	suggestions := make([]string, len(top))
	for i, m := range top {
		suggestions[i] = m.Text
	}
	return " — did you mean: " + strings.Join(suggestions, ", ") + "?"
}

// formatSymbolCandidates renders ambiguous symbol matches as a compact,
// comma-separated list, e.g. "method_declaration L10-24, method_declaration L42-58".
func formatSymbolCandidates(candidates []parser.SymbolCandidate) string {
	parts := make([]string, len(candidates))
	for i, c := range candidates {
		parts[i] = fmt.Sprintf("%s L%d-%d", c.NodeType, c.StartLine, c.EndLine)
	}
	return strings.Join(parts, ", ")
}

// writeLines writes the selected line range into output, applying all formatting options.
// compactLineNums=true emits "12│" instead of "  12│ " (no padding, no trailing space).
func writeLines(output *strings.Builder, lines []string, lineStart, lineEnd, maxLines int, noLineNumbers bool, lineInterval int, collapseBlank bool, compactLineNums bool) {
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
		case compactLineNums:
			// Feature 4: compact prefix — "12│content"
			if lineInterval <= 1 || lineNum%lineInterval == 0 || i == lineStart-1 || i == effectiveEnd-1 {
				fmt.Fprintf(output, "%d│%s\n", lineNum, line)
			} else {
				fmt.Fprintf(output, "│%s\n", line)
			}
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

// extractEtag extracts the etag value from a readOneFile result string.
// Returns empty string if not found.
func extractEtag(result string) string {
	// Look for "[etag: XXXXXXXX]" at end of result.
	const prefix = "[etag: "
	idx := strings.LastIndex(result, prefix)
	if idx < 0 {
		return ""
	}
	rest := result[idx+len(prefix):]
	close := strings.Index(rest, "]")
	if close < 0 {
		return ""
	}
	return rest[:close]
}

// splitLines splits a string into lines.
// For large files (> 1MB), uses bufio.Scanner which is more memory efficient.
// For smaller files, uses simple string split which is faster.
// countLines returns the number of source lines in content. Unlike
// len(splitLines(...)), it does not over-count newline-terminated files
// (a trailing "\n" does not introduce a phantom final line) and avoids
// materializing the line slice.
func countLines(content []byte) int {
	if len(content) == 0 {
		return 0
	}
	n := bytes.Count(content, []byte{'\n'})
	if content[len(content)-1] != '\n' {
		n++
	}
	return n
}

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
