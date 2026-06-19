// Package parser provides documentation extraction for multiple languages.
package parser

import (
	"regexp"
	"sort"
	"strings"

	"github.com/lukaszraczylo/mcp-filepuff/pkg/protocol"
	sitter "github.com/smacker/go-tree-sitter"
)

// DocComment represents an extracted documentation comment.
type DocComment struct {
	Tags      map[string]string
	Text      string
	Raw       string
	Style     CommentStyle
	StartLine int
	EndLine   int
}

// CommentStyle indicates the type of comment.
type CommentStyle string

const (
	CommentStyleLine      CommentStyle = "line"      // // comment
	CommentStyleBlock     CommentStyle = "block"     // /* comment */
	CommentStyleJSDoc     CommentStyle = "jsdoc"     // /** comment */
	CommentStyleDoxygen   CommentStyle = "doxygen"   // /** comment */ or /// comment
	CommentStyleDocstring CommentStyle = "docstring" // """comment""" or '''comment'''
	CommentStyleHash      CommentStyle = "hash"      // # comment (Python)
)

// ExtractDocComment extracts the documentation comment for a node.
func ExtractDocComment(n *sitter.Node, content []byte, lang protocol.Language) *DocComment {
	if n == nil {
		return nil
	}

	switch lang {
	case protocol.LangGo:
		return extractGoDocComment(n, content)
	case protocol.LangTypeScript, protocol.LangJavaScript:
		return extractJSDocComment(n, content)
	case protocol.LangPython:
		return extractPythonDocComment(n, content)
	case protocol.LangC, protocol.LangCpp:
		return extractCDocComment(n, content)
	case protocol.LangElixir:
		return extractElixirDocComment(n, content)
	case protocol.LangRust:
		return extractRustDocComment(n, content)
	default:
		return nil
	}
}

// extractGoDocComment extracts Go documentation comments.
// Go uses // or /* */ comments immediately preceding a declaration.
func extractGoDocComment(n *sitter.Node, content []byte) *DocComment {
	comments := collectPrecedingComments(n, content, []string{"comment"})
	if len(comments) == 0 {
		return nil
	}

	var parts []string
	var raw []string
	startLine := -1
	endLine := -1

	for _, c := range comments {
		text := GetNodeText(c, content)
		raw = append(raw, text)

		if startLine == -1 {
			startLine = int(c.StartPoint().Row) + 1
		}
		endLine = int(c.EndPoint().Row) + 1

		cleaned := cleanGoComment(text)
		if cleaned != "" {
			parts = append(parts, cleaned)
		}
	}

	if len(parts) == 0 {
		return nil
	}

	return &DocComment{
		Text:      strings.Join(parts, "\n"),
		Raw:       strings.Join(raw, "\n"),
		Style:     detectCommentStyle(raw[0]),
		Tags:      nil, // Go doesn't use JSDoc-style tags
		StartLine: startLine,
		EndLine:   endLine,
	}
}

// extractJSDocComment extracts JSDoc-style documentation comments.
func extractJSDocComment(n *sitter.Node, content []byte) *DocComment {
	comments := collectPrecedingComments(n, content, []string{"comment"})
	if len(comments) == 0 {
		return nil
	}

	// JSDoc prefers the last comment block if it's a JSDoc comment
	var jsDocComment *sitter.Node
	for i := len(comments) - 1; i >= 0; i-- {
		text := GetNodeText(comments[i], content)
		if strings.HasPrefix(strings.TrimSpace(text), "/**") {
			jsDocComment = comments[i]
			break
		}
	}

	if jsDocComment != nil {
		text := GetNodeText(jsDocComment, content)
		cleaned, tags := parseJSDoc(text)
		return &DocComment{
			Text:      cleaned,
			Raw:       text,
			Style:     CommentStyleJSDoc,
			Tags:      tags,
			StartLine: int(jsDocComment.StartPoint().Row) + 1,
			EndLine:   int(jsDocComment.EndPoint().Row) + 1,
		}
	}

	// Fall back to regular comments
	var parts []string
	var raw []string
	startLine := -1
	endLine := -1

	for _, c := range comments {
		text := GetNodeText(c, content)
		raw = append(raw, text)

		if startLine == -1 {
			startLine = int(c.StartPoint().Row) + 1
		}
		endLine = int(c.EndPoint().Row) + 1

		cleaned := cleanJSComment(text)
		if cleaned != "" {
			parts = append(parts, cleaned)
		}
	}

	if len(parts) == 0 {
		return nil
	}

	return &DocComment{
		Text:      strings.Join(parts, "\n"),
		Raw:       strings.Join(raw, "\n"),
		Style:     CommentStyleLine,
		Tags:      nil,
		StartLine: startLine,
		EndLine:   endLine,
	}
}

