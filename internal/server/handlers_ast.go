// Package server implements the MCP server for file operations.
package server

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/lukaszraczylo/mcp-filepuff/internal/parser"
	"github.com/lukaszraczylo/mcp-filepuff/internal/query"
	"github.com/lukaszraczylo/mcp-filepuff/pkg/protocol"
	"github.com/mark3labs/mcp-go/mcp"
)

// handleASTQuery handles the ast_query tool.
func (s *Server) handleASTQuery(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Acquire semaphore to limit concurrent queries (prevents CPU exhaustion)
	select {
	case s.querySem <- struct{}{}:
		defer func() { <-s.querySem }()
	case <-ctx.Done():
		return mcp.NewToolResultError("request cancelled"), nil
	}

	pattern, err := request.RequireString("pattern")
	if err != nil {
		return mcp.NewToolResultError("pattern is required"), nil
	}

	language, err := request.RequireString("language")
	if err != nil {
		return mcp.NewToolResultError("language is required"), nil
	}

	// Build query
	astQuery := &query.ASTQuery{
		Pattern:  pattern,
		Language: language,
		Filters: query.QueryFilters{
			NameMatches: request.GetString("name_matches", ""),
			NameExact:   request.GetString("name_exact", ""),
			KindIn:      request.GetStringSlice("kind_in", nil),
		},
	}

	maxResults := request.GetInt("max_results", 100)
	paths := request.GetStringSlice("paths", nil)

	// Default to workspace root if no paths specified
	if len(paths) == 0 {
		paths = []string{s.cfg.WorkspaceRoot}
	}

	// Find files to search based on language
	exts := languageToExtensions(language)
	if len(exts) == 0 {
		return mcp.NewToolResultError(fmt.Sprintf("unsupported language: %s (supported: go, typescript, javascript, python, c, cpp, html, vue, elixir)", language)), nil
	}

	var allResults []query.MatchResult

	// Walk through paths and find matching files
	for _, searchPath := range paths {
		// Validate path is within workspace
		if !s.cfg.IsPathAllowed(searchPath) {
			continue
		}

		err := filepath.Walk(searchPath, func(path string, info os.FileInfo, err error) error {
			// Check for context cancellation
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			if err != nil {
				return nil // Skip files with errors
			}

			if info.IsDir() {
				// Skip hidden directories
				if strings.HasPrefix(info.Name(), ".") {
					return filepath.SkipDir
				}
				return nil
			}

			// Check file extension matches language
			matched := false
			for _, ext := range exts {
				if strings.HasSuffix(path, ext) {
					matched = true
					break
				}
			}
			if !matched {
				return nil
			}

			// Read and parse file
			content, err := os.ReadFile(path)
			if err != nil {
				return nil // Skip unreadable files
			}

			// Check file size
			if int64(len(content)) > s.cfg.MaxFileSize {
				return nil // Skip large files
			}

			// Parse file
			result, err := s.parser.Parse(ctx, path, content)
			if err != nil {
				return nil // Skip unparseable files
			}

			// Run query
			matches, err := s.matcher.Match(ctx, astQuery, result.Tree, content, path)
			if err != nil {
				return nil // Skip on error
			}

			allResults = append(allResults, matches...)

			// Stop if we have enough results
			if maxResults > 0 && len(allResults) >= maxResults {
				return filepath.SkipAll
			}

			return nil
		})
		if err != nil {
			s.logger.Warn("error walking path", "path", searchPath, "error", err)
		}
	}

	// Format and return results
	output := query.FormatResults(allResults, maxResults)
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

	sb.WriteString(fmt.Sprintf("**%s** (%d lines, %s)\n\n", relPath, len(splitLines(string(content))), lang))
	sb.WriteString("Symbols:\n")

	for _, sym := range symbols {
		kindStr := symbolKindIcon(sym.Kind)
		sb.WriteString(fmt.Sprintf("  %s %s  L%d\n", kindStr, sym.Name, sym.Location.Line))
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
	default:
		return nil
	}
}
