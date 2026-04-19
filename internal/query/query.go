// Package query implements a hybrid AST query language with pattern matching.
package query

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/lukaszraczylo/mcp-filepuff/internal/parser"
	"github.com/lukaszraczylo/mcp-filepuff/internal/util"
	"github.com/lukaszraczylo/mcp-filepuff/pkg/protocol"
	sitter "github.com/smacker/go-tree-sitter"
)

// ASTQuery defines a query for matching AST patterns.
type ASTQuery struct {
	Pattern  string       `json:"pattern"`  // code pattern with $VAR placeholders
	Language string       `json:"language"` // required
	Filters  QueryFilters `json:"filters,omitempty"`
}

// QueryFilters provide additional filtering criteria.
type QueryFilters struct {
	NameMatches string   `json:"name_matches,omitempty"`
	NameExact   string   `json:"name_exact,omitempty"`
	InFile      string   `json:"in_file,omitempty"`
	NotInFile   string   `json:"not_in_file,omitempty"`
	KindIn      []string `json:"kind_in,omitempty"`
}

// MatchResult represents a single match from a query.
type MatchResult struct {
	Node     *sitter.Node
	Captures map[string]CapturedNode
	File     string
	Text     string
	Location protocol.Location
}

// CapturedNode represents a captured node or nodes.
type CapturedNode struct {
	Text  string
	Nodes []*sitter.Node
}

// CaptureType indicates the type of capture.
type CaptureType int

const (
	CaptureSingle   CaptureType = iota // $NAME - single node
	CaptureMultiple                    // $$$NAME - multiple nodes
	CaptureWildcard                    // $_ - wildcard (don't capture)
)

// Capture represents a placeholder in a pattern.
type Capture struct {
	Name     string
	Type     CaptureType
	Position int // position in the pattern
}

// ParsedPattern represents a parsed code pattern.
type ParsedPattern struct {
	Original string
	Template string
	Captures []Capture
}

// Matcher performs AST pattern matching.
type Matcher struct {
	registry *parser.Registry
}

// NewMatcher creates a new pattern matcher.
func NewMatcher(registry *parser.Registry) *Matcher {
	return &Matcher{registry: registry}
}

// ParsePattern parses a pattern string and extracts captures.
func ParsePattern(pattern string) (*ParsedPattern, error) {
	if pattern == "" {
		return nil, fmt.Errorf("empty pattern")
	}

	var captures []Capture
	template := pattern
	captureID := 0

	// Find all captures: $$$ (multi), $_ (wildcard), $NAME (single)
	// Order matters: check $$$ first
	multiRe := regexp.MustCompile(`\$\$\$([A-Za-z_][A-Za-z0-9_]*)`)
	wildcardRe := regexp.MustCompile(`\$_`)
	singleRe := regexp.MustCompile(`\$([A-Za-z_][A-Za-z0-9_]*)`)

	// Extract multi-node captures ($$$NAME)
	for _, match := range multiRe.FindAllStringSubmatchIndex(pattern, -1) {
		name := pattern[match[2]:match[3]]
		captures = append(captures, Capture{
			Name:     name,
			Type:     CaptureMultiple,
			Position: match[0],
		})
	}

	// Replace multi-captures with placeholder identifiers
	template = multiRe.ReplaceAllStringFunc(template, func(s string) string {
		captureID++
		return fmt.Sprintf("__multi_%d__", captureID)
	})

	// Extract wildcards ($_)
	for _, match := range wildcardRe.FindAllStringIndex(pattern, -1) {
		captures = append(captures, Capture{
			Name:     "_",
			Type:     CaptureWildcard,
			Position: match[0],
		})
	}

	// Replace wildcards with placeholder identifiers
	template = wildcardRe.ReplaceAllStringFunc(template, func(s string) string {
		captureID++
		return fmt.Sprintf("__wild_%d__", captureID)
	})

	// Extract single-node captures ($NAME) - exclude those that are part of $$$NAME
	// Check which $NAME patterns are not preceded by $$
	remaining := template
	for _, match := range singleRe.FindAllStringSubmatchIndex(remaining, -1) {
		name := remaining[match[2]:match[3]]
		// Skip if this looks like our placeholder
		if strings.HasPrefix(name, "_multi_") || strings.HasPrefix(name, "_wild_") {
			continue
		}
		captures = append(captures, Capture{
			Name:     name,
			Type:     CaptureSingle,
			Position: match[0],
		})
	}

	// Replace single captures with placeholder identifiers
	template = singleRe.ReplaceAllStringFunc(template, func(s string) string {
		name := strings.TrimPrefix(s, "$")
		if strings.HasPrefix(name, "_multi_") || strings.HasPrefix(name, "_wild_") {
			return s // keep our placeholders as is
		}
		captureID++
		return fmt.Sprintf("__single_%d__", captureID)
	})

	return &ParsedPattern{
		Original: pattern,
		Captures: captures,
		Template: template,
	}, nil
}

