// Package protocol defines shared types used across the MCP file operations server.
package protocol

// Location represents a position in a file.
type Location struct {
	File   string `json:"file"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
}

// Range represents a range in a file.
type Range struct {
	Start Location `json:"start"`
	End   Location `json:"end"`
}

// SymbolKind represents the kind of a symbol.
type SymbolKind string

const (
	SymbolFunction  SymbolKind = "function"
	SymbolMethod    SymbolKind = "method"
	SymbolClass     SymbolKind = "class"
	SymbolStruct    SymbolKind = "struct"
	SymbolInterface SymbolKind = "interface"
	SymbolVariable  SymbolKind = "variable"
	SymbolConstant  SymbolKind = "constant"
	SymbolType      SymbolKind = "type"
	SymbolField     SymbolKind = "field"
	SymbolProperty  SymbolKind = "property"
	SymbolModule    SymbolKind = "module"
	SymbolPackage   SymbolKind = "package"
)

// Symbol represents a code symbol (function, class, variable, etc.).
type Symbol struct {
	Name     string     `json:"name"`
	Kind     SymbolKind `json:"kind"`
	Doc      string     `json:"doc,omitempty"`
	Location Location   `json:"location"`
}

// SyntaxError represents a syntax error in a file.
type SyntaxError struct {
	Message  string   `json:"message"`
	Location Location `json:"location"`
}

// Language represents a programming language.
type Language string

const (
	LangGo         Language = "go"
	LangTypeScript Language = "typescript"
	LangJavaScript Language = "javascript"
	LangPython     Language = "python"
	LangC          Language = "c"
	LangCpp        Language = "cpp"
	LangHTML       Language = "html"
	LangVue        Language = "vue"
	LangJSON       Language = "json"
	LangYAML       Language = "yaml"
	LangUnknown    Language = "unknown"
)

// DetectLanguage detects the language from a filename.
func DetectLanguage(filename string) Language {
	ext := getExtension(filename)
	switch ext {
	case ".go":
		return LangGo
	case ".ts", ".tsx":
		return LangTypeScript
	case ".js", ".jsx", ".mjs", ".cjs":
		return LangJavaScript
	case ".py", ".pyw":
		return LangPython
	case ".c", ".h":
		return LangC
	case ".cpp", ".cc", ".cxx", ".hpp", ".hxx":
		return LangCpp
	case ".html", ".htm":
		return LangHTML
	case ".vue":
		return LangVue
	case ".json":
		return LangJSON
	case ".yaml", ".yml":
		return LangYAML
	default:
		return LangUnknown
	}
}

func getExtension(filename string) string {
	for i := len(filename) - 1; i >= 0; i-- {
		if filename[i] == '.' {
			return filename[i:]
		}
		if filename[i] == '/' || filename[i] == '\\' {
			break
		}
	}
	return ""
}
