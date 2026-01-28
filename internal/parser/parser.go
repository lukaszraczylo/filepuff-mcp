// Package parser provides Tree-sitter based parsing for multiple languages.
package parser

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cespare/xxhash/v2"
	lru "github.com/hashicorp/golang-lru/v2"
	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/c"
	"github.com/smacker/go-tree-sitter/cpp"
	"github.com/smacker/go-tree-sitter/elixir"
	"github.com/smacker/go-tree-sitter/golang"
	"github.com/smacker/go-tree-sitter/html"
	"github.com/smacker/go-tree-sitter/javascript"
	"github.com/smacker/go-tree-sitter/python"
	"github.com/smacker/go-tree-sitter/typescript/typescript"

	"github.com/lukaszraczylo/mcp-filepuff/pkg/errors"
	"github.com/lukaszraczylo/mcp-filepuff/pkg/protocol"
)

// MaxFileSize is the default maximum file size we'll parse (10MB).
// Deprecated: Use Registry.maxParseSize instead.
const MaxFileSize = 10 * 1024 * 1024

// Registry manages Tree-sitter parsers for different languages.
type Registry struct {
	parsers      map[protocol.Language]*sitter.Parser
	cache        *lru.Cache[string, *CachedTree]
	maxParseSize int64
	mu           sync.RWMutex

	// Cache metrics (atomic for thread-safety)
	cacheHits   atomic.Int64
	cacheMisses atomic.Int64

	// Parse duration tracking
	totalParseTime    atomic.Int64 // nanoseconds
	parseCount        atomic.Int64
	lastParseDuration atomic.Int64 // nanoseconds
}

// CachedTree stores a parsed tree with its metadata.
// Content is not stored to reduce memory usage.
type CachedTree struct {
	Tree     *sitter.Tree
	Language protocol.Language
}

// ParseResult contains the result of parsing a file.
type ParseResult struct {
	Tree     *sitter.Tree
	Language protocol.Language
	Errors   []SyntaxError
	Content  []byte
}

// SyntaxError represents a syntax error found during parsing.
type SyntaxError struct {
	Message  string
	NodeType string
	Location protocol.Location
}

// CacheStatsResult contains cache statistics.
type CacheStatsResult struct {
	Hits           int64   `json:"hits"`
	Misses         int64   `json:"misses"`
	HitRate        float64 `json:"hit_rate"`
	Size           int     `json:"size"`
	TotalParseTime int64   `json:"total_parse_time_ns"`
	ParseCount     int64   `json:"parse_count"`
	AvgParseTime   int64   `json:"avg_parse_time_ns"`
	LastParseTime  int64   `json:"last_parse_time_ns"`
}

// NewRegistry creates a new parser registry with the default max parse size.
// For custom max parse size, use NewRegistryWithSize.
func NewRegistry() *Registry {
	return NewRegistryWithSize(0)
}

// NewRegistryWithSize creates a new parser registry with the specified max parse size.
// If maxParseSize is 0 or negative, uses the default MaxFileSize constant.
func NewRegistryWithSize(maxParseSize int64) *Registry {
	// Create LRU cache with capacity of 100 trees
	cache, err := lru.New[string, *CachedTree](100)
	if err != nil {
		// LRU.New only errors if size <= 0, which won't happen here
		panic(fmt.Sprintf("failed to create LRU cache: %v", err))
	}

	if maxParseSize <= 0 {
		maxParseSize = MaxFileSize
	}

	return &Registry{
		parsers:      make(map[protocol.Language]*sitter.Parser),
		cache:        cache,
		maxParseSize: maxParseSize,
	}
}

// getLanguage returns the Tree-sitter language for a given language.
func getLanguage(lang protocol.Language) (*sitter.Language, error) {
	switch lang {
	case protocol.LangGo:
		return golang.GetLanguage(), nil
	case protocol.LangTypeScript:
		return typescript.GetLanguage(), nil
	case protocol.LangJavaScript:
		return javascript.GetLanguage(), nil
	case protocol.LangPython:
		return python.GetLanguage(), nil
	case protocol.LangC:
		return c.GetLanguage(), nil
	case protocol.LangCpp:
		return cpp.GetLanguage(), nil
	case protocol.LangHTML:
		return html.GetLanguage(), nil
	case protocol.LangVue:
		// Vue SFC files use HTML-like template syntax, so we use the HTML parser
		return html.GetLanguage(), nil
	case protocol.LangElixir:
		return elixir.GetLanguage(), nil
	default:
		return nil, errors.New(errors.ErrInvalidLanguage, fmt.Sprintf("language %s is not supported", lang)).
			WithContext("language", string(lang)).
			WithRemediation("Supported languages: Go, TypeScript, JavaScript, Python, C, C++, HTML, Vue, Elixir")
	}
}

// GetParser returns a parser for the given language.
func (r *Registry) GetParser(lang protocol.Language) (*sitter.Parser, error) {
	r.mu.RLock()
	if p, ok := r.parsers[lang]; ok {
		r.mu.RUnlock()
		return p, nil
	}
	r.mu.RUnlock()

	// Create new parser
	r.mu.Lock()
	defer r.mu.Unlock()

	// Double-check after acquiring write lock
	if p, ok := r.parsers[lang]; ok {
		return p, nil
	}

	sitterLang, err := getLanguage(lang)
	if err != nil {
		return nil, err
	}

	parser := sitter.NewParser()
	parser.SetLanguage(sitterLang)
	r.parsers[lang] = parser

	return parser, nil
}

