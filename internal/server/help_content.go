// Package server implements the MCP server for file operations.
package server

// helpFileRead is the full flag documentation and examples for the file_read tool,
// served at filepuff://help/file_read.
const helpFileRead = "# file_read — flags and examples\n\n" +
	"## Token-saving features\n\n" +
	"| Flag | Effect |\n" +
	"|------|--------|\n" +
	"| `previous_etag` | Skip re-reading unchanged files. Returns `[unchanged, etag: ...]` if file is unchanged. |\n" +
	"| `symbol_name` | Read only a named function/struct/class — eliminates an ast_query round-trip. |\n" +
	"| `symbols_only=true` | Return only symbol list (~95% fewer tokens). Requires `include_ast=true`. Alias: `mode='symbols_only'`. |\n" +
	"| `mode` | `full` (default) \\| `skeleton` (signatures + `{ ... }` stubs, bodies elided) \\| `symbols_only` |\n" +
	"| `strip` | Remove content classes before line-numbering: `imports`, `license`, `block_comments`. Emits `[stripped: ...]` footer. |\n" +
	"| `no_line_numbers=true` | Omit the `  12│ ` line-number prefix (~10% savings). `line_number_interval=0` has the same effect. |\n" +
	"| `line_number_interval=N` | Print line numbers only every N lines. |\n" +
	"| `compact_line_numbers=true` | Use compact `12│` prefix instead of `  12│ ` (no padding, no trailing space). |\n" +
	"| `collapse_blank_lines=true` | Collapse consecutive blank lines to one. |\n" +
	"| `max_lines=N` | Truncate output with omitted count notice. Applied after `line_start`/`line_end`. |\n" +
	"| `paths=[...]` | Read multiple files in one call. Each file gets a `--- path ---` header. |\n\n" +
	"All responses include `[etag: hex]` footer (8 hex chars) for use as `previous_etag` in subsequent reads.\n\n" +
	"## Examples\n\n" +
	"```json\n" +
	"// Full file\n" +
	`{"path": "main.go"}` + "\n\n" +
	"// Etag check — returns unchanged notice if file hasn't changed\n" +
	`{"path": "main.go", "previous_etag": "a3f9c2b1"}` + "\n\n" +
	"// Read only one named symbol\n" +
	`{"path": "server.go", "symbol_name": "handleFileRead"}` + "\n\n" +
	"// Skeleton mode — signatures only, bodies elided\n" +
	`{"path": "server.go", "mode": "skeleton"}` + "\n\n" +
	"// Strip imports and license header\n" +
	`{"path": "main.go", "strip": ["imports", "license"]}` + "\n\n" +
	"// Batch read multiple files\n" +
	`{"paths": ["a.go", "b.go"]}` + "\n\n" +
	"// Specific line range\n" +
	`{"path": "main.go", "line_start": 10, "line_end": 50}` + "\n" +
	"```\n"

// helpFileSearch is the full flag documentation and examples for the file_search tool,
// served at filepuff://help/file_search.
const helpFileSearch = "# file_search — flags and examples\n\n" +
	"## Output format\n\n" +
	"Matches grouped by file. Each file section has matching lines prefixed by `L{line}│` and context lines prefixed by `   │`. Zero matches: `No matches found.`\n\n" +
	"## Flags\n\n" +
	"| Flag | Effect |\n" +
	"|------|--------|\n" +
	"| `verbose=true` | Emit `Found N matches in M files:` preamble (v1 behaviour). Default: false. |\n" +
	"| `cluster=true` | Coalesce consecutive match lines into ranges (`L12-14│ text`). Drops context lines for density. |\n" +
	"| `cursor` | Opaque pagination token from a previous truncated response — fetches next page. |\n" +
	"| `max_results` | Page size for pagination. Re-run with `cursor` to get next page. |\n" +
	"| `context_lines` | Number of context lines around matches (default: 2). |\n" +
	"| `ignore_case` | Case-insensitive search. |\n" +
	"| `regex` | Treat pattern as regex (default: true). |\n" +
	"| `file_types` | Restrict to file extensions, e.g. `[\"go\", \"ts\"]`. |\n" +
	"| `paths` | Paths to search in (defaults to workspace root). |\n\n" +
	"## Examples\n\n" +
	"```json\n" +
	"// Search for error-returning functions in Go files\n" +
	`{"pattern": "func.*Error", "file_types": ["go"], "max_results": 20}` + "\n\n" +
	"// Case-insensitive literal search\n" +
	`{"pattern": "TODO", "ignore_case": true}` + "\n\n" +
	"// Paginated search — fetch next page\n" +
	`{"pattern": "import", "max_results": 50, "cursor": "<token from previous response>"}` + "\n\n" +
	"// Clustered — dense view of many matches\n" +
	`{"pattern": "return err", "file_types": ["go"], "cluster": true}` + "\n" +
	"```\n"

