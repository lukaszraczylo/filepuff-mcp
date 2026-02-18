// Package server implements the MCP server for file operations.
package server

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/lukaszraczylo/mcp-filepuff/internal/lsp"
	"github.com/lukaszraczylo/mcp-filepuff/internal/parser"
	"github.com/lukaszraczylo/mcp-filepuff/pkg/errors"
	"github.com/mark3labs/mcp-go/mcp"
)

// handleSymbolAt handles the symbol_at tool.
func (s *Server) handleSymbolAt(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	file, err := request.RequireString("file")
	if err != nil {
		return mcp.NewToolResultError("file is required"), nil
	}

	line := request.GetInt("line", 0)
	if line <= 0 {
		return mcp.NewToolResultError("line must be positive"), nil
	}

	col := request.GetInt("column", 0)
	if col <= 0 {
		return mcp.NewToolResultError("column must be positive"), nil
	}

	// Validate path
	if !s.cfg.IsPathAllowed(file) {
		return mcp.NewToolResultError("file is outside workspace root"), nil
	}

	// Try LSP hover
	hover, err := s.lspManager.Hover(ctx, file, line, col)
	if err != nil {
		// Fall back to AST-based info
		return s.handleSymbolAtFallback(ctx, file, line, col)
	}

	if hover == nil {
		return mcp.NewToolResultText("No symbol information available at this position."), nil
	}

	var output strings.Builder
	output.WriteString("**Symbol Information**\n\n")
	output.WriteString(hover.Contents.Value)

	return mcp.NewToolResultText(output.String()), nil
}

// handleSymbolAtFallback provides AST-based symbol info when LSP is unavailable.
func (s *Server) handleSymbolAtFallback(ctx context.Context, file string, line, col int) (*mcp.CallToolResult, error) {
	content, err := os.ReadFile(file)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to read file: %s", errors.SanitizeError(err))), nil
	}

	result, err := s.parser.Parse(ctx, file, content)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to parse file: %s", errors.SanitizeError(err))), nil
	}

	node := parser.FindNodeAtPosition(result.Tree, line, col)
	if node == nil {
		return mcp.NewToolResultText("No symbol at this position."), nil
	}

	var output strings.Builder
	output.WriteString("**Symbol Information** (AST fallback)\n\n")
	output.WriteString(fmt.Sprintf("Node type: `%s`\n", node.Type()))
	output.WriteString(fmt.Sprintf("Text: `%s`\n", parser.GetNodeText(node, content)))

	return mcp.NewToolResultText(output.String()), nil
}

// handleFindDefinition handles the find_definition tool.
func (s *Server) handleFindDefinition(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	file, err := request.RequireString("file")
	if err != nil {
		return mcp.NewToolResultError("file is required"), nil
	}

	line := request.GetInt("line", 0)
	if line <= 0 {
		return mcp.NewToolResultError("line must be positive"), nil
	}

	col := request.GetInt("column", 0)
	if col <= 0 {
		return mcp.NewToolResultError("column must be positive"), nil
	}

	// Validate path
	if !s.cfg.IsPathAllowed(file) {
		return mcp.NewToolResultError("file is outside workspace root"), nil
	}

	locations, err := s.lspManager.Definition(ctx, file, line, col)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("definition lookup failed: %s", errors.SanitizeError(err))), nil
	}

	if len(locations) == 0 {
		return mcp.NewToolResultText("No definition found."), nil
	}

	var output strings.Builder
	output.WriteString(fmt.Sprintf("Found %d definition(s):\n\n", len(locations)))

	for _, loc := range locations {
		filePath := lsp.URIToFile(loc.URI)
		output.WriteString(fmt.Sprintf("**%s:%d:%d**\n", filePath, loc.Range.Start.Line+1, loc.Range.Start.Character+1))

		// Try to read a preview snippet
		preview := readFilePreview(filePath, loc.Range.Start.Line+1, 3)
		if preview != "" {
			output.WriteString("```\n")
			output.WriteString(preview)
			output.WriteString("```\n")
		}
		output.WriteString("\n")
	}

	return mcp.NewToolResultText(output.String()), nil
}

// handleFindReferences handles the find_references tool.
func (s *Server) handleFindReferences(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	file, err := request.RequireString("file")
	if err != nil {
		return mcp.NewToolResultError("file is required"), nil
	}

	line := request.GetInt("line", 0)
	if line <= 0 {
		return mcp.NewToolResultError("line must be positive"), nil
	}

	col := request.GetInt("column", 0)
	if col <= 0 {
		return mcp.NewToolResultError("column must be positive"), nil
	}

	includeDecl := request.GetBool("include_declaration", true)

	// Validate path
	if !s.cfg.IsPathAllowed(file) {
		return mcp.NewToolResultError("file is outside workspace root"), nil
	}

	locations, err := s.lspManager.References(ctx, file, line, col, includeDecl)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("references lookup failed: %s", errors.SanitizeError(err))), nil
	}

	if len(locations) == 0 {
		return mcp.NewToolResultText("No references found."), nil
	}

	var output strings.Builder
	output.WriteString(fmt.Sprintf("Found %d reference(s):\n\n", len(locations)))

	// Group by file
	fileGroups := make(map[string][]lsp.Location)
	for _, loc := range locations {
		filePath := lsp.URIToFile(loc.URI)
		fileGroups[filePath] = append(fileGroups[filePath], loc)
	}

	for filePath, locs := range fileGroups {
		output.WriteString(fmt.Sprintf("**%s** (%d)\n", filePath, len(locs)))
		for _, loc := range locs {
			output.WriteString(fmt.Sprintf("  L%d:%d\n", loc.Range.Start.Line+1, loc.Range.Start.Character+1))
		}
		output.WriteString("\n")
	}

	return mcp.NewToolResultText(output.String()), nil
}

// readFilePreview reads a few lines from a file around the given line.
func readFilePreview(file string, line, contextLines int) string {
	content, err := os.ReadFile(file)
	if err != nil {
		return ""
	}

	lines := splitLines(string(content))
	startLine := max(1, line-contextLines)
	endLine := min(line+contextLines, len(lines))

	var preview strings.Builder
	for i := startLine - 1; i < endLine && i < len(lines); i++ {
		lineText := lines[i]
		if len(lineText) > PreviewLineMaxLength {
			lineText = lineText[:PreviewLineMaxLength] + "..."
		}
		prefix := "  "
		if i+1 == line {
			prefix = "> "
		}
		preview.WriteString(fmt.Sprintf("%s%4d: %s\n", prefix, i+1, lineText))
	}

	return preview.String()
}
