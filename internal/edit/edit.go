// Package edit provides AST-aware file editing capabilities.
package edit

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"

	"github.com/lukaszraczylo/mcp-filepuff/internal/parser"
	"github.com/lukaszraczylo/mcp-filepuff/internal/util"
	"github.com/lukaszraczylo/mcp-filepuff/pkg/errors"
	"github.com/lukaszraczylo/mcp-filepuff/pkg/protocol"
	"github.com/sergi/go-diff/diffmatchpatch"
	sitter "github.com/smacker/go-tree-sitter"
)

// EditOperation defines the type of edit operation.
type EditOperation string

const (
	EditReplace      EditOperation = "replace"
	EditInsertBefore EditOperation = "insert_before"
	EditInsertAfter  EditOperation = "insert_after"
	EditDelete       EditOperation = "delete"
)

// ASTEdit represents an AST-aware edit request.
type ASTEdit struct {
	File       string        `json:"file"`
	Operation  EditOperation `json:"operation"`
	NewContent string        `json:"new_content,omitempty"`
	Selector   ASTSelector   `json:"selector"`
}

// ASTSelector specifies how to find the target node.
type ASTSelector struct {
	Kind        string `json:"kind,omitempty"`
	Name        string `json:"name,omitempty"`
	Pattern     string `json:"pattern,omitempty"`
	Text        string `json:"text,omitempty"`
	TextPattern string `json:"text_pattern,omitempty"`
	AtLine      int    `json:"at_line,omitempty"`
	Index       int    `json:"index,omitempty"`
	LineEnd     int    `json:"line_end,omitempty"`
}

// EditResult contains the result of an edit operation.
type EditResult struct {
	Diff            string `json:"diff,omitempty"`
	OriginalContent string `json:"original_content,omitempty"`
	NewContent      string `json:"new_content,omitempty"`
	Error           string `json:"error,omitempty"`
	Success         bool   `json:"success"`
	Applied         bool   `json:"applied"`
}

// Engine performs AST-aware edits.
type Engine struct {
	registry  *parser.Registry
	dmp       *diffmatchpatch.DiffMatchPatch
	fileLocks sync.Map // map[string]*sync.Mutex for per-file locking
}

// NewEngine creates a new edit engine.
func NewEngine(registry *parser.Registry) *Engine {
	return &Engine{
		registry:  registry,
		dmp:       diffmatchpatch.New(),
		fileLocks: sync.Map{},
	}
}

