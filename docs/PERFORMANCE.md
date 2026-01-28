# MCP Filepuff Performance Tuning Guide

This guide provides detailed information on optimizing mcp-filepuff performance, understanding resource usage, and configuring the server for your workload.

## Table of Contents

- [Parser Cache Configuration](#parser-cache-configuration)
- [File Size Limits](#file-size-limits)
- [LSP Configuration](#lsp-configuration)
- [Memory Usage Patterns](#memory-usage-patterns)
- [Benchmarking](#benchmarking)
- [Production Recommendations](#production-recommendations)

---

## Parser Cache Configuration

The parser cache is critical for performance as it avoids re-parsing files that haven't changed.

### How the Cache Works

```
┌─────────────────────────────────────────────────────────────┐
│                      Parse Request                          │
│                    (file, content)                          │
└─────────────────────────┬───────────────────────────────────┘
                          │
                          ▼
               ┌──────────────────────┐
               │  Content Hash Check  │
               │    (xxHash64)        │
               └──────────┬───────────┘
                          │
              ┌───────────┴───────────┐
              │                       │
              ▼                       ▼
        Cache Hit               Cache Miss
              │                       │
              ▼                       ▼
     Return cached tree      Parse with Tree-sitter
              │                       │
              │                       ▼
              │                 Store in LRU cache
              │                       │
              └───────────┬───────────┘
                          │
                          ▼
                   Return ParseResult
```

### Cache Statistics

The parser tracks detailed statistics:

```go
type CacheStatsResult struct {
    Hits           int64   // Number of cache hits
    Misses         int64   // Number of cache misses
    HitRate        float64 // Ratio of hits to total requests
    Size           int     // Current number of cached items
    TotalParseTime int64   // Total time spent parsing (nanoseconds)
    ParseCount     int64   // Number of parse operations
    AvgParseTime   int64   // Average parse time (nanoseconds)
    LastParseTime  int64   // Most recent parse duration
}
```

### Cache Configuration

The LRU cache holds up to **100 parsed AST trees** by default. This is sufficient for most development workflows where you interact with a subset of files.

**Cache Key**: xxHash64 of file content (extremely fast, ~5GB/s)

**Eviction Policy**: Least Recently Used (LRU) - when the cache is full, the least recently accessed entry is evicted.

### Optimizing Cache Performance

1. **Batch Related Operations**: When working on related files, perform all operations on one file before moving to the next. This maximizes cache hits.

2. **Monitor Hit Rate**: A healthy cache has >80% hit rate. Lower rates suggest:
   - Working with too many files simultaneously
   - Files changing frequently between operations

3. **Cache Invalidation**: The cache is content-based (hash), so modified files automatically get re-parsed.

---

## File Size Limits

### Default Limits

| Limit | Default Value | Environment Variable |
|-------|---------------|---------------------|
| Max File Size | 10 MB | - |
| Max Parse Size | 10 MB | - |
| Max Edit Size | 100 KB | - |
| Max Search Results | 1000 | - |

### Configuration

Configure via `.mcp-filepuff.json` in workspace root:

```json
{
  "max_file_size": 10485760,
  "max_parse_size": 10485760,
  "max_search_results": 1000,
  "max_edit_size": 102400
}
```

### Understanding Limits

**Max File Size (10 MB)**
- Maximum file size that can be read via `file_read`
- Prevents memory exhaustion with large files
- Increase for codebases with large generated files

**Max Parse Size (10 MB)**
- Maximum file size for AST parsing
- Tree-sitter parsing memory usage is ~3-5x file size
- A 10 MB file needs ~30-50 MB RAM for parsing

**Max Edit Size (100 KB)**
- Maximum size for files being edited
- Keeps diff generation fast
- Prevents accidental edits to large generated files

### Token-Efficient Reading

For large files, use token-efficient options:

```json
// Get only symbol summary (~90-98% token reduction)
{"path": "large_file.go", "include_ast": true, "symbols_only": true}

// Limit output lines
{"path": "large_file.go", "max_lines": 50}

// Read specific line range
{"path": "large_file.go", "line_start": 100, "line_end": 150}
```

---

## LSP Configuration

### Timeout Configuration

```bash
# LSP operation timeout (default: 5 minutes)
export MCP_LSP_TIMEOUT="5m"

# Search timeout (default: 30 seconds)
export MCP_SEARCH_TIMEOUT="30s"
```

### LSP Server Lifecycle

```
┌─────────────────────────────────────────────────────────────┐
│                     LSP Request                             │
│              (hover, definition, references)                │
└─────────────────────────┬───────────────────────────────────┘
                          │
                          ▼
               ┌──────────────────────┐
               │  Check Server Pool   │
               │   (by language)      │
               └──────────┬───────────┘
                          │
              ┌───────────┴───────────┐
              │                       │
              ▼                       ▼
       Server Exists           No Server
              │                       │
              ▼                       ▼
       Update lastUsed         Start New Server
              │                       │
              │                       ▼
              │              Initialize (handshake)
              │                       │
              └───────────┬───────────┘
                          │
                          ▼
                 ┌─────────────────┐
                 │ Open Document   │
                 │ (if not open)   │
                 └────────┬────────┘
                          │
                          ▼
                 ┌─────────────────┐
                 │  Execute LSP    │
                 │    Request      │
                 └────────┬────────┘
                          │
                          ▼
                   Return Result
```

### Server Pool Management

- **Idle Timeout**: 5 minutes (servers closed after inactivity)
- **Pool Reaper**: Checks every 60 seconds for idle servers
- **One Server Per Language**: Efficient resource usage

### Optimizing LSP Performance

1. **First Request Latency**: Initial LSP requests are slow due to server startup and project indexing. Subsequent requests are fast.

2. **gopls Optimization**: For Go projects, gopls performance depends on module cache:
   ```bash
   # Pre-populate module cache
   go mod download
   ```

3. **typescript-language-server**: Ensure `node_modules` is populated:
   ```bash
   npm install
   ```

4. **clangd**: Requires `compile_commands.json` for best results:
   ```bash
   # Generate with CMake
   cmake -DCMAKE_EXPORT_COMPILE_COMMANDS=ON .
   ```

---

## Memory Usage Patterns

### Component Memory Usage

| Component | Memory Pattern | Notes |
|-----------|---------------|-------|
| Parser Registry | Per-language parsers | ~5-10 MB per language |
| AST Cache | LRU, 100 entries max | ~50-200 MB typically |
| LSP Servers | External processes | ~100-500 MB per server |
| Search (ripgrep) | Streaming | Minimal memory |
| Edit Engine | Per-operation | Proportional to file size |

### Memory Calculation Example

For a typical Go project:

```
Base Server:           ~20 MB
Go Parser:             ~10 MB
AST Cache (50 files):  ~100 MB
gopls:                 ~300 MB
────────────────────────────────
Total:                 ~430 MB
```

### Reducing Memory Usage

1. **Disable LSP**: If you don't need go-to-definition/references:
   ```bash
   export MCP_ENABLE_LSP="false"
   ```
   This saves ~100-500 MB per language server.

2. **Reduce Cache Size**: For memory-constrained environments, you can recompile with a smaller cache size (requires code change).

3. **Close Idle Servers**: LSP servers are automatically closed after 5 minutes of inactivity.

---

## Benchmarking

### Running Benchmarks

The project includes comprehensive benchmarks:

```bash
# Run all benchmarks
go test -bench=. ./...

# Run parser benchmarks with memory stats
go test -bench=. -benchmem ./internal/parser/...

# Run with specific count for stability
go test -bench=. -count=5 ./internal/parser/...
```

### Available Benchmarks

**Parser Benchmarks** (`internal/parser/parser_bench_test.go`):
- `BenchmarkParseGo` - Go file parsing
- `BenchmarkParseTypeScript` - TypeScript file parsing
- `BenchmarkParsePython` - Python file parsing
- `BenchmarkParseC` - C file parsing
- `BenchmarkParseCpp` - C++ file parsing
- `BenchmarkCacheHit` - Cache hit performance
- `BenchmarkCacheMiss` - Cache miss performance
- `BenchmarkContentHash` - xxHash performance
- `BenchmarkExtractSymbols` - Symbol extraction

### Expected Performance

Typical benchmark results (M1 Mac):

```
BenchmarkParseGo-8           5000    220000 ns/op    45000 B/op    850 allocs/op
BenchmarkParseTypeScript-8   3000    380000 ns/op    62000 B/op   1200 allocs/op
BenchmarkCacheHit-8        500000      2400 ns/op      128 B/op      3 allocs/op
BenchmarkContentHash-8    2000000       600 ns/op        0 B/op      0 allocs/op
```

Key observations:
- Cache hits are **~100x faster** than cache misses
- Content hashing is extremely fast (xxHash64)
- Parsing speed varies by language complexity

### Profiling

```bash
# CPU profiling
go test -bench=BenchmarkParseGo -cpuprofile=cpu.prof ./internal/parser/...
go tool pprof cpu.prof

# Memory profiling
go test -bench=BenchmarkParseGo -memprofile=mem.prof ./internal/parser/...
go tool pprof mem.prof

# Generate flame graph (requires pprof)
go tool pprof -http=:8080 cpu.prof
```

---

## Production Recommendations

### Environment Variables

```bash
# Essential configuration
export MCP_WORKSPACE_ROOT="/path/to/workspace"
export MCP_LSP_TIMEOUT="5m"
export MCP_SEARCH_TIMEOUT="30s"
export MCP_ENABLE_LSP="true"

# Optional optimizations
export MCP_FOLLOW_SYMLINKS="true"
export MCP_RESPECT_GITIGNORE="true"
```

### Logging Configuration

```bash
# Development
./mcp-filepuff -log-level debug -log-file /tmp/mcp-filepuff.log

# Production (minimal logging)
./mcp-filepuff -log-level warn
```

### Health Monitoring

Use the `ping` tool to verify server health:

```json
{"tool": "ping"}
```

Expected response: `"pong"`

### Performance Checklist

- [ ] Language servers installed and in PATH
- [ ] Project initialized (go.mod, package.json, etc.)
- [ ] Reasonable file size limits for your codebase
- [ ] LSP timeout appropriate for project size
- [ ] Adequate system memory (recommend 2+ GB free)

### Troubleshooting Slow Performance

1. **Slow Initial Operations**
   - LSP servers need to index project
   - Wait for initial indexing to complete
   - Check LSP server logs for progress

2. **Slow Search**
   - Check for overly broad patterns
   - Exclude large directories (node_modules, vendor)
   - Verify .gitignore is respected

3. **High Memory Usage**
   - Disable unused LSP servers
   - Check for memory leaks in language servers
   - Monitor cache size

4. **Timeouts**
   - Increase timeout values
   - Check for I/O bottlenecks
   - Verify network filesystems are responsive

---

## See Also

- [API.md](API.md) - Complete API reference
- [ERROR_CODES.md](ERROR_CODES.md) - Error code reference
- [README.md](../README.md) - Getting started
