package parser

import (
	"context"
	"testing"

	"github.com/lukaszraczylo/mcp-filepuff/pkg/protocol"
	sitter "github.com/smacker/go-tree-sitter"
)

func TestExtractGoDocComment(t *testing.T) {
	registry := NewRegistry()
	defer registry.Close()

	tests := []struct {
		name      string
		code      string
		nodeKind  string
		wantText  string
		wantStyle CommentStyle
	}{
		{
			name: "single line comment",
			code: `package main

// Hello says hello
func Hello() {}
`,
			nodeKind:  "function_declaration",
			wantText:  "Hello says hello",
			wantStyle: CommentStyleLine,
		},
		{
			name: "multi-line comments",
			code: `package main

// This is a function
// that does something
// important
func DoSomething() {}
`,
			nodeKind:  "function_declaration",
			wantText:  "This is a function\nthat does something\nimportant",
			wantStyle: CommentStyleLine,
		},
		{
			name: "block comment",
			code: `package main

/* This is a block comment
   describing the function */
func BlockCommented() {}
`,
			nodeKind:  "function_declaration",
			wantText:  "This is a block comment\ndescribing the function",
			wantStyle: CommentStyleBlock,
		},
		{
			name: "doc comment with asterisks",
			code: `package main

/*
 * This is a properly formatted
 * block comment with asterisks
 */
func FormattedBlock() {}
`,
			nodeKind:  "function_declaration",
			wantText:  "This is a properly formatted\nblock comment with asterisks",
			wantStyle: CommentStyleBlock,
		},
		{
			name: "no comment",
			code: `package main

func NoComment() {}
`,
			nodeKind: "function_declaration",
			wantText: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := registry.Parse(context.Background(), "test.go", []byte(tt.code))
			if err != nil {
				t.Fatalf("parse failed: %v", err)
			}

			// Find the target node
			targetNode := findNodeByKind(result.Tree.RootNode(), tt.nodeKind)
			if targetNode == nil {
				t.Fatalf("could not find node of type %s", tt.nodeKind)
			}

			doc := ExtractDocComment(targetNode, []byte(tt.code), protocol.LangGo)

			if tt.wantText == "" {
				if doc != nil && doc.Text != "" {
					t.Errorf("expected no doc, got %q", doc.Text)
				}
				return
			}

			if doc == nil {
				t.Fatal("expected doc, got nil")
				return
			}

			if doc.Text != tt.wantText {
				t.Errorf("text mismatch:\ngot:  %q\nwant: %q", doc.Text, tt.wantText)
			}

			if doc.Style != tt.wantStyle {
				t.Errorf("style mismatch: got %v, want %v", doc.Style, tt.wantStyle)
			}
		})
	}
}

func TestExtractJSDocComment(t *testing.T) {
	registry := NewRegistry()
	defer registry.Close()

	tests := []struct {
		wantTags  map[string]string
		name      string
		code      string
		nodeKind  string
		wantText  string
		wantStyle CommentStyle
	}{
		{
			name: "JSDoc comment",
			code: `/**
 * Adds two numbers together.
 * @param a The first number
 * @param b The second number
 * @returns The sum of a and b
 */
function add(a, b) {
    return a + b;
}
`,
			nodeKind:  "function_declaration",
			wantText:  "Adds two numbers together.",
			wantStyle: CommentStyleJSDoc,
			wantTags: map[string]string{
				"param":   "a The first number\nb The second number",
				"returns": "The sum of a and b",
			},
		},
		{
			name: "simple line comment",
			code: `// This is a simple function
function simple() {}
`,
			nodeKind:  "function_declaration",
			wantText:  "This is a simple function",
			wantStyle: CommentStyleLine,
		},
		{
			name: "JSDoc with types",
			code: `/**
 * @param {string} name - The name
 * @returns {boolean} True if valid
 */
function validate(name) {}
`,
			nodeKind:  "function_declaration",
			wantText:  "",
			wantStyle: CommentStyleJSDoc,
			wantTags: map[string]string{
				"param":   "{string} name - The name",
				"returns": "{boolean} True if valid",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := registry.Parse(context.Background(), "test.js", []byte(tt.code))
			if err != nil {
				t.Fatalf("parse failed: %v", err)
			}

			targetNode := findNodeByKind(result.Tree.RootNode(), tt.nodeKind)
			if targetNode == nil {
				t.Fatalf("could not find node of type %s", tt.nodeKind)
			}

			doc := ExtractDocComment(targetNode, []byte(tt.code), protocol.LangJavaScript)

			if doc == nil {
				t.Fatal("expected doc, got nil")
				return
			}

			if doc.Text != tt.wantText {
				t.Errorf("text mismatch:\ngot:  %q\nwant: %q", doc.Text, tt.wantText)
			}

			if doc.Style != tt.wantStyle {
				t.Errorf("style mismatch: got %v, want %v", doc.Style, tt.wantStyle)
			}

			if tt.wantTags != nil {
				for k, want := range tt.wantTags {
					if got := doc.Tags[k]; got != want {
						t.Errorf("tag %q mismatch:\ngot:  %q\nwant: %q", k, got, want)
					}
				}
			}
		})
	}
}

