package server

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lukaszraczylo/mcp-filepuff/internal/config"
	"github.com/mark3labs/mcp-go/mcp"
)

// newTestServer creates a minimal server pointing at tmpDir.
func newTestServer(t *testing.T, tmpDir string) *Server {
	t.Helper()
	cfg := &config.Config{
		WorkspaceRoot: tmpDir,
		EnableLSP:     false,
		MaxFileSize:   10 * 1024 * 1024, // 10 MB — required for file reads to succeed
		MaxParseSize:  10 * 1024 * 1024,
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	srv, err := New(cfg, logger)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return srv
}

// callRead calls handleFileRead with the given args map and returns the text content.
func callRead(t *testing.T, srv *Server, args map[string]interface{}) string {
	t.Helper()
	req := mcp.CallToolRequest{}
	req.Params.Arguments = args
	result, err := srv.handleFileRead(context.Background(), req)
	if err != nil {
		t.Fatalf("handleFileRead error: %v", err)
	}
	if result == nil {
		t.Fatal("handleFileRead returned nil")
	}
	if len(result.Content) == 0 {
		t.Fatal("handleFileRead returned empty content")
	}
	tc, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatal("handleFileRead did not return TextContent")
	}
	return tc.Text
}

// callReadResult calls handleFileRead and returns the raw CallToolResult (not just text).
func callReadResult(t *testing.T, srv *Server, args map[string]interface{}) *mcp.CallToolResult {
	t.Helper()
	req := mcp.CallToolRequest{}
	req.Params.Arguments = args
	result, err := srv.handleFileRead(context.Background(), req)
	if err != nil {
		t.Fatalf("handleFileRead error: %v", err)
	}
	if result == nil {
		t.Fatal("handleFileRead returned nil")
	}
	return result
}

// newTestServerWithThreshold creates a server with a custom ResourceLinkThresholdBytes.
func newTestServerWithThreshold(t *testing.T, tmpDir string, thresholdBytes int) *Server {
	t.Helper()
	cfg := &config.Config{
		WorkspaceRoot:              tmpDir,
		EnableLSP:                  false,
		MaxFileSize:                10 * 1024 * 1024,
		MaxParseSize:               10 * 1024 * 1024,
		ResourceLinkThresholdBytes: thresholdBytes,
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	srv, err := New(cfg, logger)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return srv
}

// writeFile writes content to a file in tmpDir and returns its absolute path.
func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0600); err != nil {
		t.Fatalf("WriteFile(%s): %v", name, err)
	}
	return p
}

// ---- Feature 1: skeleton mode ----

func TestSkeletonModeGo(t *testing.T) {
	tmpDir := t.TempDir()
	srv := newTestServer(t, tmpDir)

	goSrc := `package main

// Hello says hello
func Hello() {
	println("Hello, World!")
	println("more body")
}

func Add(a, b int) int {
	return a + b
}
`
	f := writeFile(t, tmpDir, "test.go", goSrc)

	out := callRead(t, srv, map[string]interface{}{
		"path": f,
		"mode": "skeleton",
	})

	// Should contain function signatures
	if !strings.Contains(out, "func Hello()") {
		t.Errorf("skeleton output missing Hello signature, got:\n%s", out)
	}
	if !strings.Contains(out, "func Add(") {
		t.Errorf("skeleton output missing Add signature, got:\n%s", out)
	}
	// Should NOT contain body contents
	if strings.Contains(out, `println("more body")`) {
		t.Errorf("skeleton output should not contain body contents, got:\n%s", out)
	}
	// Should contain placeholder
	if !strings.Contains(out, "{ ... }") {
		t.Errorf("skeleton output missing { ... } placeholder, got:\n%s", out)
	}
	// Should contain etag footer
	if !strings.Contains(out, "[etag:") {
		t.Errorf("skeleton output missing etag footer, got:\n%s", out)
	}
}

