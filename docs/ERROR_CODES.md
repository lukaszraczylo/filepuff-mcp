# MCP Filepuff Error Codes Reference

This document provides a comprehensive reference for all error codes that can be returned by mcp-filepuff, along with descriptions and remediation steps.

## Error Code Structure

All errors in mcp-filepuff use structured error handling with the following components:

- **Error Code**: A numeric identifier (1000-1999) for programmatic handling
- **Message**: Human-readable description of the error
- **Context**: Additional key-value pairs providing error context
- **Remediation**: Suggested steps to resolve the error
- **Cause**: Underlying error (if wrapping another error)

## Error Categories

| Range | Category | Description |
|-------|----------|-------------|
| 1000-1099 | Search Errors | File search and ripgrep operations |
| 1100-1199 | Parser Errors | AST parsing and tree-sitter operations |
| 1200-1299 | LSP Errors | Language server protocol operations |
| 1300-1399 | Edit Errors | File editing operations |
| 1400-1499 | Query Errors | AST pattern matching operations |
| 1500-1599 | Config Errors | Configuration and validation |
| 1900-1999 | Internal Errors | Internal server errors |

---

## Search Errors (1000-1099)

### 1001 - ErrRipgrepNotFound

**Description**: The ripgrep (`rg`) binary is not found in the system PATH.

**Common Causes**:
- ripgrep is not installed
- ripgrep is not in PATH
- PATH environment variable not properly set

**Remediation**:

```bash
# macOS
brew install ripgrep

# Ubuntu/Debian
sudo apt install ripgrep

# Windows (Chocolatey)
choco install ripgrep

# Windows (Scoop)
scoop install ripgrep
```

---

### 1002 - ErrRipgrepTimeout

**Description**: Search operation exceeded the configured timeout limit.

**Context Fields**:
- `pattern`: The search pattern that timed out
- `duration`: How long the search ran before timing out

**Common Causes**:
- Search pattern too broad (e.g., `.` matching everything)
- Searching a very large directory tree
- Complex regex pattern causing backtracking

**Remediation**:
1. Use more specific search patterns
2. Narrow the search scope with `paths` parameter
3. Increase timeout via `MCP_SEARCH_TIMEOUT` environment variable:
   ```bash
   export MCP_SEARCH_TIMEOUT="2m"
   ```

---

### 1003 - ErrInvalidPattern

**Description**: The search pattern is invalid or malformed.

**Context Fields**:
- `pattern`: The invalid pattern
- `error`: Specific parsing error

**Common Causes**:
- Invalid regex syntax
- Unclosed brackets or parentheses
- Invalid escape sequences

