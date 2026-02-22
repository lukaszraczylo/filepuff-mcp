package query

import (
	"context"
	"testing"

	"github.com/lukaszraczylo/mcp-filepuff/internal/parser"
	"github.com/lukaszraczylo/mcp-filepuff/pkg/protocol"
)

func TestParsePattern(t *testing.T) {
	tests := []struct {
		name         string
		pattern      string
		captureNames []string
		captureTypes []CaptureType
		wantCaptures int
		wantErr      bool
	}{
		{
			name:         "empty pattern",
			pattern:      "",
			wantErr:      true,
			wantCaptures: 0,
		},
		{
			name:         "single capture",
			pattern:      "func $NAME() {}",
			wantErr:      false,
			wantCaptures: 1,
			captureNames: []string{"NAME"},
			captureTypes: []CaptureType{CaptureSingle},
		},
		{
			name:         "multiple single captures",
			pattern:      "func $NAME($ARGS) $RETURN",
			wantErr:      false,
			wantCaptures: 3,
			captureNames: []string{"NAME", "ARGS", "RETURN"},
			captureTypes: []CaptureType{CaptureSingle, CaptureSingle, CaptureSingle},
		},
		{
			name:         "multi-node capture",
			pattern:      "func $NAME($$$ARGS) { $$$BODY }",
			wantErr:      false,
			wantCaptures: 3,
			captureNames: []string{"ARGS", "BODY", "NAME"},
			captureTypes: []CaptureType{CaptureMultiple, CaptureMultiple, CaptureSingle},
		},
		{
			name:         "wildcard capture",
			pattern:      "func $NAME($_) {}",
			wantErr:      false,
			wantCaptures: 2,
			captureNames: []string{"NAME", "_"},
			captureTypes: []CaptureType{CaptureSingle, CaptureWildcard},
		},
		{
			name:         "no captures",
			pattern:      "func main() {}",
			wantErr:      false,
			wantCaptures: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed, err := ParsePattern(tt.pattern)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(parsed.Captures) != tt.wantCaptures {
				t.Errorf("expected %d captures, got %d", tt.wantCaptures, len(parsed.Captures))
			}

			// Check capture names (order may vary)
			if tt.captureNames != nil {
				captureMap := make(map[string]CaptureType)
				for _, cap := range parsed.Captures {
					captureMap[cap.Name] = cap.Type
				}

				for i, name := range tt.captureNames {
					if _, ok := captureMap[name]; !ok {
						t.Errorf("expected capture %s not found", name)
					}
					if captureMap[name] != tt.captureTypes[i] {
						t.Errorf("capture %s: expected type %v, got %v", name, tt.captureTypes[i], captureMap[name])
					}
				}
			}
		})
	}
}

func TestMatchGoFunctions(t *testing.T) {
	reg := parser.NewRegistry()
	defer reg.Close()

	matcher := NewMatcher(reg)

	content := `package main

func Hello() {
	println("hello")
}

func Greet(name string) error {
	println("hello", name)
	return nil
}

type Server struct {
	Port int
}

func (s *Server) Start() error {
	return nil
}
`

	ctx := context.Background()
	result, err := reg.Parse(ctx, "test.go", []byte(content))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	tests := []struct {
		query       *ASTQuery
		name        string
		wantMatches int
	}{
		{
			name: "match all functions",
			query: &ASTQuery{
				Pattern:  "func $NAME($$$ARGS) { $$$BODY }",
				Language: "go",
			},
			wantMatches: 3, // Hello, Greet, Start
		},
		{
			name: "match functions starting with H",
			query: &ASTQuery{
				Pattern:  "func $NAME() {}",
				Language: "go",
				Filters: QueryFilters{
					NameMatches: "^H",
				},
			},
			wantMatches: 1, // Hello
		},
		{
			name: "match specific function",
			query: &ASTQuery{
				Pattern:  "func $NAME() {}",
				Language: "go",
				Filters: QueryFilters{
					NameExact: "Hello",
				},
			},
			wantMatches: 1, // Hello
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, err := matcher.Match(ctx, tt.query, result.Tree, []byte(content), "test.go")
			if err != nil {
				t.Fatalf("match failed: %v", err)
			}

			if len(results) != tt.wantMatches {
				t.Errorf("expected %d matches, got %d", tt.wantMatches, len(results))
				for i, r := range results {
					t.Logf("match %d: %s at line %d", i, r.Node.Type(), r.Location.Line)
				}
			}
		})
	}
}

