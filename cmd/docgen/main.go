// Package main implements an API documentation generator for MCP tools.
package main

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// ToolParameter represents a parameter for an MCP tool.
type ToolParameter struct {
	Name        string
	Type        string
	Description string
	Required    bool
}

// Tool represents an MCP tool with its documentation.
type Tool struct {
	Name        string
	Description string
	Parameters  []ToolParameter
	Examples    []string
	Notes       []string
	ReadOnly    bool
	Category    string
}

// AllTools returns the complete list of MCP tools with their documentation.
func AllTools() []Tool {
	return []Tool{
		{
			Name:        "ping",
			Description: "Health check - returns pong to verify the server is running",
			Category:    "System",
			ReadOnly:    true,
			Parameters:  []ToolParameter{},
			Examples: []string{
				`{"tool": "ping"}`,
			},
			Notes: []string{
				"Returns: \"pong\"",
				"Use to verify server connectivity",
			},
		},
		{
			Name:        "file_search",
			Description: "Search for text patterns in files using ripgrep. Supports regex patterns, file type filtering, and context lines.",
			Category:    "Search",
			ReadOnly:    true,
			Parameters: []ToolParameter{
				{Name: "pattern", Type: "string", Required: true, Description: "The search pattern (regex by default)"},
				{Name: "paths", Type: "array[string]", Required: false, Description: "Paths to search in (defaults to workspace root)"},
				{Name: "file_types", Type: "array[string]", Required: false, Description: "File types to search (e.g., ['go', 'ts', 'py'])"},
				{Name: "ignore_case", Type: "boolean", Required: false, Description: "Case insensitive search"},
				{Name: "regex", Type: "boolean", Required: false, Description: "Treat pattern as regex (default: true)"},
				{Name: "context_lines", Type: "number", Required: false, Description: "Number of context lines around matches (default: 2)"},
				{Name: "max_results", Type: "number", Required: false, Description: "Maximum number of results to return"},
			},
			Examples: []string{
				`{"pattern": "func.*Error", "file_types": ["go"]}`,
				`{"pattern": "TODO", "ignore_case": true, "context_lines": 3}`,
			},
			Notes: []string{
				"Requires ripgrep (rg) to be installed",
				"Respects .gitignore by default",
			},
		},
		{
			Name:        "file_read",
			Description: "Read a file's contents with optional line range and AST symbol summary",
			Category:    "File Operations",
			ReadOnly:    true,
			Parameters: []ToolParameter{
				{Name: "path", Type: "string", Required: true, Description: "Path to the file to read"},
				{Name: "line_start", Type: "number", Required: false, Description: "Starting line number (1-indexed)"},
				{Name: "line_end", Type: "number", Required: false, Description: "Ending line number (inclusive)"},
				{Name: "include_ast", Type: "boolean", Required: false, Description: "Include AST symbol summary (functions, classes, types, etc.)"},
				{Name: "symbols_only", Type: "boolean", Required: false, Description: "Return only symbol summary without file content (token-efficient mode). Requires include_ast=true."},
				{Name: "max_lines", Type: "number", Required: false, Description: "Maximum number of lines to return (for token efficiency). Applied after line_start/line_end."},
			},
			Examples: []string{
				`{"path": "server.go", "include_ast": true}`,
				`{"path": "server.go", "include_ast": true, "symbols_only": true}`,
				`{"path": "server.go", "line_start": 10, "line_end": 50}`,
				`{"path": "large_file.go", "max_lines": 100}`,
			},
			Notes: []string{
				"symbols_only mode reduces token usage by ~90-98%",
				"max_lines truncates output with notification",
				"AST symbols show line numbers for quick navigation",
			},
		},
		{
			Name:        "ast_query",
			Description: "Search for AST patterns in code files. Use code patterns with $VAR placeholders to match and capture code structures like functions, classes, and types.",
			Category:    "AST Operations",
			ReadOnly:    true,
			Parameters: []ToolParameter{
				{Name: "pattern", Type: "string", Required: true, Description: "Code pattern with placeholders: $NAME (single), $$$ARGS (multiple), $_ (wildcard). Examples: 'func $NAME($$$ARGS) error', 'class $NAME { $$$BODY }'"},
				{Name: "language", Type: "string", Required: true, Description: "Target language: go, typescript, javascript, python, c, cpp"},
				{Name: "paths", Type: "array[string]", Required: false, Description: "Paths to search in (defaults to workspace root)"},
				{Name: "name_matches", Type: "string", Required: false, Description: "Regex pattern to filter by name"},
				{Name: "name_exact", Type: "string", Required: false, Description: "Exact name to match"},
				{Name: "kind_in", Type: "array[string]", Required: false, Description: "Node types to match (e.g., function_declaration, class_declaration)"},
				{Name: "max_results", Type: "number", Required: false, Description: "Maximum number of results to return (default: 100)"},
			},
			Examples: []string{
				`{"pattern": "func $NAME($$$ARGS) error", "language": "go"}`,
				`{"pattern": "class $NAME: $$$BODY", "language": "python"}`,
				`{"pattern": "function $NAME($PROPS) { $$$BODY }", "language": "javascript", "name_matches": "^[A-Z]"}`,
			},
			Notes: []string{
				"$NAME captures identifiers",
				"$$$ARGS captures multiple items (parameters, body, etc.)",
				"$_ is a wildcard that matches but doesn't capture",
				"Powered by Tree-sitter for accurate AST parsing",
			},
		},
		{
			Name:        "symbol_at",
			Description: "Get information about the symbol at a specific position in a file. Returns type, documentation, and definition location using LSP when available.",
			Category:    "LSP Operations",
			ReadOnly:    true,
			Parameters: []ToolParameter{
				{Name: "file", Type: "string", Required: true, Description: "Path to the file"},
				{Name: "line", Type: "number", Required: true, Description: "Line number (1-indexed)"},
				{Name: "column", Type: "number", Required: true, Description: "Column number (1-indexed)"},
			},
			Examples: []string{
				`{"file": "server.go", "line": 25, "column": 10}`,
			},
			Notes: []string{
				"Requires LSP server for full type information",
				"Falls back to AST-based info if LSP unavailable",
			},
		},
		{
			Name:        "find_definition",
			Description: "Find the definition of the symbol at a specific position. Uses LSP to locate where a function, variable, type, etc. is defined.",
			Category:    "LSP Operations",
			ReadOnly:    true,
			Parameters: []ToolParameter{
				{Name: "file", Type: "string", Required: true, Description: "Path to the file"},
				{Name: "line", Type: "number", Required: true, Description: "Line number (1-indexed)"},
				{Name: "column", Type: "number", Required: true, Description: "Column number (1-indexed)"},
			},
			Examples: []string{
				`{"file": "server.go", "line": 42, "column": 15}`,
			},
			Notes: []string{
				"Requires language server for the file type",
				"Returns file path, line, and column of definition",
				"Shows code preview at definition location",
			},
		},
		{
			Name:        "find_references",
			Description: "Find all references to the symbol at a specific position. Uses LSP to locate all usages of a function, variable, type, etc.",
			Category:    "LSP Operations",
			ReadOnly:    true,
			Parameters: []ToolParameter{
				{Name: "file", Type: "string", Required: true, Description: "Path to the file"},
				{Name: "line", Type: "number", Required: true, Description: "Line number (1-indexed)"},
				{Name: "column", Type: "number", Required: true, Description: "Column number (1-indexed)"},
				{Name: "include_declaration", Type: "boolean", Required: false, Description: "Include the declaration in results (default: true)"},
			},
			Examples: []string{
				`{"file": "server.go", "line": 42, "column": 15}`,
				`{"file": "server.go", "line": 42, "column": 15, "include_declaration": false}`,
			},
			Notes: []string{
				"Requires language server for the file type",
				"Results grouped by file",
			},
		},
		{
			Name:        "edit_apply",
			Description: "Apply an edit to a file. Uses AST-aware editing for code files (Go, TypeScript, JavaScript, Python, C, C++) with syntax validation, and text-based editing for other files (Markdown, JSON, YAML, config files, etc.).",
			Category:    "Edit Operations",
			ReadOnly:    false,
			Parameters: []ToolParameter{
				{Name: "file", Type: "string", Required: true, Description: "Path to the file to edit"},
				{Name: "operation", Type: "string", Required: true, Description: "Edit operation: replace, insert_before, insert_after, delete"},
				{Name: "new_content", Type: "string", Required: false, Description: "New content (required for replace/insert operations)"},
				{Name: "selector_kind", Type: "string", Required: false, Description: "AST node type to match (e.g., function_declaration, class_declaration). For code files only."},
				{Name: "selector_name", Type: "string", Required: false, Description: "Name of the symbol to match. For code files only."},
				{Name: "selector_line", Type: "number", Required: false, Description: "Line number (1-indexed). For AST mode: narrows search. For text mode: start of line range."},
				{Name: "selector_index", Type: "number", Required: false, Description: "Index of the match to use if multiple matches found (default: 0)"},
				{Name: "selector_line_end", Type: "number", Required: false, Description: "End line number for range selection (text mode). Used with selector_line."},
				{Name: "selector_text", Type: "string", Required: false, Description: "Exact text to match (text mode). Must be unique or use selector_index."},
				{Name: "selector_pattern", Type: "string", Required: false, Description: "Regex pattern to match (text mode). Must be unique or use selector_index."},
			},
			Examples: []string{
				`{"file": "server.go", "operation": "replace", "selector_kind": "function_declaration", "selector_name": "Hello", "new_content": "func Hello() {\\n\\tprintln(\\"Updated\\")\\n}"}`,
				`{"file": "config.yaml", "operation": "replace", "selector_line": 5, "selector_line_end": 10, "new_content": "database:\\n  host: production.db.example.com\\n  port: 5432"}`,
			},
			Notes: []string{
				"For code files: validates syntax before and after edit",
				"Preserves file permissions",
				"Uses atomic writes for safety",
				"File locking prevents concurrent edits",
			},
		},
	}
}