// Match executes a query against a parsed tree.
func (m *Matcher) Match(ctx context.Context, query *ASTQuery, tree *sitter.Tree, content []byte, filename string) ([]MatchResult, error) {
	if query.Pattern == "" {
		return nil, fmt.Errorf("query pattern is required")
	}

	lang := protocol.Language(query.Language)
	if lang == "" || lang == protocol.LangUnknown {
		return nil, fmt.Errorf("valid language is required")
	}

	// Parse the pattern
	parsed, err := ParsePattern(query.Pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid pattern: %w", err)
	}

	var results []MatchResult

	// Walk the tree and find matches
	root := tree.RootNode()
	if root == nil {
		return results, nil
	}

	parser.WalkTree(root, func(n *sitter.Node) bool {
		// Check for context cancellation
		select {
		case <-ctx.Done():
			return false
		default:
		}

		// Try to match this node against the pattern
		if matched, captures := matchNode(n, parsed, content); matched {
			// Apply filters
			if !passesFilters(n, query.Filters, content) {
				return true // continue walking
			}

			startPoint := n.StartPoint()
			results = append(results, MatchResult{
				Node:     n,
				Captures: captures,
				File:     filename,
				Location: protocol.Location{
					Line:   int(startPoint.Row) + 1,
					Column: int(startPoint.Column) + 1,
				},
				Text: parser.GetNodeText(n, content),
			})
		}

		return true
	})

	return results, nil
}

// matchNode attempts to match a node against a parsed pattern.
// This is a simplified matcher that looks for structural similarity.
func matchNode(node *sitter.Node, pattern *ParsedPattern, content []byte) (bool, map[string]CapturedNode) {
	if node == nil {
		return false, nil
	}

	captures := make(map[string]CapturedNode)

	// Use pattern keyword matching as a heuristic to find matching nodes
	// A full implementation would parse both pattern and node and compare AST structure
	matched := matchPatternHeuristic(node, pattern, content, captures)

	return matched, captures
}

// matchPatternHeuristic uses heuristics to match patterns.
// This is a simplified implementation that matches based on node type and structure.
func matchPatternHeuristic(node *sitter.Node, pattern *ParsedPattern, content []byte, captures map[string]CapturedNode) bool {
	patternLower := strings.ToLower(pattern.Original)
	nodeType := node.Type()

	// Match function patterns
	if strings.Contains(patternLower, "func ") || strings.Contains(patternLower, "function ") {
		if nodeType != "function_declaration" && nodeType != "method_declaration" && nodeType != "function_definition" {
			return false
		}
		extractFunctionCaptures(node, pattern.Captures, content, captures)
		return true
	}

	// Match class patterns
	if strings.Contains(patternLower, "class ") {
		if nodeType != "class_declaration" && nodeType != "class_definition" {
			return false
		}
		extractClassCaptures(node, pattern.Captures, content, captures)
		return true
	}

	// Match struct patterns (Go, C, C++)
	if (strings.Contains(patternLower, "struct ") || strings.Contains(patternLower, "type ")) && strings.Contains(patternLower, "struct") {
		if nodeType != "type_declaration" && nodeType != "struct_specifier" {
			return false
		}
		extractStructCaptures(node, pattern.Captures, content, captures)
		return true
	}

	// Match interface patterns (Go, TypeScript)
	if strings.Contains(patternLower, "interface ") {
		if nodeType != "interface_declaration" && nodeType != "type_declaration" {
			return false
		}
		extractInterfaceCaptures(node, pattern.Captures, content, captures)
		return true
	}

	return false
}