// extractPythonDocComment extracts Python docstrings.
// Python docstrings are triple-quoted strings inside the function/class body.
func extractPythonDocComment(n *sitter.Node, content []byte) *DocComment {
	// Python docstrings are inside the body, not before
	body := n.ChildByFieldName("body")
	if body == nil {
		return nil
	}

	// First statement should be the docstring if present
	if body.NamedChildCount() > 0 {
		first := body.NamedChild(0)
		if first != nil && first.Type() == "expression_statement" {
			if first.NamedChildCount() > 0 {
				expr := first.NamedChild(0)
				if expr != nil && expr.Type() == "string" {
					text := GetNodeText(expr, content)
					cleaned := cleanPythonDocstring(text)
					return &DocComment{
						Text:      cleaned,
						Raw:       text,
						Style:     CommentStyleDocstring,
						Tags:      nil,
						StartLine: int(expr.StartPoint().Row) + 1,
						EndLine:   int(expr.EndPoint().Row) + 1,
					}
				}
			}
		}
	}

	// Also check for # comments before the definition
	comments := collectPrecedingComments(n, content, []string{"comment"})
	if len(comments) == 0 {
		return nil
	}

	var parts []string
	var raw []string
	startLine := -1
	endLine := -1

	for _, c := range comments {
		text := GetNodeText(c, content)
		raw = append(raw, text)

		if startLine == -1 {
			startLine = int(c.StartPoint().Row) + 1
		}
		endLine = int(c.EndPoint().Row) + 1

		// Clean # comment
		cleaned := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(text), "#"))
		if cleaned != "" {
			parts = append(parts, cleaned)
		}
	}

	if len(parts) == 0 {
		return nil
	}

	return &DocComment{
		Text:      strings.Join(parts, "\n"),
		Raw:       strings.Join(raw, "\n"),
		Style:     CommentStyleHash,
		Tags:      nil,
		StartLine: startLine,
		EndLine:   endLine,
	}
}

// extractCDocComment extracts C/C++ documentation comments (Doxygen style).
func extractCDocComment(n *sitter.Node, content []byte) *DocComment {
	comments := collectPrecedingComments(n, content, []string{"comment"})
	if len(comments) == 0 {
		return nil
	}

	// Look for Doxygen-style comment
	var doxyComment *sitter.Node
	for i := len(comments) - 1; i >= 0; i-- {
		text := GetNodeText(comments[i], content)
		trimmed := strings.TrimSpace(text)
		if strings.HasPrefix(trimmed, "/**") || strings.HasPrefix(trimmed, "///") || strings.HasPrefix(trimmed, "//!") {
			doxyComment = comments[i]
			break
		}
	}

	if doxyComment != nil {
		text := GetNodeText(doxyComment, content)
		cleaned, tags := parseDoxygen(text)
		return &DocComment{
			Text:      cleaned,
			Raw:       text,
			Style:     CommentStyleDoxygen,
			Tags:      tags,
			StartLine: int(doxyComment.StartPoint().Row) + 1,
			EndLine:   int(doxyComment.EndPoint().Row) + 1,
		}
	}

	// Fall back to regular comments
	var parts []string
	var raw []string
	startLine := -1
	endLine := -1

	for _, c := range comments {
		text := GetNodeText(c, content)
		raw = append(raw, text)

		if startLine == -1 {
			startLine = int(c.StartPoint().Row) + 1
		}
		endLine = int(c.EndPoint().Row) + 1

		cleaned := cleanCComment(text)
		if cleaned != "" {
			parts = append(parts, cleaned)
		}
	}

	if len(parts) == 0 {
		return nil
	}

	return &DocComment{
		Text:      strings.Join(parts, "\n"),
		Raw:       strings.Join(raw, "\n"),
		Style:     detectCommentStyle(raw[0]),
		Tags:      nil,
		StartLine: startLine,
		EndLine:   endLine,
	}
}

