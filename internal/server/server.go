// Package server implements the MCP server for file operations.
package server

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/lukaszraczylo/mcp-filepuff/internal/config"
	"github.com/lukaszraczylo/mcp-filepuff/internal/edit"
	"github.com/lukaszraczylo/mcp-filepuff/internal/lsp"
	"github.com/lukaszraczylo/mcp-filepuff/internal/parser"
	"github.com/lukaszraczylo/mcp-filepuff/internal/query"
	"github.com/lukaszraczylo/mcp-filepuff/internal/search"
	"github.com/lukaszraczylo/mcp-filepuff/pkg/protocol"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// Server represents the MCP file operations server.
type Server struct {
	cfg        *config.Config
	logger     *slog.Logger
	mcp        *server.MCPServer
	searcher   *search.Searcher
	parser     *parser.Registry
	matcher    *query.Matcher
	lspManager *lsp.Manager
	editor     *edit.Engine
}

// New creates a new MCP server instance.
func New(cfg *config.Config, logger *slog.Logger) (*Server, error) {
	parserRegistry := parser.NewRegistry()
	s := &Server{
		cfg:     cfg,
		logger:  logger,
		parser:  parserRegistry,
		matcher: query.NewMatcher(parserRegistry),
		editor:  edit.NewEngine(parserRegistry),
	}

	// Initialize searcher
	searcher, err := search.New(cfg, logger)
	if err != nil {
		logger.Warn("ripgrep not available, search functionality disabled", "error", err)
	}
	s.searcher = searcher

	// Initialize LSP manager if enabled
	if cfg.EnableLSP {
		s.lspManager = lsp.NewManager(cfg.WorkspaceRoot, logger)
	}

	// Create MCP server
	mcpServer := server.NewMCPServer(
		"mcp-filepuff",
		"1.0.0",
		server.WithLogging(),
	)
	s.mcp = mcpServer

	// Register tools
	s.registerTools()

	return s, nil
}