func TestSkeletonModeFullFlagAlias(t *testing.T) {
	// mode="full" should behave identically to not specifying mode
	tmpDir := t.TempDir()
	srv := newTestServer(t, tmpDir)

	goSrc := "package main\nfunc F() { println(1) }\n"
	f := writeFile(t, tmpDir, "test.go", goSrc)

	outFull := callRead(t, srv, map[string]interface{}{"path": f, "mode": "full"})
	outDefault := callRead(t, srv, map[string]interface{}{"path": f})

	// Both should have same content (etag will be same, line content same)
	if outFull != outDefault {
		t.Errorf("mode=full differs from default\nfull: %q\ndefault: %q", outFull, outDefault)
	}
}

func TestSkeletonModeSymbolsOnlyAlias(t *testing.T) {
	tmpDir := t.TempDir()
	srv := newTestServer(t, tmpDir)

	goSrc := "package main\nfunc F() { println(1) }\n"
	f := writeFile(t, tmpDir, "test.go", goSrc)

	// mode=symbols_only should return symbols summary (needs include_ast implicitly)
	out := callRead(t, srv, map[string]interface{}{
		"path": f,
		"mode": "symbols_only",
	})
	// Should contain etag but NOT the function body
	if !strings.Contains(out, "[etag:") {
		t.Errorf("symbols_only output missing etag, got:\n%s", out)
	}
	if strings.Contains(out, "println") {
		t.Errorf("symbols_only should not contain body, got:\n%s", out)
	}
}

func TestSkeletonModeTypeScript(t *testing.T) {
	tmpDir := t.TempDir()
	srv := newTestServer(t, tmpDir)

	tsSrc := `// a function
function greet(name: string): string {
  return "Hello " + name;
}

class Greeter {
  greet(name: string) {
    return "hi " + name;
  }
}
`
	f := writeFile(t, tmpDir, "test.ts", tsSrc)

	out := callRead(t, srv, map[string]interface{}{"path": f, "mode": "skeleton"})

	if !strings.Contains(out, "function greet") {
		t.Errorf("TS skeleton missing function signature, got:\n%s", out)
	}
	if !strings.Contains(out, "{ ... }") {
		t.Errorf("TS skeleton missing placeholder, got:\n%s", out)
	}
	if strings.Contains(out, `"Hello " + name`) {
		t.Errorf("TS skeleton should not contain body, got:\n%s", out)
	}
}

func TestSkeletonModePython(t *testing.T) {
	tmpDir := t.TempDir()
	srv := newTestServer(t, tmpDir)

	pySrc := `def greet(name):
    print("Hello " + name)
    print("extra line")

class Foo:
    def bar(self):
        return 42
`
	f := writeFile(t, tmpDir, "test.py", pySrc)

	out := callRead(t, srv, map[string]interface{}{"path": f, "mode": "skeleton"})

	if !strings.Contains(out, "def greet") {
		t.Errorf("Python skeleton missing greet signature, got:\n%s", out)
	}
	if strings.Contains(out, "extra line") {
		t.Errorf("Python skeleton should not contain body, got:\n%s", out)
	}
	// Python uses "..." as placeholder
	if !strings.Contains(out, "...") {
		t.Errorf("Python skeleton missing ... placeholder, got:\n%s", out)
	}
}

func TestSkeletonModeRust(t *testing.T) {
	tmpDir := t.TempDir()
	srv := newTestServer(t, tmpDir)

	rsSrc := `fn add(a: i32, b: i32) -> i32 {
    let result = a + b;
    result
}

struct Foo {
    x: i32,
}
`
	f := writeFile(t, tmpDir, "test.rs", rsSrc)

	out := callRead(t, srv, map[string]interface{}{"path": f, "mode": "skeleton"})

	if !strings.Contains(out, "fn add(") {
		t.Errorf("Rust skeleton missing fn signature, got:\n%s", out)
	}
	if !strings.Contains(out, "{ ... }") {
		t.Errorf("Rust skeleton missing placeholder, got:\n%s", out)
	}
	if strings.Contains(out, "let result") {
		t.Errorf("Rust skeleton should not contain body, got:\n%s", out)
	}
}

// ---- Feature 2: strip flag ----

