// Package server implements the MCP server for file operations.
package server

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/lukaszraczylo/mcp-filepuff/internal/config"
	"github.com/lukaszraczylo/mcp-filepuff/internal/edit"
	"github.com/lukaszraczylo/mcp-filepuff/internal/lsp"
	"github.com/lukaszraczylo/mcp-filepuff/internal/parser"
	"github.com/lukaszraczylo/mcp-filepuff/internal/query"
	"github.com/lukaszraczylo/mcp-filepuff/internal/search"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// MaxConcurrentReads limits concurrent file read operations to prevent memory exhaustion.
const MaxConcurrentReads = 10

// MaxConcurrentQueries limits concurrent AST query operations to prevent CPU exhaustion.
const MaxConcurrentQueries = 5

// ServerShutdownTimeout is the timeout for graceful server shutdown.
const ServerShutdownTimeout = 10 * time.Second

// PreviewLineMaxLength is the maximum length for preview lines before truncation.
const PreviewLineMaxLength = 100

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
	readSem    chan struct{} // Semaphore for limiting concurrent file reads
	querySem   chan struct{} // Semaphore for limiting concurrent AST queries
}

// New creates a new MCP server instance.
func New(cfg *config.Config, logger *slog.Logger) (*Server, error) {
	parserRegistry := parser.NewRegistryWithSize(cfg.MaxParseSize)
	s := &Server{
		cfg:      cfg,
		logger:   logger,
		parser:   parserRegistry,
		matcher:  query.NewMatcher(parserRegistry),
		editor:   edit.NewEngine(parserRegistry),
		readSem:  make(chan struct{}, MaxConcurrentReads),
		querySem: make(chan struct{}, MaxConcurrentQueries),
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
			mcp.WithDescription("Health check - returns pong to verify the server is running.\n\n"+
				"Returns: \"pong\" text string."),
			mcp.WithReadOnlyHintAnnotation(true),
		),
		s.handlePing,
	)

	// Register file_search tool
	if s.searcher != nil {
		s.mcp.AddTool(
			mcp.NewTool("file_search",
				mcp.WithDescription("Search for text patterns in files using ripgrep. Supports regex patterns, file type filtering, and context lines.\n\n"+
					"Returns: Results grouped by file with match context. Format: \"Found N matches in M files:\" followed by file sections, "+
					"each with matching lines prefixed by \"L{line}│\" and context lines prefixed by \"   │\".\n\n"+
					"Example: {\"pattern\": \"func.*Error\", \"file_types\": [\"go\"], \"max_results\": 20}"),
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
			mcp.WithDescription("Read a file's contents with optional line range and AST symbol summary.\n\n"+
				"Returns: File content with numbered lines (format: \"  12│ line text\"). "+
				"When include_ast=true: prepends symbol summary (\"**file.go** (N lines, go)\\nSymbols:\\n  func Name  L12\\n  struct Config  L45\"). "+
				"When symbols_only=true: returns only the symbol summary (~95% fewer tokens). "+
				"When max_lines is set: truncates output with \"[... N more lines omitted]\" notice.\n\n"+
				"Examples:\n"+
				"  Full file: {\"path\": \"main.go\"}\n"+
				"  With AST: {\"path\": \"main.go\", \"include_ast\": true}\n"+
				"  Symbols only: {\"path\": \"main.go\", \"include_ast\": true, \"symbols_only\": true}\n"+
				"  Line range: {\"path\": \"main.go\", \"line_start\": 10, \"line_end\": 50}"),
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
			mcp.WithDescription("Search for AST patterns in code files. Use code patterns with $VAR placeholders to match and capture code structures like functions, classes, and types.\n\n"+
				"Returns: \"Found N match(es):\" followed by entries in format \"**file:line** (node_type)\" with code blocks "+
				"and captured variables ($NAME=value). Returns \"No matches found.\" when no results.\n\n"+
				"Examples:\n"+
				"  Go error funcs: {\"pattern\": \"func $NAME($$$ARGS) error\", \"language\": \"go\"}\n"+
				"  Python classes:  {\"pattern\": \"class $NAME: $$$BODY\", \"language\": \"python\"}\n"+
				"  Named function: {\"pattern\": \"func $NAME($$$ARGS)\", \"language\": \"go\", \"name_exact\": \"NewServer\"}"),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithString("pattern",
				mcp.Required(),
				mcp.Description("Code pattern with placeholders: $NAME (single), $$$ARGS (multiple), $_ (wildcard). Examples: 'func $NAME($$$ARGS) error', 'class $NAME { $$$BODY }'"),
			),
			mcp.WithString("language",
				mcp.Required(),
				mcp.Description("Target language: go, typescript, javascript, python, c, cpp, html, vue, elixir, rust"),
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
				mcp.WithDescription("Get information about the symbol at a specific position in a file. Returns type, documentation, and definition location using LSP when available.\n\n"+
					"Returns: \"**Symbol Information**\" followed by hover/type information from LSP, or \"**Symbol Information** (AST fallback)\" "+
					"with node type and text when LSP unavailable. Returns \"No symbol information available at this position.\" when nothing is found.\n\n"+
					"Example: {\"file\": \"server.go\", \"line\": 45, \"column\": 6}"),
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
				mcp.WithDescription("Find the definition of the symbol at a specific position. Uses LSP to locate where a function, variable, type, etc. is defined.\n\n"+
					"Returns: \"Found N definition(s):\" with entries showing \"**file:line:column**\" and a 3-line code preview "+
					"with the target line marked by \">\". Returns \"No definition found.\" when the symbol has no definition.\n\n"+
					"Example: {\"file\": \"handler.go\", \"line\": 23, \"column\": 10}"),
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
				mcp.WithDescription("Find all references to the symbol at a specific position. Uses LSP to locate all usages of a function, variable, type, etc.\n\n"+
					"Returns: \"Found N reference(s):\" grouped by file, each showing \"**file** (count)\" with locations as "+
					"\"L{line}:{column}\". Returns \"No references found.\" when no usages exist.\n\n"+
					"Example: {\"file\": \"types.go\", \"line\": 5, \"column\": 6}"),
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
			mcp.WithDescription("Preview an edit without applying it. Uses AST-aware editing for code files (Go, TypeScript, JavaScript, Python, C, C++, Rust), and text-based editing for other files (Markdown, JSON, YAML, config files, etc.).\n\n"+
				"Returns: \"**Edit Preview**\" followed by a unified diff showing proposed changes. Does not modify the file. "+
				"For code files: uses AST-aware mode with syntax validation. For other files: uses text-based mode.\n\n"+
				"Examples:\n"+
				"  AST mode: {\"file\": \"main.go\", \"operation\": \"replace\", \"selector_kind\": \"function_declaration\", \"selector_name\": \"Hello\", \"new_content\": \"func Hello() {\\n\\treturn\\n}\"}\n"+
				"  Text mode: {\"file\": \"README.md\", \"operation\": \"replace\", \"selector_text\": \"## Old Header\", \"new_content\": \"## New Header\"}\n"+
				"  Line range: {\"file\": \"config.yaml\", \"operation\": \"replace\", \"selector_line\": 5, \"selector_line_end\": 10, \"new_content\": \"key: value\"}"),
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
			mcp.WithDescription("Apply an edit to a file. Uses AST-aware editing for code files (Go, TypeScript, JavaScript, Python, C, C++, Rust) with syntax validation, and text-based editing for other files (Markdown, JSON, YAML, config files, etc.).\n\n"+
				"Returns: \"**Edit Applied Successfully**\" followed by a unified diff of the changes made. "+
				"For code files, validates syntax before writing — returns an error if the edit would produce invalid syntax.\n\n"+
				"Examples:\n"+
				"  AST mode: {\"file\": \"main.go\", \"operation\": \"replace\", \"selector_kind\": \"function_declaration\", \"selector_name\": \"Hello\", \"new_content\": \"func Hello() {\\n\\treturn\\n}\"}\n"+
				"  Text mode: {\"file\": \"README.md\", \"operation\": \"replace\", \"selector_text\": \"## Old Header\", \"new_content\": \"## New Header\"}\n"+
				"  Line range: {\"file\": \"config.yaml\", \"operation\": \"replace\", \"selector_line\": 5, \"selector_line_end\": 10, \"new_content\": \"key: value\"}"),
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

// Run starts the MCP server and blocks until shutdown.
func (s *Server) Run(ctx context.Context) error {
	// Set up signal handling for graceful shutdown
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigChan)

	// Channel to communicate server errors
	errChan := make(chan error, 1)

	// Start server in goroutine
	go func() {
		s.logger.Info("starting MCP server",
			"workspace", s.cfg.WorkspaceRoot,
			"lsp_enabled", s.cfg.EnableLSP,
		)
		errChan <- server.ServeStdio(s.mcp)
	}()

	// Wait for either signal or server error
	select {
	case sig := <-sigChan:
		s.logger.Info("received shutdown signal", "signal", sig)

		// Create timeout context for shutdown
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), ServerShutdownTimeout)
		defer shutdownCancel()

		// Call graceful shutdown
		if err := s.Shutdown(shutdownCtx); err != nil {
			s.logger.Error("error during shutdown", "error", err)
			return err
		}

		s.logger.Info("server shutdown complete")
		return nil

	case err := <-errChan:
		// Server stopped on its own
		return err

	case <-ctx.Done():
		// Context cancelled externally
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), ServerShutdownTimeout)
		defer shutdownCancel()

		if err := s.Shutdown(shutdownCtx); err != nil {
			s.logger.Error("error during shutdown", "error", err)
		}

		return ctx.Err()
	}
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
