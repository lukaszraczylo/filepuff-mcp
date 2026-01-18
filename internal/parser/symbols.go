package parser

import (
	"github.com/lukaszraczylo/mcp-filepuff/pkg/protocol"
	sitter "github.com/smacker/go-tree-sitter"
)

// ExtractSymbols extracts symbols from a parsed tree.
func ExtractSymbols(tree *sitter.Tree, content []byte, lang protocol.Language, filename string) []protocol.Symbol {
	if tree == nil {
		return nil
	}

	root := tree.RootNode()
	if root == nil {
		return nil
	}

	switch lang {
	case protocol.LangGo:
		return extractGoSymbols(root, content, filename)
	case protocol.LangTypeScript, protocol.LangJavaScript:
		return extractJSSymbols(root, content, filename)
	case protocol.LangPython:
		return extractPythonSymbols(root, content, filename)
	case protocol.LangC, protocol.LangCpp:
		return extractCSymbols(root, content, filename)
	default:
		return nil
	}
}

// extractGoSymbols extracts symbols from Go code.
func extractGoSymbols(root *sitter.Node, content []byte, filename string) []protocol.Symbol {
	var symbols []protocol.Symbol

	WalkTree(root, func(n *sitter.Node) bool {
		var symbol *protocol.Symbol

		switch n.Type() {
		case "function_declaration":
			symbol = extractGoFunction(n, content, filename)
		case "method_declaration":
			symbol = extractGoMethod(n, content, filename)
		case "type_declaration":
			symbol = extractGoType(n, content, filename)
		case "const_declaration", "var_declaration":
			syms := extractGoVarConst(n, content, filename)
			symbols = append(symbols, syms...)
			return true
		}

		if symbol != nil {
			if doc := ExtractDocComment(n, content, protocol.LangGo); doc != nil {
				symbol.Doc = FormatDocComment(doc)
			}
			symbols = append(symbols, *symbol)
		}

		return true
	})

	return symbols
}

func extractGoFunction(n *sitter.Node, content []byte, filename string) *protocol.Symbol {
	nameNode := n.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}

	return &protocol.Symbol{
		Name:     GetNodeText(nameNode, content),
		Kind:     protocol.SymbolFunction,
		Location: NodeLocation(n, filename),
	}
}

func extractGoMethod(n *sitter.Node, content []byte, filename string) *protocol.Symbol {
	nameNode := n.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}

	// Get receiver type
	receiver := n.ChildByFieldName("receiver")
	receiverType := ""
	if receiver != nil {
		// Find the type in the receiver
		WalkTree(receiver, func(node *sitter.Node) bool {
			if node.Type() == "type_identifier" {
				receiverType = GetNodeText(node, content)
				return false
			}
			return true
		})
	}

	name := GetNodeText(nameNode, content)
	if receiverType != "" {
		name = "(" + receiverType + ")." + name
	}

	return &protocol.Symbol{
		Name:     name,
		Kind:     protocol.SymbolMethod,
		Location: NodeLocation(n, filename),
	}
}

func extractGoType(n *sitter.Node, content []byte, filename string) *protocol.Symbol {
	// Find type_spec child
	for i := 0; i < int(n.NamedChildCount()); i++ {
		child := n.NamedChild(i)
		if child != nil && child.Type() == "type_spec" {
			nameNode := child.ChildByFieldName("name")
			if nameNode == nil {
				continue
			}

			kind := protocol.SymbolType
			typeNode := child.ChildByFieldName("type")
			if typeNode != nil {
				switch typeNode.Type() {
				case "struct_type":
					kind = protocol.SymbolStruct
				case "interface_type":
					kind = protocol.SymbolInterface
				}
			}

			return &protocol.Symbol{
				Name:     GetNodeText(nameNode, content),
				Kind:     kind,
				Location: NodeLocation(child, filename),
			}
		}
	}
	return nil
}