func TestStripImportsGo(t *testing.T) {
	tmpDir := t.TempDir()
	srv := newTestServer(t, tmpDir)

	goSrc := `package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Println("hello")
}
`
	f := writeFile(t, tmpDir, "test.go", goSrc)

	out := callRead(t, srv, map[string]interface{}{
		"path":  f,
		"strip": []interface{}{"imports"},
	})

	if strings.Contains(out, `"fmt"`) {
		t.Errorf("strip=imports should remove import block, got:\n%s", out)
	}
	if !strings.Contains(out, "func main") {
		t.Errorf("strip=imports should keep function, got:\n%s", out)
	}
	if !strings.Contains(out, "[stripped: imports]") {
		t.Errorf("strip footer missing, got:\n%s", out)
	}
}

func TestStripLicenseGoBlockComment(t *testing.T) {
	tmpDir := t.TempDir()
	srv := newTestServer(t, tmpDir)

	goSrc := `/* Copyright 2024 Acme Corp. All rights reserved.
   License: MIT
*/
package main

func main() {}
`
	f := writeFile(t, tmpDir, "main.go", goSrc)

	out := callRead(t, srv, map[string]interface{}{
		"path":  f,
		"strip": []interface{}{"license"},
	})

	if strings.Contains(out, "Copyright") {
		t.Errorf("strip=license should remove license comment, got:\n%s", out)
	}
	if !strings.Contains(out, "func main") {
		t.Errorf("strip=license should keep code, got:\n%s", out)
	}
	if !strings.Contains(out, "[stripped: license]") {
		t.Errorf("license strip footer missing, got:\n%s", out)
	}
}

func TestStripBlockCommentsGo(t *testing.T) {
	tmpDir := t.TempDir()
	srv := newTestServer(t, tmpDir)

	goSrc := `package main

/* This is a block comment
   spanning multiple lines */
func main() {
	/* inline block */
	println("hi")
}
`
	f := writeFile(t, tmpDir, "test.go", goSrc)

	out := callRead(t, srv, map[string]interface{}{
		"path":  f,
		"strip": []interface{}{"block_comments"},
	})

	if strings.Contains(out, "This is a block comment") {
		t.Errorf("strip=block_comments should remove block comments, got:\n%s", out)
	}
	if strings.Contains(out, "inline block") {
		t.Errorf("strip=block_comments should remove inline block comment, got:\n%s", out)
	}
	if !strings.Contains(out, "func main") {
		t.Errorf("strip=block_comments should keep code, got:\n%s", out)
	}
	if !strings.Contains(out, "[stripped: block_comments]") {
		t.Errorf("block_comments strip footer missing, got:\n%s", out)
	}
}

func TestStripMultipleFlags(t *testing.T) {
	tmpDir := t.TempDir()
	srv := newTestServer(t, tmpDir)

	goSrc := `/* Copyright 2024. License: MIT */
package main

import "fmt"

func main() {
	fmt.Println("hello")
}
`
	f := writeFile(t, tmpDir, "main.go", goSrc)

	out := callRead(t, srv, map[string]interface{}{
		"path":  f,
		"strip": []interface{}{"license", "imports"},
	})

	if strings.Contains(out, "Copyright") {
		t.Errorf("license not stripped, got:\n%s", out)
	}
	if strings.Contains(out, `"fmt"`) {
		t.Errorf("imports not stripped, got:\n%s", out)
	}
	if !strings.Contains(out, "[stripped:") {
		t.Errorf("strip footer missing, got:\n%s", out)
	}
}

func TestStripNoRemovalProducesNoFooter(t *testing.T) {
	// A file with no imports: strip=imports should not add footer
	tmpDir := t.TempDir()
	srv := newTestServer(t, tmpDir)

	goSrc := "package main\nfunc main() {}\n"
	f := writeFile(t, tmpDir, "test.go", goSrc)

	out := callRead(t, srv, map[string]interface{}{
		"path":  f,
		"strip": []interface{}{"imports"},
	})

	if strings.Contains(out, "[stripped:") {
		t.Errorf("should not have stripped footer when nothing removed, got:\n%s", out)
	}
}

func TestStripImportsPython(t *testing.T) {
	tmpDir := t.TempDir()
	srv := newTestServer(t, tmpDir)

	pySrc := `import os
from sys import argv

def main():
    print(argv[0])
`
	f := writeFile(t, tmpDir, "test.py", pySrc)

	out := callRead(t, srv, map[string]interface{}{
		"path":  f,
		"strip": []interface{}{"imports"},
	})

	if strings.Contains(out, "import os") {
		t.Errorf("Python imports not stripped, got:\n%s", out)
	}
	if strings.Contains(out, "from sys") {
		t.Errorf("Python from-import not stripped, got:\n%s", out)
	}
	if !strings.Contains(out, "def main") {
		t.Errorf("Python function missing after strip, got:\n%s", out)
	}
}

