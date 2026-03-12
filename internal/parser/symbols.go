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
	case protocol.LangElixir:
		return extractElixirSymbols(root, content, filename)
	case protocol.LangRust:
		return extractRustSymbols(root, content, filename)
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

// extractElixirSymbols extracts symbols from Elixir code.
// Elixir uses `defmodule` for modules, `def`/`defp` for functions, and `defmacro`/`defmacrop` for macros.
func extractElixirSymbols(root *sitter.Node, content []byte, filename string) []protocol.Symbol {
	var symbols []protocol.Symbol

	WalkTree(root, func(n *sitter.Node) bool {
		var symbol *protocol.Symbol

		switch n.Type() {
		case "call":
			symbol = extractElixirCall(n, content, filename)
		}

		if symbol != nil {
			if doc := ExtractDocComment(n, content, protocol.LangElixir); doc != nil {
				symbol.Doc = FormatDocComment(doc)
			}
			symbols = append(symbols, *symbol)
		}

		return true
	})

	return symbols
}

// extractElixirCall extracts symbols from Elixir call nodes (def, defp, defmodule, defmacro, etc.).
func extractElixirCall(n *sitter.Node, content []byte, filename string) *protocol.Symbol {
	// Get the function being called (first child is usually the target)
	if n.NamedChildCount() < 1 {
		return nil
	}

	target := n.NamedChild(0)
	if target == nil {
		return nil
	}

	targetText := GetNodeText(target, content)

	switch targetText {
	case "defmodule":
		return extractElixirModule(n, content, filename)
	case "def", "defp":
		return extractElixirFunction(n, content, filename, targetText == "defp")
	case "defmacro", "defmacrop":
		return extractElixirMacro(n, content, filename)
	case "defstruct":
		return extractElixirStruct(n, content, filename)
	case "defprotocol":
		return extractElixirProtocol(n, content, filename)
	case "defimpl":
		return extractElixirImpl(n, content, filename)
	}

	return nil
}

// extractElixirModule extracts a module definition.
func extractElixirModule(n *sitter.Node, content []byte, filename string) *protocol.Symbol {
	// defmodule ModuleName do ... end
	// The module name is in the arguments
	args := n.ChildByFieldName("arguments")
	if args == nil {
		// Try finding it as the second named child
		if n.NamedChildCount() >= 2 {
			args = n.NamedChild(1)
		}
	}
	if args == nil {
		return nil
	}

	// Find the alias (module name) in the arguments
	var moduleName string
	WalkTree(args, func(node *sitter.Node) bool {
		if node.Type() == "alias" {
			moduleName = GetNodeText(node, content)
			return false
		}
		return true
	})

	if moduleName == "" {
		return nil
	}

	return &protocol.Symbol{
		Name:     moduleName,
		Kind:     protocol.SymbolModule,
		Location: NodeLocation(n, filename),
	}
}

// extractElixirFunction extracts a function definition.
func extractElixirFunction(n *sitter.Node, content []byte, filename string, isPrivate bool) *protocol.Symbol {
	// def function_name(args) do ... end
	// The function name and args are in the arguments of the call
	if n.NamedChildCount() < 2 {
		return nil
	}

	// Second child contains the function definition
	funcDef := n.NamedChild(1)
	if funcDef == nil {
		return nil
	}

	var funcName string

	// The function definition can be:
	// 1. A call node (function with args): func_name(arg1, arg2)
	// 2. An identifier (function without args): func_name
	switch funcDef.Type() {
	case "call":
		// Get the function name from the call target
		if funcDef.NamedChildCount() >= 1 {
			nameNode := funcDef.NamedChild(0)
			if nameNode != nil {
				funcName = GetNodeText(nameNode, content)
			}
		}
	case "identifier":
		funcName = GetNodeText(funcDef, content)
	case "binary_operator":
		// Guard clause: def func_name(args) when guard do ... end
		// The left side contains the actual function call
		WalkTree(funcDef, func(node *sitter.Node) bool {
			if node.Type() == "call" && node.NamedChildCount() >= 1 {
				nameNode := node.NamedChild(0)
				if nameNode != nil && nameNode.Type() == "identifier" {
					funcName = GetNodeText(nameNode, content)
					return false
				}
			}
			if node.Type() == "identifier" && funcName == "" {
				funcName = GetNodeText(node, content)
				return false
			}
			return true
		})
	}

	if funcName == "" {
		return nil
	}

	kind := protocol.SymbolFunction
	if isPrivate {
		funcName = funcName + " (private)"
	}

	return &protocol.Symbol{
		Name:     funcName,
		Kind:     kind,
		Location: NodeLocation(n, filename),
	}
}

