// Package parser provides skeleton rendering for source files.
package parser

import (
	"context"
	"fmt"
	"strings"

	"github.com/lukaszraczylo/mcp-filepuff/pkg/protocol"
	sitter "github.com/smacker/go-tree-sitter"
)

// SkeletonFile returns a skeleton representation of the file: top-level declarations
// with signatures and doc-comments intact, but function/method bodies replaced with
// a language-appropriate placeholder.
//
// Supported: go, typescript, javascript, python, rust.
// Other languages fall back to symbols_only (AST summary text).
// Returns (skeletonText, isFullSkeleton, error).
func SkeletonFile(ctx context.Context, reg *Registry, filename string, content []byte) (string, bool, error) {
	result, err := reg.Parse(ctx, filename, content)
	if err != nil {
		return "", false, err
	}
	lang := protocol.DetectLanguage(filename)

	switch lang {
	case protocol.LangGo:
		return skeletonGo(result.Tree, content), true, nil
	case protocol.LangTypeScript, protocol.LangJavaScript:
		return skeletonTS(result.Tree, content), true, nil
	case protocol.LangPython:
		return skeletonPython(result.Tree, content), true, nil
	case protocol.LangRust:
		return skeletonRust(result.Tree, content), true, nil
	default:
		// TODO: skeleton for c, cpp, elixir, html, vue — fall back to symbols_only
		syms := ExtractSymbols(result.Tree, content, lang, filename)
		return renderSymbolsOnly(syms, filename, lang, content), false, nil
	}
}

// renderSymbolsOnly renders a simple symbol list (fallback for unsupported languages).
func renderSymbolsOnly(syms []protocol.Symbol, _ string, lang protocol.Language, content []byte) string {
	lines := strings.Split(string(content), "\n")
	var sb strings.Builder
	fmt.Fprintf(&sb, "// skeleton unavailable for %s — symbol list only\n", lang)
	for _, s := range syms {
		fmt.Fprintf(&sb, "// %s %s  (line %d)\n", s.Kind, s.Name, s.Location.Line)
		if s.Doc != "" {
			fmt.Fprintf(&sb, "// doc: %s\n", s.Doc)
		}
		if s.Location.Line >= 1 && s.Location.Line <= len(lines) {
			sb.WriteString(lines[s.Location.Line-1] + " { ... }\n")
		}
	}
	return sb.String()
}

// ---- Go skeleton ----

// skeletonGoBodyNodes lists Go node types that have a body field to replace.
var skeletonGoBodyNodes = map[string]string{
	"function_declaration": "body",
	"method_declaration":   "body",
}

func skeletonGo(tree *sitter.Tree, content []byte) string {
	if tree == nil {
		return string(content)
	}
	root := tree.RootNode()
	var sb strings.Builder
	skeletonGoNode(root, content, &sb)
	return sb.String()
}

func skeletonGoNode(node *sitter.Node, content []byte, sb *strings.Builder) {
	if node == nil {
		return
	}
	nodeType := node.Type()

	if bodyField, ok := skeletonGoBodyNodes[nodeType]; ok {
		body := node.ChildByFieldName(bodyField)
		if body != nil {
			sig := strings.TrimRight(string(content[node.StartByte():body.StartByte()]), " \t")
			sb.WriteString(sig)
			sb.WriteString("{ ... }\n\n")
			return
		}
		sb.WriteString(string(content[node.StartByte():node.EndByte()]))
		sb.WriteString("\n")
		return
	}

	switch nodeType {
	case "source_file":
		for i := 0; i < int(node.ChildCount()); i++ {
			child := node.Child(i)
			if child == nil {
				continue
			}
			skeletonGoNode(child, content, sb)
		}
		return
	case "comment":
		sb.WriteString(string(content[node.StartByte():node.EndByte()]))
		sb.WriteString("\n")
		return
	default:
		sb.WriteString(string(content[node.StartByte():node.EndByte()]))
		sb.WriteString("\n")
	}
}

// ---- TypeScript / JavaScript skeleton ----

func skeletonTS(tree *sitter.Tree, content []byte) string {
	if tree == nil {
		return string(content)
	}
	root := tree.RootNode()
	var sb strings.Builder
	skeletonTSNode(root, content, &sb)
	return sb.String()
}

func skeletonTSNode(node *sitter.Node, content []byte, sb *strings.Builder) {
	if node == nil {
		return
	}
	nodeType := node.Type()

	switch nodeType {
	case "program":
		for i := 0; i < int(node.ChildCount()); i++ {
			child := node.Child(i)
			if child == nil {
				continue
			}
			skeletonTSNode(child, content, sb)
		}
		return
	case "function_declaration":
		body := node.ChildByFieldName("body")
		if body != nil {
			sig := strings.TrimRight(string(content[node.StartByte():body.StartByte()]), " \t")
			sb.WriteString(sig)
			sb.WriteString("{ ... }\n\n")
			return
		}
	case "class_declaration":
		body := node.ChildByFieldName("body")
		if body != nil {
			header := strings.TrimRight(string(content[node.StartByte():body.StartByte()]), " \t")
			sb.WriteString(header)
			sb.WriteString("{\n")
			for i := 0; i < int(body.ChildCount()); i++ {
				child := body.Child(i)
				if child == nil {
					continue
				}
				if child.Type() == "method_definition" {
					methBody := child.ChildByFieldName("body")
					if methBody != nil {
						methSig := strings.TrimRight(string(content[child.StartByte():methBody.StartByte()]), " \t")
						sb.WriteString("  ")
						sb.WriteString(methSig)
						sb.WriteString("{ ... }\n")
						continue
					}
				}
				sb.WriteString("  ")
				sb.WriteString(string(content[child.StartByte():child.EndByte()]))
				sb.WriteString("\n")
			}
			sb.WriteString("}\n\n")
			return
		}
	case "comment":
		sb.WriteString(string(content[node.StartByte():node.EndByte()]))
		sb.WriteString("\n")
		return
	}

	sb.WriteString(string(content[node.StartByte():node.EndByte()]))
	sb.WriteString("\n")
}