// collectPrecedingComments collects all comment nodes immediately before a node.
func collectPrecedingComments(n *sitter.Node, _ []byte, commentTypes []string) []*sitter.Node {
	var comments []*sitter.Node

	// Walk backwards through siblings
	prev := n.PrevSibling()
	lastCommentLine := int(n.StartPoint().Row)

	for prev != nil {
		isComment := false
		nodeType := prev.Type()
		for _, ct := range commentTypes {
			if nodeType == ct {
				isComment = true
				break
			}
		}

		if !isComment {
			break
		}

		commentEndLine := int(prev.EndPoint().Row)

		// Check if there's a blank line gap
		if lastCommentLine-commentEndLine > 1 {
			break
		}

		comments = append([]*sitter.Node{prev}, comments...)
		lastCommentLine = int(prev.StartPoint().Row)
		prev = prev.PrevSibling()
	}

	return comments
}

// detectCommentStyle determines the style of a comment.
func detectCommentStyle(comment string) CommentStyle {
	trimmed := strings.TrimSpace(comment)
	if strings.HasPrefix(trimmed, "/**") {
		return CommentStyleJSDoc
	}
	if strings.HasPrefix(trimmed, "///") || strings.HasPrefix(trimmed, "//!") {
		return CommentStyleDoxygen
	}
	if strings.HasPrefix(trimmed, "/*") {
		return CommentStyleBlock
	}
	if strings.HasPrefix(trimmed, "//") {
		return CommentStyleLine
	}
	if strings.HasPrefix(trimmed, "#") {
		return CommentStyleHash
	}
	if strings.HasPrefix(trimmed, `"""`) || strings.HasPrefix(trimmed, `'''`) {
		return CommentStyleDocstring
	}
	return CommentStyleLine
}

// cleanGoComment cleans a Go comment.
func cleanGoComment(comment string) string {
	comment = strings.TrimSpace(comment)

	// Handle // comments
	if after, found := strings.CutPrefix(comment, "//"); found {
		return strings.TrimSpace(after)
	}

	// Handle /* */ comments
	if strings.HasPrefix(comment, "/*") && strings.HasSuffix(comment, "*/") {
		comment = strings.TrimPrefix(comment, "/*")
		comment = strings.TrimSuffix(comment, "*/")
		return cleanBlockComment(comment)
	}

	return strings.TrimSpace(comment)
}

// cleanJSComment cleans a JavaScript/TypeScript comment.
func cleanJSComment(comment string) string {
	return cleanGoComment(comment) // Same rules
}

// cleanCComment cleans a C/C++ comment.
func cleanCComment(comment string) string {
	return cleanGoComment(comment) // Same rules
}

// cleanBlockComment cleans the content of a block comment.
func cleanBlockComment(comment string) string {
	lines := strings.Split(comment, "\n")
	var cleaned []string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		// Remove leading * from each line (common in block comments)
		line = strings.TrimPrefix(line, "*")
		line = strings.TrimSpace(line)
		cleaned = append(cleaned, line)
	}

	// Remove empty leading/trailing lines
	for len(cleaned) > 0 && cleaned[0] == "" {
		cleaned = cleaned[1:]
	}
	for len(cleaned) > 0 && cleaned[len(cleaned)-1] == "" {
		cleaned = cleaned[:len(cleaned)-1]
	}

	return strings.Join(cleaned, "\n")
}

// parseJSDoc parses a JSDoc comment and extracts tags.
func parseJSDoc(comment string) (string, map[string]string) {
	comment = strings.TrimSpace(comment)

	// Remove /** and */
	comment = strings.TrimPrefix(comment, "/**")
	comment = strings.TrimSuffix(comment, "*/")

	lines := strings.Split(comment, "\n")
	var descLines []string
	tags := make(map[string]string)

	// Regex for JSDoc tags
	tagPattern := regexp.MustCompile(`^\s*\*?\s*@(\w+)\s*(.*)$`)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		line = strings.TrimPrefix(line, "*")
		line = strings.TrimSpace(line)

		if matches := tagPattern.FindStringSubmatch(line); matches != nil {
			tagName := matches[1]
			tagValue := strings.TrimSpace(matches[2])
			if existing, ok := tags[tagName]; ok {
				tags[tagName] = existing + "\n" + tagValue
			} else {
				tags[tagName] = tagValue
			}
		} else if line != "" {
			descLines = append(descLines, line)
		}
	}

	return strings.Join(descLines, "\n"), tags
}

