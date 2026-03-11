// Package server implements the MCP server for file operations.
package server

import (
	"context"
	"fmt"
	"strings"

	"github.com/lukaszraczylo/mcp-filepuff/internal/edit"
	"github.com/lukaszraczylo/mcp-filepuff/pkg/errors"
	"github.com/mark3labs/mcp-go/mcp"
)

// unescapeNewlines converts literal \n, \t, \" sequences to actual characters.
// This handles cases where MCP clients send double-escaped JSON strings.
func unescapeNewlines(s string) string {
	// Replace common escape sequences
	s = strings.ReplaceAll(s, "\\n", "\n")
	s = strings.ReplaceAll(s, "\\t", "\t")
	s = strings.ReplaceAll(s, "\\\"", "\"")
	s = strings.ReplaceAll(s, "\\\\", "\\")
	return s
}

// handleEditPreview handles the edit_preview tool.
func (s *Server) handleEditPreview(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return s.handleEdit(ctx, request, false)
}

// handleEditApply handles the edit_apply tool.
func (s *Server) handleEditApply(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return s.handleEdit(ctx, request, true)
}

// handleEdit is the shared implementation for edit_preview and edit_apply.
func (s *Server) handleEdit(ctx context.Context, request mcp.CallToolRequest, apply bool) (*mcp.CallToolResult, error) {
	file, err := request.RequireString("file")
	if err != nil {
		return mcp.NewToolResultError("file is required"), nil
	}

	operation, err := request.RequireString("operation")
	if err != nil {
		return mcp.NewToolResultError("operation is required"), nil
	}

	// Validate operation against known values
	switch edit.EditOperation(operation) {
	case edit.EditReplace, edit.EditInsertBefore, edit.EditInsertAfter, edit.EditDelete:
		// valid
	default:
		return mcp.NewToolResultError(fmt.Sprintf(
			"invalid operation %q: must be one of: replace, insert_before, insert_after, delete", operation,
		)), nil
	}

	// Validate path
	if !s.cfg.IsPathAllowed(file) {
		return mcp.NewToolResultError("file is outside workspace root"), nil
	}

	// Note: We no longer validate language support here.
	// The edit engine automatically detects whether to use AST or text mode.

	// Build edit request with both AST and text-mode selectors
	newContent := request.GetString("new_content", "")

	// Unescape common escape sequences that may be double-encoded by MCP clients
	newContent = unescapeNewlines(newContent)

	astEdit := &edit.ASTEdit{
		File:       file,
		Operation:  edit.EditOperation(operation),
		NewContent: newContent,
		Selector: edit.ASTSelector{
			// AST-mode selectors
			Kind:   request.GetString("selector_kind", ""),
			Name:   request.GetString("selector_name", ""),
			AtLine: request.GetInt("selector_line", 0),
			Index:  request.GetInt("selector_index", 0),
			// Text-mode selectors
			LineEnd:     request.GetInt("selector_line_end", 0),
			Text:        request.GetString("selector_text", ""),
			TextPattern: request.GetString("selector_pattern", ""),
		},
	}

	// Perform edit
	var result *edit.EditResult
	if apply {
		result, err = s.editor.Apply(ctx, astEdit)
	} else {
		result, err = s.editor.Preview(ctx, astEdit)
	}

	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("edit failed: %s", errors.SanitizeError(err))), nil
	}

	if !result.Success {
		return mcp.NewToolResultError(result.Error), nil
	}

	// Format output
	var output strings.Builder
	if apply {
		output.WriteString("**Edit Applied Successfully**\n\n")
	} else {
		output.WriteString("**Edit Preview**\n\n")
	}

	output.WriteString("Diff:\n```diff\n")
	output.WriteString(result.Diff)
	output.WriteString("```\n")

	return mcp.NewToolResultText(output.String()), nil
}