// extractElixirMacro extracts a macro definition.
func extractElixirMacro(n *sitter.Node, content []byte, filename string) *protocol.Symbol {
	// Similar to function extraction
	if n.NamedChildCount() < 2 {
		return nil
	}

	funcDef := n.NamedChild(1)
	if funcDef == nil {
		return nil
	}

	var macroName string

	switch funcDef.Type() {
	case "call":
		if funcDef.NamedChildCount() >= 1 {
			nameNode := funcDef.NamedChild(0)
			if nameNode != nil {
				macroName = GetNodeText(nameNode, content)
			}
		}
	case "identifier":
		macroName = GetNodeText(funcDef, content)
	}

	if macroName == "" {
		return nil
	}

	return &protocol.Symbol{
		Name:     macroName + " (macro)",
		Kind:     protocol.SymbolFunction,
		Location: NodeLocation(n, filename),
	}
}

// extractElixirStruct extracts a struct definition.
func extractElixirStruct(n *sitter.Node, content []byte, filename string) *protocol.Symbol {
	// defstruct is typically inside a module, the struct name is the module name
	// We just mark this as a struct symbol
	return &protocol.Symbol{
		Name:     "defstruct",
		Kind:     protocol.SymbolStruct,
		Location: NodeLocation(n, filename),
	}
}

// extractElixirProtocol extracts a protocol definition.
func extractElixirProtocol(n *sitter.Node, content []byte, filename string) *protocol.Symbol {
	// defprotocol ProtocolName do ... end
	if n.NamedChildCount() < 2 {
		return nil
	}

	args := n.NamedChild(1)
	if args == nil {
		return nil
	}

	var protocolName string
	WalkTree(args, func(node *sitter.Node) bool {
		if node.Type() == "alias" {
			protocolName = GetNodeText(node, content)
			return false
		}
		return true
	})

	if protocolName == "" {
		return nil
	}

	return &protocol.Symbol{
		Name:     protocolName,
		Kind:     protocol.SymbolInterface,
		Location: NodeLocation(n, filename),
	}
}

// extractRustSymbols extracts symbols from Rust code.
func extractRustSymbols(root *sitter.Node, content []byte, filename string) []protocol.Symbol {
	var symbols []protocol.Symbol

	WalkTree(root, func(n *sitter.Node) bool {
		var symbol *protocol.Symbol

		switch n.Type() {
		case "function_item":
			nameNode := n.ChildByFieldName("name")
			if nameNode != nil {
				symbol = &protocol.Symbol{
					Name:     GetNodeText(nameNode, content),
					Kind:     protocol.SymbolFunction,
					Location: NodeLocation(n, filename),
				}
			}
		case "struct_item":
			nameNode := n.ChildByFieldName("name")
			if nameNode != nil {
				symbol = &protocol.Symbol{
					Name:     GetNodeText(nameNode, content),
					Kind:     protocol.SymbolStruct,
					Location: NodeLocation(n, filename),
				}
			}
		case "enum_item":
			nameNode := n.ChildByFieldName("name")
			if nameNode != nil {
				symbol = &protocol.Symbol{
					Name:     GetNodeText(nameNode, content),
					Kind:     protocol.SymbolEnum,
					Location: NodeLocation(n, filename),
				}
			}
		case "trait_item":
			nameNode := n.ChildByFieldName("name")
			if nameNode != nil {
				symbol = &protocol.Symbol{
					Name:     GetNodeText(nameNode, content),
					Kind:     protocol.SymbolTrait,
					Location: NodeLocation(n, filename),
				}
			}
		case "impl_item":
			symbol = extractRustImpl(n, content, filename)
		case "type_item":
			nameNode := n.ChildByFieldName("name")
			if nameNode != nil {
				symbol = &protocol.Symbol{
					Name:     GetNodeText(nameNode, content),
					Kind:     protocol.SymbolType,
					Location: NodeLocation(n, filename),
				}
			}
		case "const_item":
			nameNode := n.ChildByFieldName("name")
			if nameNode != nil {
				symbol = &protocol.Symbol{
					Name:     GetNodeText(nameNode, content),
					Kind:     protocol.SymbolConstant,
					Location: NodeLocation(n, filename),
				}
			}
		case "static_item":
			nameNode := n.ChildByFieldName("name")
			if nameNode != nil {
				symbol = &protocol.Symbol{
					Name:     GetNodeText(nameNode, content),
					Kind:     protocol.SymbolVariable,
					Location: NodeLocation(n, filename),
				}
			}
		case "macro_definition":
			nameNode := n.ChildByFieldName("name")
			if nameNode != nil {
				symbol = &protocol.Symbol{
					Name:     GetNodeText(nameNode, content) + " (macro)",
					Kind:     protocol.SymbolFunction,
					Location: NodeLocation(n, filename),
				}
			}
		case "mod_item":
			nameNode := n.ChildByFieldName("name")
			if nameNode != nil {
				symbol = &protocol.Symbol{
					Name:     GetNodeText(nameNode, content),
					Kind:     protocol.SymbolModule,
					Location: NodeLocation(n, filename),
				}
			}
		}

		if symbol != nil {
			if doc := ExtractDocComment(n, content, protocol.LangRust); doc != nil {
				symbol.Doc = FormatDocComment(doc)
			}
			symbols = append(symbols, *symbol)
		}

		return true
	})

	return symbols
}