func TestExtractPythonDocComment(t *testing.T) {
	registry := NewRegistry()
	defer registry.Close()

	tests := []struct {
		name      string
		code      string
		nodeKind  string
		wantText  string
		wantStyle CommentStyle
	}{
		{
			name: "docstring",
			code: `def greet(name):
    """Greet a person by name."""
    print(f"Hello, {name}!")
`,
			nodeKind:  "function_definition",
			wantText:  "Greet a person by name.",
			wantStyle: CommentStyleDocstring,
		},
		{
			name: "multi-line docstring",
			code: `def calculate(x, y):
    """
    Calculate the sum of two numbers.

    Args:
        x: First number
        y: Second number

    Returns:
        The sum of x and y
    """
    return x + y
`,
			nodeKind:  "function_definition",
			wantText:  "Calculate the sum of two numbers.\n\n    Args:\n        x: First number\n        y: Second number\n\n    Returns:\n        The sum of x and y",
			wantStyle: CommentStyleDocstring,
		},
		{
			name: "class docstring",
			code: `class MyClass:
    """This is a class description."""
    pass
`,
			nodeKind:  "class_definition",
			wantText:  "This is a class description.",
			wantStyle: CommentStyleDocstring,
		},
		{
			name: "single quote docstring",
			code: `def func():
    '''Single quote docstring'''
    pass
`,
			nodeKind:  "function_definition",
			wantText:  "Single quote docstring",
			wantStyle: CommentStyleDocstring,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := registry.Parse(context.Background(), "test.py", []byte(tt.code))
			if err != nil {
				t.Fatalf("parse failed: %v", err)
			}

			targetNode := findNodeByKind(result.Tree.RootNode(), tt.nodeKind)
			if targetNode == nil {
				t.Fatalf("could not find node of type %s", tt.nodeKind)
			}

			doc := ExtractDocComment(targetNode, []byte(tt.code), protocol.LangPython)

			if doc == nil {
				t.Fatal("expected doc, got nil")
				return
			}

			if doc.Text != tt.wantText {
				t.Errorf("text mismatch:\ngot:  %q\nwant: %q", doc.Text, tt.wantText)
			}

			if doc.Style != tt.wantStyle {
				t.Errorf("style mismatch: got %v, want %v", doc.Style, tt.wantStyle)
			}
		})
	}
}

func TestExtractCDocComment(t *testing.T) {
	registry := NewRegistry()
	defer registry.Close()

	tests := []struct {
		wantTags  map[string]string
		name      string
		code      string
		nodeKind  string
		wantText  string
		wantStyle CommentStyle
	}{
		{
			name: "Doxygen block comment",
			code: `/**
 * Adds two numbers.
 * @param a First number
 * @param b Second number
 * @return Sum of a and b
 */
int add(int a, int b) {
    return a + b;
}
`,
			nodeKind:  "function_definition",
			wantText:  "Adds two numbers.",
			wantStyle: CommentStyleDoxygen,
			wantTags: map[string]string{
				"param":  "a First number\nb Second number",
				"return": "Sum of a and b",
			},
		},
		{
			name: "regular block comment",
			code: `/* This is a regular comment */
int regular() { return 0; }
`,
			nodeKind:  "function_definition",
			wantText:  "This is a regular comment",
			wantStyle: CommentStyleBlock,
		},
		{
			name: "line comment",
			code: `// Simple function
int simple() { return 1; }
`,
			nodeKind:  "function_definition",
			wantText:  "Simple function",
			wantStyle: CommentStyleLine,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := registry.Parse(context.Background(), "test.c", []byte(tt.code))
			if err != nil {
				t.Fatalf("parse failed: %v", err)
			}

			targetNode := findNodeByKind(result.Tree.RootNode(), tt.nodeKind)
			if targetNode == nil {
				t.Fatalf("could not find node of type %s", tt.nodeKind)
			}

			doc := ExtractDocComment(targetNode, []byte(tt.code), protocol.LangC)

			if doc == nil {
				t.Fatal("expected doc, got nil")
				return
			}

			if doc.Text != tt.wantText {
				t.Errorf("text mismatch:\ngot:  %q\nwant: %q", doc.Text, tt.wantText)
			}

			if doc.Style != tt.wantStyle {
				t.Errorf("style mismatch: got %v, want %v", doc.Style, tt.wantStyle)
			}

			if tt.wantTags != nil {
				for k, want := range tt.wantTags {
					if got := doc.Tags[k]; got != want {
						t.Errorf("tag %q mismatch:\ngot:  %q\nwant: %q", k, got, want)
					}
				}
			}
		})
	}
}

