package edit

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lukaszraczylo/mcp-filepuff/internal/parser"
	"github.com/sergi/go-diff/diffmatchpatch"
	sitter "github.com/smacker/go-tree-sitter"
)

func TestValidateEdit(t *testing.T) {
	e := NewEngine(parser.NewRegistry())

	tests := []struct {
		edit    *ASTEdit
		name    string
		wantErr bool
	}{
		{
			name: "valid replace",
			edit: &ASTEdit{
				File:       "test.go",
				Operation:  EditReplace,
				Selector:   ASTSelector{Kind: "function_declaration"},
				NewContent: "func NewFunc() {}",
			},
			wantErr: false,
		},
		{
			name: "valid delete",
			edit: &ASTEdit{
				File:      "test.go",
				Operation: EditDelete,
				Selector:  ASTSelector{Name: "oldFunc"},
			},
			wantErr: false,
		},
		{
			name: "missing file",
			edit: &ASTEdit{
				Operation:  EditReplace,
				Selector:   ASTSelector{Kind: "function_declaration"},
				NewContent: "func NewFunc() {}",
			},
			wantErr: true,
		},
		{
			name: "missing operation",
			edit: &ASTEdit{
				File:       "test.go",
				Selector:   ASTSelector{Kind: "function_declaration"},
				NewContent: "func NewFunc() {}",
			},
			wantErr: true,
		},
		{
			name: "replace without content",
			edit: &ASTEdit{
				File:      "test.go",
				Operation: EditReplace,
				Selector:  ASTSelector{Kind: "function_declaration"},
			},
			wantErr: true,
		},
		{
			name: "empty selector",
			edit: &ASTEdit{
				File:       "test.go",
				Operation:  EditReplace,
				Selector:   ASTSelector{},
				NewContent: "func NewFunc() {}",
			},
			wantErr: true,
		},
		{
			name: "unknown operation",
			edit: &ASTEdit{
				File:       "test.go",
				Operation:  "unknown",
				Selector:   ASTSelector{Kind: "function_declaration"},
				NewContent: "func NewFunc() {}",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := e.validateASTEdit(tt.edit)
			if tt.wantErr && err == nil {
				t.Error("expected error")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

// A replace with empty new_content should point the caller at operation=delete.
func TestValidateReplaceEmptySuggestsDelete(t *testing.T) {
	e := NewEngine(parser.NewRegistry())
	err := e.validateASTEdit(&ASTEdit{
		File:      "test.go",
		Operation: EditReplace,
		Selector:  ASTSelector{Kind: "function_declaration"},
	})
	if err == nil {
		t.Fatal("expected error for replace with empty new_content")
	}
	if !strings.Contains(err.Error(), "operation=delete") {
		t.Errorf("error should suggest operation=delete, got: %v", err)
	}
}

func TestResolveSelector(t *testing.T) {
	registry := parser.NewRegistry()
	defer registry.Close()
	e := NewEngine(registry)

	content := []byte(`package main

func Hello() {
	println("hello")
}

func Goodbye() {
	println("goodbye")
}
`)

	ctx := context.Background()
	result, err := registry.Parse(ctx, "test.go", content)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	tests := []struct {
		name    string
		sel     ASTSelector
		wantErr bool
	}{
		{
			name:    "by kind",
			sel:     ASTSelector{Kind: "function_declaration"},
			wantErr: false,
		},
		{
			name:    "by name",
			sel:     ASTSelector{Name: "Hello"},
			wantErr: false,
		},
		{
			name:    "by kind and name",
			sel:     ASTSelector{Kind: "function_declaration", Name: "Goodbye"},
			wantErr: false,
		},
		{
			name:    "by line",
			sel:     ASTSelector{AtLine: 3},
			wantErr: false,
		},
		{
			name:    "no match",
			sel:     ASTSelector{Name: "NonExistent"},
			wantErr: true,
		},
		{
			name:    "index out of range",
			sel:     ASTSelector{Kind: "function_declaration", Index: 10},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node, err := e.resolveSelector(tt.sel, result.Tree, content)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if node == nil {
					t.Error("expected node")
				}
			}
		})
	}
}

func TestApplyEdit(t *testing.T) {
	registry := parser.NewRegistry()
	defer registry.Close()
	e := NewEngine(registry)

	content := []byte(`package main

func Hello() {
	println("hello")
}
`)

	ctx := context.Background()
	result, err := registry.Parse(ctx, "test.go", content)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	tests := []struct {
		name      string
		operation EditOperation
		newCode   string
		wantIn    string // substring that should be in result
	}{
		{
			name:      "replace",
			operation: EditReplace,
			newCode:   "func NewHello() {}",
			wantIn:    "NewHello",
		},
		{
			name:      "insert after",
			operation: EditInsertAfter,
			newCode:   "func After() {}",
			wantIn:    "After",
		},
		{
			name:      "insert before",
			operation: EditInsertBefore,
			newCode:   "func Before() {}",
			wantIn:    "Before",
		},
		{
			name:      "delete",
			operation: EditDelete,
			newCode:   "",
			wantIn:    "package main", // Should still have package declaration
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Find the function node
			node, err := e.resolveSelector(ASTSelector{Kind: "function_declaration"}, result.Tree, content)
			if err != nil {
				t.Fatalf("resolve failed: %v", err)
			}

			edit := &ASTEdit{
				File:       "test.go",
				Operation:  tt.operation,
				NewContent: tt.newCode,
			}

			newContent, err := e.applyEdit(edit, node, content)
			if err != nil {
				t.Fatalf("apply failed: %v", err)
			}

			if !strings.Contains(string(newContent), tt.wantIn) {
				t.Errorf("result does not contain %q:\n%s", tt.wantIn, string(newContent))
			}
		})
	}
}

func TestPreview(t *testing.T) {
	registry := parser.NewRegistry()
	defer registry.Close()
	e := NewEngine(registry)

	// Create a temp file
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.go")

	content := `package main

func Hello() {
	println("hello")
}
`
	if err := os.WriteFile(tmpFile, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	ctx := context.Background()
	edit := &ASTEdit{
		File:       tmpFile,
		Operation:  EditReplace,
		Selector:   ASTSelector{Kind: "function_declaration"},
		NewContent: "func NewHello() {\n\tprintln(\"new hello\")\n}",
	}

	result, err := e.Preview(ctx, edit)
	if err != nil {
		t.Fatalf("preview failed: %v", err)
	}

	if !result.Success {
		t.Fatalf("preview was not successful: %s", result.Error)
	}

	if result.Applied {
		t.Error("preview should not apply changes")
	}

	if result.Diff == "" {
		t.Error("expected diff in result")
	}

	// Verify original file is unchanged
	fileContent, _ := os.ReadFile(tmpFile)
	if string(fileContent) != content {
		t.Error("original file was modified during preview")
	}
}

func TestApplyToFile(t *testing.T) {
	registry := parser.NewRegistry()
	defer registry.Close()
	e := NewEngine(registry)

	// Create a temp file
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.go")

	content := `package main

func Hello() {
	println("hello")
}
`
	if err := os.WriteFile(tmpFile, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	ctx := context.Background()
	edit := &ASTEdit{
		File:       tmpFile,
		Operation:  EditReplace,
		Selector:   ASTSelector{Kind: "function_declaration"},
		NewContent: "func NewHello() {\n\tprintln(\"new hello\")\n}",
	}

	result, err := e.Apply(ctx, edit)
	if err != nil {
		t.Fatalf("apply failed: %v", err)
	}

	if !result.Success {
		t.Fatalf("apply was not successful: %s", result.Error)
	}

	if !result.Applied {
		t.Error("apply should set Applied=true")
	}

	// Verify file was modified
	fileContent, _ := os.ReadFile(tmpFile)
	if !strings.Contains(string(fileContent), "NewHello") {
		t.Error("file was not modified")
	}
}

// TestApplyRevalidatesPathBeforeWrite verifies the write-time admission re-check:
// an Engine whose validator rejects the target must refuse the write and leave
// the file untouched (the TOCTOU guard against a symlink swapped in after the
// handler's original IsPathAllowed check).
func TestApplyRevalidatesPathBeforeWrite(t *testing.T) {
	registry := parser.NewRegistry()
	defer registry.Close()

	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.go")
	original := "package main\n\nfunc Hello() {}\n"
	if err := os.WriteFile(tmpFile, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}

	// Validator denies the path (as IsPathAllowed would for a path that now
	// resolves outside the workspace root).
	e := NewEngineWithValidator(registry, func(string) bool { return false })

	result, err := e.Apply(context.Background(), &ASTEdit{
		File:       tmpFile,
		Operation:  EditReplace,
		Selector:   ASTSelector{Kind: "function_declaration"},
		NewContent: "func Evil() {}",
	})
	if err != nil {
		t.Fatalf("apply returned transport error: %v", err)
	}
	if result.Success || result.Applied {
		t.Errorf("rejected-path edit must not apply (Success=%v Applied=%v)", result.Success, result.Applied)
	}
	if got, _ := os.ReadFile(tmpFile); string(got) != original {
		t.Errorf("file must be unchanged when path validation fails; got:\n%s", got)
	}
}

// TestApplyPreservesFileMode verifies an edit keeps the file's existing
// permission bits rather than resetting them.
func TestApplyPreservesFileMode(t *testing.T) {
	registry := parser.NewRegistry()
	defer registry.Close()
	e := NewEngine(registry)

	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "script.go")
	if err := os.WriteFile(tmpFile, []byte("package main\n\nfunc Hello() {}\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	result, err := e.Apply(context.Background(), &ASTEdit{
		File:       tmpFile,
		Operation:  EditReplace,
		Selector:   ASTSelector{Kind: "function_declaration"},
		NewContent: "func Hi() {}",
	})
	if err != nil || !result.Success {
		t.Fatalf("apply failed: err=%v result=%+v", err, result)
	}
	info, err := os.Stat(tmpFile)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o755 {
		t.Errorf("file mode not preserved: got %o, want 0755", perm)
	}
}

// An edit on a CRLF file must normalize inserted LF content to CRLF so the file
// keeps a single, consistent line-ending convention (a documented guarantee).
func TestApplyPreservesCRLF(t *testing.T) {
	registry := parser.NewRegistry()
	defer registry.Close()
	e := NewEngine(registry)

	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "crlf.go")
	content := "package main\r\n\r\nfunc Hello() {\r\n\tprintln(\"hi\")\r\n}\r\n"
	if err := os.WriteFile(tmpFile, []byte(content), 0600); err != nil {
		t.Fatalf("write: %v", err)
	}

	_, err := e.Apply(context.Background(), &ASTEdit{
		File:       tmpFile,
		Operation:  EditReplace,
		Selector:   ASTSelector{Kind: "function_declaration"},
		NewContent: "func Bye() {\n\tprintln(\"bye\")\n}", // LF newlines on input
	})
	if err != nil {
		t.Fatalf("apply failed: %v", err)
	}

	out, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, "func Bye()") {
		t.Fatalf("edit not applied:\n%q", s)
	}
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' && (i == 0 || s[i-1] != '\r') {
			t.Fatalf("bare LF at offset %d — CRLF not preserved:\n%q", i, s)
		}
	}
}