// extractRustImpl extracts an impl block symbol from Rust code.
func extractRustImpl(n *sitter.Node, content []byte, filename string) *protocol.Symbol {
	typeNode := n.ChildByFieldName("type")
	traitNode := n.ChildByFieldName("trait")

	var name string
	if traitNode != nil && typeNode != nil {
		name = "impl " + GetNodeText(traitNode, content) + " for " + GetNodeText(typeNode, content)
	} else if typeNode != nil {
		name = "impl " + GetNodeText(typeNode, content)
	} else {
		return nil
	}

	return &protocol.Symbol{
		Name:     name,
		Kind:     protocol.SymbolType,
		Location: NodeLocation(n, filename),
	}
}

// extractElixirImpl extracts a protocol implementation.
func extractElixirImpl(n *sitter.Node, content []byte, filename string) *protocol.Symbol {
	// defimpl Protocol, for: Type do ... end
	if n.NamedChildCount() < 2 {
		return nil
	}

	args := n.NamedChild(1)
	if args == nil {
		return nil
	}

	var implName string
	WalkTree(args, func(node *sitter.Node) bool {
		if node.Type() == "alias" {
			if implName == "" {
				implName = GetNodeText(node, content)
			} else {
				implName = implName + " for " + GetNodeText(node, content)
				return false
			}
		}
		return true
	})

	if implName == "" {
		return nil
	}

	return &protocol.Symbol{
		Name:     implName,
		Kind:     protocol.SymbolClass,
		Location: NodeLocation(n, filename),
	}
}

// kindMatchesNode returns true if the given SymbolKind matches the node type.
// An empty kind matches all symbol-bearing nodes.
func kindMatchesNode(kind protocol.SymbolKind, nodeType string) bool {
	if kind == "" {
		return true
	}
	switch kind {
	case protocol.SymbolFunction:
		return nodeType == "function_declaration" || nodeType == "function_definition" || nodeType == "function_item"
	case protocol.SymbolMethod:
		return nodeType == "method_declaration" || nodeType == "method_definition"
	case protocol.SymbolClass:
		return nodeType == "class_declaration" || nodeType == "class_definition"
	case protocol.SymbolStruct:
		return nodeType == "struct_item" || nodeType == "type_declaration"
	case protocol.SymbolInterface:
		return nodeType == "interface_declaration"
	case protocol.SymbolType:
		return nodeType == "type_declaration" || nodeType == "type_alias_declaration" || nodeType == "type_item"
	case protocol.SymbolEnum:
		return nodeType == "enum_item"
	case protocol.SymbolTrait:
		return nodeType == "trait_item"
	case protocol.SymbolConstant:
		return nodeType == "const_item"
	case protocol.SymbolModule:
		return nodeType == "mod_item"
	}
	return true
}

// FindSymbolRange finds a named symbol in the AST and returns its line range (1-indexed, inclusive).
// symbolKind filters by symbol kind (e.g. "function", "struct"); empty string matches any kind.
// Returns (0, 0, false) if the symbol is not found.
func FindSymbolRange(tree *sitter.Tree, content []byte, filename, symbolName string, symbolKind protocol.SymbolKind) (startLine, endLine int, found bool) {
	if tree == nil || symbolName == "" {
		return
	}
	WalkTree(tree.RootNode(), func(n *sitter.Node) bool {
		if found {
			return false
		}
		nameNode := n.ChildByFieldName("name")
		if nameNode == nil {
			return true
		}
		if GetNodeText(nameNode, content) != symbolName {
			return true
		}
		switch n.Type() {
		case "function_declaration", "method_declaration", "type_declaration",
			"function_definition", "class_definition", "class_declaration",
			"interface_declaration", "type_alias_declaration",
			"function_item", "struct_item", "enum_item", "trait_item",
			"type_item", "const_item", "mod_item":
			if !kindMatchesNode(symbolKind, n.Type()) {
				return true // kind mismatch, keep searching
			}
			r := NodeRange(n, filename)
			startLine = r.Start.Line
			endLine = r.End.Line
			found = true
			return false
		}
		return true
	})
	return
}
