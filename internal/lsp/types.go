package lsp

// InitializeParams are the parameters for the initialize request.
type InitializeParams struct {
	RootURI      string       `json:"rootUri"`
	Capabilities Capabilities `json:"capabilities"`
	ProcessID    int          `json:"processId"`
}

// Capabilities represents client capabilities.
type Capabilities struct {
	TextDocument TextDocumentClientCapabilities `json:"textDocument"`
}

// TextDocumentClientCapabilities represents text document capabilities.
type TextDocumentClientCapabilities struct {
	Hover      HoverCapability      `json:"hover,omitempty"`
	Definition DefinitionCapability `json:"definition,omitempty"`
	References ReferencesCapability `json:"references,omitempty"`
}

// HoverCapability represents hover capabilities.
type HoverCapability struct {
	ContentFormat []string `json:"contentFormat,omitempty"`
}

// DefinitionCapability represents definition capabilities.
type DefinitionCapability struct {
	LinkSupport bool `json:"linkSupport,omitempty"`
}

// ReferencesCapability represents references capabilities.
type ReferencesCapability struct{}

// InitializeResult is the result of the initialize request.
type InitializeResult struct {
	Capabilities ServerCapabilities `json:"capabilities"`
}

// ServerCapabilities represents server capabilities.
type ServerCapabilities struct {
	HoverProvider          bool `json:"hoverProvider,omitempty"`
	DefinitionProvider     bool `json:"definitionProvider,omitempty"`
	ReferencesProvider     bool `json:"referencesProvider,omitempty"`
	DocumentSymbolProvider bool `json:"documentSymbolProvider,omitempty"`
	TextDocumentSync       int  `json:"textDocumentSync,omitempty"`
}

// Position represents a position in a document.
type Position struct {
	Line      int `json:"line"`      // 0-indexed
	Character int `json:"character"` // 0-indexed
}

// Range represents a range in a document.
type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

// Location represents a location in a document.
type Location struct {
	URI   string `json:"uri"`
	Range Range  `json:"range"`
}

// TextDocumentIdentifier identifies a text document.
type TextDocumentIdentifier struct {
	URI string `json:"uri"`
}

// TextDocumentPositionParams represents position parameters.
type TextDocumentPositionParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
}

// HoverParams are the parameters for the hover request.
type HoverParams struct {
	TextDocumentPositionParams
}

// HoverResult is the result of the hover request.
type HoverResult struct {
	Range    *Range        `json:"range,omitempty"`
	Contents MarkupContent `json:"contents"`
}

// MarkupContent represents markup content.
type MarkupContent struct {
	Kind  string `json:"kind"` // "plaintext" or "markdown"
	Value string `json:"value"`
}

// DefinitionParams are the parameters for the definition request.
type DefinitionParams struct {
	TextDocumentPositionParams
}

// ReferenceParams are the parameters for the references request.
type ReferenceParams struct {
	TextDocumentPositionParams
	Context ReferenceContext `json:"context"`
}

// ReferenceContext represents reference context.
type ReferenceContext struct {
	IncludeDeclaration bool `json:"includeDeclaration"`
}

// TextDocumentItem represents a text document.
type TextDocumentItem struct {
	URI        string `json:"uri"`
	LanguageID string `json:"languageId"`
	Text       string `json:"text"`
	Version    int    `json:"version"`
}

// DidOpenTextDocumentParams are the parameters for didOpen.
type DidOpenTextDocumentParams struct {
	TextDocument TextDocumentItem `json:"textDocument"`
}

// DidCloseTextDocumentParams are the parameters for didClose.
type DidCloseTextDocumentParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
}

// DocumentSymbol represents a symbol in a document.
type DocumentSymbol struct {
	Name           string           `json:"name"`
	Detail         string           `json:"detail,omitempty"`
	Children       []DocumentSymbol `json:"children,omitempty"`
	Range          Range            `json:"range"`
	SelectionRange Range            `json:"selectionRange"`
	Kind           int              `json:"kind"`
}

// SymbolInformation represents symbol information.
type SymbolInformation struct {
	Name          string   `json:"name"`
	ContainerName string   `json:"containerName,omitempty"`
	Location      Location `json:"location"`
	Kind          int      `json:"kind"`
}

// DocumentSymbolParams are the parameters for documentSymbol.
type DocumentSymbolParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
}