func TestGenerateDiff(t *testing.T) {
	original := "line1\nline2\nline3"
	modified := "line1\nmodified\nline3"
	filename := "test.txt"

	diff := (&Engine{dmp: diffmatchpatch.New()}).generateDiff(original, modified, filename)

	if !strings.Contains(diff, "---") {
		t.Error("diff should contain --- header")
	}
	if !strings.Contains(diff, "+++") {
		t.Error("diff should contain +++ header")
	}
	if !strings.Contains(diff, "-line2") {
		t.Error("diff should show removed line")
	}
	if !strings.Contains(diff, "+modified") {
		t.Error("diff should show added line")
	}
}

func TestGenerateDiffLineLevelAccuracy(t *testing.T) {
	// Regression test: diff must operate at line level, not character level.
	// A character-level diff would split "hello" and "hello world" mid-line,
	// producing broken output like:
	//   fmt.Println("hello
	//   + world
	//   ")
	original := "package main\n\nfunc hello() {\n\tfmt.Println(\"hello\")\n}\n"
	modified := "package main\n\nfunc hello() {\n\tfmt.Println(\"hello world\")\n}\n"

	diff := (&Engine{dmp: diffmatchpatch.New()}).generateDiff(original, modified, "test.go")

	// The diff must show whole-line removals and additions
	if !strings.Contains(diff, "-\tfmt.Println(\"hello\")\n") {
		t.Errorf("diff should show full removed line, got:\n%s", diff)
	}
	if !strings.Contains(diff, "+\tfmt.Println(\"hello world\")\n") {
		t.Errorf("diff should show full added line, got:\n%s", diff)
	}

	// The diff must NOT split lines at character boundaries
	if strings.Contains(diff, "+hello") && !strings.Contains(diff, "Println") {
		t.Errorf("diff appears to be character-level (split mid-line), got:\n%s", diff)
	}

	// Context lines should not be marked as changed
	for line := range strings.SplitSeq(diff, "\n") {
		if strings.HasPrefix(line, "-") || strings.HasPrefix(line, "+") {
			// Changed lines should only be the Println lines
			if strings.Contains(line, "package main") ||
				strings.Contains(line, "func hello()") {
				t.Errorf("unchanged line incorrectly marked as changed: %q", line)
			}
		}
	}
}

