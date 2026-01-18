package lsp

import (
	"testing"

	"github.com/lukaszraczylo/mcp-filepuff/pkg/protocol"
)

func TestFileToURI(t *testing.T) {
	tests := []struct {
		name string
		file string
		want string
	}{
		{
			name: "absolute path",
			file: "/Users/test/file.go",
			want: "file:///Users/test/file.go",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fileToURI(tt.file)
			if got != tt.want {
				t.Errorf("fileToURI() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestURIToFile(t *testing.T) {
	tests := []struct {
		name string
		uri  string
		want string
	}{
		{
			name: "file uri",
			uri:  "file:///Users/test/file.go",
			want: "/Users/test/file.go",
		},
		{
			name: "not a file uri",
			uri:  "/Users/test/file.go",
			want: "/Users/test/file.go",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := URIToFile(tt.uri)
			if got != tt.want {
				t.Errorf("URIToFile() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLanguageToLSPID(t *testing.T) {
	tests := []struct {
		lang protocol.Language
		want string
	}{
		{protocol.LangGo, "go"},
		{protocol.LangTypeScript, "typescript"},
		{protocol.LangJavaScript, "javascript"},
		{protocol.LangPython, "python"},
		{protocol.LangC, "c"},
		{protocol.LangCpp, "cpp"},
		{protocol.LangUnknown, "unknown"},
	}

	for _, tt := range tests {
		t.Run(string(tt.lang), func(t *testing.T) {
			got := languageToLSPID(tt.lang)
			if got != tt.want {
				t.Errorf("languageToLSPID() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsAvailable(t *testing.T) {
	// This tests the structure of the manager without actually spawning servers
	// which requires the actual LSP servers to be installed

	// Just verify the DefaultServerConfigs structure
	expectedLanguages := []protocol.Language{
		protocol.LangGo,
		protocol.LangTypeScript,
		protocol.LangJavaScript,
		protocol.LangPython,
		protocol.LangC,
		protocol.LangCpp,
	}

	for _, lang := range expectedLanguages {
		if _, ok := DefaultServerConfigs[lang]; !ok {
			t.Errorf("missing server config for language: %s", lang)
		}
	}
}

func TestDefaultServerConfigs(t *testing.T) {
	// Verify the command structure
	for lang, config := range DefaultServerConfigs {
		if len(config.Command) == 0 {
			t.Errorf("language %s has empty command", lang)
		}
	}
}