// lockFile acquires a lock for the specified file and returns an unlock function.
// This prevents concurrent edits to the same file which could cause corruption.
func (e *Engine) lockFile(filePath string) func() {
	// Get or create mutex for this file
	actual, _ := e.fileLocks.LoadOrStore(filePath, &sync.Mutex{})
	mu := actual.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

// Preview generates a preview of an edit without applying it.
func (e *Engine) Preview(ctx context.Context, edit *ASTEdit) (*EditResult, error) {
	return e.performEdit(ctx, edit, false)
}

// Apply performs an edit and writes the result to disk.
// Uses file locking to prevent concurrent edits to the same file.
func (e *Engine) Apply(ctx context.Context, edit *ASTEdit) (*EditResult, error) {
	unlock := e.lockFile(edit.File)
	defer unlock()
	return e.performEdit(ctx, edit, true)
}

// performEdit executes an edit operation.
func (e *Engine) performEdit(ctx context.Context, edit *ASTEdit, apply bool) (*EditResult, error) {
	// Determine if we should use text mode
	useTextMode := e.shouldUseTextMode(edit)

	if useTextMode {
		return e.performTextEdit(ctx, edit, apply)
	}
	return e.performASTEdit(ctx, edit, apply)
}

// shouldUseTextMode determines if text-based editing should be used.
func (e *Engine) shouldUseTextMode(edit *ASTEdit) bool {
	// Use text mode if text-specific selectors are provided
	if edit.Selector.Text != "" || edit.Selector.TextPattern != "" {
		return true
	}

	// Use text mode if line range is specified without AST selectors
	if edit.Selector.AtLine > 0 && edit.Selector.LineEnd > 0 &&
		edit.Selector.Kind == "" && edit.Selector.Name == "" && edit.Selector.Pattern == "" {
		return true
	}

	// Use text mode if language is not supported for AST
	lang := protocol.DetectLanguage(edit.File)
	return lang == protocol.LangUnknown
}

// performASTEdit executes an AST-aware edit operation.
func (e *Engine) performASTEdit(ctx context.Context, edit *ASTEdit, apply bool) (*EditResult, error) {
	// Validate operation
	if err := e.validateASTEdit(edit); err != nil {
		return &EditResult{Success: false, Error: err.Error()}, nil
	}

	// Read file
	content, err := os.ReadFile(edit.File)
	if err != nil {
		structuredErr := errors.NewFileNotReadableError(edit.File, err)
		return &EditResult{Success: false, Error: structuredErr.Error()}, nil
	}

	// Parse file
	parseResult, err := e.registry.Parse(ctx, edit.File, content)
	if err != nil {
		return &EditResult{Success: false, Error: err.Error()}, nil
	}

	// Find target node
	node, err := e.resolveSelector(edit.Selector, parseResult.Tree, content)
	if err != nil {
		return &EditResult{Success: false, Error: err.Error()}, nil
	}

	// Apply edit
	newContent, err := e.applyEdit(edit, node, content)
	if err != nil {
		return &EditResult{Success: false, Error: err.Error()}, nil
	}

	// Validate new content (re-parse)
	_, err = e.registry.Parse(ctx, edit.File, newContent)
	if err != nil {
		structuredErr := errors.NewEditValidationError(edit.File, err)
		return &EditResult{
			Success: false,
			Error:   structuredErr.Error(),
		}, nil
	}

	// Generate diff
	diff := e.generateDiff(string(content), string(newContent), edit.File)

	result := &EditResult{
		Success:         true,
		Diff:            diff,
		OriginalContent: string(content),
		NewContent:      string(newContent),
		Applied:         false,
	}

	// Apply changes if requested
	if apply {
		// Preserve original file permissions
		fileInfo, err := os.Stat(edit.File)
		perm := os.FileMode(0o600) // default fallback
		if err == nil {
			perm = fileInfo.Mode().Perm()
		}

		if err := os.WriteFile(edit.File, newContent, perm); err != nil {
			structuredErr := errors.NewFileNotWritableError(edit.File, err)
			return &EditResult{
				Success: false,
				Error:   structuredErr.Error(),
			}, nil
		}
		result.Applied = true
	}

	return result, nil
}

// performTextEdit executes a text-based edit operation for non-AST files.
func (e *Engine) performTextEdit(_ context.Context, edit *ASTEdit, apply bool) (*EditResult, error) {
	// Validate operation
	if err := e.validateTextEdit(edit); err != nil {
		return &EditResult{Success: false, Error: err.Error()}, nil
	}

	// Read file
	content, err := os.ReadFile(edit.File)
	if err != nil {
		structuredErr := errors.NewFileNotReadableError(edit.File, err)
		return &EditResult{Success: false, Error: structuredErr.Error()}, nil
	}

	// Find the text selection (byte range)
	start, end, err := e.resolveTextSelector(edit.Selector, content)
	if err != nil {
		return &EditResult{Success: false, Error: err.Error()}, nil
	}

	// Apply edit
	newContent, err := e.applyTextEditOperation(edit.Operation, content, start, end, edit.NewContent)
	if err != nil {
		return &EditResult{Success: false, Error: err.Error()}, nil
	}

	// Generate diff
	diff := e.generateDiff(string(content), string(newContent), edit.File)

	result := &EditResult{
		Success:         true,
		Diff:            diff,
		OriginalContent: string(content),
		NewContent:      string(newContent),
		Applied:         false,
	}

	// Apply changes if requested
	if apply {
		// Preserve original file permissions
		fileInfo, err := os.Stat(edit.File)
		perm := os.FileMode(0o600) // default fallback
		if err == nil {
			perm = fileInfo.Mode().Perm()
		}

		if err := os.WriteFile(edit.File, newContent, perm); err != nil {
			structuredErr := errors.NewFileNotWritableError(edit.File, err)
			return &EditResult{
				Success: false,
				Error:   structuredErr.Error(),
			}, nil
		}
		result.Applied = true
	}

	return result, nil
}

// validateBaseEdit checks common edit request fields.
func (e *Engine) validateBaseEdit(edit *ASTEdit) error {
	if edit.File == "" {
		return errors.NewInvalidEditError("file is required")
	}

	if edit.Operation == "" {
		return errors.NewInvalidEditError("operation is required")
	}

	// Validate operation type
	switch edit.Operation {
	case EditReplace, EditInsertBefore, EditInsertAfter:
		if edit.NewContent == "" {
			return errors.NewInvalidEditError(fmt.Sprintf("new_content is required for %s operation", edit.Operation))
		}
	case EditDelete:
		// new_content not required
	default:
		return errors.NewInvalidEditError(fmt.Sprintf("unknown operation: %s", edit.Operation))
	}

	return nil
}

// validateASTEdit checks if an AST edit request is valid.
func (e *Engine) validateASTEdit(edit *ASTEdit) error {
	if err := e.validateBaseEdit(edit); err != nil {
		return err
	}

	// Validate AST selector
	if edit.Selector.Kind == "" && edit.Selector.Name == "" && edit.Selector.Pattern == "" && edit.Selector.AtLine == 0 {
		return errors.NewInvalidEditError("AST selector must specify at least one of: kind, name, pattern, or at_line")
	}

	return nil
}

// validateTextEdit checks if a text edit request is valid.
func (e *Engine) validateTextEdit(edit *ASTEdit) error {
	if err := e.validateBaseEdit(edit); err != nil {
		return err
	}

	// Validate text selector - need at least one text selection method
	hasTextSelector := edit.Selector.Text != "" ||
		edit.Selector.TextPattern != "" ||
		edit.Selector.AtLine > 0

	if !hasTextSelector {
		return errors.NewInvalidEditError("text selector must specify at least one of: text, text_pattern, or at_line")
	}

	// Validate regex pattern if provided (uses cached compilation)
	if edit.Selector.TextPattern != "" {
		if _, err := util.CompileRegex(edit.Selector.TextPattern); err != nil {
			return errors.Wrap(errors.ErrInvalidEdit, "invalid text_pattern regex", err)
		}
	}

	return nil
}

// resolveSelector finds the target node based on the selector.
func (e *Engine) resolveSelector(sel ASTSelector, tree *sitter.Tree, content []byte) (*sitter.Node, error) {
	if tree == nil {
		return nil, errors.NewNodeNotFoundError("no AST tree available")
	}

	root := tree.RootNode()
	if root == nil {
		return nil, errors.NewNodeNotFoundError("empty AST tree")
	}

	var matches []*sitter.Node

	parser.WalkTree(root, func(n *sitter.Node) bool {
		if e.matchesSelector(sel, n, content) {
			matches = append(matches, n)
		}
		return true
	})

	if len(matches) == 0 {
		selectorDesc := fmt.Sprintf("kind=%s name=%s pattern=%s line=%d", sel.Kind, sel.Name, sel.Pattern, sel.AtLine)
		return nil, errors.NewNodeNotFoundError(selectorDesc)
	}

	// When using AtLine without a specific Kind, prefer the smallest (most specific) node.
	// This prevents matching large parent nodes like source_file when we want a specific declaration.
	if sel.AtLine > 0 && sel.Kind == "" {
		matches = sortBySpecificity(matches)
	}

	// Use index to select specific match
	index := sel.Index
	if index < 0 || index >= len(matches) {
		return nil, errors.NewInvalidSelectionError(fmt.Sprintf("selector matched %d nodes, but index %d is out of range", len(matches), index))
	}

	return matches[index], nil
}

// sortBySpecificity sorts nodes so that the most useful nodes come first.
// Prefers: 1) Named nodes (declarations/statements) over anonymous tokens
// 2) Smaller nodes over larger ones (more specific)
func sortBySpecificity(nodes []*sitter.Node) []*sitter.Node {
	if len(nodes) <= 1 {
		return nodes
	}

	result := make([]*sitter.Node, len(nodes))
	copy(result, nodes)

	slices.SortFunc(result, func(a, b *sitter.Node) int {
		if shouldPrefer(a, b) {
			return -1
		}
		if shouldPrefer(b, a) {
			return 1
		}
		return 0
	})

	return result
}

// shouldPrefer returns true if node a should come before node b.
func shouldPrefer(a, b *sitter.Node) bool {
	// Prefer named nodes over anonymous tokens
	aIsNamed := a.IsNamed()
	bIsNamed := b.IsNamed()
	if aIsNamed && !bIsNamed {
		return true
	}
	if !aIsNamed && bIsNamed {
		return false
	}

	// Both named or both anonymous: prefer smaller meaningful nodes
	// But filter out very small nodes (likely just identifiers/literals)
	aSize := a.EndByte() - a.StartByte()
	bSize := b.EndByte() - b.StartByte()

	// If both are named, prefer "declaration" or "statement" types
	aIsDecl := isDeclarationLike(a.Type())
	bIsDecl := isDeclarationLike(b.Type())
	if aIsDecl && !bIsDecl {
		return true
	}
	if !aIsDecl && bIsDecl {
		return false
	}

	// Same category: prefer smaller
	return aSize < bSize
}

// isDeclarationLike returns true for node types that represent declarations or statements.
func isDeclarationLike(nodeType string) bool {
	// Common declaration/statement patterns across languages
	declarationPatterns := []string{
		"declaration", "definition", "statement", "spec", "clause",
		"function", "method", "class", "struct", "interface", "type",
		"import", "package", "module", "const", "var", "let",
	}
	for _, pattern := range declarationPatterns {
		if strings.Contains(nodeType, pattern) {
			return true
		}
	}
	return false
}

// matchesSelector checks if a node matches the selector criteria.
func (e *Engine) matchesSelector(sel ASTSelector, n *sitter.Node, content []byte) bool {
	// Check kind
	if sel.Kind != "" && n.Type() != sel.Kind {
		return false
	}

	// Check name (look for identifier in the node)
	if sel.Name != "" {
		nameNode := n.ChildByFieldName("name")
		if nameNode == nil {
			// Also try to find an identifier child
			found := false
			for i := 0; i < int(n.NamedChildCount()); i++ {
				child := n.NamedChild(i)
				if child != nil && child.Type() == "identifier" {
					if parser.GetNodeText(child, content) == sel.Name {
						found = true
						break
					}
				}
			}
			if !found {
				return false
			}
		} else if parser.GetNodeText(nameNode, content) != sel.Name {
			return false
		}
	}

	// Check line
	if sel.AtLine > 0 {
		startLine := int(n.StartPoint().Row) + 1
		endLine := int(n.EndPoint().Row) + 1
		if sel.AtLine < startLine || sel.AtLine > endLine {
			return false
		}
	}

	// Pattern matching is handled separately (simplified here)
	if sel.Pattern != "" {
		nodeText := parser.GetNodeText(n, content)
		if !strings.Contains(nodeText, sel.Pattern) {
			return false
		}
	}

	return true
}

// applyEdit applies the edit operation to the content.
// AST mode uses exact byte positions — new_content is inserted verbatim without auto-indentation.
func (e *Engine) applyEdit(edit *ASTEdit, node *sitter.Node, content []byte) ([]byte, error) {
	startByte := node.StartByte()
	endByte := node.EndByte()

	newContent := edit.NewContent

	var result []byte

	switch edit.Operation {
	case EditReplace:
		result = append(result, content[:startByte]...)
		result = append(result, []byte(newContent)...)
		result = append(result, content[endByte:]...)

	case EditInsertBefore:
		insertion := newContent
		if !strings.HasSuffix(insertion, "\n") {
			insertion += "\n"
		}
		result = append(result, content[:startByte]...)
		result = append(result, []byte(insertion)...)
		result = append(result, content[startByte:]...)

	case EditInsertAfter:
		insertion := newContent
		if !strings.HasPrefix(insertion, "\n") {
			insertion = "\n" + insertion
		}
		result = append(result, content[:endByte]...)
		result = append(result, []byte(insertion)...)
		result = append(result, content[endByte:]...)

	case EditDelete:
		result = append(result, content[:startByte]...)
		result = append(result, content[endByte:]...)

	default:
		return nil, errors.NewInvalidEditError(fmt.Sprintf("unknown operation: %s", edit.Operation))
	}

	return result, nil
}

// detectIndentation detects the indentation at a given byte position.
func detectIndentation(content []byte, bytePos int) string {
	// Find the start of the line
	lineStart := bytePos
	for lineStart > 0 && content[lineStart-1] != '\n' {
		lineStart--
	}

	// Extract leading whitespace
	var indent strings.Builder
	for i := lineStart; i < bytePos && i < len(content); i++ {
		c := content[i]
		if c == ' ' || c == '\t' {
			indent.WriteByte(c)
		} else {
			break
		}
	}

	return indent.String()
}

// indentContent applies indentation to multi-line content.
func indentContent(content string, indent string) string {
	if indent == "" {
		return content
	}

	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if i > 0 && line != "" {
			lines[i] = indent + line
		}
	}

	return strings.Join(lines, "\n")
}