// Hunk @@ headers must be correct at file boundaries (previously approximate).
func TestGenerateDiffHunkHeaderBounds(t *testing.T) {
	e := &Engine{dmp: diffmatchpatch.New()}
	tests := []struct {
		name     string
		original string
		modified string
		wantHdr  string
	}{
		{name: "insert into empty file", original: "", modified: "X\nY\n", wantHdr: "@@ -0,0 +1,2 @@"},
		{name: "delete entire file", original: "a\nb\n", modified: "", wantHdr: "@@ -1,2 +0,0 @@"},
		{name: "mid-file change keeps 1-based start", original: "l1\nl2\nl3\nl4\nl5\n", modified: "l1\nl2\nCHANGED\nl4\nl5\n", wantHdr: "@@ -1,5 +1,5 @@"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diff := e.generateDiff(tt.original, tt.modified, "f.txt")
			if !strings.Contains(diff, tt.wantHdr) {
				t.Errorf("expected hunk header %q, got:\n%s", tt.wantHdr, diff)
			}
		})
	}
}

func TestGenerateDiffNoPhantomChanges(t *testing.T) {
	// Regression test: replacing a line range should not produce phantom
	// +/- lines for unchanged code after the edit region.
	original := "line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\n"
	modified := "line1\nREPLACED\nline3\nline4\nline5\nline6\nline7\nline8\n"

	diff := (&Engine{dmp: diffmatchpatch.New()}).generateDiff(original, modified, "test.txt")

	// Count changed lines (excluding headers)
	addCount := 0
	delCount := 0
	for line := range strings.SplitSeq(diff, "\n") {
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			addCount++
		}
		if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
			delCount++
		}
	}

	if addCount != 1 {
		t.Errorf("expected 1 added line, got %d. Diff:\n%s", addCount, diff)
	}
	if delCount != 1 {
		t.Errorf("expected 1 deleted line, got %d. Diff:\n%s", delCount, diff)
	}
}

// ==================== Text-based editing tests ====================

func TestTextEditWithExactText(t *testing.T) {
	registry := parser.NewRegistry()
	defer registry.Close()
	e := NewEngine(registry)

	// Create a temp markdown file
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "README.md")

	content := `# My Project

## Installation

Run the following command:

## Usage

See the docs.
`
	if err := os.WriteFile(tmpFile, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	ctx := context.Background()
	edit := &ASTEdit{
		File:       tmpFile,
		Operation:  EditReplace,
		Selector:   ASTSelector{Text: "## Installation"},
		NewContent: "## Getting Started",
	}

	result, err := e.Apply(ctx, edit)
	if err != nil {
		t.Fatalf("apply failed: %v", err)
	}

	if !result.Success {
		t.Fatalf("apply was not successful: %s", result.Error)
	}

	// Verify file was modified
	fileContent, _ := os.ReadFile(tmpFile)
	if !strings.Contains(string(fileContent), "## Getting Started") {
		t.Error("file was not modified correctly")
	}
	if strings.Contains(string(fileContent), "## Installation") {
		t.Error("old text should be replaced")
	}
}

func TestTextEditWithLineRange(t *testing.T) {
	registry := parser.NewRegistry()
	defer registry.Close()
	e := NewEngine(registry)

	// Create a temp config file
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "config.yaml")

	content := `name: myapp
version: 1.0.0
database:
  host: localhost
  port: 5432
logging:
  level: debug
`
	if err := os.WriteFile(tmpFile, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	ctx := context.Background()
	edit := &ASTEdit{
		File:      tmpFile,
		Operation: EditReplace,
		Selector: ASTSelector{
			AtLine:  3,
			LineEnd: 5,
		},
		NewContent: "database:\n  host: production.db.example.com\n  port: 5433",
	}

	result, err := e.Apply(ctx, edit)
	if err != nil {
		t.Fatalf("apply failed: %v", err)
	}

	if !result.Success {
		t.Fatalf("apply was not successful: %s", result.Error)
	}

	// Verify file was modified
	fileContent, _ := os.ReadFile(tmpFile)
	if !strings.Contains(string(fileContent), "production.db.example.com") {
		t.Error("file was not modified correctly")
	}
}