func TestMatchGoStructs(t *testing.T) {
	reg := parser.NewRegistry()
	defer reg.Close()

	matcher := NewMatcher(reg)

	content := `package main

type Server struct {
	Port int
	Host string
}

type Config struct {
	Timeout int
}

type Logger interface {
	Log(msg string)
}
`

	ctx := context.Background()
	result, err := reg.Parse(ctx, "test.go", []byte(content))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	tests := []struct {
		query       *ASTQuery
		name        string
		wantMinimum int
	}{
		{
			name: "match all structs",
			query: &ASTQuery{
				Pattern:  "type $NAME struct { $$$FIELDS }",
				Language: "go",
			},
			wantMinimum: 2, // Server, Config (may also match interface as type_declaration)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, err := matcher.Match(ctx, tt.query, result.Tree, []byte(content), "test.go")
			if err != nil {
				t.Fatalf("match failed: %v", err)
			}

			if len(results) < tt.wantMinimum {
				t.Errorf("expected at least %d matches, got %d", tt.wantMinimum, len(results))
			}
		})
	}
}

func TestMatchJSFunctions(t *testing.T) {
	reg := parser.NewRegistry()
	defer reg.Close()

	matcher := NewMatcher(reg)

	content := `
function greet(name) {
  console.log("Hello, " + name);
}

function sayHello() {
  console.log("Hello!");
}

class User {
  constructor(name) {
    this.name = name;
  }

  getName() {
    return this.name;
  }
}
`

	ctx := context.Background()
	result, err := reg.Parse(ctx, "test.js", []byte(content))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	tests := []struct {
		query       *ASTQuery
		name        string
		wantMatches int
	}{
		{
			name: "match all functions",
			query: &ASTQuery{
				Pattern:  "function $NAME($$$ARGS) { $$$BODY }",
				Language: "javascript",
			},
			wantMatches: 2, // greet, sayHello
		},
		{
			name: "match classes",
			query: &ASTQuery{
				Pattern:  "class $NAME { $$$BODY }",
				Language: "javascript",
			},
			wantMatches: 1, // User
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, err := matcher.Match(ctx, tt.query, result.Tree, []byte(content), "test.js")
			if err != nil {
				t.Fatalf("match failed: %v", err)
			}

			if len(results) != tt.wantMatches {
				t.Errorf("expected %d matches, got %d", tt.wantMatches, len(results))
			}
		})
	}
}

func TestMatchPythonSymbols(t *testing.T) {
	reg := parser.NewRegistry()
	defer reg.Close()

	matcher := NewMatcher(reg)

	content := `
def greet(name):
    print(f"Hello, {name}")

def calculate(a, b):
    return a + b

class User:
    def __init__(self, name):
        self.name = name

    def get_name(self):
        return self.name
`

	ctx := context.Background()
	result, err := reg.Parse(ctx, "test.py", []byte(content))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	tests := []struct {
		query       *ASTQuery
		name        string
		wantMinimum int
	}{
		{
			name: "match classes",
			query: &ASTQuery{
				Pattern:  "class $NAME: $$$BODY",
				Language: "python",
			},
			wantMinimum: 1, // User
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, err := matcher.Match(ctx, tt.query, result.Tree, []byte(content), "test.py")
			if err != nil {
				t.Fatalf("match failed: %v", err)
			}

			if len(results) < tt.wantMinimum {
				t.Errorf("expected at least %d matches, got %d", tt.wantMinimum, len(results))
			}
		})
	}
}

