// Package server implements the MCP server for file operations.
package server

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/lukaszraczylo/mcp-filepuff/internal/cursor"
	"github.com/lukaszraczylo/mcp-filepuff/internal/parser"
	"github.com/lukaszraczylo/mcp-filepuff/internal/query"
	"github.com/lukaszraczylo/mcp-filepuff/pkg/protocol"
	"github.com/mark3labs/mcp-go/mcp"
)

// astQueryParams holds resolved parameters for an AST query invocation.
type astQueryParams struct {
	pattern     string
	language    string
	nameMatches string
	nameExact   string
	kindIn      []string
	paths       []string
	maxResults  int
	format      string
	offset      int
	queryHash   string
	verbose     bool
}

// resolveASTQueryParams parses the request and resolves session-pref defaults.
// Returns (params, errorResult, error); when errorResult is non-nil, the caller
// should return it directly.
func (s *Server) resolveASTQueryParams(request mcp.CallToolRequest) (*astQueryParams, *mcp.CallToolResult) {
	pattern, err := request.RequireString("pattern")
	if err != nil {
		return nil, mcp.NewToolResultError("pattern is required")
	}
	language, err := request.RequireString("language")
	if err != nil {
		return nil, mcp.NewToolResultError("language is required")
	}

	p := &astQueryParams{
		pattern:     pattern,
		language:    language,
		nameMatches: request.GetString("name_matches", ""),
		nameExact:   request.GetString("name_exact", ""),
		kindIn:      request.GetStringSlice("kind_in", nil),
		paths:       request.GetStringSlice("paths", nil),
		verbose:     request.GetBool("verbose", false),
	}

	sp := s.sessionPrefs.Load()
	var prefsMaxResults int
	var prefsFormat string
	if sp != nil {
		prefsMaxResults = sp.DefaultMaxResults
		prefsFormat = sp.ASTQueryFormat
	}
	p.maxResults = effectiveInt(request, "max_results", prefsMaxResults, 100)

	p.format = request.GetString("format", "")
	if p.format == "" {
		if prefsFormat != "" {
			p.format = prefsFormat
		} else {
			// Default to compact (one line per match): far fewer tokens than
			// verbose (full code + captures) on multi-match queries. Callers can
			// opt into verbose per-call or via the default_format session pref.
			p.format = "compact"
		}
	}

	p.queryHash = cursor.HashParams(map[string]string{
		"pattern":      p.pattern,
		"language":     p.language,
		"name_matches": p.nameMatches,
		"name_exact":   p.nameExact,
		"kind_in":      strings.Join(p.kindIn, ","),
		"paths":        strings.Join(p.paths, ","),
	})

	if cursorStr := request.GetString("cursor", ""); cursorStr != "" {
		off, hash, decErr := cursor.Decode(cursorStr)
		if decErr != nil {
			return nil, mcp.NewToolResultError(fmt.Sprintf("invalid cursor: %s", decErr))
		}
		if hash != p.queryHash {
			return nil, mcp.NewToolResultError("cursor is for a different query, re-run without cursor")
		}
		p.offset = off
	}

	return p, nil
}

// runASTQueryWalk walks the configured paths and collects matches.
func (s *Server) runASTQueryWalk(ctx context.Context, p *astQueryParams, exts []string) []query.MatchResult {
	astQuery := &query.ASTQuery{
		Pattern:  p.pattern,
		Language: p.language,
		Filters: query.QueryFilters{
			NameMatches: p.nameMatches,
			NameExact:   p.nameExact,
			KindIn:      p.kindIn,
		},
	}

	// Collect limit for early-exit: when paginating we need all results first.
	collectLimit := p.maxResults
	if p.offset > 0 {
		collectLimit = 0
	}

	var allResults []query.MatchResult
	for _, searchPath := range p.paths {
		if !s.cfg.IsPathAllowed(searchPath) {
			continue
		}

		walkErr := filepath.Walk(searchPath, func(path string, info os.FileInfo, err error) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			if err != nil {
				return nil
			}
			if info.IsDir() {
				// Skip hidden and dependency dirs (third-party code that would
				// flood results), but never the walk root itself — the caller
				// explicitly targeted it, e.g. paths:["vendor/foo"].
				if path != searchPath && (strings.HasPrefix(info.Name(), ".") || isDependencyDir(info.Name())) {
					return filepath.SkipDir
				}
				return nil
			}
			if !hasAnySuffix(path, exts) {
				return nil
			}

			// Skip oversized files before reading them off disk.
			if info.Size() > s.cfg.MaxFileSize {
				return nil
			}

			content, err := os.ReadFile(path)
			if err != nil {
				return nil
			}

			result, err := s.parser.Parse(ctx, path, content)
			if err != nil {
				return nil
			}
			matches, err := s.matcher.Match(ctx, astQuery, result.Tree, content, path)
			if err != nil {
				return nil
			}
			allResults = append(allResults, matches...)

			if collectLimit > 0 && len(allResults) >= collectLimit {
				return filepath.SkipAll
			}
			return nil
		})
		if walkErr != nil {
			s.logger.Warn("error walking path", "path", searchPath, "error", walkErr)
		}
	}
	return allResults
}

// isDependencyDir reports whether name is a well-known third-party dependency
// directory that ast_query skips by default. These hold vendored/installed code,
// not the caller's source, and parsing them wastes time and floods results with
// irrelevant matches. Target one explicitly via paths to query inside it.
func isDependencyDir(name string) bool {
	switch name {
	case "node_modules", "vendor", "bower_components":
		return true
	default:
		return false
	}
}