// registerTools registers all available tools with the MCP server.
func (s *Server) registerTools() {
	// Register ping tool for health checks
	s.mcp.AddTool(
		mcp.NewTool("ping",
			mcp.WithDescription("Health check - returns pong to verify the server is running"),
			mcp.WithReadOnlyHintAnnotation(true),
		),
		s.handlePing,
	)

	// Register file_search tool
	if s.searcher != nil {
		s.mcp.AddTool(
			mcp.NewTool("file_search",
				mcp.WithDescription("Search for text patterns in files using ripgrep. Supports regex patterns, file type filtering, and context lines."),
				mcp.WithReadOnlyHintAnnotation(true),
				mcp.WithString("pattern",
					mcp.Required(),
					mcp.Description("The search pattern (regex by default)"),
				),
				mcp.WithArray("paths",
					mcp.Description("Paths to search in (defaults to workspace root)"),
					mcp.WithStringItems(),
				),
				mcp.WithArray("file_types",
					mcp.Description("File types to search (e.g., ['go', 'ts', 'py'])"),
					mcp.WithStringItems(),
				),
				mcp.WithBoolean("ignore_case",
					mcp.Description("Case insensitive search"),
				),
				mcp.WithBoolean("regex",
					mcp.Description("Treat pattern as regex (default: true)"),
				),
				mcp.WithNumber("context_lines",
					mcp.Description("Number of context lines around matches (default: 2)"),
				),
				mcp.WithNumber("max_results",
					mcp.Description("Maximum number of results to return"),
				),
			),
			s.handleFileSearch,
		)
	}

	// Register file_read tool
	s.mcp.AddTool(
		mcp.NewTool("file_read",
			mcp.WithDescription("Read a file's contents with optional line range and AST symbol summary"),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithString("path",
				mcp.Required(),
				mcp.Description("Path to the file to read"),
			),
			mcp.WithNumber("line_start",
				mcp.Description("Starting line number (1-indexed)"),
			),
			mcp.WithNumber("line_end",
				mcp.Description("Ending line number (inclusive)"),
			),
			mcp.WithBoolean("include_ast",
				mcp.Description("Include AST symbol summary (functions, classes, types, etc.)"),
			),
			mcp.WithBoolean("symbols_only",
				mcp.Description("Return only symbol summary without file content (token-efficient mode). Requires include_ast=true."),
			),
			mcp.WithNumber("max_lines",
				mcp.Description("Maximum number of lines to return (for token efficiency). Applied after line_start/line_end."),
			),
		),
		s.handleFileRead,
	)

	// Register ast_query tool
	s.mcp.AddTool(
		mcp.NewTool("ast_query",
			mcp.WithDescription("Search for AST patterns in code files. Use code patterns with $VAR placeholders to match and capture code structures like functions, classes, and types."),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithString("pattern",
				mcp.Required(),
				mcp.Description("Code pattern with placeholders: $NAME (single), $$$ARGS (multiple), $_ (wildcard). Examples: 'func $NAME($$$ARGS) error', 'class $NAME { $$$BODY }'"),
			),
			mcp.WithString("language",
				mcp.Required(),
				mcp.Description("Target language: go, typescript, javascript, python, c, cpp"),
			),
			mcp.WithArray("paths",
				mcp.Description("Paths to search in (defaults to workspace root)"),
				mcp.WithStringItems(),
			),
			mcp.WithString("name_matches",
				mcp.Description("Regex pattern to filter by name"),
			),
			mcp.WithString("name_exact",
				mcp.Description("Exact name to match"),
			),
			mcp.WithArray("kind_in",
				mcp.Description("Node types to match (e.g., function_declaration, class_declaration)"),
				mcp.WithStringItems(),
			),
			mcp.WithNumber("max_results",
				mcp.Description("Maximum number of results to return (default: 100)"),
			),
		),
		s.handleASTQuery,
	)

	// Register LSP-based tools if LSP is enabled
	if s.lspManager != nil {
		// Register symbol_at tool
		s.mcp.AddTool(
			mcp.NewTool("symbol_at",
				mcp.WithDescription("Get information about the symbol at a specific position in a file. Returns type, documentation, and definition location using LSP when available."),
				mcp.WithReadOnlyHintAnnotation(true),
				mcp.WithString("file",
					mcp.Required(),
					mcp.Description("Path to the file"),
				),
				mcp.WithNumber("line",
					mcp.Required(),
					mcp.Description("Line number (1-indexed)"),
				),
				mcp.WithNumber("column",
					mcp.Required(),
					mcp.Description("Column number (1-indexed)"),
				),
			),
			s.handleSymbolAt,
		)

		// Register find_definition tool
		s.mcp.AddTool(
			mcp.NewTool("find_definition",
				mcp.WithDescription("Find the definition of the symbol at a specific position. Uses LSP to locate where a function, variable, type, etc. is defined."),
				mcp.WithReadOnlyHintAnnotation(true),
				mcp.WithString("file",
					mcp.Required(),
					mcp.Description("Path to the file"),
				),
				mcp.WithNumber("line",
					mcp.Required(),
					mcp.Description("Line number (1-indexed)"),
				),
				mcp.WithNumber("column",
					mcp.Required(),
					mcp.Description("Column number (1-indexed)"),
				),
			),
			s.handleFindDefinition,
		)

		// Register find_references tool
		s.mcp.AddTool(
			mcp.NewTool("find_references",
				mcp.WithDescription("Find all references to the symbol at a specific position. Uses LSP to locate all usages of a function, variable, type, etc."),
				mcp.WithReadOnlyHintAnnotation(true),
				mcp.WithString("file",
					mcp.Required(),
					mcp.Description("Path to the file"),
				),
				mcp.WithNumber("line",
					mcp.Required(),
					mcp.Description("Line number (1-indexed)"),
				),
				mcp.WithNumber("column",
					mcp.Required(),
					mcp.Description("Column number (1-indexed)"),
				),
				mcp.WithBoolean("include_declaration",
					mcp.Description("Include the declaration in results (default: true)"),
				),
			),
			s.handleFindReferences,
		)
	}

	// Register edit tools
	s.mcp.AddTool(
		mcp.NewTool("edit_preview",
			mcp.WithDescription("Preview an edit without applying it. Uses AST-aware editing for code files (Go, TypeScript, JavaScript, Python, C, C++), and text-based editing for other files (Markdown, JSON, YAML, config files, etc.)."),
			mcp.WithString("file",
				mcp.Required(),
				mcp.Description("Path to the file to edit"),
			),
			mcp.WithString("operation",
				mcp.Required(),
				mcp.Description("Edit operation: replace, insert_before, insert_after, delete"),
			),
			mcp.WithString("new_content",
				mcp.Description("New content (required for replace/insert operations)"),
			),
			// AST-mode selectors (for code files)
			mcp.WithString("selector_kind",
				mcp.Description("AST node type to match (e.g., function_declaration, class_declaration). For code files only."),
			),
			mcp.WithString("selector_name",
				mcp.Description("Name of the symbol to match. For code files only."),
			),
			// Shared selectors
			mcp.WithNumber("selector_line",
				mcp.Description("Line number (1-indexed). For AST mode: narrows search. For text mode: start of line range."),
			),
			mcp.WithNumber("selector_index",
				mcp.Description("Index of the match to use if multiple matches found (default: 0)"),
			),
			// Text-mode selectors (for non-code files or explicit text matching)
			mcp.WithNumber("selector_line_end",
				mcp.Description("End line number for range selection (text mode). Used with selector_line."),
			),
			mcp.WithString("selector_text",
				mcp.Description("Exact text to match (text mode). Must be unique or use selector_index."),
			),
			mcp.WithString("selector_pattern",
				mcp.Description("Regex pattern to match (text mode). Must be unique or use selector_index."),
			),
		),
		s.handleEditPreview,
	)

	s.mcp.AddTool(
		mcp.NewTool("edit_apply",
			mcp.WithDescription("Apply an edit to a file. Uses AST-aware editing for code files (Go, TypeScript, JavaScript, Python, C, C++) with syntax validation, and text-based editing for other files (Markdown, JSON, YAML, config files, etc.)."),
			mcp.WithString("file",
				mcp.Required(),
				mcp.Description("Path to the file to edit"),
			),
			mcp.WithString("operation",
				mcp.Required(),
				mcp.Description("Edit operation: replace, insert_before, insert_after, delete"),
			),
			mcp.WithString("new_content",
				mcp.Description("New content (required for replace/insert operations)"),
			),
			// AST-mode selectors (for code files)
			mcp.WithString("selector_kind",
				mcp.Description("AST node type to match (e.g., function_declaration, class_declaration). For code files only."),
			),
			mcp.WithString("selector_name",
				mcp.Description("Name of the symbol to match. For code files only."),
			),
			// Shared selectors
			mcp.WithNumber("selector_line",
				mcp.Description("Line number (1-indexed). For AST mode: narrows search. For text mode: start of line range."),
			),
			mcp.WithNumber("selector_index",
				mcp.Description("Index of the match to use if multiple matches found (default: 0)"),
			),
			// Text-mode selectors (for non-code files or explicit text matching)
			mcp.WithNumber("selector_line_end",
				mcp.Description("End line number for range selection (text mode). Used with selector_line."),
			),
			mcp.WithString("selector_text",
				mcp.Description("Exact text to match (text mode). Must be unique or use selector_index."),
			),
			mcp.WithString("selector_pattern",
				mcp.Description("Regex pattern to match (text mode). Must be unique or use selector_index."),
			),
		),
		s.handleEditApply,
	)
}