// generateDiff creates a unified diff between original and modified content.
// Uses line-level Myers diff algorithm for accurate and readable diffs.
func (e *Engine) generateDiff(original, modified, filename string) string {
	dmp := e.dmp

	// Use line-level diffing: encode each line as a single character,
	// diff the encoded strings, then decode back to real lines.
	// This prevents character-level diffs from splitting lines incorrectly.
	chars1, chars2, lineArray := dmp.DiffLinesToChars(original, modified)
	diffs := dmp.DiffMain(chars1, chars2, false)
	diffs = dmp.DiffCharsToLines(diffs, lineArray)

	// Cleanup for readability
	diffs = dmp.DiffCleanupSemantic(diffs)

	// Convert to unified diff format
	var buf bytes.Buffer
	buf.WriteString(fmt.Sprintf("--- %s\n", filename))
	buf.WriteString(fmt.Sprintf("+++ %s\n", filename))

	for _, diff := range diffs {
		// SplitAfter preserves the trailing \n on each line, so we can
		// distinguish real lines from a trailing empty split artifact.
		lines := strings.SplitAfter(diff.Text, "\n")
		for _, line := range lines {
			if line == "" {
				continue
			}

			// Remove trailing newline for display — we add our own.
			cleanLine := strings.TrimSuffix(line, "\n")

			switch diff.Type {
			case diffmatchpatch.DiffDelete:
				buf.WriteString(fmt.Sprintf("-%s\n", cleanLine))
			case diffmatchpatch.DiffInsert:
				buf.WriteString(fmt.Sprintf("+%s\n", cleanLine))
			case diffmatchpatch.DiffEqual:
				buf.WriteString(fmt.Sprintf(" %s\n", cleanLine))
			}
		}
	}

	return buf.String()
}