// parseDoxygen parses a Doxygen comment and extracts tags.
func parseDoxygen(comment string) (string, map[string]string) {
	comment = strings.TrimSpace(comment)

	// Handle /// and //! style comments
	comment = strings.TrimPrefix(comment, "///")
	comment = strings.TrimPrefix(comment, "//!")

	// Handle /** */ style comments
	comment = strings.TrimPrefix(comment, "/**")
	comment = strings.TrimSuffix(comment, "*/")

	lines := strings.Split(comment, "\n")
	var descLines []string
	tags := make(map[string]string)

	// Regex for Doxygen tags (@param, @return, \param, \return, etc.)
	tagPattern := regexp.MustCompile(`^\s*\*?\s*[@\\](\w+)\s*(.*)$`)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		line = strings.TrimPrefix(line, "*")
		line = strings.TrimSpace(line)

		if matches := tagPattern.FindStringSubmatch(line); matches != nil {
			tagName := matches[1]
			tagValue := strings.TrimSpace(matches[2])
			if existing, ok := tags[tagName]; ok {
				tags[tagName] = existing + "\n" + tagValue
			} else {
				tags[tagName] = tagValue
			}
		} else if line != "" {
			descLines = append(descLines, line)
		}
	}

	return strings.Join(descLines, "\n"), tags
}

// FormatDocComment formats a DocComment for display.
func FormatDocComment(doc *DocComment) string {
	if doc == nil || doc.Text == "" {
		return ""
	}

	var sb strings.Builder
	sb.WriteString(doc.Text)

	if len(doc.Tags) > 0 {
		sb.WriteString("\n\n")
		// Order: description, params, returns, other
		paramOrder := []string{"param", "parameter", "arg", "argument"}
		returnOrder := []string{"return", "returns", "retval"}

		// Write params first
		for _, tagName := range paramOrder {
			if val, ok := doc.Tags[tagName]; ok {
				for _, line := range strings.Split(val, "\n") {
					sb.WriteString("@" + tagName + " " + line + "\n")
				}
			}
		}

		// Write returns
		for _, tagName := range returnOrder {
			if val, ok := doc.Tags[tagName]; ok {
				sb.WriteString("@" + tagName + " " + val + "\n")
			}
		}

		// Write remaining tags
		written := make(map[string]bool)
		for _, t := range paramOrder {
			written[t] = true
		}
		for _, t := range returnOrder {
			written[t] = true
		}

		remaining := make([]string, 0, len(doc.Tags))
		for tagName := range doc.Tags {
			if !written[tagName] {
				remaining = append(remaining, tagName)
			}
		}
		sort.Strings(remaining) // deterministic output (map order is random)
		for _, tagName := range remaining {
			sb.WriteString("@" + tagName + " " + doc.Tags[tagName] + "\n")
		}
	}

	return strings.TrimSpace(sb.String())
}

// cleanPythonDocstring cleans a Python docstring.
func cleanPythonDocstring(doc string) string {
	doc = strings.TrimSpace(doc)

	// Remove triple quotes
	doc = strings.TrimPrefix(doc, `"""`)
	doc = strings.TrimSuffix(doc, `"""`)
	doc = strings.TrimPrefix(doc, `'''`)
	doc = strings.TrimSuffix(doc, `'''`)

	return strings.TrimSpace(doc)
}