**Remediation**:
1. Validate regex syntax at [regex101.com](https://regex101.com)
2. Escape special characters properly
3. Use `regex: false` for literal string searches

---

### 1004 - ErrSearchFailed

**Description**: General search operation failure.

**Context Fields**:
- `error`: Underlying error message

**Common Causes**:
- Permission denied on files/directories
- I/O errors
- ripgrep process crashed

**Remediation**:
1. Check file and directory permissions
2. Verify workspace path is accessible
3. Check system logs for I/O errors

---

### 1005 - ErrNoResults

**Description**: Search completed but found no matches.

**Note**: This is an informational code, not necessarily an error.

---

## Parser Errors (1100-1199)

### 1101 - ErrParserNotFound

**Description**: No Tree-sitter parser is available for the requested language.

**Context Fields**:
- `language`: The unsupported language

**Supported Languages**:
- Go, TypeScript, JavaScript, Python, C, C++, HTML, Vue, Elixir, JSON, YAML

**Remediation**:
1. Use a supported language
2. Check file extension is correct

---

### 1102 - ErrParseFailed

**Description**: Tree-sitter failed to parse the file.

**Context Fields**:
- `file`: Path to the file
- `language`: Detected language
- `error`: Specific parsing error

**Common Causes**:
- File has syntax errors
- File encoding is not UTF-8
- Binary file detected

**Remediation**:
1. Fix syntax errors in the source file
2. Ensure file is valid UTF-8
3. Check file is not a binary

---

### 1103 - ErrInvalidLanguage

**Description**: The specified or detected language is not supported.

**Context Fields**:
- `language`: The unsupported language
- `file`: File path (if detected from extension)

**Remediation**:
- Use a file with a supported extension
- Supported: `.go`, `.ts`, `.tsx`, `.js`, `.jsx`, `.py`, `.c`, `.h`, `.cpp`, `.hpp`, `.html`, `.vue`, `.ex`, `.exs`, `.json`, `.yaml`, `.yml`

---

### 1104 - ErrFileTooBig

**Description**: File exceeds the maximum size limit for parsing.

**Context Fields**:
- `file`: Path to the file
- `size_bytes`: Actual file size
- `limit_bytes`: Maximum allowed size

**Default Limit**: 10 MB

**Remediation**:
1. Process smaller files
2. Increase limit via configuration:
   ```json
   {
     "max_parse_size": 20971520
   }
   ```

---

### 1105 - ErrInvalidSyntax

**Description**: File contains syntax errors that prevent proper parsing.

**Context Fields**:
- `file`: Path to the file
- `errors`: List of syntax errors with locations

**Remediation**:
1. Fix syntax errors in the source file
2. Run language-specific linters/compilers to identify issues

---

## LSP Errors (1200-1299)

### 1201 - ErrLSPServerNotFound

**Description**: The LSP server for the requested language is not installed or not in PATH.

**Context Fields**:
- `language`: The language needing LSP
- `server`: The expected server binary name

**LSP Servers by Language**:
| Language | Server | Installation |
|----------|--------|--------------|
| Go | `gopls` | `go install golang.org/x/tools/gopls@latest` |
| TypeScript/JavaScript | `typescript-language-server` | `npm install -g typescript-language-server typescript` |
| Python | `pylsp` | `pip install python-lsp-server` |
| C/C++ | `clangd` | Install via system package manager |

**Remediation**:
Install the appropriate language server for your language.

---

### 1202 - ErrLSPInitFailed

**Description**: LSP server failed to initialize.

**Context Fields**:
- `language`: The language
- `command`: The server command
- `error`: Specific initialization error

**Common Causes**:
- Workspace configuration issues
- Missing dependencies for the project
- LSP server crashed during startup

**Remediation**:
1. Check LSP server logs
2. Verify workspace has proper configuration files (go.mod, package.json, etc.)
3. Try running the LSP server manually to see errors

---

### 1203 - ErrLSPTimeout

**Description**: LSP operation exceeded timeout.

**Context Fields**:
- `operation`: The LSP method that timed out
- `duration`: How long the operation ran

**Common Causes**:
- LSP server indexing large project
- Complex type analysis
- Server deadlock

**Remediation**:
1. Wait for LSP server to complete initial indexing
2. Increase timeout via `MCP_LSP_TIMEOUT`:
   ```bash
   export MCP_LSP_TIMEOUT="10m"
   ```
3. Restart the MCP server to restart LSP servers

---

### 1204 - ErrLSPCommunication

**Description**: Communication error with LSP server.

**Context Fields**:
- `error`: Specific communication error

**Common Causes**:
- LSP server crashed
- Broken pipe (server terminated)
- Protocol version mismatch

**Remediation**:
1. Restart the MCP server
2. Update LSP server to latest version
3. Check for known issues with your LSP server version

---

### 1205 - ErrNoHoverInfo

**Description**: No hover information available at the requested position.

**Note**: Not all positions have hover information (e.g., whitespace, keywords).

---

### 1206 - ErrNoDefinition

**Description**: No definition found for the symbol at the requested position.

**Common Causes**:
- Position is not on a symbol
- Symbol is a built-in
- External dependency without source

---

### 1207 - ErrNoReferences

**Description**: No references found for the symbol.

**Note**: A symbol may have no references if unused.

---

## Edit Errors (1300-1399)

### 1301 - ErrEditFailed

**Description**: General edit operation failure.

**Context Fields**:
- `file`: Path to the file
- `operation`: The edit operation attempted
- `error`: Specific error details

---

### 1302 - ErrInvalidEdit

**Description**: The edit request is malformed or invalid.

**Context Fields**:
- `message`: Specific validation failure

**Common Causes**:
- Missing required fields
- Invalid operation type
- Missing `new_content` for replace/insert

**Valid Operations**:
- `replace` - Replace selected content
- `insert_before` - Insert before selection
- `insert_after` - Insert after selection
- `delete` - Delete selected content

**Remediation**:
Review the edit request and ensure all required fields are provided.

---

### 1303 - ErrFileNotFound

**Description**: The target file does not exist.

**Context Fields**:
- `file`: Path that was not found

**Remediation**:
1. Verify the file path is correct
2. Check for typos in the path
3. Ensure the file exists in the workspace

---

### 1304 - ErrFileNotReadable

**Description**: The file exists but cannot be read.

**Context Fields**:
- `file`: Path to the file
- `error`: System error details

**Common Causes**:
- Permission denied
- File locked by another process
- File is a directory

**Remediation**:
1. Check file permissions
2. Close other programs using the file
3. On macOS, ensure terminal has disk access

---

### 1305 - ErrFileNotWritable

**Description**: Cannot write to the file.

**Context Fields**:
- `file`: Path to the file
- `error`: System error details

**Common Causes**:
- Permission denied
- Read-only filesystem
- Disk full
- File locked

**Remediation**:
1. Check file and directory permissions
2. Verify disk space
3. Close other programs using the file

---

### 1306 - ErrNodeNotFound

**Description**: No AST node matches the selector criteria.

**Context Fields**:
- `selector`: Description of the selector criteria

**Common Causes**:
- Selector doesn't match any code
- Wrong node kind specified
- Name doesn't exist in file

**Remediation**:
1. Use `file_read` with `include_ast: true` to see available symbols
2. Verify selector criteria (kind, name, pattern, line)
3. Check spelling of symbol names

---

### 1307 - ErrValidationFailed

**Description**: Edit would produce invalid syntax.

**Context Fields**:
- `file`: Path to the file
- `error`: Syntax error details

**Common Causes**:
- `new_content` has syntax errors
- Edit breaks surrounding code structure

**Remediation**:
1. Validate `new_content` syntax independently
2. Use `edit_preview` to see the proposed changes
3. Ensure the edit maintains valid syntax

---

### 1308 - ErrInvalidSelection

**Description**: Selector matches multiple nodes or is ambiguous.

**Context Fields**:
- `message`: Details about the ambiguity

**Common Causes**:
- Text/pattern matches multiple locations
- Multiple functions with same name

**Remediation**:
1. Add `selector_index` to choose specific match
2. Add more selector criteria (kind, name, line)
3. Use line number to narrow selection

---

## Query Errors (1400-1499)

### 1401 - ErrInvalidQuery

**Description**: The AST query is malformed.

**Context Fields**:
- `query`: The invalid query
- `error`: Specific parsing error

**Remediation**:
Review query syntax. Valid patterns use:
- `$NAME` for single captures
- `$$$ARGS` for multiple items
- `$_` for wildcards

---

### 1402 - ErrQueryTimeout

**Description**: AST query exceeded timeout.

**Context Fields**:
- `query`: The query that timed out
- `duration`: How long it ran

**Remediation**:
1. Use more specific patterns
2. Narrow search paths
3. Reduce `max_results`

---

### 1403 - ErrNoMatches

**Description**: Query executed successfully but found no matches.

**Note**: Informational, not necessarily an error.

---

### 1404 - ErrQueryCompile

**Description**: Failed to compile the query pattern.

**Context Fields**:
- `pattern`: The invalid pattern
- `error`: Compilation error

---

## Config Errors (1500-1599)

### 1501 - ErrInvalidConfig

**Description**: Configuration file or value is invalid.

**Context Fields**:
- `field`: The invalid field
- `value`: The invalid value
- `error`: Specific validation error

**Remediation**:
Review `.mcp-filepuff.json` configuration file syntax and values.

---

### 1502 - ErrPathNotAllowed

**Description**: Requested path is outside the workspace root.

**Context Fields**:
- `path`: The requested path
- `workspace`: The workspace root

**Security Note**: This prevents path traversal attacks.

**Remediation**:
1. Ensure all paths are within workspace
2. Don't use `..` to escape workspace
3. Configure workspace root appropriately

---

### 1503 - ErrWorkspaceNotSet

**Description**: Workspace root is not configured.

**Remediation**:
Start the server with `-workspace` flag:
```bash
mcp-filepuff -workspace /path/to/workspace
```

---

## Internal Errors (1900-1999)

### 1900 - ErrInternal

**Description**: Unexpected internal error.

**Context Fields**:
- `error`: Error details
- `stack`: Stack trace (in debug mode)

**Remediation**:
1. Check server logs
2. Report issue if reproducible

---

### 1901 - ErrCacheFailed

**Description**: Cache operation failed.

**Context Fields**:
- `operation`: Cache operation that failed
- `error`: Specific error

---

### 1902 - ErrConcurrency

**Description**: Concurrency-related error (race condition, deadlock).

**Context Fields**:
- `operation`: Operation that failed
- `error`: Specific error

---

## Common Error Scenarios

### Scenario 1: New Project Setup

**Symptoms**: LSP features not working, errors like "LSP server not found"

**Solution**:
1. Install required language servers
2. Initialize project (e.g., `go mod init`, `npm init`)
3. Restart MCP server

### Scenario 2: Edit Fails with Validation Error

**Symptoms**: Edit operation rejected with syntax error

**Solution**:
1. Use `edit_preview` first to see proposed changes
2. Validate `new_content` is syntactically correct
3. Check that surrounding code structure is maintained

### Scenario 3: Search Returns Too Many Results

**Symptoms**: Search timeout or truncated results

**Solution**:
1. Use more specific patterns
2. Add file type filters: `"file_types": ["go"]`
3. Limit search paths
4. Set `max_results` limit

### Scenario 4: File Access Errors

**Symptoms**: Permission denied, file not readable

**Solution**:
1. Check file permissions: `ls -la <file>`
2. On macOS, grant disk access to terminal
3. Close programs locking the file
4. Run server with appropriate user permissions

---

## See Also

- [API.md](API.md) - Complete API reference
- [PERFORMANCE.md](PERFORMANCE.md) - Performance tuning guide
- [README.md](../README.md) - Getting started