// resolveTextSelector finds the byte range for a text-based selection.
func (e *Engine) resolveTextSelector(sel ASTSelector, content []byte) (start, end int, err error) {
	switch {
	case sel.Text != "":
		return e.findExactText(content, sel.Text, sel.Index)
	case sel.TextPattern != "":
		return e.findRegexPattern(content, sel.TextPattern, sel.Index)
	case sel.AtLine > 0:
		return e.findLineRange(content, sel.AtLine, sel.LineEnd)
	default:
		return 0, 0, errors.NewInvalidEditError("text selector requires text, text_pattern, or at_line")
	}
}

// findExactText finds an exact text match in content.
func (e *Engine) findExactText(content []byte, text string, index int) (start, end int, err error) {
	if text == "" {
		return 0, 0, errors.NewInvalidEditError("text selector cannot be empty")
	}

	textBytes := []byte(text)
	type match struct{ start, end int }
	var matches []match

	offset := 0
	for {
		idx := bytes.Index(content[offset:], textBytes)
		if idx == -1 {
			break
		}
		matches = append(matches, match{
			start: offset + idx,
			end:   offset + idx + len(textBytes),
		})
		offset += idx + 1
	}

	if len(matches) == 0 {
		return 0, 0, errors.NewInvalidSelectionError(fmt.Sprintf("text not found: %q", truncateString(text, 50)))
	}

	if index >= len(matches) {
		return 0, 0, errors.NewInvalidSelectionError(fmt.Sprintf("selector_index %d out of range (found %d matches)", index, len(matches)))
	}

	return matches[index].start, matches[index].end, nil
}

