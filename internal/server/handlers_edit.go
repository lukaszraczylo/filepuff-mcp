// Package server implements the MCP server for file operations.
package server

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/lukaszraczylo/mcp-filepuff/internal/edit"
	"github.com/lukaszraczylo/mcp-filepuff/pkg/errors"
	"github.com/lukaszraczylo/mcp-filepuff/pkg/protocol"
	"github.com/mark3labs/mcp-go/mcp"
)

// unescapeNewlines converts literal \n, \t, \" sequences to actual characters.
// This handles cases where MCP clients send double-escaped JSON strings.
func unescapeNewlines(s string) string {
	s = strings.ReplaceAll(s, "\\n", "\n")
	s = strings.ReplaceAll(s, "\\t", "\t")
	s = strings.ReplaceAll(s, "\\\"", "\"")
	s = strings.ReplaceAll(s, "\\\\", "\\")
	return s
}

// handleEditApply handles the edit_apply tool.
func (s *Server) handleEditApply(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return s.handleEdit(ctx, request)
}

// handleEdit performs an edit operation (always applies changes).
func (s *Server) handleEdit(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	file, err := request.RequireString("file")
	if err != nil {
		return mcp.NewToolResultError("file is required"), nil
	}

	operation, err := request.RequireString("operation")
	if err != nil {
		return mcp.NewToolResultError("operation is required"), nil
	}

	switch edit.EditOperation(operation) {
	case edit.EditReplace, edit.EditInsertBefore, edit.EditInsertAfter, edit.EditDelete:
		// valid
	default:
		return mcp.NewToolResultError(fmt.Sprintf(
			"invalid operation %q: must be one of: replace, insert_before, insert_after, delete", operation,
		)), nil
	}

	if !s.cfg.IsPathAllowed(file) {
		return mcp.NewToolResultError("file is outside workspace root"), nil
	}

	newContent := request.GetString("new_content", "")
	newContent = unescapeNewlines(newContent)

	selectorName := request.GetString("selector_name", "")

	astEdit := &edit.ASTEdit{
		File:       file,
		Operation:  edit.EditOperation(operation),
		NewContent: newContent,
		Selector: edit.ASTSelector{
			Kind:        request.GetString("selector_kind", ""),
			Name:        selectorName,
			AtLine:      request.GetInt("selector_line", 0),
			Index:       request.GetInt("selector_index", 0),
			LineEnd:     request.GetInt("selector_line_end", 0),
			Text:        request.GetString("selector_text", ""),
			TextPattern: request.GetString("selector_pattern", ""),
		},
	}

	result, err := s.editor.Apply(ctx, astEdit)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("edit failed: %s", errors.SanitizeError(err))), nil
	}

	if !result.Success {
		return mcp.NewToolResultError(result.Error), nil
	}

	// compact_response: return just the modified symbol instead of the full diff
	if request.GetBool("compact_response", false) && selectorName != "" {
		if content, readErr := os.ReadFile(file); readErr == nil {
			if start, end, found := s.resolveSymbolLines(ctx, file, content, selectorName, protocol.SymbolKind("")); found {
				lines := splitLines(string(content))
				var sb strings.Builder
				sb.WriteString(fmt.Sprintf("**Edit Applied** — %s (L%d-L%d):\n\n", selectorName, start, end))
				for i := start - 1; i < end && i < len(lines); i++ {
					sb.WriteString(fmt.Sprintf("%4d| %s\n", i+1, lines[i]))
				}
				return mcp.NewToolResultText(sb.String()), nil
			}
		}
		// fall through to diff if symbol lookup fails
	}

	var output strings.Builder
	output.WriteString("**Edit Applied Successfully**\n\n")
	output.WriteString("Diff:\n```diff\n")
	output.WriteString(result.Diff)
	output.WriteString("```\n")

	return mcp.NewToolResultText(output.String()), nil
}
