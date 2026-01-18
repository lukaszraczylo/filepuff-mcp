package lsp

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	json "github.com/goccy/go-json"
	"github.com/lukaszraczylo/mcp-filepuff/pkg/errors"
	"github.com/lukaszraczylo/mcp-filepuff/pkg/protocol"
)

// Manager manages LSP servers for different languages.
type Manager struct {
	servers       map[protocol.Language]*ManagedServer
	logger        *slog.Logger
	stopReaper    chan struct{}
	workspaceRoot string
	timeout       time.Duration
	idleTimeout   time.Duration
	mu            sync.RWMutex
	stopped       bool
}

// ManagedServer represents a managed LSP server instance.
type ManagedServer struct {
	lastUsed     time.Time
	initErr      error
	client       *Client
	openDocs     map[string]int
	language     protocol.Language
	capabilities ServerCapabilities
	mu           sync.Mutex
	ready        bool
}

// ServerConfig contains the configuration for an LSP server.
type ServerConfig struct {
	Command []string
	Args    []string
}

// DefaultServerConfigs contains default configurations for LSP servers.
var DefaultServerConfigs = map[protocol.Language]ServerConfig{
	protocol.LangGo: {
		Command: []string{"gopls"},
		Args:    []string{"serve"},
	},
	protocol.LangTypeScript: {
		Command: []string{"typescript-language-server"},
		Args:    []string{"--stdio"},
	},
	protocol.LangJavaScript: {
		Command: []string{"typescript-language-server"},
		Args:    []string{"--stdio"},
	},
	protocol.LangPython: {
		Command: []string{"pylsp"},
	},
	protocol.LangC: {
		Command: []string{"clangd"},
	},
	protocol.LangCpp: {
		Command: []string{"clangd"},
	},
}

// NewManager creates a new LSP manager.
func NewManager(workspaceRoot string, logger *slog.Logger) *Manager {
	m := &Manager{
		servers:       make(map[protocol.Language]*ManagedServer),
		timeout:       10 * time.Second,
		idleTimeout:   5 * time.Minute,
		workspaceRoot: workspaceRoot,
		logger:        logger,
		stopReaper:    make(chan struct{}),
	}

	// Start idle reaper
	go m.reapIdleServers()

	return m
}

// GetServer returns or creates an LSP server for the given language.
func (m *Manager) GetServer(ctx context.Context, lang protocol.Language) (*ManagedServer, error) {
	m.mu.RLock()
	srv, exists := m.servers[lang]
	m.mu.RUnlock()

	if exists && srv.ready {
		// Update lastUsed with server's own lock to avoid race condition
		srv.mu.Lock()
		srv.lastUsed = time.Now()
		srv.mu.Unlock()
		return srv, nil
	}

	// Create new server
	m.mu.Lock()
	defer m.mu.Unlock()

	// Double-check after acquiring write lock
	if srv, ok := m.servers[lang]; ok && srv.ready {
		srv.mu.Lock()
		srv.lastUsed = time.Now()
		srv.mu.Unlock()
		return srv, nil
	}

	// Check if server config exists
	config, ok := DefaultServerConfigs[lang]
	if !ok {
		return nil, errors.New(errors.ErrLSPServerNotFound, fmt.Sprintf("no LSP server configured for language: %s", lang)).
			WithContext("language", string(lang)).
			WithRemediation("Configure an LSP server for this language or use a supported language")
	}

	// Check if command is available
	cmdPath, err := exec.LookPath(config.Command[0])
	if err != nil {
		return nil, errors.NewLSPServerNotFound(string(lang), config.Command[0])
	}

	// Create command
	args := append(config.Command[1:], config.Args...)
	cmd := exec.CommandContext(ctx, cmdPath, args...)
	cmd.Env = os.Environ()
	cmd.Dir = m.workspaceRoot

	// Create client
	client, err := NewClient(cmd)
	if err != nil {
		// Ensure process is killed if client creation fails
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		return nil, errors.Wrap(errors.ErrLSPCommunication, "failed to create LSP client", err).
			WithContext("language", string(lang)).
			WithContext("command", config.Command[0]).
			WithRemediation("Ensure the LSP server binary is executable and compatible with your system")
	}

	newSrv := &ManagedServer{
		client:   client,
		language: lang,
		lastUsed: time.Now(),
		openDocs: make(map[string]int),
	}

	// Initialize server
	if err := m.initializeServer(ctx, newSrv); err != nil {
		_ = client.Close()
		// Ensure process is killed on initialization failure
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		newSrv.initErr = err
		return nil, errors.Wrap(errors.ErrLSPInitFailed, "LSP server initialization failed", err).
			WithContext("language", string(lang)).
			WithContext("command", config.Command[0]).
			WithRemediation("Check LSP server logs for initialization errors")
	}

	newSrv.ready = true
	m.servers[lang] = newSrv
	m.logger.Info("started LSP server", "language", lang, "command", config.Command[0])

	return newSrv, nil
}

