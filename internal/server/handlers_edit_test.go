package server

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

// callEdit invokes handleEditApply with the given args and returns the result.
func callEdit(t *testing.T, srv *Server, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	req := mcp.CallToolRequest{}
	req.Params.Arguments = args
	res, err := srv.handleEditApply(context.Background(), req)
	if err != nil {
		t.Fatalf("handleEditApply error: %v", err)
	}
	if res == nil {
		t.Fatal("handleEditApply returned nil result")
	}
	return res
}

// TestEditApplyPreservesBackslashSequencesText is the primary regression guard for the
// double-unescape bug: new_content arrives from the JSON-RPC layer already fully decoded,
// so the handler must write it to disk verbatim. Literal backslash sequences (\n, \t, \",
// \\) that legitimately appear in source code must survive byte-for-byte.
func TestEditApplyPreservesBackslashSequencesText(t *testing.T) {
	tmpDir := t.TempDir()
	srv := newTestServer(t, tmpDir)

	f := writeFile(t, tmpDir, "note.txt", "PLACEHOLDER\n")

	// Exactly the bytes a client intends after JSON decoding: backslash sequences are
	// real source text here, NOT escapes to be interpreted.
	newContent := `printf("a\nb\tc"); re = \d+; path = "C:\\tmp"; q = \"`

	res := callEdit(t, srv, map[string]any{
		"file":          f,
		"operation":     "replace",
		"selector_text": "PLACEHOLDER",
		"new_content":   newContent,
		"response":      "none",
	})
	if res.IsError {
		t.Fatalf("edit returned error: %+v", res.Content)
	}

	got, err := os.ReadFile(f)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	want := newContent + "\n"
	if string(got) != want {
		t.Fatalf("new_content not written verbatim (double-unescape regression).\nwant: %q\ngot:  %q", want, string(got))
	}
}

// TestCountDiffLinesHunkAware verifies the line counter skips file headers structurally and
// counts content lines whose own text begins with + or - (previously dropped as "+++"/"---").
func TestCountDiffLinesHunkAware(t *testing.T) {
	diff := "--- a.txt\n+++ a.txt\n@@ -1,3 +1,3 @@\n context\n+++plusprefix\n+normal add\n---minusprefix\n-normal del\n"
	added, removed := countDiffLines(diff)
	if added != 2 || removed != 2 {
		t.Fatalf("hunk-aware diff count wrong: added=%d removed=%d (want 2 and 2)", added, removed)
	}
}

// TestEditApplyPreservesBackslashSequencesCode covers the AST/code path: the same verbatim
// guarantee must hold for syntactically-validated code files.
func TestEditApplyPreservesBackslashSequencesCode(t *testing.T) {
	tmpDir := t.TempDir()
	srv := newTestServer(t, tmpDir)

	src := "package main\n\nfunc demo() {\n\tprintln(\"old\")\n}\n"
	f := writeFile(t, tmpDir, "demo.go", src)

	// Real tab indentation + literal backslash sequences inside string/raw-string literals.
	newBody := "func demo() {\n\tprintln(\"a\\nb\\tc\")\n\t_ = `\\d+`\n\t_ = \"C:\\\\tmp\"\n}"

	res := callEdit(t, srv, map[string]any{
		"file":          f,
		"operation":     "replace",
		"selector_kind": "function_declaration",
		"selector_name": "demo",
		"new_content":   newBody,
		"response":      "none",
	})
	if res.IsError {
		t.Fatalf("edit returned error: %+v", res.Content)
	}

	got, err := os.ReadFile(f)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if !strings.Contains(string(got), newBody) {
		t.Fatalf("backslash sequences corrupted in code edit.\nwant substring:\n%q\ngot file:\n%q", newBody, string(got))
	}
}