// ---- Feature 3: short etag (8 hex chars) ----

func TestEtagIs8Chars(t *testing.T) {
	tmpDir := t.TempDir()
	srv := newTestServer(t, tmpDir)

	f := writeFile(t, tmpDir, "test.go", "package main\n")

	out := callRead(t, srv, map[string]interface{}{"path": f})

	// Find "[etag: XXXXXXXX]"
	idx := strings.Index(out, "[etag: ")
	if idx < 0 {
		t.Fatalf("no etag in output: %q", out)
	}
	rest := out[idx+7:]
	end := strings.Index(rest, "]")
	if end < 0 {
		t.Fatalf("malformed etag in output: %q", out)
	}
	etagVal := rest[:end]
	if len(etagVal) != 8 {
		t.Errorf("etag should be 8 hex chars, got %d chars: %q", len(etagVal), etagVal)
	}
	// Validate hex
	for _, c := range etagVal {
		isDigit := c >= '0' && c <= '9'
		isHexLower := c >= 'a' && c <= 'f'
		if !isDigit && !isHexLower {
			t.Errorf("etag contains non-hex char %q in %q", c, etagVal)
		}
	}
}

func TestEtagPreviousEtagShortCircuit(t *testing.T) {
	tmpDir := t.TempDir()
	srv := newTestServer(t, tmpDir)

	f := writeFile(t, tmpDir, "test.go", "package main\n")

	// First read: get the etag
	out1 := callRead(t, srv, map[string]interface{}{"path": f})
	idx := strings.Index(out1, "[etag: ")
	etag := out1[idx+7 : idx+7+8]

	// Second read with same etag: should short-circuit
	out2 := callRead(t, srv, map[string]interface{}{
		"path":          f,
		"previous_etag": etag,
	})
	if !strings.Contains(out2, "[unchanged, etag:") {
		t.Errorf("expected [unchanged, etag:] for same etag, got: %q", out2)
	}
}

func TestEtagOldLongEtagStillWorks(t *testing.T) {
	// Simulate old client sending 16-char etag: should still short-circuit via prefix match.
	tmpDir := t.TempDir()
	srv := newTestServer(t, tmpDir)

	f := writeFile(t, tmpDir, "test.go", "package main\n")

	// Get the 8-char etag
	out1 := callRead(t, srv, map[string]interface{}{"path": f})
	idx := strings.Index(out1, "[etag: ")
	shortEtag := out1[idx+7 : idx+7+8]

	// Construct a fake 16-char etag that starts with the short one
	fakeOldEtag := shortEtag + "00000000"

	out2 := callRead(t, srv, map[string]interface{}{
		"path":          f,
		"previous_etag": fakeOldEtag,
	})
	if !strings.Contains(out2, "[unchanged, etag:") {
		t.Errorf("old 16-char etag should still short-circuit, got: %q", out2)
	}
}

func TestEtagDifferentFileChanges(t *testing.T) {
	tmpDir := t.TempDir()
	srv := newTestServer(t, tmpDir)

	f := writeFile(t, tmpDir, "test.go", "package main\n")

	out1 := callRead(t, srv, map[string]interface{}{"path": f})
	idx := strings.Index(out1, "[etag: ")
	etag1 := out1[idx+7 : idx+7+8]

	// Modify the file
	if err := os.WriteFile(f, []byte("package main\n// changed\n"), 0600); err != nil {
		t.Fatal(err)
	}

	// Read again with old etag: should NOT short-circuit
	out2 := callRead(t, srv, map[string]interface{}{
		"path":          f,
		"previous_etag": etag1,
	})
	if strings.Contains(out2, "[unchanged") {
		t.Errorf("modified file should not return unchanged, got: %q", out2)
	}
}

// ---- Feature 4: compact_line_numbers ----