// extractFunctionCaptures extracts captures from a function node.
func extractFunctionCaptures(node *sitter.Node, capturesDef []Capture, content []byte, captures map[string]CapturedNode) {
	for _, cap := range capturesDef {
		switch cap.Name {
		case "NAME", "name":
			if nameNode := node.ChildByFieldName("name"); nameNode != nil {
				captures[cap.Name] = CapturedNode{
					Nodes: []*sitter.Node{nameNode},
					Text:  parser.GetNodeText(nameNode, content),
				}
			}
		case "ARGS", "args", "PARAMS", "params":
			if paramsNode := node.ChildByFieldName("parameters"); paramsNode != nil {
				var paramNodes []*sitter.Node
				for i := 0; i < int(paramsNode.NamedChildCount()); i++ {
					paramNodes = append(paramNodes, paramsNode.NamedChild(i))
				}
				captures[cap.Name] = CapturedNode{
					Nodes: paramNodes,
					Text:  parser.GetNodeText(paramsNode, content),
				}
			}
		case "BODY", "body":
			if bodyNode := node.ChildByFieldName("body"); bodyNode != nil {
				captures[cap.Name] = CapturedNode{
					Nodes: []*sitter.Node{bodyNode},
					Text:  parser.GetNodeText(bodyNode, content),
				}
			}
		case "RETURN", "return", "RESULT", "result":
			if resultNode := node.ChildByFieldName("result"); resultNode != nil {
				captures[cap.Name] = CapturedNode{
					Nodes: []*sitter.Node{resultNode},
					Text:  parser.GetNodeText(resultNode, content),
				}
			}
		}
	}
}

// extractClassCaptures extracts captures from a class node.
func extractClassCaptures(node *sitter.Node, capturesDef []Capture, content []byte, captures map[string]CapturedNode) {
	for _, cap := range capturesDef {
		switch cap.Name {
		case "NAME", "name":
			if nameNode := node.ChildByFieldName("name"); nameNode != nil {
				captures[cap.Name] = CapturedNode{
					Nodes: []*sitter.Node{nameNode},
					Text:  parser.GetNodeText(nameNode, content),
				}
			}
		case "BODY", "body":
			if bodyNode := node.ChildByFieldName("body"); bodyNode != nil {
				captures[cap.Name] = CapturedNode{
					Nodes: []*sitter.Node{bodyNode},
					Text:  parser.GetNodeText(bodyNode, content),
				}
			}
		case "EXTENDS", "extends", "SUPERCLASS", "superclass":
			if extendsNode := node.ChildByFieldName("superclass"); extendsNode != nil {
				captures[cap.Name] = CapturedNode{
					Nodes: []*sitter.Node{extendsNode},
					Text:  parser.GetNodeText(extendsNode, content),
				}
			}
		}
	}
}

// extractStructCaptures extracts captures from a struct node.
func extractStructCaptures(node *sitter.Node, capturesDef []Capture, content []byte, captures map[string]CapturedNode) {
	for _, cap := range capturesDef {
		switch cap.Name {
		case "NAME", "name":
			// For Go type_declaration, we need to look at the type_spec child
			if node.Type() == "type_declaration" {
				for i := 0; i < int(node.NamedChildCount()); i++ {
					child := node.NamedChild(i)
					if child != nil && child.Type() == "type_spec" {
						if nameNode := child.ChildByFieldName("name"); nameNode != nil {
							captures[cap.Name] = CapturedNode{
								Nodes: []*sitter.Node{nameNode},
								Text:  parser.GetNodeText(nameNode, content),
							}
						}
					}
				}
			} else if nameNode := node.ChildByFieldName("name"); nameNode != nil {
				captures[cap.Name] = CapturedNode{
					Nodes: []*sitter.Node{nameNode},
					Text:  parser.GetNodeText(nameNode, content),
				}
			}
		case "FIELDS", "fields", "BODY", "body":
			if bodyNode := node.ChildByFieldName("body"); bodyNode != nil {
				captures[cap.Name] = CapturedNode{
					Nodes: []*sitter.Node{bodyNode},
					Text:  parser.GetNodeText(bodyNode, content),
				}
			}
		}
	}
}

// extractInterfaceCaptures extracts captures from an interface node.
func extractInterfaceCaptures(node *sitter.Node, capturesDef []Capture, content []byte, captures map[string]CapturedNode) {
	for _, cap := range capturesDef {
		switch cap.Name {
		case "NAME", "name":
			if nameNode := node.ChildByFieldName("name"); nameNode != nil {
				captures[cap.Name] = CapturedNode{
					Nodes: []*sitter.Node{nameNode},
					Text:  parser.GetNodeText(nameNode, content),
				}
			}
		case "BODY", "body", "METHODS", "methods":
			if bodyNode := node.ChildByFieldName("body"); bodyNode != nil {
				captures[cap.Name] = CapturedNode{
					Nodes: []*sitter.Node{bodyNode},
					Text:  parser.GetNodeText(bodyNode, content),
				}
			}
		}
	}
}