func TestQueryFilters(t *testing.T) {
	reg := parser.NewRegistry()
	defer reg.Close()

	matcher := NewMatcher(reg)

	content := `package main

func HelloWorld() {}
func helloWorld() {}
func GoodbyeWorld() {}
func Main() {}
`

	ctx := context.Background()
	result, err := reg.Parse(ctx, "test.go", []byte(content))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	tests := []struct {
		name        string
		filters     QueryFilters
		wantMatches int
	}{
		{
			name: "regex filter - starts with H",
			filters: QueryFilters{
				NameMatches: "^[Hh]ello",
			},
			wantMatches: 2, // HelloWorld, helloWorld
		},
		{
			name: "exact name filter",
			filters: QueryFilters{
				NameExact: "Main",
			},
			wantMatches: 1, // Main
		},
		{
			name: "kind filter",
			filters: QueryFilters{
				KindIn: []string{"function_declaration"},
			},
			wantMatches: 4, // all functions
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query := &ASTQuery{
				Pattern:  "func $NAME() {}",
				Language: "go",
				Filters:  tt.filters,
			}

			results, err := matcher.Match(ctx, query, result.Tree, []byte(content), "test.go")
			if err != nil {
				t.Fatalf("match failed: %v", err)
			}

			if len(results) != tt.wantMatches {
				t.Errorf("expected %d matches, got %d", tt.wantMatches, len(results))
				for _, r := range results {
					if nameNode := r.Node.ChildByFieldName("name"); nameNode != nil {
						t.Logf("matched: %s", parser.GetNodeText(nameNode, []byte(content)))
					}
				}
			}
		})
	}
}

func TestFormatResults(t *testing.T) {
	tests := []struct {
		name       string
		results    []MatchResult
		maxResults int
		wantEmpty  bool
	}{
		{
			name:       "empty results",
			results:    []MatchResult{},
			maxResults: 100,
			wantEmpty:  true,
		},
		{
			name: "single result",
			results: []MatchResult{
				{
					File:     "test.go",
					Location: protocol.Location{Line: 10, Column: 1},
					Text:     "func Hello() {}",
					Captures: map[string]CapturedNode{
						"NAME": {Text: "Hello"},
					},
				},
			},
			maxResults: 100,
			wantEmpty:  false,
		},
		{
			name: "truncated results",
			results: []MatchResult{
				{File: "a.go", Location: protocol.Location{Line: 1}, Text: "func A() {}"},
				{File: "b.go", Location: protocol.Location{Line: 1}, Text: "func B() {}"},
				{File: "c.go", Location: protocol.Location{Line: 1}, Text: "func C() {}"},
			},
			maxResults: 2,
			wantEmpty:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := FormatResults(tt.results, tt.maxResults)

			if tt.wantEmpty {
				if output != "No matches found." {
					t.Errorf("expected 'No matches found.', got: %s", output)
				}
			} else {
				if output == "No matches found." {
					t.Error("expected results, got 'No matches found.'")
				}
			}
		})
	}
}

