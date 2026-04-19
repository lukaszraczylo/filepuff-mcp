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

	// Determine response mode.
	// compact_response is a deprecated alias for response="count".
	respMode := request.GetString("response", "count")
	if request.GetBool("compact_response", false) {
		// Deprecated: use response=count
		respMode = "count"
	}

	switch respMode {
	case "none":
		return mcp.NewToolResultText(""), nil

	case "count":
		// Compute +added/-removed line counts from the unified diff.
		added, removed := countDiffLines(result.Diff)
		return mcp.NewToolResultText(fmt.Sprintf("+%d -%d", added, removed)), nil

	case "diff":
		var output strings.Builder
		output.WriteString("Diff:\n```diff\n")
		output.WriteString(result.Diff)
		output.WriteString("```\n")
		return mcp.NewToolResultText(output.String()), nil

	default:
		// Fallback: treat unknown values as "diff" for safety.
		var output strings.Builder
		output.WriteString("Diff:\n```diff\n")
		output.WriteString(result.Diff)
		output.WriteString("```\n")
		return mcp.NewToolResultText(output.String()), nil
	}
}

// countDiffLines counts added (+) and removed (-) lines in a unified diff string.
func countDiffLines(diff string) (added, removed int) {
	for _, line := range strings.Split(diff, "\n") {
		if len(line) == 0 {
			continue
		}
		switch line[0] {
		case '+':
			if !strings.HasPrefix(line, "+++") {
				added++
			}
		case '-':
			if !strings.HasPrefix(line, "---") {
				removed++
			}
		}
	}
	return
}
