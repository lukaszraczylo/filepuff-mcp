package server

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"

	xxhash "github.com/cespare/xxhash/v2"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

// helpResources maps a tool name to its help content constant.
var helpResources = map[string]string{
	"file_read":   helpFileRead,
	"file_search": helpFileSearch,
	"ast_query":   helpASTQuery,
	"lsp_query":   helpLSPQuery,
	"edit_apply":  helpEditApply,
}

// registerResources registers one filepuff://help/<tool> resource per tool.
// Each resource returns Markdown-formatted flag docs and examples.
func (s *Server) registerResources() {
	for toolName, content := range helpResources {
		uri := "filepuff://help/" + toolName
		name := "help/" + toolName
		description := "Flag documentation and examples for the " + toolName + " tool."
		captured := content // capture for closure

		s.mcp.AddResource(
			mcp.NewResource(uri, name,
				mcp.WithResourceDescription(description),
				mcp.WithMIMEType("text/markdown"),
			),
			func(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
				return []mcp.ResourceContents{
					mcp.TextResourceContents{
						URI:      uri,
						MIMEType: "text/markdown",
						Text:     captured,
					},
				}, nil
			},
		)
	}
}

// readHelpResource is a convenience handler that can be used directly when a
// single resource handler is needed. It is kept exported for testability.
func readHelpResource(uri string) mcpserver.ResourceHandlerFunc {
	return func(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		// Extract tool name from filepuff://help/<tool>
		const prefix = "filepuff://help/"
		if len(uri) <= len(prefix) {
			return nil, fmt.Errorf("invalid help URI: %s", uri)
		}
		toolName := uri[len(prefix):]
		content, ok := helpResources[toolName]
		if !ok {
			return nil, fmt.Errorf("no help content for tool: %s", toolName)
		}
		return []mcp.ResourceContents{
			mcp.TextResourceContents{
				URI:      uri,
				MIMEType: "text/markdown",
				Text:     content,
			},
		}, nil
	}
}

// registerReadResource registers the filepuff://read/{+path} resource template.
// The handler re-reads the file, validates the etag query param if provided,
// and returns the raw file content (no line-number formatting).
//
// URI format: filepuff://read/<absolute-path>?etag=<etag>
// The etag param is optional. If supplied and the file has changed, the handler
// returns an error so the caller re-runs file_read to get a fresh ResourceLink.
func (s *Server) registerReadResource() {
	const uriTemplate = "filepuff://read/{+path}"

	s.mcp.AddResourceTemplate(
		mcp.NewResourceTemplate(uriTemplate, "file-read",
			mcp.WithTemplateDescription("Raw content of a file previously read via file_read. "+
				"Fetch when file_read returns a ResourceLink instead of inlining content. "+
				"URI: filepuff://read/<path>?etag=<etag>"),
		),
		func(_ context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
			return s.handleReadResource(req)
		},
	)
}

// handleReadResource is the resource handler for filepuff://read/{+path} URIs.
func (s *Server) handleReadResource(req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	rawURI := req.Params.URI

	// Parse path and etag from the URI.
	// URI shape: filepuff://read/<path>[?etag=<hex>]
	const scheme = "filepuff://read/"
	if !strings.HasPrefix(rawURI, scheme) {
		return nil, fmt.Errorf("invalid read resource URI: %s", rawURI)
	}
	rest := rawURI[len(scheme):]

	// Split off query string to get the path.
	filePath := rest
	var expectedEtag string
	if qIdx := strings.IndexByte(rest, '?'); qIdx >= 0 {
		filePath = rest[:qIdx]
		qs, err := url.ParseQuery(rest[qIdx+1:])
		if err == nil {
			expectedEtag = qs.Get("etag")
		}
	}

	if filePath == "" {
		return nil, fmt.Errorf("read resource URI missing path")
	}

	if !s.cfg.IsPathAllowed(filePath) {
		return nil, fmt.Errorf("path is outside workspace root")
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("file not found: %s", filePath)
		}
		if os.IsPermission(err) {
			return nil, fmt.Errorf("permission denied: %s", filePath)
		}
		return nil, fmt.Errorf("error reading file: %s", filePath)
	}

	// Validate etag if provided — detect stale references.
	if expectedEtag != "" {
		fullHash := fmt.Sprintf("%016x", xxhash.Sum64(content))
		currentEtag := fullHash[:8]
		if expectedEtag != currentEtag && !strings.HasPrefix(fullHash, expectedEtag) && !strings.HasPrefix(expectedEtag, currentEtag) {
			return nil, fmt.Errorf("file changed since ResourceLink was issued (expected etag %s, got %s); re-run file_read to get fresh content", expectedEtag, currentEtag)
		}
	}

	mimeType := detectMIMEType(filePath)
	return []mcp.ResourceContents{
		mcp.TextResourceContents{
			URI:      rawURI,
			MIMEType: mimeType,
			Text:     string(content),
		},
	}, nil
}