func TestTextEditWithRegexPattern(t *testing.T) {
	registry := parser.NewRegistry()
	defer registry.Close()
	e := NewEngine(registry)

	// Create a temp JSON file
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "package.json")

	content := `{
  "name": "my-package",
  "version": "1.0.0",
  "description": "A test package"
}
`
	if err := os.WriteFile(tmpFile, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	ctx := context.Background()
	edit := &ASTEdit{
		File:       tmpFile,
		Operation:  EditReplace,
		Selector:   ASTSelector{TextPattern: `"version":\s*"[^"]+"`},
		NewContent: `"version": "2.0.0"`,
	}

	result, err := e.Apply(ctx, edit)
	if err != nil {
		t.Fatalf("apply failed: %v", err)
	}

	if !result.Success {
		t.Fatalf("apply was not successful: %s", result.Error)
	}

	// Verify file was modified
	fileContent, _ := os.ReadFile(tmpFile)
	if !strings.Contains(string(fileContent), `"version": "2.0.0"`) {
		t.Error("file was not modified correctly")
	}
}

func TestTextEditInsertAfter(t *testing.T) {
	registry := parser.NewRegistry()
	defer registry.Close()
	e := NewEngine(registry)

	// Create a temp env file
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, ".env")

	content := `DATABASE_URL=postgres://localhost/mydb
SECRET_KEY=abc123
`
	if err := os.WriteFile(tmpFile, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	ctx := context.Background()
	edit := &ASTEdit{
		File:       tmpFile,
		Operation:  EditInsertAfter,
		Selector:   ASTSelector{Text: "DATABASE_URL=postgres://localhost/mydb"},
		NewContent: "REDIS_URL=redis://localhost:6379",
	}

	result, err := e.Apply(ctx, edit)
	if err != nil {
		t.Fatalf("apply failed: %v", err)
	}

	if !result.Success {
		t.Fatalf("apply was not successful: %s", result.Error)
	}

	// Verify file was modified
	fileContent, _ := os.ReadFile(tmpFile)
	if !strings.Contains(string(fileContent), "REDIS_URL=redis://localhost:6379") {
		t.Error("file was not modified correctly")
	}
}