func extractGoVarConst(n *sitter.Node, content []byte, filename string) []protocol.Symbol {
	var symbols []protocol.Symbol
	kind := protocol.SymbolVariable
	if n.Type() == "const_declaration" {
		kind = protocol.SymbolConstant
	}

	WalkTree(n, func(node *sitter.Node) bool {
		if node.Type() == "const_spec" || node.Type() == "var_spec" {
			nameNode := node.ChildByFieldName("name")
			if nameNode != nil {
				symbols = append(symbols, protocol.Symbol{
					Name:     GetNodeText(nameNode, content),
					Kind:     kind,
					Location: NodeLocation(node, filename),
				})
			}
		}
		return true
	})

	return symbols
}

// extractJSSymbols extracts symbols from JavaScript/TypeScript code.
func extractJSSymbols(root *sitter.Node, content []byte, filename string) []protocol.Symbol {
	var symbols []protocol.Symbol

	WalkTree(root, func(n *sitter.Node) bool {
		var symbol *protocol.Symbol

		switch n.Type() {
		case "function_declaration":
			symbol = extractJSFunction(n, content, filename)
		case "class_declaration":
			symbol = extractJSClass(n, content, filename)
		case "method_definition":
			symbol = extractJSMethod(n, content, filename)
		case "lexical_declaration", "variable_declaration":
			syms := extractJSVariable(n, content, filename)
			symbols = append(symbols, syms...)
			return true
		case "interface_declaration":
			symbol = extractTSInterface(n, content, filename)
		case "type_alias_declaration":
			symbol = extractTSTypeAlias(n, content, filename)
		}

		if symbol != nil {
			if doc := ExtractDocComment(n, content, protocol.LangJavaScript); doc != nil {
				symbol.Doc = FormatDocComment(doc)
			}
			symbols = append(symbols, *symbol)
		}

		return true
	})

	return symbols
}

func extractJSFunction(n *sitter.Node, content []byte, filename string) *protocol.Symbol {
	nameNode := n.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}

	return &protocol.Symbol{
		Name:     GetNodeText(nameNode, content),
		Kind:     protocol.SymbolFunction,
		Location: NodeLocation(n, filename),
	}
}

func extractJSClass(n *sitter.Node, content []byte, filename string) *protocol.Symbol {
	nameNode := n.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}

	return &protocol.Symbol{
		Name:     GetNodeText(nameNode, content),
		Kind:     protocol.SymbolClass,
		Location: NodeLocation(n, filename),
	}
}

func extractJSMethod(n *sitter.Node, content []byte, filename string) *protocol.Symbol {
	nameNode := n.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}

	return &protocol.Symbol{
		Name:     GetNodeText(nameNode, content),
		Kind:     protocol.SymbolMethod,
		Location: NodeLocation(n, filename),
	}
}

func extractJSVariable(n *sitter.Node, content []byte, filename string) []protocol.Symbol {
	var symbols []protocol.Symbol

	WalkTree(n, func(node *sitter.Node) bool {
		if node.Type() == "variable_declarator" {
			nameNode := node.ChildByFieldName("name")
			if nameNode != nil {
				symbols = append(symbols, protocol.Symbol{
					Name:     GetNodeText(nameNode, content),
					Kind:     protocol.SymbolVariable,
					Location: NodeLocation(node, filename),
				})
			}
		}
		return true
	})

	return symbols
}

func extractTSInterface(n *sitter.Node, content []byte, filename string) *protocol.Symbol {
	nameNode := n.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}

	return &protocol.Symbol{
		Name:     GetNodeText(nameNode, content),
		Kind:     protocol.SymbolInterface,
		Location: NodeLocation(n, filename),
	}
}

func extractTSTypeAlias(n *sitter.Node, content []byte, filename string) *protocol.Symbol {
	nameNode := n.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}

	return &protocol.Symbol{
		Name:     GetNodeText(nameNode, content),
		Kind:     protocol.SymbolType,
		Location: NodeLocation(n, filename),
	}
}

// extractPythonSymbols extracts symbols from Python code.
func extractPythonSymbols(root *sitter.Node, content []byte, filename string) []protocol.Symbol {
	var symbols []protocol.Symbol

	WalkTree(root, func(n *sitter.Node) bool {
		var symbol *protocol.Symbol

		switch n.Type() {
		case "function_definition":
			symbol = extractPythonFunction(n, content, filename)
		case "class_definition":
			symbol = extractPythonClass(n, content, filename)
		}

		if symbol != nil {
			if doc := ExtractDocComment(n, content, protocol.LangPython); doc != nil {
				symbol.Doc = FormatDocComment(doc)
			}
			symbols = append(symbols, *symbol)
		}

		return true
	})

	return symbols
}