// TestMatchStructOperatorPrecedence verifies the C-07 operator precedence fix.
// Before the fix, patterns like "struct Foo" would match because
// strings.Contains(p, "struct ") short-circuited the entire condition.
// After the fix, both "struct" must be present for the struct branch to match.
func TestMatchStructOperatorPrecedence(t *testing.T) {
	reg := parser.NewRegistry()
	defer reg.Close()

	matcher := NewMatcher(reg)

	content := `package main

type Server struct {
	Port int
}

func main() {}
`

	ctx := context.Background()
	result, err := reg.Parse(ctx, "test.go", []byte(content))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	tests := []struct {
		name        string
		pattern     string
		wantMatches int
	}{
		{
			name:        "type struct pattern should match",
			pattern:     "type $NAME struct { $$$FIELDS }",
			wantMatches: 1, // Server
		},
		{
			name:        "struct keyword alone should match",
			pattern:     "struct $NAME { $$$FIELDS }",
			wantMatches: 1, // Server
		},
		{
			name:        "func pattern should not match struct branch",
			pattern:     "func $NAME() {}",
			wantMatches: 1, // main (matches function branch, not struct)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query := &ASTQuery{
				Pattern:  tt.pattern,
				Language: "go",
			}
			results, err := matcher.Match(ctx, query, result.Tree, []byte(content), "test.go")
			if err != nil {
				t.Fatalf("match failed: %v", err)
			}
			if len(results) != tt.wantMatches {
				t.Errorf("expected %d matches, got %d", tt.wantMatches, len(results))
				for i, r := range results {
					t.Logf("match %d: type=%s, text=%q", i, r.Node.Type(), truncateForLog(r.Text, 80))
				}
			}
		})
	}
}

func truncateForLog(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

// TestPassesFilters_AllBranches tests passesFilters for each filter type.
func TestPassesFilters_AllBranches(t *testing.T) {
	reg := parser.NewRegistry()
	defer reg.Close()

	content := `package main

func Alpha() {}
func Beta() {}
func Gamma() {}
`

	ctx := context.Background()
	result, err := reg.Parse(ctx, "test.go", []byte(content))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	matcher := NewMatcher(reg)

	tests := []struct {
		name        string
		filters     QueryFilters
		wantMatches int
	}{
		{
			name:        "no filters matches all",
			filters:     QueryFilters{},
			wantMatches: 3,
		},
		{
			name:        "name_exact filter",
			filters:     QueryFilters{NameExact: "Alpha"},
			wantMatches: 1,
		},
		{
			name:        "name_matches regex filter",
			filters:     QueryFilters{NameMatches: "^[AB]"},
			wantMatches: 2,
		},
		{
			name:        "kind_in filter",
			filters:     QueryFilters{KindIn: []string{"function_declaration"}},
			wantMatches: 3,
		},
		{
			name:        "kind_in filter excludes non-matching kinds",
			filters:     QueryFilters{KindIn: []string{"class_declaration"}},
			wantMatches: 0,
		},
		{
			name:        "combined name_exact and kind_in",
			filters:     QueryFilters{NameExact: "Beta", KindIn: []string{"function_declaration"}},
			wantMatches: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query := &ASTQuery{
				Pattern:  "func $NAME() {}",
				Language: "go",
				Filters:  tt.filters,
			}
			results, err := matcher.Match(ctx, query, result.Tree, []byte(content), "test.go")
			if err != nil {
				t.Fatalf("match failed: %v", err)
			}
			if len(results) != tt.wantMatches {
				t.Errorf("expected %d matches, got %d", tt.wantMatches, len(results))
			}
		})
	}
}

func TestQueryValidation(t *testing.T) {
	reg := parser.NewRegistry()
	defer reg.Close()

	matcher := NewMatcher(reg)
	ctx := context.Background()

	// Parse some valid content
	content := `package main
func main() {}
`
	result, err := reg.Parse(ctx, "test.go", []byte(content))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	tests := []struct {
		query   *ASTQuery
		name    string
		wantErr bool
	}{
		{
			name:    "empty pattern",
			query:   &ASTQuery{Pattern: "", Language: "go"},
			wantErr: true,
		},
		{
			name:    "missing language",
			query:   &ASTQuery{Pattern: "func $NAME() {}", Language: ""},
			wantErr: true,
		},
		{
			name:    "unknown language",
			query:   &ASTQuery{Pattern: "func $NAME() {}", Language: "unknown"},
			wantErr: true,
		},
		{
			name:    "valid query",
			query:   &ASTQuery{Pattern: "func $NAME() {}", Language: "go"},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := matcher.Match(ctx, tt.query, result.Tree, []byte(content), "test.go")
			if tt.wantErr {
				if err == nil {
					t.Error("expected error")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}
