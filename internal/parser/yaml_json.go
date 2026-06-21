// Package parser provides YAML and JSON parsing with AST support.
package parser

import (
	"context"
	"encoding/json"

	"gopkg.in/yaml.v3"

	"github.com/lukaszraczylo/mcp-filepuff/pkg/errors"
	"github.com/lukaszraczylo/mcp-filepuff/pkg/protocol"
)

// ParseYAML parses YAML content and returns a tree-sitter-compatible result.
// YAML does not use tree-sitter, so the returned ParseResult.Tree is nil.
func (r *Registry) ParseYAML(ctx context.Context, filename string, content []byte) (*ParseResult, error) {
	if len(content) > maxFileSize {
		return nil, errors.NewFileTooLarge(filename, int64(len(content)), maxFileSize)
	}

	var root yaml.Node
	if err := yaml.Unmarshal(content, &root); err != nil {
		return nil, errors.NewParseError("yaml", filename, err)
	}

	return &ParseResult{
		Tree:     nil, // YAML uses yaml.Node instead of tree-sitter
		Language: protocol.LangYAML,
		Errors:   extractYAMLErrors(),
		Content:  content,
	}, nil
}

// ParseJSON parses JSON content and returns a tree-sitter-compatible result.
// JSON does not use tree-sitter, so the returned ParseResult.Tree is nil.
func (r *Registry) ParseJSON(ctx context.Context, filename string, content []byte) (*ParseResult, error) {
	if len(content) > maxFileSize {
		return nil, errors.NewFileTooLarge(filename, int64(len(content)), maxFileSize)
	}

	var jsonData any
	if err := json.Unmarshal(content, &jsonData); err != nil {
		return nil, errors.NewParseError("json", filename, err)
	}

	return &ParseResult{
		Tree:     nil, // JSON uses native Go structures
		Language: protocol.LangJSON,
		Errors:   []SyntaxError{},
		Content:  content,
	}, nil
}

// extractYAMLErrors returns YAML syntax errors. yaml.Unmarshal already reports
// parse errors, so reaching here means the document was syntactically valid.
func extractYAMLErrors() []SyntaxError {
	return []SyntaxError{}
}