func extractPythonFunction(n *sitter.Node, content []byte, filename string) *protocol.Symbol {
	nameNode := n.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}

	// Check if this is a method (inside a class)
	parent := n.Parent()
	kind := protocol.SymbolFunction
	if parent != nil && parent.Type() == "block" {
		grandparent := parent.Parent()
		if grandparent != nil && grandparent.Type() == "class_definition" {
			kind = protocol.SymbolMethod
		}
	}

	return &protocol.Symbol{
		Name:     GetNodeText(nameNode, content),
		Kind:     kind,
		Location: NodeLocation(n, filename),
	}
}

func extractPythonClass(n *sitter.Node, content []byte, filename string) *protocol.Symbol {
	nameNode := n.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}

	return &protocol.Symbol{
		Name:     GetNodeText(nameNode, content),
		Kind:     protocol.SymbolClass,
		Location: NodeLocation(n, filename),
	}
}

// extractCSymbols extracts symbols from C/C++ code.
func extractCSymbols(root *sitter.Node, content []byte, filename string) []protocol.Symbol {
	var symbols []protocol.Symbol

	WalkTree(root, func(n *sitter.Node) bool {
		var symbol *protocol.Symbol

		switch n.Type() {
		case "function_definition":
			symbol = extractCFunction(n, content, filename)
		case "struct_specifier":
			symbol = extractCStruct(n, content, filename)
		case "class_specifier":
			symbol = extractCppClass(n, content, filename)
		case "declaration":
			// Could be function declaration or variable
			if hasFunctionDeclarator(n) {
				symbol = extractCFunctionDecl(n, content, filename)
			}
		}

		if symbol != nil {
			if doc := ExtractDocComment(n, content, protocol.LangC); doc != nil {
				symbol.Doc = FormatDocComment(doc)
			}
			symbols = append(symbols, *symbol)
		}

		return true
	})

	return symbols
}

func extractCFunction(n *sitter.Node, content []byte, filename string) *protocol.Symbol {
	declarator := n.ChildByFieldName("declarator")
	if declarator == nil {
		return nil
	}

	// Find the function name within the declarator
	var name string
	WalkTree(declarator, func(node *sitter.Node) bool {
		if node.Type() == "identifier" {
			name = GetNodeText(node, content)
			return false
		}
		return true
	})

	if name == "" {
		return nil
	}

	return &protocol.Symbol{
		Name:     name,
		Kind:     protocol.SymbolFunction,
		Location: NodeLocation(n, filename),
	}
}

func extractCStruct(n *sitter.Node, content []byte, filename string) *protocol.Symbol {
	nameNode := n.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}

	return &protocol.Symbol{
		Name:     GetNodeText(nameNode, content),
		Kind:     protocol.SymbolStruct,
		Location: NodeLocation(n, filename),
	}
}

func extractCppClass(n *sitter.Node, content []byte, filename string) *protocol.Symbol {
	nameNode := n.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}

	return &protocol.Symbol{
		Name:     GetNodeText(nameNode, content),
		Kind:     protocol.SymbolClass,
		Location: NodeLocation(n, filename),
	}
}

func extractCFunctionDecl(n *sitter.Node, content []byte, filename string) *protocol.Symbol {
	declarator := n.ChildByFieldName("declarator")
	if declarator == nil {
		return nil
	}

	var name string
	WalkTree(declarator, func(node *sitter.Node) bool {
		if node.Type() == "identifier" {
			name = GetNodeText(node, content)
			return false
		}
		return true
	})

	if name == "" {
		return nil
	}

	return &protocol.Symbol{
		Name:     name,
		Kind:     protocol.SymbolFunction,
		Location: NodeLocation(n, filename),
	}
}

func hasFunctionDeclarator(n *sitter.Node) bool {
	found := false
	WalkTree(n, func(node *sitter.Node) bool {
		if node.Type() == "function_declarator" {
			found = true
			return false
		}
		return true
	})
	return found
}
