# MCP Filepuff API Reference

> Auto-generated on 2026-01-28

This document provides detailed API documentation for all MCP tools available in filepuff.

## Table of Contents

### System
- [`ping`](#ping)

### Search
- [`file_search`](#file_search)

### File Operations
- [`file_read`](#file_read)

### AST Operations
- [`ast_query`](#ast_query)

### LSP Operations
- [`symbol_at`](#symbol_at)
- [`find_definition`](#find_definition)
- [`find_references`](#find_references)

### Edit Operations
- [`edit_preview`](#edit_preview)
- [`edit_apply`](#edit_apply)

---

## Tool Reference

### System

#### `ping`

Health check - returns pong to verify the server is running

🔒 **Read-only**: This tool does not modify files.

**Parameters:** None

**Examples:**

```json
{"tool": "ping"}
```

**Notes:**

- Returns: "pong"
- Use to verify server connectivity

---

### Search

#### `file_search`

Search for text patterns in files using ripgrep. Supports regex patterns, file type filtering, and context lines.

🔒 **Read-only**: This tool does not modify files.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `pattern` | `string` | **Yes** | The search pattern (regex by default) |
| `paths` | `array[string]` | No | Paths to search in (defaults to workspace root) |
| `file_types` | `array[string]` | No | File types to search (e.g., ['go', 'ts', 'py']) |
| `ignore_case` | `boolean` | No | Case insensitive search |
| `regex` | `boolean` | No | Treat pattern as regex (default: true) |
| `context_lines` | `number` | No | Number of context lines around matches (default: 2) |
| `max_results` | `number` | No | Maximum number of results to return |

**Examples:**

```json
{"pattern": "func.*Error", "file_types": ["go"]}
```

```json
{"pattern": "TODO", "ignore_case": true, "context_lines": 3}
```

**Notes:**

- Requires ripgrep (rg) to be installed
- Respects .gitignore by default

---

### File Operations

#### `file_read`

Read a file's contents with optional line range and AST symbol summary

🔒 **Read-only**: This tool does not modify files.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `path` | `string` | **Yes** | Path to the file to read |
| `line_start` | `number` | No | Starting line number (1-indexed) |
| `line_end` | `number` | No | Ending line number (inclusive) |
| `include_ast` | `boolean` | No | Include AST symbol summary (functions, classes, types, etc.) |
| `symbols_only` | `boolean` | No | Return only symbol summary without file content (token-efficient mode). Requires include_ast=true. |
| `max_lines` | `number` | No | Maximum number of lines to return (for token efficiency). Applied after line_start/line_end. |

**Examples:**

```json
{"path": "server.go", "include_ast": true}
```

```json
{"path": "server.go", "include_ast": true, "symbols_only": true}
```

```json
{"path": "server.go", "line_start": 10, "line_end": 50}
```

```json
{"path": "large_file.go", "max_lines": 100}
```

**Notes:**

- symbols_only mode reduces token usage by ~90-98%
- max_lines truncates output with notification
- AST symbols show line numbers for quick navigation

---

### AST Operations

#### `ast_query`

Search for AST patterns in code files. Use code patterns with $VAR placeholders to match and capture code structures like functions, classes, and types.

🔒 **Read-only**: This tool does not modify files.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `pattern` | `string` | **Yes** | Code pattern with placeholders: $NAME (single), $$$ARGS (multiple), $_ (wildcard). Examples: 'func $NAME($$$ARGS) error', 'class $NAME { $$$BODY }' |
| `language` | `string` | **Yes** | Target language: go, typescript, javascript, python, c, cpp |
| `paths` | `array[string]` | No | Paths to search in (defaults to workspace root) |
| `name_matches` | `string` | No | Regex pattern to filter by name |
| `name_exact` | `string` | No | Exact name to match |
| `kind_in` | `array[string]` | No | Node types to match (e.g., function_declaration, class_declaration) |
| `max_results` | `number` | No | Maximum number of results to return (default: 100) |

**Examples:**

```json
{"pattern": "func $NAME($$$ARGS) error", "language": "go"}
```

```json
{"pattern": "class $NAME: $$$BODY", "language": "python"}
```

```json
{"pattern": "function $NAME($PROPS) { $$$BODY }", "language": "javascript", "name_matches": "^[A-Z]"}
```

**Notes:**

- $NAME captures identifiers
- $$$ARGS captures multiple items (parameters, body, etc.)
- $_ is a wildcard that matches but doesn't capture
- Powered by Tree-sitter for accurate AST parsing

---

### LSP Operations

#### `symbol_at`

Get information about the symbol at a specific position in a file. Returns type, documentation, and definition location using LSP when available.

🔒 **Read-only**: This tool does not modify files.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `file` | `string` | **Yes** | Path to the file |
| `line` | `number` | **Yes** | Line number (1-indexed) |
| `column` | `number` | **Yes** | Column number (1-indexed) |

**Examples:**

```json
{"file": "server.go", "line": 25, "column": 10}
```

**Notes:**

- Requires LSP server for full type information
- Falls back to AST-based info if LSP unavailable

---

#### `find_definition`

Find the definition of the symbol at a specific position. Uses LSP to locate where a function, variable, type, etc. is defined.

🔒 **Read-only**: This tool does not modify files.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `file` | `string` | **Yes** | Path to the file |
| `line` | `number` | **Yes** | Line number (1-indexed) |
| `column` | `number` | **Yes** | Column number (1-indexed) |

**Examples:**

```json
{"file": "server.go", "line": 42, "column": 15}
```

**Notes:**

- Requires language server for the file type
- Returns file path, line, and column of definition
- Shows code preview at definition location

---

#### `find_references`

Find all references to the symbol at a specific position. Uses LSP to locate all usages of a function, variable, type, etc.

🔒 **Read-only**: This tool does not modify files.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `file` | `string` | **Yes** | Path to the file |
| `line` | `number` | **Yes** | Line number (1-indexed) |
| `column` | `number` | **Yes** | Column number (1-indexed) |
| `include_declaration` | `boolean` | No | Include the declaration in results (default: true) |

**Examples:**

```json
{"file": "server.go", "line": 42, "column": 15}
```

```json
{"file": "server.go", "line": 42, "column": 15, "include_declaration": false}
```

**Notes:**

- Requires language server for the file type
- Results grouped by file

---

### Edit Operations

#### `edit_preview`

Preview an edit without applying it. Uses AST-aware editing for code files (Go, TypeScript, JavaScript, Python, C, C++), and text-based editing for other files (Markdown, JSON, YAML, config files, etc.).

🔒 **Read-only**: This tool does not modify files.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `file` | `string` | **Yes** | Path to the file to edit |
| `operation` | `string` | **Yes** | Edit operation: replace, insert_before, insert_after, delete |
| `new_content` | `string` | No | New content (required for replace/insert operations) |
| `selector_kind` | `string` | No | AST node type to match (e.g., function_declaration, class_declaration). For code files only. |
| `selector_name` | `string` | No | Name of the symbol to match. For code files only. |
| `selector_line` | `number` | No | Line number (1-indexed). For AST mode: narrows search. For text mode: start of line range. |
| `selector_index` | `number` | No | Index of the match to use if multiple matches found (default: 0) |
| `selector_line_end` | `number` | No | End line number for range selection (text mode). Used with selector_line. |
| `selector_text` | `string` | No | Exact text to match (text mode). Must be unique or use selector_index. |
| `selector_pattern` | `string` | No | Regex pattern to match (text mode). Must be unique or use selector_index. |

**Examples:**

```json
{"file": "server.go", "operation": "replace", "selector_kind": "function_declaration", "selector_name": "Hello", "new_content": "func Hello() {\\n\\tprintln(\\"New Hello\\")\\n}"}
```

```json
{"file": "README.md", "operation": "replace", "selector_text": "## Installation", "new_content": "## Getting Started"}
```

```json
{"file": "package.json", "operation": "replace", "selector_pattern": "\\"version\\":\\\\s*\\"[^\\"]+\\"", "new_content": "\\"version\\": \\"2.0.0\\""}
```

**Notes:**

- Returns a diff showing proposed changes
- Does not modify the file
- Use to validate changes before applying

---

#### `edit_apply`

Apply an edit to a file. Uses AST-aware editing for code files (Go, TypeScript, JavaScript, Python, C, C++) with syntax validation, and text-based editing for other files (Markdown, JSON, YAML, config files, etc.).

⚠️ **Modifies files**: This tool writes to the filesystem.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `file` | `string` | **Yes** | Path to the file to edit |
| `operation` | `string` | **Yes** | Edit operation: replace, insert_before, insert_after, delete |
| `new_content` | `string` | No | New content (required for replace/insert operations) |
| `selector_kind` | `string` | No | AST node type to match (e.g., function_declaration, class_declaration). For code files only. |
| `selector_name` | `string` | No | Name of the symbol to match. For code files only. |
| `selector_line` | `number` | No | Line number (1-indexed). For AST mode: narrows search. For text mode: start of line range. |
| `selector_index` | `number` | No | Index of the match to use if multiple matches found (default: 0) |
| `selector_line_end` | `number` | No | End line number for range selection (text mode). Used with selector_line. |
| `selector_text` | `string` | No | Exact text to match (text mode). Must be unique or use selector_index. |
| `selector_pattern` | `string` | No | Regex pattern to match (text mode). Must be unique or use selector_index. |

**Examples:**

```json
{"file": "server.go", "operation": "replace", "selector_kind": "function_declaration", "selector_name": "Hello", "new_content": "func Hello() {\\n\\tprintln(\\"Updated\\")\\n}"}
```

```json
{"file": "config.yaml", "operation": "replace", "selector_line": 5, "selector_line_end": 10, "new_content": "database:\\n  host: production.db.example.com\\n  port: 5432"}
```

**Notes:**

- For code files: validates syntax before and after edit
- Preserves file permissions
- Uses atomic writes for safety
- File locking prevents concurrent edits

---

## Supported Languages

| Language | Extensions | Search | AST | LSP | Edit |
|----------|-----------|--------|-----|-----|------|
| Go | .go | ✅ | ✅ | gopls | ✅ |
| TypeScript | .ts, .tsx | ✅ | ✅ | typescript-language-server | ✅ |
| JavaScript | .js, .jsx, .mjs, .cjs | ✅ | ✅ | typescript-language-server | ✅ |
| Python | .py, .pyw | ✅ | ✅ | pylsp | ✅ |
| C | .c, .h | ✅ | ✅ | clangd | ✅ |
| C++ | .cpp, .cc, .cxx, .hpp, .hxx | ✅ | ✅ | clangd | ✅ |
| HTML | .html, .htm | ✅ | ✅ | - | ✅ |
| Vue | .vue | ✅ | ✅* | - | ✅ |
| React | .jsx, .tsx | ✅ | ✅ | typescript-language-server | ✅ |
| Elixir | .ex, .exs | ✅ | ✅ | elixir-ls | ✅ |
| JSON | .json | ✅ | ✅ | - | ✅ |
| YAML | .yaml, .yml | ✅ | ✅ | - | ✅ |

\* Vue uses HTML parser for template sections

## Error Handling

All tools return structured errors with:
- **Error code**: Numeric identifier for the error type
- **Message**: Human-readable error description
- **Context**: Additional information about the error
- **Remediation**: Suggested fix for the error

See [ERROR_CODES.md](ERROR_CODES.md) for a complete error reference.

## See Also

- [README.md](../README.md) - Project overview and installation
- [ERROR_CODES.md](ERROR_CODES.md) - Error code reference
- [PERFORMANCE.md](PERFORMANCE.md) - Performance tuning guide