// initializeServer performs the LSP initialization handshake.
func (m *Manager) initializeServer(ctx context.Context, srv *ManagedServer) error {
	// Create context with timeout
	ctx, cancel := context.WithTimeout(ctx, m.timeout)
	defer cancel()

	// Build root URI
	rootURI := "file://" + m.workspaceRoot

	// Send initialize request
	params := InitializeParams{
		ProcessID: os.Getpid(),
		RootURI:   rootURI,
		Capabilities: Capabilities{
			TextDocument: TextDocumentClientCapabilities{
				Hover: HoverCapability{
					ContentFormat: []string{"markdown", "plaintext"},
				},
				Definition: DefinitionCapability{
					LinkSupport: true,
				},
				References: ReferencesCapability{},
			},
		},
	}

	resp, err := srv.client.Call(ctx, "initialize", params)
	if err != nil {
		return fmt.Errorf("initialize failed: %w", err)
	}

	// Parse capabilities
	var result InitializeResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return fmt.Errorf("failed to parse initialize result: %w", err)
	}
	srv.capabilities = result.Capabilities

	// Send initialized notification
	if err := srv.client.Notify("initialized", struct{}{}); err != nil {
		return fmt.Errorf("initialized notification failed: %w", err)
	}

	return nil
}

// Hover performs a hover request at the given position.
func (m *Manager) Hover(ctx context.Context, file string, line, col int) (*HoverResult, error) {
	lang := protocol.DetectLanguage(file)
	srv, err := m.GetServer(ctx, lang)
	if err != nil {
		return nil, err
	}

	// Ensure document is open
	err = m.ensureDocumentOpen(ctx, srv, file)
	if err != nil {
		return nil, err
	}

	params := HoverParams{
		TextDocumentPositionParams: TextDocumentPositionParams{
			TextDocument: TextDocumentIdentifier{
				URI: fileToURI(file),
			},
			Position: Position{
				Line:      line - 1, // Convert to 0-indexed
				Character: col - 1,
			},
		},
	}

	ctx, cancel := context.WithTimeout(ctx, m.timeout)
	defer cancel()

	resp, err := srv.client.Call(ctx, "textDocument/hover", params)
	if err != nil {
		return nil, fmt.Errorf("hover request failed: %w", err)
	}

	if resp.Result == nil || string(resp.Result) == "null" {
		return nil, nil // No hover info
	}

	var result HoverResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("failed to parse hover result: %w", err)
	}

	return &result, nil
}

// Definition finds the definition of the symbol at the given position.
func (m *Manager) Definition(ctx context.Context, file string, line, col int) ([]Location, error) {
	lang := protocol.DetectLanguage(file)
	srv, err := m.GetServer(ctx, lang)
	if err != nil {
		return nil, err
	}

	// Ensure document is open
	err = m.ensureDocumentOpen(ctx, srv, file)
	if err != nil {
		return nil, err
	}

	params := DefinitionParams{
		TextDocumentPositionParams: TextDocumentPositionParams{
			TextDocument: TextDocumentIdentifier{
				URI: fileToURI(file),
			},
			Position: Position{
				Line:      line - 1,
				Character: col - 1,
			},
		},
	}

	ctx, cancel := context.WithTimeout(ctx, m.timeout)
	defer cancel()

	resp, err := srv.client.Call(ctx, "textDocument/definition", params)
	if err != nil {
		return nil, fmt.Errorf("definition request failed: %w", err)
	}

	if resp.Result == nil || string(resp.Result) == "null" {
		return nil, nil
	}

	// Result can be Location, []Location, or []LocationLink
	var locations []Location
	if err := json.Unmarshal(resp.Result, &locations); err != nil {
		// Try single location
		var single Location
		if err := json.Unmarshal(resp.Result, &single); err == nil {
			locations = []Location{single}
		}
	}

	return locations, nil
}

