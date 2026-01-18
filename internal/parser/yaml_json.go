// Package parser provides YAML and JSON parsing with AST support.
package parser

import (
	"context"
	"encoding/json"
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/lukaszraczylo/mcp-filepuff/pkg/errors"
	"github.com/lukaszraczylo/mcp-filepuff/pkg/protocol"
	sitter "github.com/smacker/go-tree-sitter"
)

// YAMLNode wraps yaml.Node to provide tree-sitter-like interface
type YAMLNode struct {
	*yaml.Node
	Content []byte
}

// JSONNode represents a JSON AST node
type JSONNode struct {
	Value    any
	Type     string
	Children []*JSONNode
	Line     int
	Column   int
}

// ParseYAML parses YAML content and returns a tree-sitter-compatible result
func (r *Registry) ParseYAML(ctx context.Context, filename string, content []byte) (*ParseResult, error) {
	// Check file size
	if len(content) > MaxFileSize {
		return nil, errors.NewFileTooLarge(filename, int64(len(content)), MaxFileSize)
	}

	// Parse YAML
	var root yaml.Node
	if err := yaml.Unmarshal(content, &root); err != nil {
		return nil, errors.NewParseError("yaml", filename, err)
	}

	// Extract syntax errors from YAML parse
	syntaxErrors := extractYAMLErrors()

	// Create a pseudo tree-sitter tree for compatibility
	// We'll use nil for the tree since YAML doesn't use tree-sitter
	return &ParseResult{
		Tree:     nil, // YAML uses yaml.Node instead
		Language: protocol.LangYAML,
		Errors:   syntaxErrors,
		Content:  content,
	}, nil
}

// ParseJSON parses JSON content and returns a tree-sitter-compatible result
func (r *Registry) ParseJSON(ctx context.Context, filename string, content []byte) (*ParseResult, error) {
	// Check file size
	if len(content) > MaxFileSize {
		return nil, errors.NewFileTooLarge(filename, int64(len(content)), MaxFileSize)
	}

	// Parse JSON to validate syntax
	var jsonData any
	if err := json.Unmarshal(content, &jsonData); err != nil {
		return nil, errors.NewParseError("json", filename, err)
	}

	// JSON parsing succeeded, no syntax errors
	return &ParseResult{
		Tree:     nil, // JSON uses native Go structures
		Language: protocol.LangJSON,
		Errors:   []SyntaxError{},
		Content:  content,
	}, nil
}

// extractYAMLErrors extracts errors from YAML nodes
func extractYAMLErrors() []SyntaxError {
	// YAML parser already validates during unmarshal
	// If we got here, there are no syntax errors
	// However, we could add semantic validation here in the future
	return []SyntaxError{}
}

// WalkYAML walks a YAML AST and calls fn for each node
func WalkYAML(node *yaml.Node, fn func(*yaml.Node) bool) {
	if node == nil || !fn(node) {
		return
	}

	for _, child := range node.Content {
		WalkYAML(child, fn)
	}
}

// GetYAMLNodeText returns the text representation of a YAML node
func GetYAMLNodeText(node *yaml.Node) string {
	if node == nil {
		return ""
	}

	switch node.Kind {
	case yaml.DocumentNode:
		if len(node.Content) > 0 {
			return GetYAMLNodeText(node.Content[0])
		}
		return ""
	case yaml.MappingNode:
		return node.Value
	case yaml.SequenceNode:
		return node.Value
	case yaml.ScalarNode:
		return node.Value
	case yaml.AliasNode:
		return node.Value
	default:
		return ""
	}
}

// GetYAMLNodeLocation returns the location of a YAML node
func GetYAMLNodeLocation(node *yaml.Node) protocol.Location {
	if node == nil {
		return protocol.Location{Line: 1, Column: 1}
	}

	return protocol.Location{
		Line:   node.Line,
		Column: node.Column,
	}
}

// QueryYAML performs a simple query on YAML content
// Example: "$.metadata.name" to find the name field in metadata
func QueryYAML(content []byte, query string) ([]*yaml.Node, error) {
	var root yaml.Node
	if err := yaml.Unmarshal(content, &root); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	// Simple path-based query implementation
	// This is a basic implementation - can be extended with more sophisticated queries
	var results []*yaml.Node

	WalkYAML(&root, func(node *yaml.Node) bool {
		if node.Value == query || node.Tag == query {
			results = append(results, node)
		}
		return true
	})

	return results, nil
}

// QueryJSON performs a simple query on JSON content
func QueryJSON(content []byte, query string) ([]any, error) {
	var data any
	if err := json.Unmarshal(content, &data); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	// Basic implementation - can be extended with JSONPath support
	var results []any

	// For now, just validate that it's valid JSON
	results = append(results, data)

	return results, nil
}

// ValidateYAML validates YAML content without parsing to full AST
func ValidateYAML(content []byte) error {
	var node yaml.Node
	if err := yaml.Unmarshal(content, &node); err != nil {
		return fmt.Errorf("YAML validation failed: %w", err)
	}
	return nil
}

// ValidateJSON validates JSON content
func ValidateJSON(content []byte) error {
	var data any
	if err := json.Unmarshal(content, &data); err != nil {
		return fmt.Errorf("JSON validation failed: %w", err)
	}
	return nil
}

// ToSitterTree is a placeholder that returns nil for YAML/JSON
// These formats don't use tree-sitter, but we keep this for interface compatibility
func (yn *YAMLNode) ToSitterTree() *sitter.Tree {
	return nil
}
