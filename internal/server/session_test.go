package server

import (
	"testing"
)

// TestParseSessionPrefsEmpty verifies zero-value result for nil/empty input.
func TestParseSessionPrefsEmpty(t *testing.T) {
	p := ParseSessionPrefs(nil)
	if p.ASTQueryFormat != "" {
		t.Errorf("ASTQueryFormat: want \"\", got %q", p.ASTQueryFormat)
	}
	if p.DefaultMaxResults != 0 {
		t.Errorf("DefaultMaxResults: want 0, got %d", p.DefaultMaxResults)
	}
	if p.DefaultCluster != nil {
		t.Errorf("DefaultCluster: want nil, got %v", *p.DefaultCluster)
	}
	if p.CompactRefs != nil {
		t.Errorf("CompactRefs: want nil, got %v", *p.CompactRefs)
	}
	if p.LineNumbers != "" {
		t.Errorf("LineNumbers: want \"\", got %q", p.LineNumbers)
	}
	if p.ResourceLinkThreshold != 0 {
		t.Errorf("ResourceLinkThreshold: want 0, got %d", p.ResourceLinkThreshold)
	}

	// Also test with an empty (non-nil) map.
	p2 := ParseSessionPrefs(map[string]any{})
	if p2.ASTQueryFormat != "" || p2.DefaultMaxResults != 0 {
		t.Error("empty map should produce zero-value prefs")
	}
}

// TestParseSessionPrefsAllFields verifies full round-trip with all supported keys.
func TestParseSessionPrefsAllFields(t *testing.T) {
	raw := map[string]any{
		"terse":                   true, // no-op; should not produce an error
		"default_format":          "compact",
		"default_max_results":     float64(50), // JSON numbers decode as float64
		"default_cluster":         true,
		"compact_refs":            true,
		"line_numbers":            "none",
		"resource_link_threshold": float64(32768),
	}
	p := ParseSessionPrefs(raw)

	if p.ASTQueryFormat != "compact" {
		t.Errorf("ASTQueryFormat: want \"compact\", got %q", p.ASTQueryFormat)
	}
	if p.DefaultMaxResults != 50 {
		t.Errorf("DefaultMaxResults: want 50, got %d", p.DefaultMaxResults)
	}
	if p.DefaultCluster == nil || !*p.DefaultCluster {
		t.Errorf("DefaultCluster: want true, got %v", p.DefaultCluster)
	}
	if p.CompactRefs == nil || !*p.CompactRefs {
		t.Errorf("CompactRefs: want true, got %v", p.CompactRefs)
	}
	if p.LineNumbers != "none" {
		t.Errorf("LineNumbers: want \"none\", got %q", p.LineNumbers)
	}
	if p.ResourceLinkThreshold != 32768 {
		t.Errorf("ResourceLinkThreshold: want 32768, got %d", p.ResourceLinkThreshold)
	}
}

// TestParseSessionPrefsLineNumbersVariants tests all valid line_numbers values.
func TestParseSessionPrefsLineNumbersVariants(t *testing.T) {
	for _, want := range []string{"none", "compact", "full"} {
		p := ParseSessionPrefs(map[string]any{"line_numbers": want})
		if p.LineNumbers != want {
			t.Errorf("line_numbers=%q: got %q", want, p.LineNumbers)
		}
	}
	// Invalid value → ignored (empty string).
	p := ParseSessionPrefs(map[string]any{"line_numbers": "bogus"})
	if p.LineNumbers != "" {
		t.Errorf("invalid line_numbers should be ignored, got %q", p.LineNumbers)
	}
}

// TestParseSessionPrefsFormatVariants tests all valid default_format values.
func TestParseSessionPrefsFormatVariants(t *testing.T) {
	for _, want := range []string{"verbose", "compact", "location"} {
		p := ParseSessionPrefs(map[string]any{"default_format": want})
		if p.ASTQueryFormat != want {
			t.Errorf("default_format=%q: got %q", want, p.ASTQueryFormat)
		}
	}
	// Invalid value → ignored.
	p := ParseSessionPrefs(map[string]any{"default_format": "yaml"})
	if p.ASTQueryFormat != "" {
		t.Errorf("invalid format should be ignored, got %q", p.ASTQueryFormat)
	}
}