// References finds all references to the symbol at the given position.
func (m *Manager) References(ctx context.Context, file string, line, col int, includeDeclaration bool) ([]Location, error) {
	lang := protocol.DetectLanguage(file)
	srv, err := m.GetServer(ctx, lang)
	if err != nil {
		return nil, err
	}

	// Ensure document is open
	err = m.ensureDocumentOpen(ctx, srv, file)
	if err != nil {
		return nil, err
	}

	params := ReferenceParams{
		TextDocumentPositionParams: TextDocumentPositionParams{
			TextDocument: TextDocumentIdentifier{
				URI: fileToURI(file),
			},
			Position: Position{
				Line:      line - 1,
				Character: col - 1,
			},
		},
		Context: ReferenceContext{
			IncludeDeclaration: includeDeclaration,
		},
	}

	ctx, cancel := context.WithTimeout(ctx, m.timeout)
	defer cancel()

	resp, err := srv.client.Call(ctx, "textDocument/references", params)
	if err != nil {
		return nil, fmt.Errorf("references request failed: %w", err)
	}

	if resp.Result == nil || string(resp.Result) == "null" {
		return nil, nil
	}

	var locations []Location
	if err := json.Unmarshal(resp.Result, &locations); err != nil {
		return nil, fmt.Errorf("failed to parse references result: %w", err)
	}

	return locations, nil
}

// ensureDocumentOpen opens a document if not already open.
func (m *Manager) ensureDocumentOpen(ctx context.Context, srv *ManagedServer, file string) error {
	uri := fileToURI(file)

	srv.mu.Lock()
	if _, ok := srv.openDocs[uri]; ok {
		srv.mu.Unlock()
		return nil
	}
	srv.mu.Unlock()

	// Read file content
	content, err := os.ReadFile(file)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	// Get language ID
	langID := languageToLSPID(srv.language)

	params := DidOpenTextDocumentParams{
		TextDocument: TextDocumentItem{
			URI:        uri,
			LanguageID: langID,
			Version:    1,
			Text:       string(content),
		},
	}

	if err := srv.client.Notify("textDocument/didOpen", params); err != nil {
		return fmt.Errorf("didOpen failed: %w", err)
	}

	srv.mu.Lock()
	srv.openDocs[uri] = 1
	srv.mu.Unlock()

	return nil
}

// CloseDocument closes a document in the server.
func (m *Manager) CloseDocument(_ context.Context, lang protocol.Language, file string) error {
	m.mu.RLock()
	srv, ok := m.servers[lang]
	m.mu.RUnlock()

	if !ok || !srv.ready {
		return nil
	}

	uri := fileToURI(file)

	srv.mu.Lock()
	if _, ok := srv.openDocs[uri]; !ok {
		srv.mu.Unlock()
		return nil
	}
	delete(srv.openDocs, uri)
	srv.mu.Unlock()

	params := DidCloseTextDocumentParams{
		TextDocument: TextDocumentIdentifier{
			URI: uri,
		},
	}

	return srv.client.Notify("textDocument/didClose", params)
}

// reapIdleServers periodically closes idle servers.
func (m *Manager) reapIdleServers() {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-m.stopReaper:
			return
		case <-ticker.C:
			m.mu.Lock()
			for lang, srv := range m.servers {
				// Check lastUsed with server's lock to avoid race condition
				srv.mu.Lock()
				idle := time.Since(srv.lastUsed) > m.idleTimeout
				srv.mu.Unlock()

				if idle {
					m.logger.Info("closing idle LSP server", "language", lang)
					_ = srv.client.Close()
					delete(m.servers, lang)
				}
			}
			m.mu.Unlock()
		}
	}
}

// Close shuts down all LSP servers.
func (m *Manager) Close() error {
	close(m.stopReaper)

	m.mu.Lock()
	defer m.mu.Unlock()

	m.stopped = true

	for lang, srv := range m.servers {
		m.logger.Info("shutting down LSP server", "language", lang)
		// Try graceful shutdown
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_, _ = srv.client.Call(ctx, "shutdown", nil)
		cancel()
		_ = srv.client.Notify("exit", nil)
		_ = srv.client.Close()
	}

	m.servers = make(map[protocol.Language]*ManagedServer)
	return nil
}

// IsAvailable checks if an LSP server is available for the given language.
func (m *Manager) IsAvailable(lang protocol.Language) bool {
	config, ok := DefaultServerConfigs[lang]
	if !ok {
		return false
	}

	_, err := exec.LookPath(config.Command[0])
	return err == nil
}

// fileToURI converts a file path to a file URI.
func fileToURI(file string) string {
	absPath, err := filepath.Abs(file)
	if err != nil {
		return "file://" + file
	}
	return "file://" + absPath
}

// URIToFile converts a file URI to a file path.
func URIToFile(uri string) string {
	if len(uri) > 7 && uri[:7] == "file://" {
		return uri[7:]
	}
	return uri
}

// languageToLSPID converts a language to LSP language ID.
func languageToLSPID(lang protocol.Language) string {
	switch lang {
	case protocol.LangGo:
		return "go"
	case protocol.LangTypeScript:
		return "typescript"
	case protocol.LangJavaScript:
		return "javascript"
	case protocol.LangPython:
		return "python"
	case protocol.LangC:
		return "c"
	case protocol.LangCpp:
		return "cpp"
	default:
		return string(lang)
	}
}