// extractRustDocComment extracts Rust documentation comments (/// style).
func extractRustDocComment(n *sitter.Node, content []byte) *DocComment {
	comments := collectPrecedingComments(n, content, []string{"line_comment"})
	if len(comments) == 0 {
		return nil
	}

	// Filter for /// doc comments only
	var docComments []*sitter.Node
	for _, c := range comments {
		text := GetNodeText(c, content)
		trimmed := strings.TrimSpace(text)
		if strings.HasPrefix(trimmed, "///") {
			docComments = append(docComments, c)
		}
	}

	if len(docComments) == 0 {
		return nil
	}

	var parts []string
	var raw []string
	startLine := -1
	endLine := -1

	for _, c := range docComments {
		text := GetNodeText(c, content)
		raw = append(raw, text)

		if startLine == -1 {
			startLine = int(c.StartPoint().Row) + 1
		}
		endLine = int(c.EndPoint().Row) + 1

		// Clean /// prefix
		cleaned := strings.TrimSpace(text)
		cleaned = strings.TrimPrefix(cleaned, "///")
		if len(cleaned) > 0 && cleaned[0] == ' ' {
			cleaned = cleaned[1:]
		}
		parts = append(parts, cleaned)
	}

	if len(parts) == 0 {
		return nil
	}

	return &DocComment{
		Text:      strings.Join(parts, "\n"),
		Raw:       strings.Join(raw, "\n"),
		Style:     CommentStyleDoxygen,
		Tags:      nil,
		StartLine: startLine,
		EndLine:   endLine,
	}
}

// extractElixirDocComment extracts Elixir documentation from @doc and @moduledoc attributes.
// Elixir uses module attributes like @doc and @moduledoc for documentation.
func extractElixirDocComment(n *sitter.Node, content []byte) *DocComment {
	// Look for @doc or @moduledoc attribute preceding this node
	prev := n.PrevSibling()

	for prev != nil {
		// Check if this is an unary_operator with @ (module attribute)
		if prev.Type() == "unary_operator" {
			text := GetNodeText(prev, content)
			trimmed := strings.TrimSpace(text)

			// Check for @doc or @moduledoc
			if strings.HasPrefix(trimmed, "@doc") || strings.HasPrefix(trimmed, "@moduledoc") {
				// Extract the documentation string
				docText := extractElixirDocString(prev, content)
				if docText != "" {
					return &DocComment{
						Text:      docText,
						Raw:       text,
						Style:     CommentStyleDocstring,
						Tags:      nil,
						StartLine: int(prev.StartPoint().Row) + 1,
						EndLine:   int(prev.EndPoint().Row) + 1,
					}
				}
			}
		}

		// Also check for regular # comments
		if prev.Type() == "comment" {
			comments := collectPrecedingComments(n, content, []string{"comment"})
			if len(comments) > 0 {
				var parts []string
				var raw []string
				startLine := -1
				endLine := -1

				for _, c := range comments {
					text := GetNodeText(c, content)
					raw = append(raw, text)

					if startLine == -1 {
						startLine = int(c.StartPoint().Row) + 1
					}
					endLine = int(c.EndPoint().Row) + 1

					// Clean # comment
					cleaned := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(text), "#"))
					if cleaned != "" {
						parts = append(parts, cleaned)
					}
				}

				if len(parts) > 0 {
					return &DocComment{
						Text:      strings.Join(parts, "\n"),
						Raw:       strings.Join(raw, "\n"),
						Style:     CommentStyleHash,
						Tags:      nil,
						StartLine: startLine,
						EndLine:   endLine,
					}
				}
			}
			break
		}

		prev = prev.PrevSibling()
	}

	return nil
}

// extractElixirDocString extracts the documentation string from an Elixir @doc/@moduledoc attribute.
func extractElixirDocString(n *sitter.Node, content []byte) string {
	// The doc attribute typically looks like:
	// @doc """
	// Documentation here
	// """
	// or
	// @doc "Single line doc"

	text := GetNodeText(n, content)

	// Find the string content after @doc or @moduledoc
	var docContent string

	// Check for heredoc style (triple quotes)
	if idx := strings.Index(text, `"""`); idx != -1 {
		// Find the closing triple quotes
		rest := text[idx+3:]
		if endIdx := strings.Index(rest, `"""`); endIdx != -1 {
			docContent = rest[:endIdx]
		}
	} else if idx := strings.Index(text, `"`); idx != -1 {
		// Single quoted string
		rest := text[idx+1:]
		if endIdx := strings.Index(rest, `"`); endIdx != -1 {
			docContent = rest[:endIdx]
		}
	}

	return strings.TrimSpace(docContent)
}