// handlePing handles the ping health check tool.
func (s *Server) handlePing(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return mcp.NewToolResultText("pong"), nil
}

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
		return mcp.NewToolResultError(fmt.Sprintf("search error: %v", err)), nil
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
		return mcp.NewToolResultError(fmt.Sprintf("error reading file: %v", err)), nil
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

func splitLines(s string) []string {
	// Use optimized stdlib implementation (2-3x faster than manual loop)
	return strings.Split(s, "\n")
}

// handleASTQuery handles the ast_query tool.
func (s *Server) handleASTQuery(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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
	ext := languageToExtension(language)
	if ext == "" {
		return mcp.NewToolResultError(fmt.Sprintf("unsupported language: %s", language)), nil
	}

	var allResults []query.MatchResult

	// Walk through paths and find matching files
	for _, searchPath := range paths {
		// Validate path is within workspace
		if !s.cfg.IsPathAllowed(searchPath) {
			continue
		}

		err := filepath.Walk(searchPath, func(path string, info os.FileInfo, err error) error {
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
			if !strings.HasSuffix(path, ext) {
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

// languageToExtension maps language names to file extensions.
func languageToExtension(language string) string {
	switch strings.ToLower(language) {
	case "go":
		return ".go"
	case "typescript":
		return ".ts"
	case "javascript":
		return ".js"
	case "python":
		return ".py"
	case "c":
		return ".c"
	case "cpp", "c++":
		return ".cpp"
	default:
		return ""
	}
}

// handleSymbolAt handles the symbol_at tool.
func (s *Server) handleSymbolAt(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	file, err := request.RequireString("file")
	if err != nil {
		return mcp.NewToolResultError("file is required"), nil
	}

	line := request.GetInt("line", 0)
	if line <= 0 {
		return mcp.NewToolResultError("line must be positive"), nil
	}

	col := request.GetInt("column", 0)
	if col <= 0 {
		return mcp.NewToolResultError("column must be positive"), nil
	}

	// Validate path
	if !s.cfg.IsPathAllowed(file) {
		return mcp.NewToolResultError("file is outside workspace root"), nil
	}

	// Try LSP hover
	hover, err := s.lspManager.Hover(ctx, file, line, col)
	if err != nil {
		// Fall back to AST-based info
		return s.handleSymbolAtFallback(ctx, file, line, col)
	}

	if hover == nil {
		return mcp.NewToolResultText("No symbol information available at this position."), nil
	}

	var output strings.Builder
	output.WriteString("**Symbol Information**\n\n")
	output.WriteString(hover.Contents.Value)

	return mcp.NewToolResultText(output.String()), nil
}

// handleSymbolAtFallback provides AST-based symbol info when LSP is unavailable.
func (s *Server) handleSymbolAtFallback(ctx context.Context, file string, line, col int) (*mcp.CallToolResult, error) {
	content, err := os.ReadFile(file)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to read file: %v", err)), nil
	}

	result, err := s.parser.Parse(ctx, file, content)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to parse file: %v", err)), nil
	}

	node := parser.FindNodeAtPosition(result.Tree, line, col)
	if node == nil {
		return mcp.NewToolResultText("No symbol at this position."), nil
	}

	var output strings.Builder
	output.WriteString("**Symbol Information** (AST fallback)\n\n")
	output.WriteString(fmt.Sprintf("Node type: `%s`\n", node.Type()))
	output.WriteString(fmt.Sprintf("Text: `%s`\n", parser.GetNodeText(node, content)))

	return mcp.NewToolResultText(output.String()), nil
}

// handleFindDefinition handles the find_definition tool.
func (s *Server) handleFindDefinition(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	file, err := request.RequireString("file")
	if err != nil {
		return mcp.NewToolResultError("file is required"), nil
	}

	line := request.GetInt("line", 0)
	if line <= 0 {
		return mcp.NewToolResultError("line must be positive"), nil
	}

	col := request.GetInt("column", 0)
	if col <= 0 {
		return mcp.NewToolResultError("column must be positive"), nil
	}

	// Validate path
	if !s.cfg.IsPathAllowed(file) {
		return mcp.NewToolResultError("file is outside workspace root"), nil
	}

	locations, err := s.lspManager.Definition(ctx, file, line, col)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("definition lookup failed: %v", err)), nil
	}

	if len(locations) == 0 {
		return mcp.NewToolResultText("No definition found."), nil
	}

	var output strings.Builder
	output.WriteString(fmt.Sprintf("Found %d definition(s):\n\n", len(locations)))

	for _, loc := range locations {
		filePath := lsp.URIToFile(loc.URI)
		output.WriteString(fmt.Sprintf("**%s:%d:%d**\n", filePath, loc.Range.Start.Line+1, loc.Range.Start.Character+1))

		// Try to read a preview snippet
		preview := readFilePreview(filePath, loc.Range.Start.Line+1, 3)
		if preview != "" {
			output.WriteString("```\n")
			output.WriteString(preview)
			output.WriteString("```\n")
		}
		output.WriteString("\n")
	}

	return mcp.NewToolResultText(output.String()), nil
}

// handleFindReferences handles the find_references tool.
func (s *Server) handleFindReferences(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	file, err := request.RequireString("file")
	if err != nil {
		return mcp.NewToolResultError("file is required"), nil
	}

	line := request.GetInt("line", 0)
	if line <= 0 {
		return mcp.NewToolResultError("line must be positive"), nil
	}

	col := request.GetInt("column", 0)
	if col <= 0 {
		return mcp.NewToolResultError("column must be positive"), nil
	}

	includeDecl := request.GetBool("include_declaration", true)

	// Validate path
	if !s.cfg.IsPathAllowed(file) {
		return mcp.NewToolResultError("file is outside workspace root"), nil
	}

	locations, err := s.lspManager.References(ctx, file, line, col, includeDecl)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("references lookup failed: %v", err)), nil
	}

	if len(locations) == 0 {
		return mcp.NewToolResultText("No references found."), nil
	}

	var output strings.Builder
	output.WriteString(fmt.Sprintf("Found %d reference(s):\n\n", len(locations)))

	// Group by file
	fileGroups := make(map[string][]lsp.Location)
	for _, loc := range locations {
		filePath := lsp.URIToFile(loc.URI)
		fileGroups[filePath] = append(fileGroups[filePath], loc)
	}

	for filePath, locs := range fileGroups {
		output.WriteString(fmt.Sprintf("**%s** (%d)\n", filePath, len(locs)))
		for _, loc := range locs {
			output.WriteString(fmt.Sprintf("  L%d:%d\n", loc.Range.Start.Line+1, loc.Range.Start.Character+1))
		}
		output.WriteString("\n")
	}

	return mcp.NewToolResultText(output.String()), nil
}

