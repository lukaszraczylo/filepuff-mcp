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
	cfg          *config.Config
	logger       *slog.Logger
	mcp          *server.MCPServer
	searcher     *search.Searcher
	parser       *parser.Registry
	matcher      *query.Matcher
	lspManager   *lsp.Manager
	editor       *edit.Engine
	readSem      chan struct{}   // Semaphore for limiting concurrent file reads
	querySem     chan struct{}   // Semaphore for limiting concurrent AST queries
	sessionPrefs sessionPrefsPtr // Atomic pointer; populated by OnAfterInitialize hook
}

// New creates a new MCP server instance.
func New(cfg *config.Config, logger *slog.Logger) (*Server, error) {
	parserRegistry := parser.NewRegistryWithSize(cfg.MaxParseSize)
	s := &Server{
		cfg:      cfg,
		logger:   logger,
		parser:   parserRegistry,
		matcher:  query.NewMatcher(parserRegistry),
		editor:   edit.NewEngineWithValidator(parserRegistry, cfg.IsPathAllowed),
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

	// Build OnAfterInitialize hook that parses client capability prefs.
	// Signature (from mcp-go v0.48.0 hooks.go):
	//   func(ctx context.Context, id any, message *mcp.InitializeRequest, result *mcp.InitializeResult)
	hooks := &server.Hooks{}
	hooks.AddAfterInitialize(func(_ context.Context, _ any, msg *mcp.InitializeRequest, _ *mcp.InitializeResult) {
		if msg == nil {
			return
		}
		raw, _ := msg.Params.Capabilities.Experimental["filepuff"].(map[string]any)
		prefs := ParseSessionPrefs(raw)
		s.sessionPrefs.Store(&prefs)
	})

	// Create MCP server
	mcpServer := server.NewMCPServer(
		"mcp-filepuff",
		"2.0.0",
		server.WithLogging(),
		server.WithHooks(hooks),
	)
	s.mcp = mcpServer

	// Register tools
	s.registerTools()

	// Register help resources (filepuff://help/<tool>)
	s.registerResources()

	// Register filepuff://read/{+path} resource template for large-file access.
	s.registerReadResource()

	return s, nil
}

// registerTools registers all available tools with the MCP server.
func (s *Server) registerTools() {
	// Register file_search tool
	if s.searcher != nil {
		s.mcp.AddTool(
			mcp.NewTool("file_search",
				mcp.WithDescription("Search for text patterns in files using ripgrep. Supports regex patterns, file type filtering, and context lines. "+
					"See resource filepuff://help/file_search for flags and examples."),
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
				mcp.WithBoolean("include_hidden",
					mcp.Description("Search hidden files and directories (dotfiles like .github/). Default: false."),
				),
				mcp.WithBoolean("regex",
					mcp.Description("Treat pattern as regex (default: true)"),
				),
				mcp.WithNumber("context_lines",
					mcp.Description("Number of context lines around matches (default: 2)"),
				),
				mcp.WithNumber("max_results",
					mcp.Description("Maximum number of results to return (page size for pagination)"),
				),
				mcp.WithBoolean("cluster",
					mcp.Description("Coalesce consecutive match lines into ranges (L12-14│ text). Drops context lines. Default: false."),
				),
				mcp.WithString("cursor",
					mcp.Description("Pagination cursor from a previous truncated response. Pass back to fetch the next page."),
				),
				mcp.WithBoolean("verbose",
					mcp.Description("Emit \"Found N matches in M files:\" preamble. Default: false (v2 default)."),
				),
			),
			s.handleFileSearch,
		)
	}

	// Register file_read tool
	s.mcp.AddTool(
		mcp.NewTool("file_read",
			mcp.WithDescription("Read a file's contents with optional line range and AST symbol summary. "+
				"See resource filepuff://help/file_read for flags and examples."),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithString("path",
				mcp.Description("Path to the file to read (required unless paths is provided)"),
			),
			mcp.WithArray("paths",
				mcp.Description("Read multiple files in one call. Each file gets a '--- path ---' header. Overrides path if both provided."),
				mcp.WithStringItems(),
			),
			mcp.WithString("previous_etag",
				mcp.Description("Etag from a previous read of this file. If the file is unchanged, returns '[unchanged, etag: ...]' with no content — saving all content tokens."),
			),
			mcp.WithString("symbol_name",
				mcp.Description("Read only the named symbol (function, struct, class, etc.) instead of the whole file. Resolves line range via AST — eliminates an ast_query round-trip."),
			),
			mcp.WithString("symbol_kind",
				mcp.Description("Disambiguate symbol_name by kind when multiple symbols share the same name. Accepted values: function, method, struct, class, interface, type, enum, trait, constant, module."),
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
				mcp.Description("Return only symbol summary without file content (token-efficient mode). Requires include_ast=true. Alias: mode='symbols_only'."),
			),
			mcp.WithString("mode",
				mcp.Description("Output mode: 'full' (default, full file), 'skeleton' (signatures + { ... } stubs, bodies elided), 'symbols_only' (symbol list only, alias for symbols_only=true)."),
			),
			mcp.WithArray("strip",
				mcp.Description("Strip content classes before line-numbering. Values: 'imports' (remove import blocks), 'license' (remove leading license comment), 'block_comments' (remove /* */ and Python triple-quoted strings). Emits [stripped: ...] footer. Note: line numbers are renumbered from 1 over the stripped content, so they no longer match the file's original line numbers."),
				mcp.WithStringItems(),
			),
			mcp.WithNumber("max_lines",
				mcp.Description("Maximum number of lines to return (for token efficiency). Applied after line_start/line_end."),
			),
			mcp.WithBoolean("no_line_numbers",
				mcp.Description("Omit the '  12\u2502 ' line number prefix entirely. Saves ~10% tokens. line_number_interval=0 has the same effect."),
			),
			mcp.WithNumber("line_number_interval",
				mcp.Description("Print line numbers only every N lines (default: 1 = every line). E.g. 10 = anchor every 10th line plus first/last. 0 = no line numbers."),
			),
			mcp.WithBoolean("compact_line_numbers",
				mcp.Description("Use compact line prefix '12\u2502' instead of '  12\u2502 ' (no padding, no trailing space). Works with line_number_interval. Default off."),
			),
			mcp.WithBoolean("collapse_blank_lines",
				mcp.Description("Collapse runs of consecutive blank lines to a single blank line. Useful for token savings on heavily-spaced code."),
			),
			mcp.WithBoolean("force_inline",
				mcp.Description("Always return file content inline, bypassing the resource-link threshold. Default: false."),
			),
			mcp.WithNumber("max_inline_bytes",
				mcp.Description("Per-call inline threshold override in bytes. If set, overrides server resource_link_threshold_bytes for this call only. 0 = use server default."),
			),
		),
		s.handleFileRead,
	)

	// Register ast_query tool
	s.mcp.AddTool(
		mcp.NewTool("ast_query",
			mcp.WithDescription("Search for AST patterns in code files. Use code patterns with $VAR placeholders to match and capture code structures like functions, classes, and types. "+
				"Matching is keyword/kind-based, not fully structural: argument and return-type constraints in the pattern are not enforced — use name_exact/name_matches/kind_in to filter precisely. "+
				"Hidden dirs and dependency dirs (node_modules, vendor, bower_components) are skipped unless targeted explicitly via paths. "+
				"See resource filepuff://help/ast_query for flags and examples."),
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
				mcp.Description("Maximum number of results to return (default: 100, page size for pagination)"),
			),
			mcp.WithString("format",
				mcp.Description("Output format: \"compact\" (default, one line per match), \"verbose\" (full code+captures), \"location\" (file:line only)"),
			),
			mcp.WithString("cursor",
				mcp.Description("Pagination cursor from a previous truncated response. Pass back to fetch the next page."),
			),
			mcp.WithBoolean("verbose",
				mcp.Description("Emit \"Found N match(es):\" preamble. Default: false (v2 default)."),
			),
		),
		s.handleASTQuery,
	)

	// Register LSP-based tools if LSP is enabled
	if s.lspManager != nil {
		s.mcp.AddTool(
			mcp.NewTool("lsp_query",
				mcp.WithDescription("Query LSP for symbol info, definition, or references at a file position. "+
					"Give the position as line+column, or pass symbol_name to resolve it via AST (no line/column needed). "+
					"See resource filepuff://help/lsp_query for flags and examples."),
				mcp.WithReadOnlyHintAnnotation(true),
				mcp.WithString("action",
					mcp.Required(),
					mcp.Description("LSP operation: hover | definition | references"),
				),
				mcp.WithString("file",
					mcp.Required(),
					mcp.Description("Path to the file"),
				),
				mcp.WithString("symbol_name",
					mcp.Description("Resolve the position from a named symbol (function, struct, class, etc.) instead of line+column. Errors if the name is ambiguous or not found."),
				),
				mcp.WithString("symbol_kind",
					mcp.Description("Disambiguate symbol_name by kind when multiple symbols share the name. Accepted: function, method, struct, class, interface, type, enum, trait, constant, module."),
				),
				mcp.WithNumber("line",
					mcp.Description("Line number (1-indexed). Required unless symbol_name is given."),
				),
				mcp.WithNumber("column",
					mcp.Description("Column number (1-indexed). Required unless symbol_name is given."),
				),
				mcp.WithBoolean("include_declaration",
					mcp.Description("Include the declaration in results. Only valid for action=references (default: true)."),
				),
				mcp.WithBoolean("compact",
					mcp.Description("Compact output: one line per file with all refs in brackets. Only valid for action=references. Default: false."),
				),
				mcp.WithBoolean("verbose",
					mcp.Description("Emit count/header preamble. Applies to all actions. Default: false."),
				),
			),
			s.handleLSPQuery,
		)
	}

	// Register edit tools
	s.mcp.AddTool(
		mcp.NewTool("edit_apply",
			mcp.WithDescription("Apply an edit to a file. Uses AST-aware editing for code files (Go, TypeScript, JavaScript, Python, C, C++, Rust) with syntax validation, and text-based editing for other files (Markdown, JSON, YAML, config files, etc.). "+
				"See resource filepuff://help/edit_apply for flags and examples."),
			mcp.WithString("file",
				mcp.Required(),
				mcp.Description("Path to the file to edit"),
			),
			mcp.WithString("operation",
				mcp.Required(),
				mcp.Description("Edit operation: replace, insert_before, insert_after, delete"),
			),
			mcp.WithString("new_content",
				mcp.Description("New content (required for replace/insert operations). Inserted verbatim: send it as normal JSON (real newlines, quotes JSON-escaped as usual). The server does NOT re-interpret escape sequences, so backslash sequences in source code (e.g. \\n, \\t, \\\\ inside string literals, regexes, or Windows paths) are preserved exactly. Indentation is not auto-applied — include the leading whitespace you want. Line endings are normalized to the file's existing convention (LF or CRLF)."),
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
			mcp.WithString("response",
				mcp.Description("Response format: \"count\" (default, \"+3 -1\" line counts), \"diff\" (full unified diff), \"none\" (empty). Default: count."),
			),
			mcp.WithBoolean("compact_response",
				mcp.Description("Deprecated: use response=count. Alias for response=\"count\" kept for pre-v2 compatibility."),
			),
		),
		s.handleEditApply,
	)
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
