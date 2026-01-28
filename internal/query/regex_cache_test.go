package query

import (
	"regexp"
	"sync"
	"testing"

	"github.com/lukaszraczylo/mcp-filepuff/internal/util"
)

// TestCompileRegexCaching tests that regex compilation is cached.
func TestCompileRegexCaching(t *testing.T) {
	// Clear cache before test
	util.ClearRegexCache()

	pattern := `^test_\w+$`

	// First compilation
	re1, err := util.CompileRegex(pattern)
	if err != nil {
		t.Fatalf("First compile failed: %v", err)
	}

	// Second compilation should return cached version
	re2, err := util.CompileRegex(pattern)
	if err != nil {
		t.Fatalf("Second compile failed: %v", err)
	}

	// Should be the exact same object
	if re1 != re2 {
		t.Error("Expected cached regex to be reused, got different objects")
	}
}

// TestCompileRegexConcurrent tests concurrent regex compilation.
func TestCompileRegexConcurrent(t *testing.T) {
	// Clear cache before test
	util.ClearRegexCache()

	pattern := `[a-z]+_\d+`
	const numGoroutines = 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	results := make([]*regexp.Regexp, numGoroutines)
	errors := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		i := i
		go func() {
			defer wg.Done()

			re, err := util.CompileRegex(pattern)
			if err != nil {
				errors <- err
				return
			}

			results[i] = re
		}()
	}

	wg.Wait()
	close(errors)

	// Check for errors
	for err := range errors {
		t.Errorf("Concurrent compile failed: %v", err)
	}

	// All results should be the same object (cached)
	for i := 1; i < numGoroutines; i++ {
		if results[i] != results[0] {
			t.Errorf("Result %d is different from result 0 (cache not working)", i)
		}
	}
}

// TestCompileRegexInvalidPattern tests error handling for invalid patterns.
func TestCompileRegexInvalidPattern(t *testing.T) {
	// Clear cache before test
	util.ClearRegexCache()

	invalidPattern := `[invalid(`

	_, err := util.CompileRegex(invalidPattern)
	if err == nil {
		t.Error("Expected error for invalid pattern, got nil")
	}

	// Verify that a valid pattern still works after an invalid one
	validPattern := `^valid$`
	re, err := util.CompileRegex(validPattern)
	if err != nil {
		t.Errorf("Expected valid pattern to compile, got error: %v", err)
	}
	if re == nil {
		t.Error("Expected non-nil regex for valid pattern")
	}
}

// TestCompileRegexMultiplePatterns tests that different patterns are cached separately.
func TestCompileRegexMultiplePatterns(t *testing.T) {
	// Clear cache before test
	util.ClearRegexCache()

	patterns := []string{
		`^test_\w+$`,
		`^\d{4}-\d{2}-\d{2}$`,
		`^[A-Z][a-z]+$`,
		`\b\w+@\w+\.\w+\b`,
	}

	compiled := make([]*regexp.Regexp, len(patterns))

	// Compile all patterns
	for i, pattern := range patterns {
		re, err := util.CompileRegex(pattern)
		if err != nil {
			t.Fatalf("Compile failed for pattern %s: %v", pattern, err)
		}
		compiled[i] = re
	}

	// All should be different objects (different patterns)
	for i := 0; i < len(compiled); i++ {
		for j := i + 1; j < len(compiled); j++ {
			if compiled[i] == compiled[j] {
				t.Errorf("Pattern %d and %d have same regex object", i, j)
			}
		}
	}

	// Re-compile should return cached versions
	for i, pattern := range patterns {
		re, err := util.CompileRegex(pattern)
		if err != nil {
			t.Fatalf("Re-compile failed for pattern %s: %v", pattern, err)
		}
		if re != compiled[i] {
			t.Errorf("Pattern %s was not cached properly", pattern)
		}
	}
}

// BenchmarkCompileRegex_Uncached benchmarks regex compilation without caching.
func BenchmarkCompileRegex_Uncached(b *testing.B) {
	pattern := `^\w+_[0-9]{3,5}_[a-zA-Z]+$`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = regexp.Compile(pattern)
	}
}

// BenchmarkCompileRegex_Cached benchmarks regex compilation with caching.
func BenchmarkCompileRegex_Cached(b *testing.B) {
	// Clear cache
	util.ClearRegexCache()

	pattern := `^\w+_[0-9]{3,5}_[a-zA-Z]+$`

	// Pre-populate cache
	_, _ = util.CompileRegex(pattern)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = util.CompileRegex(pattern)
	}
}

// BenchmarkCompileRegex_MixedPatterns benchmarks realistic workload with multiple patterns.
func BenchmarkCompileRegex_MixedPatterns(b *testing.B) {
	// Clear cache
	util.ClearRegexCache()

	patterns := []string{
		`^test_\w+$`,
		`^\d{4}-\d{2}-\d{2}$`,
		`^[A-Z][a-z]+$`,
		`\b\w+@\w+\.\w+\b`,
		`^func\s+\w+\(`,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Simulate realistic access pattern
		pattern := patterns[i%len(patterns)]
		_, _ = util.CompileRegex(pattern)
	}
}
