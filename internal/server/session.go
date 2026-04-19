// Package server implements the MCP server for file operations.
package server

import (
	"sync/atomic"
	"unsafe"
)

// SessionPrefs holds client-declared session-wide preferences parsed from
// InitializeRequest.Params.Capabilities.Experimental["filepuff"].
//
// These act as defaults; explicit per-call flags always override them.
//
// Supported keys in the "filepuff" experimental map:
//
//	terse                  bool   — no-op (v2 default is already terse; reserved for future)
//	default_format         string — ast_query format default ("verbose"|"compact"|"location")
//	default_max_results    int    — file_search and ast_query max_results when not supplied
//	default_cluster        bool   — file_search cluster default
//	compact_refs           bool   — lsp_query references compact default
//	line_numbers           string — file_read line prefix default ("none"|"compact"|"full")
//	resource_link_threshold int   — per-session override for cfg.ResourceLinkThresholdBytes
type SessionPrefs struct {
	// ASTQueryFormat is the default format for ast_query ("verbose", "compact", "location").
	// Empty string means "use handler built-in default".
	ASTQueryFormat string

	// DefaultMaxResults is the default max_results for file_search and ast_query.
	// 0 means "use handler built-in default".
	DefaultMaxResults int

	// DefaultCluster is the default cluster flag for file_search.
	// nil means "use handler built-in default (false)".
	DefaultCluster *bool

	// CompactRefs is the default compact flag for lsp_query action=references.
	// nil means "use handler built-in default (false)".
	CompactRefs *bool

	// LineNumbers controls the file_read line prefix default.
	// "" = use handler built-in default (full).
	// "none" = no line numbers.
	// "compact" = compact prefix (N│content).
	// "full" = standard padded prefix (  N│ content).
	LineNumbers string

	// ResourceLinkThreshold overrides cfg.ResourceLinkThresholdBytes for this session.
	// 0 means "use config default".
	ResourceLinkThreshold int
}

// boolPtr returns a pointer to a bool value.
func boolPtr(b bool) *bool { return &b }

// ParseSessionPrefs parses the raw map from
// InitializeRequest.Params.Capabilities.Experimental["filepuff"].
// Unknown keys are silently ignored. Type mismatches for individual keys are
// silently ignored (key is treated as absent). Returns zero-value SessionPrefs
// when raw is nil or empty — callers should treat zero values as "use built-in defaults".
func ParseSessionPrefs(raw map[string]any) SessionPrefs {
	if len(raw) == 0 {
		return SessionPrefs{}
	}

	var p SessionPrefs

	if v, ok := raw["default_format"]; ok {
		if s, ok := v.(string); ok {
			switch s {
			case "verbose", "compact", "location":
				p.ASTQueryFormat = s
			}
		}
	}

	if v, ok := raw["default_max_results"]; ok {
		if n := toInt(v); n > 0 {
			p.DefaultMaxResults = n
		}
	}

	if v, ok := raw["default_cluster"]; ok {
		if b, ok := v.(bool); ok {
			p.DefaultCluster = boolPtr(b)
		}
	}

	if v, ok := raw["compact_refs"]; ok {
		if b, ok := v.(bool); ok {
			p.CompactRefs = boolPtr(b)
		}
	}

	if v, ok := raw["line_numbers"]; ok {
		if s, ok := v.(string); ok {
			switch s {
			case "none", "compact", "full":
				p.LineNumbers = s
			}
		}
	}

	if v, ok := raw["resource_link_threshold"]; ok {
		if n := toInt(v); n >= 0 {
			p.ResourceLinkThreshold = n
		}
	}

	return p
}

// toInt converts numeric JSON-decoded values (float64, int, int64) to int.
// Returns 0 for unsupported types or negative values.
func toInt(v any) int {
	switch n := v.(type) {
	case float64:
		if n >= 0 {
			return int(n)
		}
	case int:
		if n >= 0 {
			return n
		}
	case int64:
		if n >= 0 {
			return int(n)
		}
	}
	return 0
}

// sessionPrefsPtr is an atomic pointer helper for thread-safe access to *SessionPrefs.
// We use atomic store/load so the hook (called once at init) and handlers (many goroutines)
// never race. The prefs are write-once after initialization.
type sessionPrefsPtr struct {
	p unsafe.Pointer // *SessionPrefs
}

func (sp *sessionPrefsPtr) Store(prefs *SessionPrefs) {
	atomic.StorePointer(&sp.p, unsafe.Pointer(prefs))
}

func (sp *sessionPrefsPtr) Load() *SessionPrefs {
	return (*SessionPrefs)(atomic.LoadPointer(&sp.p))
}