func TestTextEditMultipleMatchesSelectsFirst(t *testing.T) {
	registry := parser.NewRegistry()
	defer registry.Close()
	e := NewEngine(registry)

	// Create a temp file with repeated text
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.txt")

	content := `TODO: fix this
some code here
TODO: also fix this
more code
`
	if err := os.WriteFile(tmpFile, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	ctx := context.Background()
	edit := &ASTEdit{
		File:       tmpFile,
		Operation:  EditReplace,
		Selector:   ASTSelector{Text: "TODO"},
		NewContent: "DONE",
	}

	result, err := e.Apply(ctx, edit)
	if err != nil {
		t.Fatalf("apply failed: %v", err)
	}

	// Index 0 (default) should select the first match
	if !result.Success {
		t.Fatalf("expected success for multiple matches with default index 0: %s", result.Error)
	}

	// Verify only first TODO was replaced
	fileContent, _ := os.ReadFile(tmpFile)
	contentStr := string(fileContent)
	if !strings.Contains(contentStr, "DONE: fix this") {
		t.Error("first TODO should be replaced with DONE")
	}
	if !strings.Contains(contentStr, "TODO: also fix this") {
		t.Error("second TODO should be unchanged")
	}
}

func TestTextEditWithIndex(t *testing.T) {
	registry := parser.NewRegistry()
	defer registry.Close()
	e := NewEngine(registry)

	// Create a temp file with repeated text
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.txt")

	content := `TODO: fix this
some code here
TODO: also fix this
more code
`
	if err := os.WriteFile(tmpFile, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	ctx := context.Background()
	edit := &ASTEdit{
		File:      tmpFile,
		Operation: EditReplace,
		Selector: ASTSelector{
			Text:  "TODO",
			Index: 1, // Select second match
		},
		NewContent: "DONE",
	}

	result, err := e.Apply(ctx, edit)
	if err != nil {
		t.Fatalf("apply failed: %v", err)
	}

	if !result.Success {
		t.Fatalf("apply was not successful: %s", result.Error)
	}

	// Verify only second TODO was replaced
	fileContent, _ := os.ReadFile(tmpFile)
	contentStr := string(fileContent)
	if !strings.Contains(contentStr, "TODO: fix this") {
		t.Error("first TODO should not be replaced")
	}
	if !strings.Contains(contentStr, "DONE: also fix this") {
		t.Error("second TODO should be replaced")
	}
}

func TestValidateTextEdit(t *testing.T) {
	e := NewEngine(parser.NewRegistry())

	tests := []struct {
		edit    *ASTEdit
		name    string
		wantErr bool
	}{
		{
			name: "valid text selector",
			edit: &ASTEdit{
				File:       "test.md",
				Operation:  EditReplace,
				Selector:   ASTSelector{Text: "some text"},
				NewContent: "new text",
			},
			wantErr: false,
		},
		{
			name: "valid pattern selector",
			edit: &ASTEdit{
				File:       "test.md",
				Operation:  EditReplace,
				Selector:   ASTSelector{TextPattern: "\\d+"},
				NewContent: "replaced",
			},
			wantErr: false,
		},
		{
			name: "valid line selector",
			edit: &ASTEdit{
				File:       "test.md",
				Operation:  EditReplace,
				Selector:   ASTSelector{AtLine: 5},
				NewContent: "new line",
			},
			wantErr: false,
		},
		{
			name: "empty selector",
			edit: &ASTEdit{
				File:       "test.md",
				Operation:  EditReplace,
				Selector:   ASTSelector{},
				NewContent: "new text",
			},
			wantErr: true,
		},
		{
			name: "invalid regex pattern",
			edit: &ASTEdit{
				File:       "test.md",
				Operation:  EditReplace,
				Selector:   ASTSelector{TextPattern: "[invalid"},
				NewContent: "new text",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := e.validateTextEdit(tt.edit)
			if tt.wantErr && err == nil {
				t.Error("expected error")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestResolveSelectorAtLineSpecificity(t *testing.T) {
	// This test verifies that when using AtLine selector without Kind,
	// the smallest (most specific) node is selected, not the largest parent.
	registry := parser.NewRegistry()
	defer registry.Close()
	e := NewEngine(registry)

	// Create a Go file with nested structures
	content := []byte(`package main

const (
	// First const group
	FOO = "foo"
	BAR = "bar"
)

const (
	// Second const group
	BAZ = "baz"
	QUX = "qux"
)

func main() {
	println(FOO)
}
`)

	ctx := context.Background()
	result, err := registry.Parse(ctx, "test.go", content)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	// Selector at line 5 (FOO = "foo") should match the specific const_spec,
	// not the entire const_declaration or source_file
	node, err := e.resolveSelector(ASTSelector{AtLine: 5}, result.Tree, content)
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}

	// The node should be small - just the "FOO = \"foo\"" part
	nodeText := string(content[node.StartByte():node.EndByte()])
	if strings.Contains(nodeText, "BAR") {
		t.Errorf("selected node is too large (contains BAR): %q", nodeText)
	}
	if strings.Contains(nodeText, "package") {
		t.Errorf("selected node is the entire file: %q", truncateString(nodeText, 50))
	}

	t.Logf("Selected node type: %s, text: %q", node.Type(), truncateString(nodeText, 100))
}

// ==================== Regression tests for file corruption bug ====================
// These tests verify that the fix for the AtLine selector specificity issue
// prevents file corruption when inserting content.

func TestRegressionInsertAfterAtLine(t *testing.T) {
	// Regression test: Insert after a specific const should not corrupt the file
	// by inserting at the wrong location (e.g., at start of file or wrong block)
	registry := parser.NewRegistry()
	defer registry.Close()
	e := NewEngine(registry)

	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "queries.go")

	// Simulate a typical Go queries file
	content := `package org

const (
	// queryGetOrg retrieves an organization by ID
	queryGetOrg = ` + "`" + `
		SELECT id, name FROM orgs WHERE id = $1
	` + "`" + `

	// queryListOrgs lists all organizations
	queryListOrgs = ` + "`" + `
		SELECT id, name FROM orgs
	` + "`" + `
)

const (
	// queryCreateOrg creates a new organization
	queryCreateOrg = ` + "`" + `
		INSERT INTO orgs (name) VALUES ($1)
	` + "`" + `
)
`
	if err := os.WriteFile(tmpFile, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	ctx := context.Background()

	// Insert after the first const block (line ~14) - should NOT corrupt file
	edit := &ASTEdit{
		File:      tmpFile,
		Operation: EditInsertAfter,
		Selector: ASTSelector{
			Kind:   "const_declaration",
			AtLine: 5, // Line within the first const block
		},
		NewContent: `const (
	// queryGetGitHub retrieves GitHub settings
	queryGetGitHub = ` + "`" + `SELECT * FROM github` + "`" + `
)`,
	}

	result, err := e.Apply(ctx, edit)
	if err != nil {
		t.Fatalf("apply failed: %v", err)
	}

	if !result.Success {
		t.Fatalf("apply was not successful: %s", result.Error)
	}

	// Verify the file structure is preserved
	newContent, _ := os.ReadFile(tmpFile)
	newStr := string(newContent)

	// Check the file still starts with package declaration
	if !strings.HasPrefix(newStr, "package org") {
		t.Errorf("file corrupted: package declaration missing or moved\nContent:\n%s", newStr)
	}

	// Check original content is preserved
	if !strings.Contains(newStr, "queryGetOrg") {
		t.Error("original queryGetOrg was lost")
	}
	if !strings.Contains(newStr, "queryListOrgs") {
		t.Error("original queryListOrgs was lost")
	}
	if !strings.Contains(newStr, "queryCreateOrg") {
		t.Error("original queryCreateOrg was lost")
	}

	// Check new content was added
	if !strings.Contains(newStr, "queryGetGitHub") {
		t.Error("new queryGetGitHub was not added")
	}

	// Verify insertion location: queryGetGitHub should appear AFTER queryListOrgs
	// but the exact position depends on where the first const block ends
	listOrgsIdx := strings.Index(newStr, "queryListOrgs")
	gitHubIdx := strings.Index(newStr, "queryGetGitHub")
	if gitHubIdx < listOrgsIdx {
		t.Errorf("queryGetGitHub inserted before queryListOrgs - wrong location")
	}

	t.Logf("Result diff:\n%s", result.Diff)
}

func TestRegressionInsertBeforeAtLine(t *testing.T) {
	// Regression test: Insert before should work correctly with AtLine
	registry := parser.NewRegistry()
	defer registry.Close()
	e := NewEngine(registry)

	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "main.go")

	content := `package main

import "fmt"

func hello() {
	fmt.Println("hello")
}

func goodbye() {
	fmt.Println("goodbye")
}
`
	if err := os.WriteFile(tmpFile, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	ctx := context.Background()

	// Insert before goodbye function
	edit := &ASTEdit{
		File:      tmpFile,
		Operation: EditInsertBefore,
		Selector: ASTSelector{
			Kind:   "function_declaration",
			Name:   "goodbye",
			AtLine: 9,
		},
		NewContent: `func middle() {
	fmt.Println("middle")
}`,
	}

	result, err := e.Apply(ctx, edit)
	if err != nil {
		t.Fatalf("apply failed: %v", err)
	}

	if !result.Success {
		t.Fatalf("apply was not successful: %s", result.Error)
	}

	newContent, _ := os.ReadFile(tmpFile)
	newStr := string(newContent)

	// Verify order: hello -> middle -> goodbye
	helloIdx := strings.Index(newStr, "func hello()")
	middleIdx := strings.Index(newStr, "func middle()")
	goodbyeIdx := strings.Index(newStr, "func goodbye()")

	if helloIdx == -1 || middleIdx == -1 || goodbyeIdx == -1 {
		t.Fatalf("missing functions in output:\n%s", newStr)
	}

	if helloIdx >= middleIdx || middleIdx >= goodbyeIdx {
		t.Errorf("functions in wrong order: hello=%d, middle=%d, goodbye=%d", helloIdx, middleIdx, goodbyeIdx)
	}
}

func TestRegressionNestedStructures(t *testing.T) {
	// Regression test: Nested structures should select the correct node
	registry := parser.NewRegistry()
	defer registry.Close()
	e := NewEngine(registry)

	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "nested.go")

	content := `package main

type Outer struct {
	Inner struct {
		Field string
	}
	Other int
}

func main() {
	o := Outer{}
	_ = o
}
`
	if err := os.WriteFile(tmpFile, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	ctx := context.Background()

	// Replace the Inner struct field - should not affect Outer
	edit := &ASTEdit{
		File:      tmpFile,
		Operation: EditReplace,
		Selector: ASTSelector{
			AtLine: 5, // Line with "Field string"
		},
		NewContent: `Name string`,
	}

	result, err := e.Apply(ctx, edit)
	if err != nil {
		t.Fatalf("apply failed: %v", err)
	}

	if !result.Success {
		t.Fatalf("apply was not successful: %s", result.Error)
	}

	newContent, _ := os.ReadFile(tmpFile)
	newStr := string(newContent)

	// Verify structure is preserved
	if !strings.Contains(newStr, "type Outer struct") {
		t.Error("Outer struct declaration lost")
	}
	if !strings.Contains(newStr, "Inner struct") {
		t.Error("Inner struct declaration lost")
	}
	if !strings.Contains(newStr, "Other int") {
		t.Error("Other field lost")
	}
	if !strings.Contains(newStr, "Name string") {
		t.Error("Name field not added")
	}
	if strings.Contains(newStr, "Field string") {
		t.Error("Old Field string should be replaced")
	}
}

func TestRegressionPreservesFileIntegrity(t *testing.T) {
	// Regression test: Edit should not corrupt unrelated parts of the file
	registry := parser.NewRegistry()
	defer registry.Close()
	e := NewEngine(registry)

	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "integrity.go")

	content := `package main

// Copyright 2024 Example Corp
// License: MIT

import (
	"fmt"
	"os"
)

const Version = "1.0.0"

func main() {
	fmt.Println(Version)
	os.Exit(0)
}

// End of file comment
`
	if err := os.WriteFile(tmpFile, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	ctx := context.Background()

	// Replace the Version constant (line 11)
	edit := &ASTEdit{
		File:      tmpFile,
		Operation: EditReplace,
		Selector: ASTSelector{
			Kind:   "const_declaration",
			AtLine: 11,
		},
		NewContent: `const Version = "2.0.0"`,
	}

	result, err := e.Apply(ctx, edit)
	if err != nil {
		t.Fatalf("apply failed: %v", err)
	}

	if !result.Success {
		t.Fatalf("apply was not successful: %s", result.Error)
	}

	newContent, _ := os.ReadFile(tmpFile)
	newStr := string(newContent)

	// Verify all unrelated parts are preserved exactly
	checks := []string{
		"package main",
		"// Copyright 2024 Example Corp",
		"// License: MIT",
		`"fmt"`,
		`"os"`,
		"func main()",
		"fmt.Println(Version)",
		"os.Exit(0)",
		"// End of file comment",
	}

	for _, check := range checks {
		if !strings.Contains(newStr, check) {
			t.Errorf("file integrity violated: missing %q\nContent:\n%s", check, newStr)
		}
	}

	// Verify the change was made
	if !strings.Contains(newStr, `"2.0.0"`) {
		t.Error("Version was not updated to 2.0.0")
	}
}

func TestRegressionMultipleConstBlocks(t *testing.T) {
	// Regression test: Multiple const blocks - edit should target correct one
	registry := parser.NewRegistry()
	defer registry.Close()
	e := NewEngine(registry)

	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "consts.go")

	content := `package main

const (
	A = 1
	B = 2
)

const (
	C = 3
	D = 4
)

const (
	E = 5
	F = 6
)
`
	if err := os.WriteFile(tmpFile, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	ctx := context.Background()

	// Insert after the second const block (containing C, D)
	edit := &ASTEdit{
		File:      tmpFile,
		Operation: EditInsertAfter,
		Selector: ASTSelector{
			Kind:   "const_declaration",
			AtLine: 9, // Line with C = 3
		},
		NewContent: `const X = 99`,
	}

	result, err := e.Apply(ctx, edit)
	if err != nil {
		t.Fatalf("apply failed: %v", err)
	}

	if !result.Success {
		t.Fatalf("apply was not successful: %s", result.Error)
	}

	newContent, _ := os.ReadFile(tmpFile)
	newStr := string(newContent)

	// X should appear after D but before E
	dIdx := strings.Index(newStr, "D = 4")
	xIdx := strings.Index(newStr, "X = 99")
	eIdx := strings.Index(newStr, "E = 5")

	if xIdx == -1 {
		t.Fatalf("X = 99 not found in output:\n%s", newStr)
	}

	if dIdx >= xIdx || xIdx >= eIdx {
		t.Errorf("X inserted in wrong position: D=%d, X=%d, E=%d\nContent:\n%s", dIdx, xIdx, eIdx, newStr)
	}
}

func TestSortBySpecificity(t *testing.T) {
	// Unit test for the sortBySpecificity helper function
	registry := parser.NewRegistry()
	defer registry.Close()

	content := []byte(`package main

const (
	FOO = "foo"
)
`)

	ctx := context.Background()
	result, err := registry.Parse(ctx, "test.go", content)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	// Collect all nodes that span line 4
	var nodesAtLine4 []*sitter.Node
	parser.WalkTree(result.Tree.RootNode(), func(n *sitter.Node) bool {
		startLine := int(n.StartPoint().Row) + 1
		endLine := int(n.EndPoint().Row) + 1
		if 4 >= startLine && 4 <= endLine {
			nodesAtLine4 = append(nodesAtLine4, n)
		}
		return true
	})

	if len(nodesAtLine4) < 2 {
		t.Skip("not enough nodes to test sorting")
	}

	// Sort and verify smallest declaration-like node comes first
	sorted := sortBySpecificity(nodesAtLine4)

	// First node should be a declaration-like node, not source_file
	firstType := sorted[0].Type()
	if firstType == "source_file" {
		t.Errorf("source_file should not be first after sorting, got types: %v",
			nodeTypes(sorted))
	}

	t.Logf("Sorted node types: %v", nodeTypes(sorted))
}

func nodeTypes(nodes []*sitter.Node) []string {
	types := make([]string, len(nodes))
	for i, n := range nodes {
		types[i] = n.Type()
	}
	return types
}

func TestFindLineRange(t *testing.T) {
	e := NewEngine(parser.NewRegistry())

	content := []byte("line1\nline2\nline3\nline4\nline5")

	// Content: "line1\nline2\nline3\nline4\nline5" (no trailing newline)
	// Positions: line1=0-5, \n=5, line2=6-10, \n=11, line3=12-16, \n=17, line4=18-22, \n=23, line5=24-28
	tests := []struct {
		name      string
		lineStart int
		lineEnd   int
		wantStart int
		wantEnd   int
		wantErr   bool
	}{
		{
			name:      "single line",
			lineStart: 2,
			lineEnd:   0, // defaults to lineStart
			wantStart: 6,
			wantEnd:   12, // includes trailing newline
			wantErr:   false,
		},
		{
			name:      "range of lines",
			lineStart: 2,
			lineEnd:   4,
			wantStart: 6,
			wantEnd:   24, // through end of line4 including newline
			wantErr:   false,
		},
		{
			name:      "first line",
			lineStart: 1,
			lineEnd:   1,
			wantStart: 0,
			wantEnd:   6, // includes trailing newline
			wantErr:   false,
		},
		{
			name:      "line out of range",
			lineStart: 10,
			lineEnd:   10,
			wantErr:   true,
		},
		{
			name:      "invalid line number",
			lineStart: 0,
			lineEnd:   1,
			wantErr:   true,
		},
		{
			name:      "end before start",
			lineStart: 3,
			lineEnd:   2,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, end, err := e.findLineRange(content, tt.lineStart, tt.lineEnd)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if start != tt.wantStart {
				t.Errorf("start = %d, want %d", start, tt.wantStart)
			}
			if end != tt.wantEnd {
				t.Errorf("end = %d, want %d", end, tt.wantEnd)
			}
		})
	}
}

// ==================== Line merge regression tests ====================
// These tests verify that edit operations don't merge adjacent lines.
// Each test checks exact line-by-line output, not just substring presence.

func TestRegressionLineMergeReplace(t *testing.T) {
	registry := parser.NewRegistry()
	defer registry.Close()
	e := NewEngine(registry)

	tests := []struct {
		name      string
		content   string
		edit      *ASTEdit
		wantLines []string
	}{
		{
			name:    "replace mid-line by line range preserves line boundaries",
			content: "line1\nline2\nline3\nline4\n",
			edit: &ASTEdit{
				Operation:  EditReplace,
				Selector:   ASTSelector{AtLine: 2},
				NewContent: "replaced",
			},
			wantLines: []string{"line1", "replaced", "line3", "line4", ""},
		},
		{
			name:    "replace multi-line range preserves boundaries",
			content: "line1\nline2\nline3\nline4\n",
			edit: &ASTEdit{
				Operation:  EditReplace,
				Selector:   ASTSelector{AtLine: 2, LineEnd: 3},
				NewContent: "newA\nnewB",
			},
			wantLines: []string{"line1", "newA", "newB", "line4", ""},
		},
		{
			name:    "replace last line without trailing newline",
			content: "line1\nline2\nline3",
			edit: &ASTEdit{
				Operation:  EditReplace,
				Selector:   ASTSelector{AtLine: 3},
				NewContent: "replaced",
			},
			wantLines: []string{"line1", "line2", "replaced"},
		},
		{
			name:    "replace single line with multi-line content",
			content: "before\ntarget\nafter\n",
			edit: &ASTEdit{
				Operation:  EditReplace,
				Selector:   ASTSelector{AtLine: 2},
				NewContent: "new1\nnew2\nnew3",
			},
			wantLines: []string{"before", "new1", "new2", "new3", "after", ""},
		},
		{
			name:    "replace by exact text preserves surrounding lines",
			content: "aaa\nbbb\nccc\n",
			edit: &ASTEdit{
				Operation:  EditReplace,
				Selector:   ASTSelector{Text: "bbb"},
				NewContent: "xxx",
			},
			wantLines: []string{"aaa", "xxx", "ccc", ""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			tmpFile := filepath.Join(tmpDir, "test.txt")
			if err := os.WriteFile(tmpFile, []byte(tt.content), 0600); err != nil {
				t.Fatalf("write: %v", err)
			}
			tt.edit.File = tmpFile

			result, err := e.Apply(context.Background(), tt.edit)
			if err != nil {
				t.Fatalf("apply: %v", err)
			}
			if !result.Success {
				t.Fatalf("not successful: %s", result.Error)
			}

			got, _ := os.ReadFile(tmpFile)
			gotLines := strings.Split(string(got), "\n")
			if len(gotLines) != len(tt.wantLines) {
				t.Fatalf("line count = %d, want %d\ngot:  %q\nwant: %q", len(gotLines), len(tt.wantLines), gotLines, tt.wantLines)
			}
			for i, want := range tt.wantLines {
				if gotLines[i] != want {
					t.Errorf("line %d = %q, want %q", i+1, gotLines[i], want)
				}
			}
		})
	}
}

func TestRegressionLineMergeInsertAfter(t *testing.T) {
	registry := parser.NewRegistry()
	defer registry.Close()
	e := NewEngine(registry)

	tests := []struct {
		name      string
		content   string
		edit      *ASTEdit
		wantLines []string
	}{
		{
			name:    "insert after mid-line doesn't merge with next line",
			content: "line1\nline2\nline3\n",
			edit: &ASTEdit{
				Operation:  EditInsertAfter,
				Selector:   ASTSelector{Text: "line2"},
				NewContent: "inserted",
			},
			wantLines: []string{"line1", "line2", "inserted", "line3", ""},
		},
		{
			name:    "insert after last line without trailing newline",
			content: "line1\nline2\nline3",
			edit: &ASTEdit{
				Operation:  EditInsertAfter,
				Selector:   ASTSelector{Text: "line3"},
				NewContent: "inserted",
			},
			wantLines: []string{"line1", "line2", "line3", "inserted"},
		},
		{
			name:    "insert multi-line block after line",
			content: "before\ntarget\nafter\n",
			edit: &ASTEdit{
				Operation:  EditInsertAfter,
				Selector:   ASTSelector{Text: "target"},
				NewContent: "new1\nnew2\nnew3",
			},
			wantLines: []string{"before", "target", "new1", "new2", "new3", "after", ""},
		},
		{
			name:    "insert after by line number",
			content: "aaa\nbbb\nccc\n",
			edit: &ASTEdit{
				Operation:  EditInsertAfter,
				Selector:   ASTSelector{AtLine: 2},
				NewContent: "inserted",
			},
			wantLines: []string{"aaa", "bbb", "inserted", "ccc", ""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			tmpFile := filepath.Join(tmpDir, "test.txt")
			if err := os.WriteFile(tmpFile, []byte(tt.content), 0600); err != nil {
				t.Fatalf("write: %v", err)
			}
			tt.edit.File = tmpFile

			result, err := e.Apply(context.Background(), tt.edit)
			if err != nil {
				t.Fatalf("apply: %v", err)
			}
			if !result.Success {
				t.Fatalf("not successful: %s", result.Error)
			}

			got, _ := os.ReadFile(tmpFile)
			gotLines := strings.Split(string(got), "\n")
			if len(gotLines) != len(tt.wantLines) {
				t.Fatalf("line count = %d, want %d\ngot:  %q\nwant: %q", len(gotLines), len(tt.wantLines), gotLines, tt.wantLines)
			}
			for i, want := range tt.wantLines {
				if gotLines[i] != want {
					t.Errorf("line %d = %q, want %q", i+1, gotLines[i], want)
				}
			}
		})
	}
}

func TestRegressionLineMergeInsertBefore(t *testing.T) {
	registry := parser.NewRegistry()
	defer registry.Close()
	e := NewEngine(registry)

	tests := []struct {
		name      string
		content   string
		edit      *ASTEdit
		wantLines []string
	}{
		{
			name:    "insert before mid-line doesn't merge",
			content: "line1\nline2\nline3\n",
			edit: &ASTEdit{
				Operation:  EditInsertBefore,
				Selector:   ASTSelector{Text: "line2"},
				NewContent: "inserted",
			},
			wantLines: []string{"line1", "inserted", "line2", "line3", ""},
		},
		{
			name:    "insert multi-line block before line",
			content: "before\ntarget\nafter\n",
			edit: &ASTEdit{
				Operation:  EditInsertBefore,
				Selector:   ASTSelector{Text: "target"},
				NewContent: "new1\nnew2",
			},
			wantLines: []string{"before", "new1", "new2", "target", "after", ""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			tmpFile := filepath.Join(tmpDir, "test.txt")
			if err := os.WriteFile(tmpFile, []byte(tt.content), 0600); err != nil {
				t.Fatalf("write: %v", err)
			}
			tt.edit.File = tmpFile

			result, err := e.Apply(context.Background(), tt.edit)
			if err != nil {
				t.Fatalf("apply: %v", err)
			}
			if !result.Success {
				t.Fatalf("not successful: %s", result.Error)
			}

			got, _ := os.ReadFile(tmpFile)
			gotLines := strings.Split(string(got), "\n")
			if len(gotLines) != len(tt.wantLines) {
				t.Fatalf("line count = %d, want %d\ngot:  %q\nwant: %q", len(gotLines), len(tt.wantLines), gotLines, tt.wantLines)
			}
			for i, want := range tt.wantLines {
				if gotLines[i] != want {
					t.Errorf("line %d = %q, want %q", i+1, gotLines[i], want)
				}
			}
		})
	}
}

func TestRegressionLineMergeDelete(t *testing.T) {
	registry := parser.NewRegistry()
	defer registry.Close()
	e := NewEngine(registry)

	tests := []struct {
		name      string
		content   string
		edit      *ASTEdit
		wantLines []string
	}{
		{
			name:    "delete mid-line preserves surrounding lines",
			content: "line1\nline2\nline3\n",
			edit: &ASTEdit{
				Operation: EditDelete,
				Selector:  ASTSelector{AtLine: 2},
			},
			wantLines: []string{"line1", "line3", ""},
		},
		{
			name:    "delete line range preserves surrounding lines",
			content: "line1\nline2\nline3\nline4\n",
			edit: &ASTEdit{
				Operation: EditDelete,
				Selector:  ASTSelector{AtLine: 2, LineEnd: 3},
			},
			wantLines: []string{"line1", "line4", ""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			tmpFile := filepath.Join(tmpDir, "test.txt")
			if err := os.WriteFile(tmpFile, []byte(tt.content), 0600); err != nil {
				t.Fatalf("write: %v", err)
			}
			tt.edit.File = tmpFile

			result, err := e.Apply(context.Background(), tt.edit)
			if err != nil {
				t.Fatalf("apply: %v", err)
			}
			if !result.Success {
				t.Fatalf("not successful: %s", result.Error)
			}

			got, _ := os.ReadFile(tmpFile)
			gotLines := strings.Split(string(got), "\n")
			if len(gotLines) != len(tt.wantLines) {
				t.Fatalf("line count = %d, want %d\ngot:  %q\nwant: %q", len(gotLines), len(tt.wantLines), gotLines, tt.wantLines)
			}
			for i, want := range tt.wantLines {
				if gotLines[i] != want {
					t.Errorf("line %d = %q, want %q", i+1, gotLines[i], want)
				}
			}
		})
	}
}
