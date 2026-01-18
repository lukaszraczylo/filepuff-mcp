package edit

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lukaszraczylo/mcp-filepuff/internal/parser"
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

func TestDetectIndentation(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
		pos     uint32
	}{
		{
			name:    "no indent",
			content: "func main() {}",
			pos:     0,
			want:    "",
		},
		{
			name:    "tab indent",
			content: "func main() {\n\tprintln(\"hello\")\n}",
			pos:     15,
			want:    "\t",
		},
		{
			name:    "space indent",
			content: "func main() {\n    println(\"hello\")\n}",
			pos:     18,
			want:    "    ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detectIndentation([]byte(tt.content), tt.pos)
			if got != tt.want {
				t.Errorf("detectIndentation() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGenerateDiff(t *testing.T) {
	original := "line1\nline2\nline3"
	modified := "line1\nmodified\nline3"
	filename := "test.txt"

	diff := generateDiff(original, modified, filename)

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

func TestTextEditMultipleMatchesError(t *testing.T) {
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

	// Should fail because of multiple matches
	if result.Success {
		t.Error("expected error for multiple matches without index")
	}
	if !strings.Contains(result.Error, "matches") {
		t.Errorf("error should mention multiple matches: %s", result.Error)
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