// helpASTQuery is the full flag documentation and examples for the ast_query tool,
// served at filepuff://help/ast_query.
const helpASTQuery = "# ast_query — flags and examples\n\n" +
	"## Output format\n\n" +
	"Entries in format `**file:line** (node_type)` with code blocks and captured variables (`$NAME=value`). Zero matches: `No matches found.`\n\n" +
	"## Flags\n\n" +
	"| Flag | Effect |\n" +
	"|------|--------|\n" +
	"| `verbose=true` | Emit `Found N match(es):` preamble (v1 behaviour). Default: false. |\n" +
	"| `format` | `compact` (default, one line per match) \\| `verbose` (full code+captures) \\| `location` (file:line only) |\n" +
	"| `cursor` | Opaque pagination token from a previous truncated response — fetches next page. |\n" +
	"| `max_results` | Page size (default: 100). |\n" +
	"| `name_exact` | Exact symbol name to match. |\n" +
	"| `name_matches` | Regex pattern to filter by name. |\n" +
	"| `kind_in` | Node types to match (e.g. `function_declaration`, `class_declaration`). |\n" +
	"| `paths` | Paths to search in (defaults to workspace root). |\n\n" +
	"## Pattern placeholders\n\n" +
	"| Placeholder | Meaning |\n" +
	"|-------------|----------|\n" +
	"| `$NAME` | Matches a single node, captures as `$NAME` |\n" +
	"| `$$$ARGS` | Matches zero or more nodes (variadic capture) |\n" +
	"| `$_` | Wildcard — matches any single node, no capture |\n\n" +
	"## Matching semantics (important)\n\n" +
	"Matching is keyword- and kind-based, not full structural matching. The leading keyword " +
	"(`func`/`function`/`class`/`struct`/`interface`) plus placeholders select the node KIND; " +
	"argument shapes, return types, and the literal bound to `$NAME` are NOT enforced as filters. " +
	"So `func $NAME($$$ARGS) error` matches every function/method, not only those returning `error`. " +
	"Captured `$VAR` values appear in output but do not constrain which nodes match.\n\n" +
	"For precise selection, combine the pattern with `name_exact`, `name_matches`, and/or `kind_in` — " +
	"these ARE applied as filters.\n\n" +
	"## Scope\n\n" +
	"The walk skips hidden directories (dot-prefixed) and dependency directories " +
	"(`node_modules`, `vendor`, `bower_components`) — these hold third-party code that would " +
	"flood results. To query inside one, name it explicitly in `paths` (e.g. `\"paths\": [\"vendor/foo\"]`); " +
	"the targeted root is never skipped. Note: unlike file_search, ast_query does not consult .gitignore.\n\n" +
	"## Examples\n\n" +
	"```json\n" +
	"// All Go functions/methods (the 'error' return is NOT enforced — add name_/kind_in to narrow)\n" +
	`{"pattern": "func $NAME($$$ARGS) error", "language": "go"}` + "\n\n" +
	"// Python classes\n" +
	`{"pattern": "class $NAME: $$$BODY", "language": "python"}` + "\n\n" +
	"// Specific named function\n" +
	`{"pattern": "func $NAME($$$ARGS)", "language": "go", "name_exact": "NewServer"}` + "\n\n" +
	"// Compact output — one line per match\n" +
	`{"pattern": "func $NAME($$$ARGS) error", "language": "go", "format": "compact"}` + "\n" +
	"```\n"