// findRegexPattern finds a regex pattern match in content.
func (e *Engine) findRegexPattern(content []byte, pattern string, index int) (start, end int, err error) {
	re, err := util.CompileRegex(pattern)
	if err != nil {
		return 0, 0, errors.Wrap(errors.ErrInvalidEdit, "invalid regex pattern", err)
	}

	matches := re.FindAllIndex(content, -1)
	if len(matches) == 0 {
		return 0, 0, errors.NewInvalidSelectionError(fmt.Sprintf("pattern not found: %q", truncateString(pattern, 50)))
	}

	if index >= len(matches) {
		return 0, 0, errors.NewInvalidSelectionError(fmt.Sprintf("selector_index %d out of range (found %d matches)", index, len(matches)))
	}

	return matches[index][0], matches[index][1], nil
}

// findLineRange finds the byte range for a line range selection.
func (e *Engine) findLineRange(content []byte, lineStart, lineEnd int) (start, end int, err error) {
	if lineEnd == 0 {
		lineEnd = lineStart
	}

	if lineStart < 1 {
		return 0, 0, errors.NewInvalidEditError(fmt.Sprintf("line number must be >= 1, got %d", lineStart))
	}

	if lineEnd < lineStart {
		return 0, 0, errors.NewInvalidEditError(fmt.Sprintf("line_end (%d) must be >= line (%d)", lineEnd, lineStart))
	}

	lines := bytes.Split(content, []byte("\n"))
	// Trim phantom empty element from trailing newline
	if len(lines) > 0 && len(lines[len(lines)-1]) == 0 {
		lines = lines[:len(lines)-1]
	}
	totalLines := len(lines)

	// Convert to 0-indexed
	startIdx := lineStart - 1
	endIdx := lineEnd - 1

	if startIdx >= totalLines {
		return 0, 0, errors.NewInvalidSelectionError(fmt.Sprintf("line %d out of range (file has %d lines)", lineStart, totalLines))
	}
	if endIdx >= totalLines {
		return 0, 0, errors.NewInvalidSelectionError(fmt.Sprintf("line_end %d out of range (file has %d lines)", lineEnd, totalLines))
	}

	// Calculate byte positions
	start = 0
	for i := range startIdx {
		start += len(lines[i]) + 1 // +1 for newline
	}

	end = start
	for i := startIdx; i <= endIdx; i++ {
		end += len(lines[i])
		if i < totalLines-1 {
			end += 1 // newline
		}
	}

	return start, end, nil
}