// GenerateMarkdown generates the complete API documentation in Markdown format.
func GenerateMarkdown() string {
	var sb strings.Builder

	sb.WriteString("# MCP Filepuff API Reference\n\n")
	sb.WriteString(fmt.Sprintf("> Auto-generated on %s\n\n", time.Now().Format("2006-01-02")))
	sb.WriteString("This document provides detailed API documentation for all MCP tools available in filepuff.\n\n")

	// Table of Contents
	sb.WriteString("## Table of Contents\n\n")
	tools := AllTools()
	categories := make(map[string][]Tool)
	categoryOrder := []string{}

	for _, tool := range tools {
		if _, ok := categories[tool.Category]; !ok {
			categoryOrder = append(categoryOrder, tool.Category)
		}
		categories[tool.Category] = append(categories[tool.Category], tool)
	}

	for _, cat := range categoryOrder {
		sb.WriteString(fmt.Sprintf("### %s\n", cat))
		for _, tool := range categories[cat] {
			sb.WriteString(fmt.Sprintf("- [`%s`](#%s)\n", tool.Name, tool.Name))
		}
		sb.WriteString("\n")
	}

	// Tool Documentation
	sb.WriteString("---\n\n")
	sb.WriteString("## Tool Reference\n\n")

	for _, cat := range categoryOrder {
		sb.WriteString(fmt.Sprintf("### %s\n\n", cat))

		for _, tool := range categories[cat] {
			sb.WriteString(fmt.Sprintf("#### `%s`\n\n", tool.Name))
			sb.WriteString(fmt.Sprintf("%s\n\n", tool.Description))

			// Read-only indicator
			if tool.ReadOnly {
				sb.WriteString("🔒 **Read-only**: This tool does not modify files.\n\n")
			} else {
				sb.WriteString("⚠️ **Modifies files**: This tool writes to the filesystem.\n\n")
			}

			// Parameters
			if len(tool.Parameters) > 0 {
				sb.WriteString("**Parameters:**\n\n")
				sb.WriteString("| Name | Type | Required | Description |\n")
				sb.WriteString("|------|------|----------|-------------|\n")
				for _, param := range tool.Parameters {
					required := "No"
					if param.Required {
						required = "**Yes**"
					}
					sb.WriteString(fmt.Sprintf("| `%s` | `%s` | %s | %s |\n",
						param.Name, param.Type, required, param.Description))
				}
				sb.WriteString("\n")
			} else {
				sb.WriteString("**Parameters:** None\n\n")
			}

			// Examples
			if len(tool.Examples) > 0 {
				sb.WriteString("**Examples:**\n\n")
				for _, example := range tool.Examples {
					sb.WriteString("```json\n")
					sb.WriteString(example)
					sb.WriteString("\n```\n\n")
				}
			}

			// Notes
			if len(tool.Notes) > 0 {
				sb.WriteString("**Notes:**\n\n")
				for _, note := range tool.Notes {
					sb.WriteString(fmt.Sprintf("- %s\n", note))
				}
				sb.WriteString("\n")
			}

			sb.WriteString("---\n\n")
		}
	}

	// Additional sections
	sb.WriteString("## Supported Languages\n\n")
	sb.WriteString("| Language | Extensions | Search | AST | LSP | Edit |\n")
	sb.WriteString("|----------|-----------|--------|-----|-----|------|\n")
	sb.WriteString("| Go | .go | ✅ | ✅ | gopls | ✅ |\n")
	sb.WriteString("| TypeScript | .ts, .tsx | ✅ | ✅ | typescript-language-server | ✅ |\n")
	sb.WriteString("| JavaScript | .js, .jsx, .mjs, .cjs | ✅ | ✅ | typescript-language-server | ✅ |\n")
	sb.WriteString("| Python | .py, .pyw | ✅ | ✅ | pylsp | ✅ |\n")
	sb.WriteString("| C | .c, .h | ✅ | ✅ | clangd | ✅ |\n")
	sb.WriteString("| C++ | .cpp, .cc, .cxx, .hpp, .hxx | ✅ | ✅ | clangd | ✅ |\n")
	sb.WriteString("| HTML | .html, .htm | ✅ | ✅ | - | ✅ |\n")
	sb.WriteString("| Vue | .vue | ✅ | ✅* | - | ✅ |\n")
	sb.WriteString("| React | .jsx, .tsx | ✅ | ✅ | typescript-language-server | ✅ |\n")
	sb.WriteString("| Elixir | .ex, .exs | ✅ | ✅ | elixir-ls | ✅ |\n")
	sb.WriteString("| JSON | .json | ✅ | ✅ | - | ✅ |\n")
	sb.WriteString("| YAML | .yaml, .yml | ✅ | ✅ | - | ✅ |\n")
	sb.WriteString("\n\\* Vue uses HTML parser for template sections\n\n")

	sb.WriteString("## Error Handling\n\n")
	sb.WriteString("All tools return structured errors with:\n")
	sb.WriteString("- **Error code**: Numeric identifier for the error type\n")
	sb.WriteString("- **Message**: Human-readable error description\n")
	sb.WriteString("- **Context**: Additional information about the error\n")
	sb.WriteString("- **Remediation**: Suggested fix for the error\n\n")
	sb.WriteString("See [ERROR_CODES.md](ERROR_CODES.md) for a complete error reference.\n\n")

	sb.WriteString("## See Also\n\n")
	sb.WriteString("- [README.md](../README.md) - Project overview and installation\n")
	sb.WriteString("- [ERROR_CODES.md](ERROR_CODES.md) - Error code reference\n")
	sb.WriteString("- [PERFORMANCE.md](PERFORMANCE.md) - Performance tuning guide\n")

	return sb.String()
}

func main() {
	output := GenerateMarkdown()

	// Default output path
	outputPath := "docs/API.md"
	if len(os.Args) > 1 {
		outputPath = os.Args[1]
	}

	// Ensure docs directory exists
	if err := os.MkdirAll("docs", 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating docs directory: %v\n", err)
		os.Exit(1)
	}

	if err := os.WriteFile(outputPath, []byte(output), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing file: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("API documentation generated: %s\n", outputPath)
}
