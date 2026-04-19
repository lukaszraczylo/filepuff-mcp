package server

import (
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestRegisterResources_AllToolsHaveResource(t *testing.T) {
	// Verify that registerResources wires up without panicking.
	_ = newTestServer(t, t.TempDir())

	expectedURIs := []string{
		"filepuff://help/file_read",
		"filepuff://help/file_search",
		"filepuff://help/ast_query",
		"filepuff://help/lsp_query",
		"filepuff://help/edit_apply",
	}

	for _, uri := range expectedURIs {
		t.Run(uri, func(t *testing.T) {
			handler := readHelpResource(uri)
			contents, err := handler(context.Background(), mcp.ReadResourceRequest{})
			if err != nil {
				t.Fatalf("readHelpResource(%q) error = %v", uri, err)
			}
			if len(contents) == 0 {
				t.Fatalf("readHelpResource(%q) returned empty contents", uri)
			}
			tc, ok := contents[0].(mcp.TextResourceContents)
			if !ok {
				t.Fatalf("readHelpResource(%q) contents[0] is not TextResourceContents", uri)
			}
			if tc.MIMEType != "text/markdown" {
				t.Errorf("MIMEType = %q, want %q", tc.MIMEType, "text/markdown")
			}
			if len(tc.Text) == 0 {
				t.Errorf("Text is empty for %q", uri)
			}
			if !strings.HasPrefix(tc.Text, "#") {
				t.Errorf("expected markdown (# heading) for %q, got: %q", uri, tc.Text[:min(50, len(tc.Text))])
			}
			if tc.URI != uri {
				t.Errorf("URI = %q, want %q", tc.URI, uri)
			}
		})
	}
}

func TestReadHelpResource_UnknownTool(t *testing.T) {
	handler := readHelpResource("filepuff://help/nonexistent")
	_, err := handler(context.Background(), mcp.ReadResourceRequest{})
	if err == nil {
		t.Fatal("expected error for unknown tool, got nil")
	}
}

func TestReadHelpResource_InvalidURI(t *testing.T) {
	handler := readHelpResource("filepuff://help/")
	_, err := handler(context.Background(), mcp.ReadResourceRequest{})
	if err == nil {
		t.Fatal("expected error for empty tool name, got nil")
	}
}

func TestHelpContent_NotEmpty(t *testing.T) {
	cases := map[string]string{
		"file_read":   helpFileRead,
		"file_search": helpFileSearch,
		"ast_query":   helpASTQuery,
		"lsp_query":   helpLSPQuery,
		"edit_apply":  helpEditApply,
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			if len(content) == 0 {
				t.Errorf("help content for %q is empty", name)
			}
			if !strings.Contains(content, "##") {
				t.Errorf("expected markdown sections (##) in help content for %q", name)
			}
			if !strings.Contains(content, "```") {
				t.Errorf("expected code fences in help content for %q", name)
			}
		})
	}
}