// applyTextEditOperation applies a text edit operation.
func (e *Engine) applyTextEditOperation(op EditOperation, content []byte, start, end int, newContent string) ([]byte, error) {
	// Detect indentation at the selection point
	indentation := detectIndentation(content, start)
	indentedContent := indentContent(newContent, indentation)

	var result []byte

	switch op {
	case EditReplace:
		result = append(result, content[:start]...)
		result = append(result, []byte(indentedContent)...)
		result = append(result, content[end:]...)

	case EditInsertBefore:
		insertion := indentedContent
		if !strings.HasSuffix(insertion, "\n") {
			insertion += "\n"
		}
		result = append(result, content[:start]...)
		result = append(result, []byte(insertion)...)
		result = append(result, content[start:]...)

	case EditInsertAfter:
		insertion := indentedContent
		if !strings.HasPrefix(insertion, "\n") {
			insertion = "\n" + insertion
		}
		result = append(result, content[:end]...)
		result = append(result, []byte(insertion)...)
		result = append(result, content[end:]...)

	case EditDelete:
		result = append(result, content[:start]...)
		result = append(result, content[end:]...)

	default:
		return nil, errors.NewInvalidEditError(fmt.Sprintf("unknown operation: %s", op))
	}

	return result, nil
}

// truncateString truncates a string to maxLen with ellipsis.
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// ValidateLanguage checks if AST editing is supported for a file.
// Returns nil for supported languages, error for unsupported.
// Note: Text-based editing is always available regardless of this check.
func ValidateLanguage(filename string) error {
	lang := protocol.DetectLanguage(filename)
	if lang == protocol.LangUnknown {
		return fmt.Errorf("unsupported file type for AST editing: %s (text-based editing is available)", filename)
	}
	return nil
}