// helpLSPQuery is the full flag documentation and examples for the lsp_query tool,
// served at filepuff://help/lsp_query.
const helpLSPQuery = "# lsp_query — flags and examples\n\n" +
	"## Actions\n\n" +
	"### hover\n" +
	"Returns type/doc from LSP, falls back to AST node info. `verbose=true` adds `**Symbol Information**` header.\n\n" +
	"### definition\n" +
	"Returns `file:line:col` + 3-line code preview for each definition. `verbose=true` adds `Found N definition(s):` header.\n\n" +
	"### references\n" +
	"Returns references grouped by file. Flags:\n" +
	"- `include_declaration` (default true) — include the declaration itself\n" +
	"- `compact=true` — collapse to one line per file\n" +
	"- `verbose=true` — add `Found N reference(s):` header\n\n" +
	"Note: `include_declaration` and `compact` are errors when used with actions other than `references`.\n\n" +
	"## Position: line+column or symbol_name\n\n" +
	"Give the position directly with `line`+`column`, or pass `symbol_name` (optionally `symbol_kind`) to " +
	"resolve it from the AST — handy when you know the name but not the coordinates. Ambiguous or unknown " +
	"names return an error (with candidates / a 'did you mean' hint).\n\n" +
	"## Examples\n\n" +
	"```json\n" +
	"// Hover — type/doc at position\n" +
	`{"action": "hover", "file": "server.go", "line": 45, "column": 6}` + "\n\n" +
	"// Definition by symbol name — no line/column needed\n" +
	`{"action": "definition", "file": "server.go", "symbol_name": "NewServer"}` + "\n\n" +
	"// Definition — where is this symbol defined?\n" +
	`{"action": "definition", "file": "handler.go", "line": 23, "column": 10}` + "\n\n" +
	"// References — all usages\n" +
	`{"action": "references", "file": "types.go", "line": 5, "column": 6}` + "\n\n" +
	"// References — compact (one line per file)\n" +
	`{"action": "references", "file": "types.go", "line": 5, "column": 6, "compact": true}` + "\n" +
	"```\n"

// helpEditApply is the full flag documentation and examples for the edit_apply tool,
// served at filepuff://help/edit_apply.
const helpEditApply = "# edit_apply — flags and examples\n\n" +
	"## Response format (`response` flag)\n\n" +
	"| Value | Output |\n" +
	"|-------|--------|\n" +
	"| `count` (default) | `+3 -1` added/removed line counts only |\n" +
	"| `diff` | Full unified diff of changes made |\n" +
	"| `none` | Empty response (silent success) |\n\n" +
	"`compact_response=true` is a deprecated alias for `response=\"count\"` kept for pre-v2 compatibility.\n\n" +
	"For code files (Go, TypeScript, JavaScript, Python, C, C++, Rust) syntax is validated before writing — the edit is rejected if it would produce invalid syntax.\n\n" +
	"## new_content handling\n\n" +
	"`new_content` is inserted **verbatim**. Send it as normal JSON — real newlines (or JSON `\\n`), with quotes JSON-escaped as usual. The server does not re-interpret escape sequences, so literal backslash sequences in source (`\\n`, `\\t`, `\\\\`, regexes, Windows paths) are written exactly as given. Indentation is not auto-applied — include the leading whitespace you want. Line endings are normalized to the file's existing convention (LF or CRLF).\n\n" +
	"## Selector types\n\n" +
	"### AST-mode selectors (code files)\n" +
	"- `selector_kind` — AST node type (e.g. `function_declaration`, `class_declaration`)\n" +
	"- `selector_name` — symbol name to match\n\n" +
	"### Text-mode selectors (all files)\n" +
	"- `selector_text` — exact text to match (must be unique, or use `selector_index`)\n" +
	"- `selector_pattern` — regex pattern to match\n" +
	"- `selector_line` / `selector_line_end` — line range\n\n" +
	"### Shared\n" +
	"- `selector_index` — index of match when multiple exist (default: 0)\n\n" +
	"## Examples\n\n" +
	"```json\n" +
	"// AST mode — replace a named function\n" +
	"{\n" +
	`  "file": "main.go",` + "\n" +
	`  "operation": "replace",` + "\n" +
	`  "selector_kind": "function_declaration",` + "\n" +
	`  "selector_name": "Hello",` + "\n" +
	`  "new_content": "func Hello() {\n\treturn\n}"` + "\n" +
	"}\n\n" +
	"// Text mode — replace a markdown header\n" +
	"{\n" +
	`  "file": "README.md",` + "\n" +
	`  "operation": "replace",` + "\n" +
	`  "selector_text": "## Old Header",` + "\n" +
	`  "new_content": "## New Header"` + "\n" +
	"}\n\n" +
	"// Line range replacement\n" +
	"{\n" +
	`  "file": "config.yaml",` + "\n" +
	`  "operation": "replace",` + "\n" +
	`  "selector_line": 5,` + "\n" +
	`  "selector_line_end": 10,` + "\n" +
	`  "new_content": "key: value"` + "\n" +
	"}\n\n" +
	"// Request full diff in response\n" +
	"{\n" +
	`  "file": "main.go",` + "\n" +
	`  "operation": "replace",` + "\n" +
	`  "selector_name": "Hello",` + "\n" +
	`  "new_content": "func Hello() {}",` + "\n" +
	`  "response": "diff"` + "\n" +
	"}\n" +
	"```\n"