func TestCompactLineNumbers(t *testing.T) {
	tmpDir := t.TempDir()
	srv := newTestServer(t, tmpDir)

	content := "line one\nline two\nline three\n"
	f := writeFile(t, tmpDir, "test.txt", content)

	out := callRead(t, srv, map[string]interface{}{
		"path":                 f,
		"compact_line_numbers": true,
	})

	// Should have "1│" not "   1│ "
	if !strings.Contains(out, "1│line one") {
		t.Errorf("compact prefix not found, got:\n%s", out)
	}
	// Should NOT have padded format
	if strings.Contains(out, "   1│ line one") {
		t.Errorf("compact should not have padded prefix, got:\n%s", out)
	}
}

func TestCompactLineNumbersOffByDefault(t *testing.T) {
	tmpDir := t.TempDir()
	srv := newTestServer(t, tmpDir)

	content := "line one\nline two\n"
	f := writeFile(t, tmpDir, "test.txt", content)

	// Default: no compact_line_numbers
	out := callRead(t, srv, map[string]interface{}{"path": f})

	// Should have padded format
	if !strings.Contains(out, "   1│ line one") {
		t.Errorf("default should have padded format, got:\n%s", out)
	}
}

func TestCompactLineNumbersWithInterval(t *testing.T) {
	// compact_line_numbers + line_number_interval should work together
	tmpDir := t.TempDir()
	srv := newTestServer(t, tmpDir)

	var sb strings.Builder
	for i := 1; i <= 10; i++ {
		sb.WriteString("line\n")
	}
	f := writeFile(t, tmpDir, "test.txt", sb.String())

	out := callRead(t, srv, map[string]interface{}{
		"path":                 f,
		"compact_line_numbers": true,
		"line_number_interval": 5,
	})

	// Line 5 should have number prefix
	if !strings.Contains(out, "5│line") {
		t.Errorf("compact+interval: line 5 should have number, got:\n%s", out)
	}
	// Line 1 (first) should have number
	if !strings.Contains(out, "1│line") {
		t.Errorf("compact+interval: line 1 should have number, got:\n%s", out)
	}
	// Line 10 (last) should have number
	if !strings.Contains(out, "10│line") {
		t.Errorf("compact+interval: line 10 should have number, got:\n%s", out)
	}
	// Non-interval line should have bare │ prefix (no number)
	if !strings.Contains(out, "│line") {
		t.Errorf("compact+interval: non-interval lines should have bare │, got:\n%s", out)
	}
}

func TestCompactLineNumbersWithNoLineNumbers(t *testing.T) {
	// compact_line_numbers + no_line_numbers: no_line_numbers wins
	tmpDir := t.TempDir()
	srv := newTestServer(t, tmpDir)

	f := writeFile(t, tmpDir, "test.txt", "line one\n")

	out := callRead(t, srv, map[string]interface{}{
		"path":                 f,
		"compact_line_numbers": true,
		"no_line_numbers":      true,
	})

	// Should have NO prefix at all
	if strings.Contains(out, "│") {
		t.Errorf("no_line_numbers should suppress all prefixes, got:\n%s", out)
	}
}

// ---- Backward compatibility: existing behavior unchanged ----

func TestDefaultBehaviorUnchanged(t *testing.T) {
	tmpDir := t.TempDir()
	srv := newTestServer(t, tmpDir)

	goSrc := `package main

func Hello() {
	println("hello")
}
`
	f := writeFile(t, tmpDir, "test.go", goSrc)

	out := callRead(t, srv, map[string]interface{}{"path": f})

	// All lines present
	if !strings.Contains(out, `println("hello")`) {
		t.Errorf("full mode missing body, got:\n%s", out)
	}
	// Padded line numbers
	if !strings.Contains(out, "   1│ package main") {
		t.Errorf("default should have padded line numbers, got:\n%s", out)
	}
	// etag present
	if !strings.Contains(out, "[etag:") {
		t.Errorf("missing etag footer, got:\n%s", out)
	}
}