// passesFilters checks if a node passes all the specified filters.
func passesFilters(node *sitter.Node, filters QueryFilters, content []byte) bool {
	// Name regex filter (uses cached compilation)
	if filters.NameMatches != "" {
		nameNode := node.ChildByFieldName("name")
		if nameNode == nil {
			return false
		}
		name := parser.GetNodeText(nameNode, content)
		re, err := util.CompileRegex(filters.NameMatches)
		if err != nil {
			return false
		}
		if !re.MatchString(name) {
			return false
		}
	}

	// Exact name filter
	if filters.NameExact != "" {
		nameNode := node.ChildByFieldName("name")
		if nameNode == nil {
			return false
		}
		name := parser.GetNodeText(nameNode, content)
		if name != filters.NameExact {
			return false
		}
	}

	// Kind filter
	if len(filters.KindIn) > 0 {
		nodeType := node.Type()
		found := false
		for _, kind := range filters.KindIn {
			if nodeType == kind {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	return true
}

// FormatResults formats match results for display (backward-compat wrapper, verbose mode).
func FormatResults(results []MatchResult, maxResults int) string {
	return FormatResultsWithOptions(results, maxResults, "verbose", 0)
}

// FormatResultsWithOptions formats match results with configurable output format.
// format: "verbose" (default) | "compact" | "location"
// offset: skip this many results before rendering (used for cursor pagination).
// verbose: opt-in variadic — pass true to restore "Found N match(es):" preamble (v1 behaviour).
func FormatResultsWithOptions(results []MatchResult, maxResults int, format string, offset int, verbose ...bool) string {
	if len(results) == 0 {
		return "No matches found."
	}

	// Apply offset (pagination skip).
	if offset > 0 {
		if offset >= len(results) {
			return "No matches found."
		}
		results = results[offset:]
	}

	// Determine how many to render and whether more remain.
	renderCount := len(results)
	remaining := 0
	if maxResults > 0 && renderCount > maxResults {
		remaining = renderCount - maxResults
		renderCount = maxResults
	}

	var sb strings.Builder

	// Emit preamble only when verbose=true is explicitly passed (opt-in, default off).
	wantVerbose := len(verbose) > 0 && verbose[0]
	if wantVerbose {
		sb.WriteString(fmt.Sprintf("Found %d match(es):\n", renderCount))
	}

	switch format {
	case "compact":
		for i := 0; i < renderCount; i++ {
			r := results[i]
			nodeType := "unknown"
			if r.Node != nil {
				nodeType = r.Node.Type()
			}
			firstLine := firstLineOf(r.Text, 80)
			sb.WriteString(fmt.Sprintf("%s:%d (%s) %s\n", r.File, r.Location.Line, nodeType, firstLine))
		}

	case "location":
		for i := 0; i < renderCount; i++ {
			r := results[i]
			sb.WriteString(fmt.Sprintf("%s:%d\n", r.File, r.Location.Line))
		}

	default: // "verbose"
		for i := 0; i < renderCount; i++ {
			r := results[i]
			nodeType := "unknown"
			if r.Node != nil {
				nodeType = r.Node.Type()
			}
			sb.WriteString(fmt.Sprintf("**%s:%d** (%s)\n", r.File, r.Location.Line, nodeType))

			// Truncate very long text
			text := r.Text
			if len(text) > 500 {
				text = text[:500] + "..."
			}
			sb.WriteString("```\n")
			sb.WriteString(text)
			sb.WriteString("\n```\n")

			// Show captures
			if len(r.Captures) > 0 {
				sb.WriteString("Captures: ")
				first := true
				for name, cap := range r.Captures {
					if !first {
						sb.WriteString(", ")
					}
					first = false
					capText := cap.Text
					if len(capText) > 50 {
						capText = capText[:50] + "..."
					}
					sb.WriteString(fmt.Sprintf("$%s=%s", name, capText))
				}
				sb.WriteString("\n")
			}
			sb.WriteString("\n")
		}
	}

	if remaining > 0 {
		// Caller must embed the cursor token; we just append the remaining count hint.
		// The actual [cursor: ...] line is written by the handler after calling MakeCursor.
		sb.WriteString(fmt.Sprintf("[remaining: %d]\n", remaining))
	}

	return sb.String()
}

// firstLineOf returns the first non-empty line of s, trimmed and capped at maxLen chars.
func firstLineOf(s string, maxLen int) string {
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if len(line) > maxLen {
			return line[:maxLen-1] + "…"
		}
		return line
	}
	return ""
}
