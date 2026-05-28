package edit

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lukaszraczylo/mcp-filepuff/internal/parser"
)

// mkEngine builds an Engine with a registry that is closed at test end.
func mkEngine(t *testing.T) *Engine {
	t.Helper()
	reg := parser.NewRegistry()
	t.Cleanup(reg.Close)
	return NewEngine(reg)
}

// applyEditFile writes content to a temp file, applies the edit, and returns the on-disk bytes.
func applyEditFile(t *testing.T, name, content string, e *ASTEdit) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatalf("write temp: %v", err)
	}
	e.File = p
	res, err := mkEngine(t).Apply(context.Background(), e)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if !res.Success {
		t.Fatalf("apply unsuccessful: %s", res.Error)
	}
	got, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("readback: %v", err)
	}
	return string(got)
}

// assertAllCRLF fails if s contains any bare LF (not preceded by CR) or a doubled CR.
func assertAllCRLF(t *testing.T, s string) {
	t.Helper()
	if strings.Count(s, "\n") != strings.Count(s, "\r\n") {
		t.Fatalf("file contains bare LF (mixed line endings): %q", s)
	}
	if strings.Contains(s, "\r\r") {
		t.Fatalf("file contains doubled CR: %q", s)
	}
}

// ---- Cluster C: text-mode edits insert new_content verbatim (no auto-indentation) ----

func TestTextEditVerbatimNoAutoIndent(t *testing.T) {
	// selector_text matches a token that sits at one tab of indentation, so the old
	// code detected "\t" and re-indented continuation lines of new_content. Because
	// new_content is itself already indented, that produced a DOUBLE tab on line 2.
	src := "func f() {\n\tOLD\n}\n"
	got := applyEditFile(t, "f.txt", src, &ASTEdit{
		Operation:  EditReplace,
		NewContent: "A\n\tB", // line 2 already carries its own tab
		Selector:   ASTSelector{Text: "OLD"},
	})
	want := "func f() {\n\tA\n\tB\n}\n"
	if got != want {
		t.Fatalf("text edit must insert new_content verbatim (no auto-indent).\nwant: %q\ngot:  %q", want, got)
	}
}

// ---- Cluster B: edits preserve the file's original line-ending convention ----

func TestASTEditPreservesCRLF(t *testing.T) {
	src := "package main\r\n\r\nfunc demo() {\r\n\tprintln(\"old\")\r\n}\r\n"
	got := applyEditFile(t, "demo.go", src, &ASTEdit{
		Operation:  EditReplace,
		NewContent: "func demo() {\n\treturn\n}",
		Selector:   ASTSelector{Kind: "function_declaration", Name: "demo"},
	})
	assertAllCRLF(t, got)
	if !strings.Contains(got, "func demo() {\r\n\treturn\r\n}") {
		t.Fatalf("replacement not normalized to CRLF: %q", got)
	}
}

func TestTextEditReplacePreservesCRLF(t *testing.T) {
	src := "alpha\r\nbravo\r\ncharlie\r\n"
	got := applyEditFile(t, "f.txt", src, &ASTEdit{
		Operation:  EditReplace,
		NewContent: "BRAVO1\nBRAVO2",
		Selector:   ASTSelector{AtLine: 2, LineEnd: 2},
	})
	assertAllCRLF(t, got)
	want := "alpha\r\nBRAVO1\r\nBRAVO2\r\ncharlie\r\n"
	if got != want {
		t.Fatalf("CRLF text replace.\nwant: %q\ngot:  %q", want, got)
	}
}

func TestTextEditInsertAfterPreservesCRLF(t *testing.T) {
	src := "alpha\r\nbravo\r\n"
	got := applyEditFile(t, "f.txt", src, &ASTEdit{
		Operation:  EditInsertAfter,
		NewContent: "INSERTED",
		Selector:   ASTSelector{AtLine: 1, LineEnd: 1},
	})
	assertAllCRLF(t, got)
	want := "alpha\r\nINSERTED\r\nbravo\r\n"
	if got != want {
		t.Fatalf("CRLF insert_after.\nwant: %q\ngot:  %q", want, got)
	}
}

func TestLFFileStaysLFWhenNewContentHasCRLF(t *testing.T) {
	src := "alpha\nbravo\ncharlie\n"
	got := applyEditFile(t, "f.txt", src, &ASTEdit{
		Operation:  EditReplace,
		NewContent: "B1\r\nB2", // stray CRLF in new_content must be normalized to the LF file
		Selector:   ASTSelector{AtLine: 2, LineEnd: 2},
	})
	if strings.Contains(got, "\r") {
		t.Fatalf("LF file must not gain CR: %q", got)
	}
	want := "alpha\nB1\nB2\ncharlie\n"
	if got != want {
		t.Fatalf("LF normalization.\nwant: %q\ngot:  %q", want, got)
	}
}

// ---- Cluster D: diff rendering ----

func TestDiffMarksNoNewlineAtEOF(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(p, []byte("alpha\nbravo"), 0o600); err != nil { // no trailing newline
		t.Fatalf("write: %v", err)
	}
	res, err := mkEngine(t).Preview(context.Background(), &ASTEdit{
		File:       p,
		Operation:  EditReplace,
		NewContent: "BRAVO",
		Selector:   ASTSelector{AtLine: 2, LineEnd: 2},
	})
	if err != nil || !res.Success {
		t.Fatalf("preview failed: %v %s", err, res.Error)
	}
	if !strings.Contains(res.Diff, "No newline at end of file") {
		t.Fatalf("diff should mark missing final newline, got:\n%s", res.Diff)
	}
}

func TestDiffHasNoRawCarriageReturn(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(p, []byte("alpha\r\nbravo\r\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	res, err := mkEngine(t).Preview(context.Background(), &ASTEdit{
		File:       p,
		Operation:  EditReplace,
		NewContent: "BRAVO",
		Selector:   ASTSelector{AtLine: 2, LineEnd: 2},
	})
	if err != nil || !res.Success {
		t.Fatalf("preview failed: %v %s", err, res.Error)
	}
	if strings.Contains(res.Diff, "\r") {
		t.Fatalf("diff display must not contain raw CR, got:\n%q", res.Diff)
	}
}