func TestSymbolsOnlyFlagStillWorks(t *testing.T) {
	// Old symbols_only=true + include_ast=true should still work
	tmpDir := t.TempDir()
	srv := newTestServer(t, tmpDir)

	goSrc := "package main\nfunc Hello() { println(1) }\n"
	f := writeFile(t, tmpDir, "test.go", goSrc)

	out := callRead(t, srv, map[string]interface{}{
		"path":         f,
		"include_ast":  true,
		"symbols_only": true,
	})

	if strings.Contains(out, "println") {
		t.Errorf("symbols_only should suppress body, got:\n%s", out)
	}
	if !strings.Contains(out, "[etag:") {
		t.Errorf("symbols_only missing etag, got:\n%s", out)
	}
}

// ---- Feature: resource_link for big reads ----

func TestResourceLinkThresholdTrip(t *testing.T) {
	// When result bytes > threshold, handleFileRead returns a ResourceLink content block.
	tmpDir := t.TempDir()
	// Low threshold (10 bytes) guarantees even a tiny file trips it.
	srv := newTestServerWithThreshold(t, tmpDir, 10)

	f := writeFile(t, tmpDir, "big.txt", strings.Repeat("x", 200))

	result := callReadResult(t, srv, map[string]interface{}{"path": f})

	if len(result.Content) == 0 {
		t.Fatal("expected content, got none")
	}
	link, ok := result.Content[0].(mcp.ResourceLink)
	if !ok {
		t.Fatalf("expected ResourceLink content, got %T", result.Content[0])
	}
	if !strings.HasPrefix(link.URI, "filepuff://read/") {
		t.Errorf("ResourceLink URI should start with filepuff://read/, got: %q", link.URI)
	}
	if link.Name != f {
		t.Errorf("ResourceLink Name should be file path %q, got %q", f, link.Name)
	}
	if !strings.Contains(link.Description, "etag=") {
		t.Errorf("ResourceLink Description should contain etag=, got %q", link.Description)
	}
	if !strings.Contains(link.Description, "size=") {
		t.Errorf("ResourceLink Description should contain size=, got %q", link.Description)
	}
	if !strings.Contains(link.Description, "lines=") {
		t.Errorf("ResourceLink Description should contain lines=, got %q", link.Description)
	}
}

func TestResourceLinkForceInlineBypass(t *testing.T) {
	// force_inline=true must always return TextContent regardless of threshold.
	tmpDir := t.TempDir()
	srv := newTestServerWithThreshold(t, tmpDir, 1) // threshold = 1 byte, always trips

	f := writeFile(t, tmpDir, "test.txt", "hello world")

	result := callReadResult(t, srv, map[string]interface{}{
		"path":         f,
		"force_inline": true,
	})

	if len(result.Content) == 0 {
		t.Fatal("expected content, got none")
	}
	tc, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("force_inline=true should return TextContent, got %T", result.Content[0])
	}
	if !strings.Contains(tc.Text, "hello world") {
		t.Errorf("force_inline result should contain file content, got: %q", tc.Text)
	}
}

func TestResourceLinkMaxInlineBytesOverride(t *testing.T) {
	// max_inline_bytes overrides server threshold per-call.
	tmpDir := t.TempDir()
	// Server threshold = 1 byte (always trips), but max_inline_bytes allows bigger.
	srv := newTestServerWithThreshold(t, tmpDir, 1)

	content := strings.Repeat("a", 50) // 50 bytes
	f := writeFile(t, tmpDir, "test.txt", content)

	// max_inline_bytes=100 > 50 bytes result → should inline
	result := callReadResult(t, srv, map[string]interface{}{
		"path":             f,
		"max_inline_bytes": 100,
	})

	if len(result.Content) == 0 {
		t.Fatal("expected content, got none")
	}
	_, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("max_inline_bytes=100 with 50-byte file should return TextContent, got %T", result.Content[0])
	}
}