// Parse parses the given content for the specified language.
func (r *Registry) Parse(ctx context.Context, filename string, content []byte) (*ParseResult, error) {
	// Check file size against configured limit
	if int64(len(content)) > r.maxParseSize {
		return nil, errors.NewFileTooLarge(filename, int64(len(content)), r.maxParseSize)
	}

	// Detect binary files
	if isBinary(content) {
		return nil, errors.New(errors.ErrParseFailed, "binary file detected").
			WithContext("file", filename).
			WithRemediation("This appears to be a binary file and cannot be parsed as source code")
	}

	// Detect language
	lang := protocol.DetectLanguage(filename)
	if lang == protocol.LangUnknown {
		return nil, errors.New(errors.ErrInvalidLanguage, "could not detect language from filename").
			WithContext("file", filename).
			WithRemediation("Ensure file has a recognized extension (e.g., .go, .ts, .py, .c, .cpp, .html, .vue, .json, .yaml)")
	}

	// Handle YAML and JSON separately (they don't use tree-sitter)
	switch lang {
	case protocol.LangYAML:
		return r.ParseYAML(ctx, filename, content)
	case protocol.LangJSON:
		return r.ParseJSON(ctx, filename, content)
	}

	// Check cache (LRU cache is thread-safe)
	hash := contentHash(content)
	if cached, ok := r.cache.Get(hash); ok && cached.Language == lang {
		r.cacheHits.Add(1)
		errors := extractErrors(cached.Tree.RootNode(), content)
		return &ParseResult{
			Tree:     cached.Tree,
			Language: lang,
			Errors:   errors,
			Content:  content,
		}, nil
	}
	r.cacheMisses.Add(1)

	// Get parser
	parser, err := r.GetParser(lang)
	if err != nil {
		return nil, err
	}

	// Parse content - tree-sitter parsers are not thread-safe,
	// so we need to hold the lock during parsing
	// Track parse duration
	start := time.Now()
	r.mu.Lock()
	tree, err := parser.ParseCtx(ctx, nil, content)
	r.mu.Unlock()
	duration := time.Since(start)

	// Update duration metrics
	r.totalParseTime.Add(duration.Nanoseconds())
	r.parseCount.Add(1)
	r.lastParseDuration.Store(duration.Nanoseconds())

	if err != nil {
		return nil, errors.NewParseError(string(lang), filename, err)
	}

	// Extract syntax errors
	errors := extractErrors(tree.RootNode(), content)

	// Cache result (LRU cache handles eviction automatically)
	r.cache.Add(hash, &CachedTree{
		Tree:     tree,
		Language: lang,
	})

	return &ParseResult{
		Tree:     tree,
		Language: lang,
		Errors:   errors,
		Content:  content,
	}, nil
}

// CacheStats returns cache hit/miss statistics.
func (r *Registry) CacheStats() (hits, misses int64) {
	return r.cacheHits.Load(), r.cacheMisses.Load()
}

// CacheStatsDetailed returns detailed cache and parse statistics.
func (r *Registry) CacheStatsDetailed() CacheStatsResult {
	hits := r.cacheHits.Load()
	misses := r.cacheMisses.Load()
	totalParseTime := r.totalParseTime.Load()
	parseCount := r.parseCount.Load()

	var hitRate float64
	total := hits + misses
	if total > 0 {
		hitRate = float64(hits) / float64(total)
	}

	var avgParseTime int64
	if parseCount > 0 {
		avgParseTime = totalParseTime / parseCount
	}

	return CacheStatsResult{
		Hits:           hits,
		Misses:         misses,
		HitRate:        hitRate,
		Size:           r.cache.Len(),
		TotalParseTime: totalParseTime,
		ParseCount:     parseCount,
		AvgParseTime:   avgParseTime,
		LastParseTime:  r.lastParseDuration.Load(),
	}
}

// ResetStats resets all cache and parse statistics.
func (r *Registry) ResetStats() {
	r.cacheHits.Store(0)
	r.cacheMisses.Store(0)
	r.totalParseTime.Store(0)
	r.parseCount.Store(0)
	r.lastParseDuration.Store(0)
}

// extractErrors finds all error nodes in the tree.
func extractErrors(node *sitter.Node, _ []byte) []SyntaxError {
	var errors []SyntaxError

	var walk func(n *sitter.Node)
	walk = func(n *sitter.Node) {
		if n == nil {
			return
		}

		if n.IsError() || n.IsMissing() {
			startPoint := n.StartPoint()
			nodeType := "ERROR"
			if n.IsMissing() {
				nodeType = "MISSING"
			}

			errors = append(errors, SyntaxError{
				Location: protocol.Location{
					Line:   int(startPoint.Row) + 1,
					Column: int(startPoint.Column) + 1,
				},
				Message:  fmt.Sprintf("syntax error: unexpected %s", n.Type()),
				NodeType: nodeType,
			})
		}

		for i := 0; i < int(n.ChildCount()); i++ {
			walk(n.Child(i))
		}
	}

	walk(node)
	return errors
}

// contentHash returns a fast hash of the content for caching.
// Uses xxHash which is 5-10x faster than SHA256 for non-cryptographic purposes.
func contentHash(content []byte) string {
	h := xxhash.Sum64(content)
	return fmt.Sprintf("%016x", h)
}

// isBinary checks if content appears to be binary.
func isBinary(content []byte) bool {
	// Check first 8000 bytes for null bytes
	checkLen := min(8000, len(content))

	for i := range checkLen {
		if content[i] == 0 {
			return true
		}
	}
	return false
}

// Close closes all parsers and clears the cache.
func (r *Registry) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, p := range r.parsers {
		p.Close()
	}
	r.parsers = make(map[protocol.Language]*sitter.Parser)

	// Purge LRU cache
	r.cache.Purge()
}