// hasAnySuffix reports whether path ends with any of the given suffixes.
func hasAnySuffix(path string, suffixes []string) bool {
	for _, ext := range suffixes {
		if strings.HasSuffix(path, ext) {
			return true
		}
	}
	return false
}

// buildASTCursorFooter computes the cursor footer line for truncated results.
func buildASTCursorFooter(total, offset, maxResults int, queryHash string) string {
	if offset > 0 && offset < total {
		totalAfterOffset := total - offset
		if maxResults > 0 && totalAfterOffset > maxResults {
			remaining := totalAfterOffset - maxResults
			nextOffset := offset + maxResults
			nextCursor := cursor.Encode(nextOffset, queryHash)
			return fmt.Sprintf("[cursor: %s, remaining: %d]", nextCursor, remaining)
		}
	} else if offset == 0 && maxResults > 0 && total > maxResults {
		remaining := total - maxResults
		nextCursor := cursor.Encode(maxResults, queryHash)
		return fmt.Sprintf("[cursor: %s, remaining: %d]", nextCursor, remaining)
	}
	return ""
}

// handleASTQuery handles the ast_query tool.
func (s *Server) handleASTQuery(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Acquire semaphore to limit concurrent queries (prevents CPU exhaustion)
	select {
	case s.querySem <- struct{}{}:
		defer func() { <-s.querySem }()
	case <-ctx.Done():
		return mcp.NewToolResultError("request cancelled"), nil
	}

	p, errResult := s.resolveASTQueryParams(request)
	if errResult != nil {
		return errResult, nil
	}

	if len(p.paths) == 0 {
		p.paths = []string{s.cfg.WorkspaceRoot}
	}

	exts := languageToExtensions(p.language)
	if len(exts) == 0 {
		return mcp.NewToolResultError(fmt.Sprintf("unsupported language: %s (supported: go, typescript, javascript, python, c, cpp, html, vue, elixir, rust)", p.language)), nil
	}

	allResults := s.runASTQueryWalk(ctx, p, exts)
	cursorFooter := buildASTCursorFooter(len(allResults), p.offset, p.maxResults, p.queryHash)

	// The formatter emits cursorFooter in place of its "[remaining: N]" hint when
	// results are truncated; no fragile placeholder string-replacement needed.
	output := query.FormatResultsWithCursor(allResults, p.maxResults, p.format, p.offset, p.verbose, cursorFooter)
	return mcp.NewToolResultText(output), nil
}

// generateASTSummary generates a summary of symbols in the file.
func (s *Server) generateASTSummary(ctx context.Context, path string, content []byte) string {
	// Parse the file
	result, err := s.parser.Parse(ctx, path, content)
	if err != nil {
		return "" // Silently skip AST if parsing fails
	}

	// Extract symbols
	lang := protocol.DetectLanguage(path)
	symbols := parser.ExtractSymbols(result.Tree, content, lang, path)
	if len(symbols) == 0 {
		return ""
	}

	var sb strings.Builder

	// Get relative path
	relPath := path
	if absPath, err := filepath.Abs(path); err == nil {
		if rel, err := filepath.Rel(s.cfg.WorkspaceRoot, absPath); err == nil && !strings.HasPrefix(rel, "..") {
			relPath = rel
		}
	}

	fmt.Fprintf(&sb, "**%s** (%d lines, %s)\n\n", relPath, countLines(content), lang)
	sb.WriteString("Symbols:\n")

	for _, sym := range symbols {
		kindStr := symbolKindIcon(sym.Kind)
		fmt.Fprintf(&sb, "  %s %s  L%d\n", kindStr, sym.Name, sym.Location.Line)
	}

	return sb.String()
}

// symbolKindIcon returns an icon/prefix for a symbol kind.
func symbolKindIcon(kind protocol.SymbolKind) string {
	switch kind {
	case protocol.SymbolFunction:
		return "func"
	case protocol.SymbolMethod:
		return "meth"
	case protocol.SymbolClass:
		return "class"
	case protocol.SymbolStruct:
		return "struct"
	case protocol.SymbolInterface:
		return "iface"
	case protocol.SymbolVariable:
		return "var"
	case protocol.SymbolConstant:
		return "const"
	case protocol.SymbolType:
		return "type"
	case protocol.SymbolField:
		return "field"
	case protocol.SymbolProperty:
		return "prop"
	case protocol.SymbolModule:
		return "mod"
	case protocol.SymbolPackage:
		return "pkg"
	case protocol.SymbolEnum:
		return "enum"
	case protocol.SymbolTrait:
		return "trait"
	default:
		return "sym"
	}
}

// languageToExtensions maps language names to file extensions.
func languageToExtensions(language string) []string {
	switch strings.ToLower(language) {
	case "go":
		return []string{".go"}
	case "typescript":
		return []string{".ts"}
	case "javascript":
		return []string{".js"}
	case "python":
		return []string{".py"}
	case "c":
		return []string{".c"}
	case "cpp", "c++":
		return []string{".cpp"}
	case "html":
		return []string{".html", ".htm"}
	case "vue":
		return []string{".vue"}
	case "elixir":
		return []string{".ex", ".exs"}
	case "rust":
		return []string{".rs"}
	default:
		return nil
	}
}