func TestResourceLinkStaleEtagRejection(t *testing.T) {
	// handleReadResource should reject fetch when file has changed since link was issued.
	tmpDir := t.TempDir()
	srv := newTestServerWithThreshold(t, tmpDir, 1) // always trips

	f := writeFile(t, tmpDir, "stale.txt", "original content")

	// Get a ResourceLink — captures etag of "original content"
	result := callReadResult(t, srv, map[string]interface{}{"path": f})
	link, ok := result.Content[0].(mcp.ResourceLink)
	if !ok {
		t.Fatalf("expected ResourceLink, got %T", result.Content[0])
	}
	// URI contains ?etag=<hash-of-original>
	if !strings.Contains(link.URI, "?etag=") {
		t.Fatalf("expected etag in URI, got %q", link.URI)
	}

	// Overwrite the file with new content.
	if err := os.WriteFile(f, []byte("modified content — different"), 0600); err != nil {
		t.Fatal(err)
	}

	// Fetch the resource using the stale URI — should error.
	req := mcp.ReadResourceRequest{}
	req.Params.URI = link.URI
	_, err := srv.handleReadResource(req)
	if err == nil {
		t.Fatal("expected error for stale etag, got nil")
	}
	if !strings.Contains(err.Error(), "file changed") {
		t.Errorf("error should mention 'file changed', got: %v", err)
	}
}

func TestResourceLinkBelowThresholdInlines(t *testing.T) {
	// When result is small (below threshold), always inline regardless of threshold setting.
	tmpDir := t.TempDir()
	// Large threshold — small file should be inlined.
	srv := newTestServerWithThreshold(t, tmpDir, 64*1024)

	f := writeFile(t, tmpDir, "small.txt", "tiny")

	result := callReadResult(t, srv, map[string]interface{}{"path": f})

	if len(result.Content) == 0 {
		t.Fatal("expected content, got none")
	}
	_, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("small file should return TextContent, got %T", result.Content[0])
	}
}

func TestResourceLinkThresholdZeroDisabled(t *testing.T) {
	// threshold=0 disables the feature entirely — always inline.
	tmpDir := t.TempDir()
	srv := newTestServerWithThreshold(t, tmpDir, 0)

	f := writeFile(t, tmpDir, "test.txt", strings.Repeat("z", 10000))

	result := callReadResult(t, srv, map[string]interface{}{"path": f})

	if len(result.Content) == 0 {
		t.Fatal("expected content")
	}
	_, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("threshold=0 should always inline, got %T", result.Content[0])
	}
}

func TestHandleReadResource_ValidFetch(t *testing.T) {
	// handleReadResource fetches file content when etag matches.
	tmpDir := t.TempDir()
	srv := newTestServerWithThreshold(t, tmpDir, 1)

	f := writeFile(t, tmpDir, "fetch.txt", "fetch me please")

	// Trigger a ResourceLink to get a valid URI with correct etag.
	toolResult := callReadResult(t, srv, map[string]interface{}{"path": f})
	link, ok := toolResult.Content[0].(mcp.ResourceLink)
	if !ok {
		t.Fatalf("expected ResourceLink, got %T", toolResult.Content[0])
	}

	req := mcp.ReadResourceRequest{}
	req.Params.URI = link.URI
	contents, err := srv.handleReadResource(req)
	if err != nil {
		t.Fatalf("handleReadResource error: %v", err)
	}
	if len(contents) == 0 {
		t.Fatal("expected resource contents, got none")
	}
	tc, ok := contents[0].(mcp.TextResourceContents)
	if !ok {
		t.Fatalf("expected TextResourceContents, got %T", contents[0])
	}
	if !strings.Contains(tc.Text, "fetch me please") {
		t.Errorf("resource contents should include file content, got: %q", tc.Text)
	}
}

func TestResourceLinkMIMEType(t *testing.T) {
	// Verify MIME types for common extensions.
	tmpDir := t.TempDir()
	srv := newTestServerWithThreshold(t, tmpDir, 1)

	cases := []struct {
		name     string
		content  string
		wantMIME string
	}{
		{"test.go", "package main\n", "text/x-go"},
		{"test.py", "# py\n", "text/x-python"},
		{"test.ts", "// ts\n", "text/typescript"},
		{"test.json", "{}\n", "application/json"},
		{"test.md", "# hi\n", "text/markdown"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := writeFile(t, tmpDir, c.name, c.content)
			result := callReadResult(t, srv, map[string]interface{}{"path": f})
			link, ok := result.Content[0].(mcp.ResourceLink)
			if !ok {
				t.Fatalf("expected ResourceLink for %s, got %T", c.name, result.Content[0])
			}
			if link.MIMEType != c.wantMIME {
				t.Errorf("%s: MIMEType = %q, want %q", c.name, link.MIMEType, c.wantMIME)
			}
		})
	}
}