// ---- Python skeleton ----

func skeletonPython(tree *sitter.Tree, content []byte) string {
	if tree == nil {
		return string(content)
	}
	root := tree.RootNode()
	var sb strings.Builder
	skeletonPythonNode(root, content, &sb, "")
	return sb.String()
}

func skeletonPythonNode(node *sitter.Node, content []byte, sb *strings.Builder, indent string) {
	if node == nil {
		return
	}
	nodeType := node.Type()

	switch nodeType {
	case "module":
		for i := 0; i < int(node.ChildCount()); i++ {
			child := node.Child(i)
			if child == nil {
				continue
			}
			skeletonPythonNode(child, content, sb, indent)
		}
		return
	case "function_definition", "decorated_definition":
		nodeText := string(content[node.StartByte():node.EndByte()])
		lines := strings.SplitN(nodeText, "\n", 2)
		sb.WriteString(indent)
		sb.WriteString(lines[0])
		sb.WriteString("\n")
		sb.WriteString(indent)
		sb.WriteString("    ...\n\n")
		return
	case "class_definition":
		body := node.ChildByFieldName("body")
		if body != nil {
			header := string(content[node.StartByte():body.StartByte()])
			firstLine := strings.SplitN(header, "\n", 2)[0]
			sb.WriteString(indent)
			sb.WriteString(firstLine)
			sb.WriteString("\n")
			for i := 0; i < int(body.ChildCount()); i++ {
				child := body.Child(i)
				if child == nil {
					continue
				}
				if child.Type() == "function_definition" || child.Type() == "decorated_definition" {
					childText := string(content[child.StartByte():child.EndByte()])
					childLines := strings.SplitN(childText, "\n", 2)
					sb.WriteString(indent + "    ")
					sb.WriteString(childLines[0])
					sb.WriteString("\n")
					sb.WriteString(indent + "        ...")
					sb.WriteString("\n")
					continue
				}
				if child.Type() == "expression_statement" {
					sb.WriteString(indent + "    ")
					sb.WriteString(string(content[child.StartByte():child.EndByte()]))
					sb.WriteString("\n")
					continue
				}
			}
			sb.WriteString("\n")
			return
		}
	case "comment":
		sb.WriteString(indent)
		sb.WriteString(string(content[node.StartByte():node.EndByte()]))
		sb.WriteString("\n")
		return
	}

	sb.WriteString(indent)
	sb.WriteString(string(content[node.StartByte():node.EndByte()]))
	sb.WriteString("\n")
}

// ---- Rust skeleton ----

func skeletonRust(tree *sitter.Tree, content []byte) string {
	if tree == nil {
		return string(content)
	}
	root := tree.RootNode()
	var sb strings.Builder
	skeletonRustNode(root, content, &sb)
	return sb.String()
}

func skeletonRustNode(node *sitter.Node, content []byte, sb *strings.Builder) {
	if node == nil {
		return
	}
	nodeType := node.Type()

	switch nodeType {
	case "source_file":
		for i := 0; i < int(node.ChildCount()); i++ {
			child := node.Child(i)
			if child == nil {
				continue
			}
			skeletonRustNode(child, content, sb)
		}
		return
	case "function_item":
		body := node.ChildByFieldName("body")
		if body != nil {
			sig := strings.TrimRight(string(content[node.StartByte():body.StartByte()]), " \t")
			sb.WriteString(sig)
			sb.WriteString("{ ... }\n\n")
			return
		}
	case "impl_item":
		body := node.ChildByFieldName("body")
		if body != nil {
			header := strings.TrimRight(string(content[node.StartByte():body.StartByte()]), " \t")
			sb.WriteString(header)
			sb.WriteString("{\n")
			for i := 0; i < int(body.ChildCount()); i++ {
				child := body.Child(i)
				if child == nil {
					continue
				}
				if child.Type() == "function_item" {
					methBody := child.ChildByFieldName("body")
					if methBody != nil {
						methSig := strings.TrimRight(string(content[child.StartByte():methBody.StartByte()]), " \t")
						sb.WriteString("    ")
						sb.WriteString(methSig)
						sb.WriteString("{ ... }\n")
						continue
					}
				}
				sb.WriteString("    ")
				sb.WriteString(string(content[child.StartByte():child.EndByte()]))
				sb.WriteString("\n")
			}
			sb.WriteString("}\n\n")
			return
		}
	case "line_comment", "block_comment":
		sb.WriteString(string(content[node.StartByte():node.EndByte()]))
		sb.WriteString("\n")
		return
	}

	sb.WriteString(string(content[node.StartByte():node.EndByte()]))
	sb.WriteString("\n")
}
