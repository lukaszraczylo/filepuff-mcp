package parser

import (
	"context"
	"testing"

	"github.com/lukaszraczylo/mcp-filepuff/pkg/protocol"
)

func TestExtractGoSymbols(t *testing.T) {
	r := NewRegistry()
	defer r.Close()

	content := `package main

// Hello prints a greeting
func Hello() {
	println("hello")
}

// Server handles requests
type Server struct {
	Port int
}

// Start starts the server
func (s *Server) Start() error {
	return nil
}

const MaxConnections = 100
var globalVar = "test"
`

	ctx := context.Background()
	result, err := r.Parse(ctx, "test.go", []byte(content))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	symbols := ExtractSymbols(result.Tree, []byte(content), protocol.LangGo, "test.go")

	expectedSymbols := map[string]protocol.SymbolKind{
		"Hello":          protocol.SymbolFunction,
		"Server":         protocol.SymbolStruct,
		"(Server).Start": protocol.SymbolMethod,
		"MaxConnections": protocol.SymbolConstant,
		"globalVar":      protocol.SymbolVariable,
	}

	found := make(map[string]bool)
	for _, sym := range symbols {
		if expectedKind, ok := expectedSymbols[sym.Name]; ok {
			found[sym.Name] = true
			if sym.Kind != expectedKind {
				t.Errorf("symbol %s: expected kind %s, got %s", sym.Name, expectedKind, sym.Kind)
			}
		}
	}

	for name := range expectedSymbols {
		if !found[name] {
			t.Errorf("expected to find symbol %s", name)
		}
	}
}

func TestExtractJSSymbols(t *testing.T) {
	r := NewRegistry()
	defer r.Close()

	content := `
function greet(name) {
  console.log("Hello, " + name);
}

class User {
  constructor(name) {
    this.name = name;
  }

  getName() {
    return this.name;
  }
}

const MAX_USERS = 100;
let currentUser = null;
`

	ctx := context.Background()
	result, err := r.Parse(ctx, "test.js", []byte(content))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	symbols := ExtractSymbols(result.Tree, []byte(content), protocol.LangJavaScript, "test.js")

	expectedSymbols := map[string]protocol.SymbolKind{
		"greet":       protocol.SymbolFunction,
		"User":        protocol.SymbolClass,
		"MAX_USERS":   protocol.SymbolVariable,
		"currentUser": protocol.SymbolVariable,
	}

	found := make(map[string]bool)
	for _, sym := range symbols {
		if expectedKind, ok := expectedSymbols[sym.Name]; ok {
			found[sym.Name] = true
			if sym.Kind != expectedKind {
				t.Errorf("symbol %s: expected kind %s, got %s", sym.Name, expectedKind, sym.Kind)
			}
		}
	}

	for name := range expectedSymbols {
		if !found[name] {
			t.Errorf("expected to find symbol %s", name)
		}
	}
}

func TestExtractPythonSymbols(t *testing.T) {
	r := NewRegistry()
	defer r.Close()

	content := `
def greet(name):
    """Greet a person by name."""
    print(f"Hello, {name}")

class User:
    """Represents a user."""

    def __init__(self, name):
        self.name = name

    def get_name(self):
        return self.name
`

	ctx := context.Background()
	result, err := r.Parse(ctx, "test.py", []byte(content))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	symbols := ExtractSymbols(result.Tree, []byte(content), protocol.LangPython, "test.py")

	expectedSymbols := map[string]protocol.SymbolKind{
		"greet":    protocol.SymbolFunction,
		"User":     protocol.SymbolClass,
		"__init__": protocol.SymbolMethod,
		"get_name": protocol.SymbolMethod,
	}

	found := make(map[string]bool)
	for _, sym := range symbols {
		if expectedKind, ok := expectedSymbols[sym.Name]; ok {
			found[sym.Name] = true
			if sym.Kind != expectedKind {
				t.Errorf("symbol %s: expected kind %s, got %s", sym.Name, expectedKind, sym.Kind)
			}
		}
	}

	for name := range expectedSymbols {
		if !found[name] {
			t.Errorf("expected to find symbol %s", name)
		}
	}
}

func TestExtractCSymbols(t *testing.T) {
	r := NewRegistry()
	defer r.Close()

	content := `
#include <stdio.h>

struct Point {
    int x;
    int y;
};

void print_point(struct Point p) {
    printf("(%d, %d)\n", p.x, p.y);
}

int main() {
    struct Point p = {1, 2};
    print_point(p);
    return 0;
}
`

	ctx := context.Background()
	result, err := r.Parse(ctx, "test.c", []byte(content))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	symbols := ExtractSymbols(result.Tree, []byte(content), protocol.LangC, "test.c")

	// Note: C symbol extraction is complex, checking for at least main and Point
	expectedSymbols := map[string]protocol.SymbolKind{
		"Point": protocol.SymbolStruct,
		"main":  protocol.SymbolFunction,
	}

	found := make(map[string]bool)
	for _, sym := range symbols {
		if expectedKind, ok := expectedSymbols[sym.Name]; ok {
			found[sym.Name] = true
			if sym.Kind != expectedKind {
				t.Errorf("symbol %s: expected kind %s, got %s", sym.Name, expectedKind, sym.Kind)
			}
		}
	}

	for name := range expectedSymbols {
		if !found[name] {
			t.Errorf("expected to find symbol %s", name)
		}
	}
}
