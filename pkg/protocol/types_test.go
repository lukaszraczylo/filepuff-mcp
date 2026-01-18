package protocol

import "testing"

func TestDetectLanguage(t *testing.T) {
	tests := []struct {
		filename string
		expected Language
	}{
		{"main.go", LangGo},
		{"server.go", LangGo},
		{"index.ts", LangTypeScript},
		{"component.tsx", LangTypeScript},
		{"Button.tsx", LangTypeScript},
		{"app.js", LangJavaScript},
		{"component.jsx", LangJavaScript},
		{"Component.jsx", LangJavaScript},
		{"module.mjs", LangJavaScript},
		{"common.cjs", LangJavaScript},
		{"script.py", LangPython},
		{"app.pyw", LangPython},
		{"main.c", LangC},
		{"header.h", LangC},
		{"main.cpp", LangCpp},
		{"main.cc", LangCpp},
		{"main.cxx", LangCpp},
		{"header.hpp", LangCpp},
		{"header.hxx", LangCpp},
		{"index.html", LangHTML},
		{"page.htm", LangHTML},
		{"Component.vue", LangVue},
		{"unknown.txt", LangUnknown},
		{"README", LangUnknown},
		{"path/to/file.go", LangGo},
		{"path/to/file.ts", LangTypeScript},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			result := DetectLanguage(tt.filename)
			if result != tt.expected {
				t.Errorf("DetectLanguage(%q) = %q, want %q", tt.filename, result, tt.expected)
			}
		})
	}
}

func TestGetExtension(t *testing.T) {
	tests := []struct {
		filename string
		expected string
	}{
		{"file.go", ".go"},
		{"file.test.go", ".go"},
		{"path/to/file.ts", ".ts"},
		{"noextension", ""},
		{".hidden", ".hidden"},
		{"file.", "."},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			result := getExtension(tt.filename)
			if result != tt.expected {
				t.Errorf("getExtension(%q) = %q, want %q", tt.filename, result, tt.expected)
			}
		})
	}
}
