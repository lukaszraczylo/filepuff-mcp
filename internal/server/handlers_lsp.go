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

// handleLSPQuery is the unified dispatcher for all LSP operations.
// action must be one of: "hover", "definition", "references".
func (s *Server) handleLSPQuery(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	action, err := request.RequireString("action")
	if err != nil {
		return mcp.NewToolResultError("action is required (hover | definition | references)"), nil
	}

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

	if !s.cfg.IsPathAllowed(file) {
		return mcp.NewToolResultError("file is outside workspace root"), nil
	}

	verbose := request.GetBool("verbose", false)

	switch action {
	case "hover":
		if _, ok := request.GetArguments()["include_declaration"]; ok {
			return mcp.NewToolResultError("include_declaration is only valid for action=references"), nil
		}
		if _, ok := request.GetArguments()["compact"]; ok {
			return mcp.NewToolResultError("compact is only valid for action=references"), nil
		}
		return s.lspHover(ctx, file, line, col, verbose)

	case "definition":
		if _, ok := request.GetArguments()["include_declaration"]; ok {
			return mcp.NewToolResultError("include_declaration is only valid for action=references"), nil
		}
		if _, ok := request.GetArguments()["compact"]; ok {
			return mcp.NewToolResultError("compact is only valid for action=references"), nil
		}
		return s.lspDefinition(ctx, file, line, col, verbose)

	case "references":
		includeDecl := request.GetBool("include_declaration", true)
		// compact: explicit call-time > session compact_refs pref > false
		var prefsCompact *bool
		if sp := s.sessionPrefs.Load(); sp != nil {
			prefsCompact = sp.CompactRefs
		}
		compact := effectiveBool(request, "compact", prefsCompact, false)
		return s.lspReferences(ctx, file, line, col, includeDecl, compact, verbose)

	default:
		return mcp.NewToolResultError(fmt.Sprintf("unknown action %q: must be hover | definition | references", action)), nil
	}
}

// lspHover performs hover (symbol info) for the given position.
func (s *Server) lspHover(ctx context.Context, file string, line, col int, verbose bool) (*mcp.CallToolResult, error) {
	hover, err := s.lspManager.Hover(ctx, file, line, col)
	if err != nil {
		return s.handleSymbolAtFallback(ctx, file, line, col, verbose)
	}

	if hover == nil {
		return mcp.NewToolResultText("No symbol information available at this position."), nil
	}

	var output strings.Builder
	if verbose {
		output.WriteString("**Symbol Information**\n\n")
	}
	output.WriteString(hover.Contents.Value)

	return mcp.NewToolResultText(output.String()), nil
}

// handleSymbolAtFallback provides AST-based symbol info when LSP is unavailable.
func (s *Server) handleSymbolAtFallback(ctx context.Context, file string, line, col int, verbose bool) (*mcp.CallToolResult, error) {
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
	if verbose {
		output.WriteString("**Symbol Information** (AST fallback)\n\n")
	}
	output.WriteString(fmt.Sprintf("Node type: `%s`\n", node.Type()))
	output.WriteString(fmt.Sprintf("Text: `%s`\n", parser.GetNodeText(node, content)))

	return mcp.NewToolResultText(output.String()), nil
}

// lspDefinition finds the definition of the symbol at the given position.
func (s *Server) lspDefinition(ctx context.Context, file string, line, col int, verbose bool) (*mcp.CallToolResult, error) {
	locations, err := s.lspManager.Definition(ctx, file, line, col)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("definition lookup failed: %s", errors.SanitizeError(err))), nil
	}

	if len(locations) == 0 {
		return mcp.NewToolResultText("No definition found."), nil
	}

	var output strings.Builder
	if verbose {
		output.WriteString(fmt.Sprintf("Found %d definition(s):\n\n", len(locations)))
	}

	for _, loc := range locations {
		filePath := lsp.URIToFile(loc.URI)
		output.WriteString(fmt.Sprintf("**%s:%d:%d**\n", filePath, loc.Range.Start.Line+1, loc.Range.Start.Character+1))

		preview := s.readFilePreview(filePath, loc.Range.Start.Line+1, 3)
		if preview != "" {
			output.WriteString("```\n")
			output.WriteString(preview)
			output.WriteString("```\n")
		}
		output.WriteString("\n")
	}

	return mcp.NewToolResultText(output.String()), nil
}