// TestParseSessionPrefsTypeMismatch verifies that wrong types are silently ignored.
func TestParseSessionPrefsTypeMismatch(t *testing.T) {
	raw := map[string]any{
		"default_format":          123,     // wrong type (int instead of string)
		"default_max_results":     "fifty", // wrong type
		"default_cluster":         "yes",   // wrong type (string instead of bool)
		"compact_refs":            42,      // wrong type
		"line_numbers":            true,    // wrong type
		"resource_link_threshold": "big",   // wrong type
	}
	p := ParseSessionPrefs(raw)
	if p.ASTQueryFormat != "" {
		t.Errorf("type mismatch for format should produce empty, got %q", p.ASTQueryFormat)
	}
	if p.DefaultMaxResults != 0 {
		t.Errorf("type mismatch for max_results should produce 0, got %d", p.DefaultMaxResults)
	}
	if p.DefaultCluster != nil {
		t.Errorf("type mismatch for cluster should produce nil")
	}
	if p.CompactRefs != nil {
		t.Errorf("type mismatch for compact_refs should produce nil")
	}
	if p.LineNumbers != "" {
		t.Errorf("type mismatch for line_numbers should produce empty, got %q", p.LineNumbers)
	}
	if p.ResourceLinkThreshold != 0 {
		t.Errorf("type mismatch for threshold should produce 0, got %d", p.ResourceLinkThreshold)
	}
}

// TestParseSessionPrefsNegativeValues verifies negative numbers are rejected.
func TestParseSessionPrefsNegativeValues(t *testing.T) {
	p := ParseSessionPrefs(map[string]any{
		"default_max_results":     float64(-5),
		"resource_link_threshold": float64(-1),
	})
	if p.DefaultMaxResults != 0 {
		t.Errorf("negative max_results should be rejected, got %d", p.DefaultMaxResults)
	}
	if p.ResourceLinkThreshold != 0 {
		t.Errorf("negative threshold should be rejected, got %d", p.ResourceLinkThreshold)
	}
}

// TestParseSessionPrefsIntCoercion verifies int and int64 inputs also work.
func TestParseSessionPrefsIntCoercion(t *testing.T) {
	p := ParseSessionPrefs(map[string]any{
		"default_max_results":     int(25),
		"resource_link_threshold": int64(16384),
	})
	if p.DefaultMaxResults != 25 {
		t.Errorf("int max_results: want 25, got %d", p.DefaultMaxResults)
	}
	if p.ResourceLinkThreshold != 16384 {
		t.Errorf("int64 threshold: want 16384, got %d", p.ResourceLinkThreshold)
	}
}

// TestParseSessionPrefsClusterFalse ensures default_cluster=false stores a non-nil false.
func TestParseSessionPrefsClusterFalse(t *testing.T) {
	p := ParseSessionPrefs(map[string]any{"default_cluster": false})
	if p.DefaultCluster == nil {
		t.Error("default_cluster=false should store non-nil pointer")
	}
	if *p.DefaultCluster != false {
		t.Error("default_cluster=false: want false pointer")
	}
}

// TestSessionPrefsAtomicStore verifies sessionPrefsPtr is readable after Store.
func TestSessionPrefsAtomicStore(t *testing.T) {
	var sp sessionPrefsPtr
	if sp.Load() != nil {
		t.Error("uninitialised Load() should return nil")
	}

	prefs := ParseSessionPrefs(map[string]any{"default_format": "compact"})
	sp.Store(&prefs)

	loaded := sp.Load()
	if loaded == nil {
		t.Fatal("Load() returned nil after Store")
	}
	if loaded.ASTQueryFormat != "compact" {
		t.Errorf("loaded format: want compact, got %q", loaded.ASTQueryFormat)
	}
}
