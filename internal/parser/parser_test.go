package parser

import (
	"context"
	"testing"

	"github.com/lukaszraczylo/mcp-filepuff/pkg/protocol"
)

func TestNewRegistry(t *testing.T) {
	r := NewRegistry()
	if r == nil {
		t.Fatal("expected non-nil registry")
	}
	defer r.Close()
}

func TestGetParser(t *testing.T) {
	r := NewRegistry()
	defer r.Close()

	tests := []struct {
		lang    protocol.Language
		wantErr bool
	}{
		{protocol.LangGo, false},
		{protocol.LangTypeScript, false},
		{protocol.LangJavaScript, false},
		{protocol.LangPython, false},
		{protocol.LangC, false},
		{protocol.LangCpp, false},
		{protocol.LangHTML, false},
		{protocol.LangVue, false},
		{protocol.LangUnknown, true},
	}

	for _, tt := range tests {
		t.Run(string(tt.lang), func(t *testing.T) {
			parser, err := r.GetParser(tt.lang)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if parser == nil {
					t.Error("expected non-nil parser")
				}
			}
		})
	}
}

func TestParse(t *testing.T) {
	r := NewRegistry()
	defer r.Close()

	tests := []struct {
		name     string
		filename string
		content  string
		wantLang protocol.Language
		wantErr  bool
	}{
		{
			name:     "go file",
			filename: "test.go",
			content:  "package main\n\nfunc main() {}\n",
			wantLang: protocol.LangGo,
			wantErr:  false,
		},
		{
			name:     "typescript file",
			filename: "test.ts",
			content:  "function hello(): void {}\n",
			wantLang: protocol.LangTypeScript,
			wantErr:  false,
		},
		{
			name:     "react tsx file",
			filename: "Component.tsx",
			content:  `import React from 'react';\n\nexport const Button: React.FC = () => <button className="btn">Click</button>;`,
			wantLang: protocol.LangTypeScript,
			wantErr:  false,
		},
		{
			name:     "react jsx file",
			filename: "Component.jsx",
			content:  `import React from 'react';\n\nexport const Button = () => <button className="btn">Click</button>;`,
			wantLang: protocol.LangJavaScript,
			wantErr:  false,
		},
		{
			name:     "python file",
			filename: "test.py",
			content:  "def hello():\n    pass\n",
			wantLang: protocol.LangPython,
			wantErr:  false,
		},
		{
			name:     "html file",
			filename: "test.html",
			content:  `<!DOCTYPE html><html><head><title>Test</title></head><body><h1 class="text-xl">Hello</h1></body></html>`,
			wantLang: protocol.LangHTML,
			wantErr:  false,
		},
		{
			name:     "vue file",
			filename: "Component.vue",
			content:  `<template><div class="container"><h1>{{ title }}</h1></div></template>`,
			wantLang: protocol.LangVue,
			wantErr:  false,
		},
		{
			name:     "unknown file",
			filename: "test.txt",
			content:  "hello world",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			result, err := r.Parse(ctx, tt.filename, []byte(tt.content))

			if tt.wantErr {
				if err == nil {
					t.Error("expected error")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if result.Language != tt.wantLang {
				t.Errorf("expected language %s, got %s", tt.wantLang, result.Language)
			}

			if result.Tree == nil {
				t.Error("expected non-nil tree")
			}
		})
	}
}

func TestParseWithSyntaxErrors(t *testing.T) {
	r := NewRegistry()
	defer r.Close()

	// Invalid Go code
	content := "package main\n\nfunc main( {}\n" // Missing closing paren
	ctx := context.Background()

	result, err := r.Parse(ctx, "test.go", []byte(content))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have parsed (tree-sitter is error-tolerant)
	if result.Tree == nil {
		t.Error("expected non-nil tree")
	}

	// Should have detected errors
	if len(result.Errors) == 0 {
		t.Error("expected syntax errors to be detected")
	}
}

func TestIsBinary(t *testing.T) {
	tests := []struct {
		name    string
		content []byte
		want    bool
	}{
		{
			name:    "text file",
			content: []byte("hello world"),
			want:    false,
		},
		{
			name:    "binary with null byte",
			content: []byte{0x68, 0x65, 0x6c, 0x00, 0x6f},
			want:    true,
		},
		{
			name:    "empty file",
			content: []byte{},
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isBinary(tt.content); got != tt.want {
				t.Errorf("isBinary() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCaching(t *testing.T) {
	r := NewRegistry()
	defer r.Close()

	content := []byte("package main\n\nfunc main() {}\n")
	ctx := context.Background()

	// Parse once
	result1, err := r.Parse(ctx, "test.go", content)
	if err != nil {
		t.Fatalf("first parse failed: %v", err)
	}

	// Parse again with same content
	result2, err := r.Parse(ctx, "test.go", content)
	if err != nil {
		t.Fatalf("second parse failed: %v", err)
	}

	// Should return cached tree (same pointer)
	if result1.Tree != result2.Tree {
		t.Error("expected cached tree to be returned")
	}
}