func TestParseJSDoc(t *testing.T) {
	tests := []struct {
		wantTags map[string]string
		name     string
		input    string
		wantText string
	}{
		{
			name: "complete jsdoc",
			input: `/**
 * This is a description.
 * Multiple lines.
 * @param {string} name The name
 * @returns {boolean} Result
 */`,
			wantText: "This is a description.\nMultiple lines.",
			wantTags: map[string]string{
				"param":   "{string} name The name",
				"returns": "{boolean} Result",
			},
		},
		{
			name:     "empty jsdoc",
			input:    `/** */`,
			wantText: "",
			wantTags: map[string]string{},
		},
		{
			name:     "only description",
			input:    `/** Simple description */`,
			wantText: "Simple description",
			wantTags: map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			text, tags := parseJSDoc(tt.input)

			if text != tt.wantText {
				t.Errorf("text mismatch:\ngot:  %q\nwant: %q", text, tt.wantText)
			}

			if len(tags) != len(tt.wantTags) {
				t.Errorf("tag count mismatch: got %d, want %d", len(tags), len(tt.wantTags))
			}

			for k, want := range tt.wantTags {
				if got := tags[k]; got != want {
					t.Errorf("tag %q mismatch:\ngot:  %q\nwant: %q", k, got, want)
				}
			}
		})
	}
}

func TestParseDoxygen(t *testing.T) {
	tests := []struct {
		wantTags map[string]string
		name     string
		input    string
		wantText string
	}{
		{
			name: "doxygen with @ tags",
			input: `/**
 * Brief description.
 * @param x Value
 * @return Result
 */`,
			wantText: "Brief description.",
			wantTags: map[string]string{
				"param":  "x Value",
				"return": "Result",
			},
		},
		{
			name: "doxygen with backslash tags",
			input: `/**
 * Description.
 * \param y Input
 * \retval Output value
 */`,
			wantText: "Description.",
			wantTags: map[string]string{
				"param":  "y Input",
				"retval": "Output value",
			},
		},
		{
			name:     "triple slash",
			input:    `/// Simple description`,
			wantText: "Simple description",
			wantTags: map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			text, tags := parseDoxygen(tt.input)

			if text != tt.wantText {
				t.Errorf("text mismatch:\ngot:  %q\nwant: %q", text, tt.wantText)
			}

			for k, want := range tt.wantTags {
				if got := tags[k]; got != want {
					t.Errorf("tag %q mismatch:\ngot:  %q\nwant: %q", k, got, want)
				}
			}
		})
	}
}

func TestFormatDocComment(t *testing.T) {
	tests := []struct {
		name string
		doc  *DocComment
		want string
	}{
		{
			name: "with tags",
			doc: &DocComment{
				Text: "This is a function.",
				Tags: map[string]string{
					"param":   "x The value",
					"returns": "The result",
				},
			},
			want: "This is a function.\n\n@param x The value\n@returns The result",
		},
		{
			name: "no tags",
			doc: &DocComment{
				Text: "Simple description.",
				Tags: nil,
			},
			want: "Simple description.",
		},
		{
			name: "nil doc",
			doc:  nil,
			want: "",
		},
		{
			name: "empty text",
			doc: &DocComment{
				Text: "",
				Tags: nil,
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatDocComment(tt.doc)
			if got != tt.want {
				t.Errorf("mismatch:\ngot:  %q\nwant: %q", got, tt.want)
			}
		})
	}
}

func TestDetectCommentStyle(t *testing.T) {
	tests := []struct {
		input string
		want  CommentStyle
	}{
		{"/** JSDoc */", CommentStyleJSDoc},
		{"/// Doxygen", CommentStyleDoxygen},
		{"//! Doxygen", CommentStyleDoxygen},
		{"/* block */", CommentStyleBlock},
		{"// line", CommentStyleLine},
		{"# hash", CommentStyleHash},
		{`"""docstring"""`, CommentStyleDocstring},
		{`'''docstring'''`, CommentStyleDocstring},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := detectCommentStyle(tt.input)
			if got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

// findNodeByKind finds the first node of the given kind.
func findNodeByKind(root *sitter.Node, nodeType string) *sitter.Node {
	if root == nil {
		return nil
	}

	var result *sitter.Node
	WalkTree(root, func(n *sitter.Node) bool {
		if n.Type() == nodeType {
			result = n
			return false // stop walking
		}
		return true
	})

	return result
}

func TestCleanBlockComment(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{
			input: "\n * Line 1\n * Line 2\n ",
			want:  "Line 1\nLine 2",
		},
		{
			input: "Simple",
			want:  "Simple",
		},
		{
			input: "\n\nWith blank lines\n\n",
			want:  "With blank lines",
		},
	}

	for _, tt := range tests {
		t.Run(tt.input[:min(10, len(tt.input))], func(t *testing.T) {
			got := cleanBlockComment(tt.input)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}