// readFilePreview reads a few lines from a file around the given line.
func readFilePreview(file string, line, contextLines int) string {
	content, err := os.ReadFile(file)
	if err != nil {
		return ""
	}

	lines := splitLines(string(content))
	startLine := max(1, line-contextLines)
	endLine := min(line+contextLines, len(lines))

	var preview strings.Builder
	for i := startLine - 1; i < endLine && i < len(lines); i++ {
		lineText := lines[i]
		if len(lineText) > 100 {
			lineText = lineText[:100] + "..."
		}
		prefix := "  "
		if i+1 == line {
			prefix = "> "
		}
		preview.WriteString(fmt.Sprintf("%s%4d: %s\n", prefix, i+1, lineText))
	}

	return preview.String()
}

// handleEditPreview handles the edit_preview tool.
func (s *Server) handleEditPreview(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return s.handleEdit(ctx, request, false)
}

// handleEditApply handles the edit_apply tool.
func (s *Server) handleEditApply(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return s.handleEdit(ctx, request, true)
}

// handleEdit is the shared implementation for edit_preview and edit_apply.
func (s *Server) handleEdit(ctx context.Context, request mcp.CallToolRequest, apply bool) (*mcp.CallToolResult, error) {
	file, err := request.RequireString("file")
	if err != nil {
		return mcp.NewToolResultError("file is required"), nil
	}

	operation, err := request.RequireString("operation")
	if err != nil {
		return mcp.NewToolResultError("operation is required"), nil
	}

	// Validate path
	if !s.cfg.IsPathAllowed(file) {
		return mcp.NewToolResultError("file is outside workspace root"), nil
	}

	// Note: We no longer validate language support here.
	// The edit engine automatically detects whether to use AST or text mode.

	// Build edit request with both AST and text-mode selectors
	astEdit := &edit.ASTEdit{
		File:       file,
		Operation:  edit.EditOperation(operation),
		NewContent: request.GetString("new_content", ""),
		Selector: edit.ASTSelector{
			// AST-mode selectors
			Kind:   request.GetString("selector_kind", ""),
			Name:   request.GetString("selector_name", ""),
			AtLine: request.GetInt("selector_line", 0),
			Index:  request.GetInt("selector_index", 0),
			// Text-mode selectors
			LineEnd:     request.GetInt("selector_line_end", 0),
			Text:        request.GetString("selector_text", ""),
			TextPattern: request.GetString("selector_pattern", ""),
		},
	}

	// Perform edit
	var result *edit.EditResult
	if apply {
		result, err = s.editor.Apply(ctx, astEdit)
	} else {
		result, err = s.editor.Preview(ctx, astEdit)
	}

	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("edit failed: %v", err)), nil
	}

	if !result.Success {
		return mcp.NewToolResultError(result.Error), nil
	}

	// Format output
	var output strings.Builder
	if apply {
		output.WriteString("**Edit Applied Successfully**\n\n")
	} else {
		output.WriteString("**Edit Preview**\n\n")
	}

	output.WriteString("Diff:\n```diff\n")
	output.WriteString(result.Diff)
	output.WriteString("```\n")

	return mcp.NewToolResultText(output.String()), nil
}

// Run starts the MCP server and blocks until shutdown.
func (s *Server) Run(ctx context.Context) error {
	// Set up signal handling for graceful shutdown
	_, cancel := context.WithCancel(ctx)
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		s.logger.Info("received shutdown signal", "signal", sig)
		cancel()
	}()

	s.logger.Info("starting MCP server",
		"workspace", s.cfg.WorkspaceRoot,
		"lsp_enabled", s.cfg.EnableLSP,
	)

	// Start the MCP server with stdio transport
	return server.ServeStdio(s.mcp)
}

// Shutdown gracefully shuts down the server.
func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("shutting down MCP server")

	// Close LSP manager
	if s.lspManager != nil {
		_ = s.lspManager.Close()
	}

	// Close parser registry
	if s.parser != nil {
		s.parser.Close()
	}

	return nil
}
