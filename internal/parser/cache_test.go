package parser

import (
	"context"
	"fmt"
	"testing"
)

// TestLRUCacheEviction tests that the LRU cache properly evicts old entries.
func TestLRUCacheEviction(t *testing.T) {
	registry := NewRegistry()
	ctx := context.Background()

	// Create 101 unique Go files (cache size is 100)
	for i := 0; i < 101; i++ {
		content := []byte(fmt.Sprintf("package main\n\nfunc test%d() {}\n", i))
		filename := "test.go"

		_, err := registry.Parse(ctx, filename, content)
		if err != nil {
			t.Fatalf("Parse failed for iteration %d: %v", i, err)
		}
	}

	// The LRU cache should have evicted the oldest entry
	// Verify cache size is capped at 100
	cacheLen := registry.cache.Len()
	if cacheLen > 100 {
		t.Errorf("Cache size %d exceeds max size 100", cacheLen)
	}
}

// TestCacheHit tests that repeated parsing of the same content uses cache.
func TestCacheHit(t *testing.T) {
	registry := NewRegistry()
	ctx := context.Background()

	content := []byte("package main\n\nfunc test() {}\n")
	filename := "test.go"

	// First parse
	result1, err := registry.Parse(ctx, filename, content)
	if err != nil {
		t.Fatalf("First parse failed: %v", err)
	}

	// Second parse should use cache
	result2, err := registry.Parse(ctx, filename, content)
	if err != nil {
		t.Fatalf("Second parse failed: %v", err)
	}

	// The tree should be the same object (cached)
	if result1.Tree != result2.Tree {
		t.Error("Expected cached tree to be reused, but got different tree objects")
	}
}

// TestContentHashCollisionResistance tests that different content produces different hashes.
func TestContentHashCollisionResistance(t *testing.T) {
	testCases := []struct {
		name     string
		content1 []byte
		content2 []byte
	}{
		{
			name:     "different content",
			content1: []byte("package main"),
			content2: []byte("package test"),
		},
		{
			name:     "same prefix different suffix",
			content1: []byte("package main\nfunc a() {}"),
			content2: []byte("package main\nfunc b() {}"),
		},
		{
			name:     "different length",
			content1: []byte("short"),
			content2: []byte("much longer content here"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			hash1 := contentHash(tc.content1)
			hash2 := contentHash(tc.content2)

			if hash1 == hash2 {
				t.Errorf("Hash collision: %s == %s for different content", hash1, hash2)
			}
		})
	}
}

// TestContentHashConsistency tests that the same content always produces the same hash.
func TestContentHashConsistency(t *testing.T) {
	content := []byte("package main\n\nfunc test() {}\n")

	hash1 := contentHash(content)
	hash2 := contentHash(content)
	hash3 := contentHash(content)

	if hash1 != hash2 || hash2 != hash3 {
		t.Errorf("Hash inconsistency: %s, %s, %s", hash1, hash2, hash3)
	}
}

// BenchmarkContentHash_xxHash benchmarks the xxHash implementation.
func BenchmarkContentHash_xxHash(b *testing.B) {
	// Typical file content size (10KB)
	content := make([]byte, 10*1024)
	for i := range content {
		content[i] = byte(i % 256)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = contentHash(content)
	}
}

// BenchmarkCacheHitRate benchmarks cache performance with realistic workload.
func BenchmarkCacheHitRate(b *testing.B) {
	registry := NewRegistry()
	ctx := context.Background()

	// Create a set of common files that get parsed repeatedly
	files := [][]byte{
		[]byte("package main\n\nfunc main() {}\n"),
		[]byte("package test\n\nimport \"testing\"\n"),
		[]byte("package util\n\nfunc helper() string { return \"\" }\n"),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Simulate realistic access pattern with cache hits
		content := files[i%len(files)]
		_, _ = registry.Parse(ctx, "test.go", content)
	}
}