// lspReferences finds all references to the symbol at the given position.
func (s *Server) lspReferences(ctx context.Context, file string, line, col int, includeDecl, compact, verbose bool) (*mcp.CallToolResult, error) {
	locations, err := s.lspManager.References(ctx, file, line, col, includeDecl)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("references lookup failed: %s", errors.SanitizeError(err))), nil
	}

	if len(locations) == 0 {
		return mcp.NewToolResultText("No references found."), nil
	}

	// Group by file, preserving encounter order.
	fileGroups := make(map[string][]lsp.Location)
	fileOrder := make([]string, 0)
	for _, loc := range locations {
		filePath := lsp.URIToFile(loc.URI)
		if _, seen := fileGroups[filePath]; !seen {
			fileOrder = append(fileOrder, filePath)
		}
		fileGroups[filePath] = append(fileGroups[filePath], loc)
	}

	return mcp.NewToolResultText(formatReferences(fileGroups, fileOrder, len(locations), compact, verbose)), nil
}

// formatReferences formats grouped reference locations as a string.
// compact=false: verbose multi-line format with L{line}:{col} per entry.
// compact=true:  one line per file — file:[line:col, ...]  (N), with same-line
// columns collapsed to line:{col1,col2,...}.
func formatReferences(fileGroups map[string][]lsp.Location, fileOrder []string, total int, compact bool, verbose bool) string {
	var output strings.Builder
	if verbose {
		output.WriteString(fmt.Sprintf("Found %d reference(s):\n\n", total))
	}

	for _, filePath := range fileOrder {
		locs := fileGroups[filePath]
		if compact {
			output.WriteString(formatReferencesCompact(filePath, locs))
		} else {
			output.WriteString(fmt.Sprintf("**%s** (%d)\n", filePath, len(locs)))
			for _, loc := range locs {
				output.WriteString(fmt.Sprintf("  L%d:%d\n", loc.Range.Start.Line+1, loc.Range.Start.Character+1))
			}
			output.WriteString("\n")
		}
	}

	return output.String()
}

// formatReferencesCompact formats one file's references as a single compact line.
// Same-line references are collapsed: 12:{5,8} instead of 12:5, 12:8.
func formatReferencesCompact(filePath string, locs []lsp.Location) string {
	// Build ordered line->col map preserving encounter order per line.
	type lineEntry struct {
		lineNum int
		cols    []int
	}
	lineMap := make(map[int]*lineEntry)
	lineOrder := make([]int, 0, len(locs))

	for _, loc := range locs {
		ln := loc.Range.Start.Line + 1
		col := loc.Range.Start.Character + 1
		if e, ok := lineMap[ln]; ok {
			e.cols = append(e.cols, col)
		} else {
			lineMap[ln] = &lineEntry{lineNum: ln, cols: []int{col}}
			lineOrder = append(lineOrder, ln)
		}
	}

	// Build the bracket contents.
	parts := make([]string, 0, len(lineOrder))
	for _, ln := range lineOrder {
		e := lineMap[ln]
		if len(e.cols) == 1 {
			parts = append(parts, fmt.Sprintf("%d:%d", ln, e.cols[0]))
		} else {
			colStrs := make([]string, len(e.cols))
			for i, c := range e.cols {
				colStrs[i] = fmt.Sprintf("%d", c)
			}
			parts = append(parts, fmt.Sprintf("%d:{%s}", ln, strings.Join(colStrs, ",")))
		}
	}

	return fmt.Sprintf("%s:[%s]  (%d)\n", filePath, strings.Join(parts, ", "), len(locs))
}

// readFilePreview reads a few lines from a file around the given line.
// It validates that the file path is within the allowed workspace before reading.
func (s *Server) readFilePreview(file string, line, contextLines int) string {
	if !s.cfg.IsPathAllowed(file) {
		s.logger.Warn("readFilePreview: path not allowed", "path", file)
		return ""
	}

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
