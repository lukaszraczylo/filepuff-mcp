package query

import (
	"regexp"
	"sync"
	"testing"
)

// TestCompileRegexCaching tests that regex compilation is cached.
func TestCompileRegexCaching(t *testing.T) {
	// Clear cache before test
	regexCache = sync.Map{}

	pattern := `^test_\w+$`

	// First compilation
	re1, err := compileRegex(pattern)
	if err != nil {
		t.Fatalf("First compile failed: %v", err)
	}

	// Second compilation should return cached version
	re2, err := compileRegex(pattern)
	if err != nil {
		t.Fatalf("Second compile failed: %v", err)
	}

	// Should be the exact same object
	if re1 != re2 {
		t.Error("Expected cached regex to be reused, got different objects")
	}

	// Verify it's in the cache
	cached, ok := regexCache.Load(pattern)
	if !ok {
		t.Error("Pattern not found in cache")
	}

	if cached.(*regexp.Regexp) != re1 {
		t.Error("Cached regex doesn't match returned regex")
	}
}

// TestCompileRegexConcurrent tests concurrent regex compilation.
func TestCompileRegexConcurrent(t *testing.T) {
	// Clear cache before test
	regexCache = sync.Map{}

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

			re, err := compileRegex(pattern)
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
	regexCache = sync.Map{}

	invalidPattern := `[invalid(`

	_, err := compileRegex(invalidPattern)
	if err == nil {
		t.Error("Expected error for invalid pattern, got nil")
	}

	// Invalid patterns should not be cached
	_, ok := regexCache.Load(invalidPattern)
	if ok {
		t.Error("Invalid pattern should not be cached")
	}
}

// TestCompileRegexMultiplePatterns tests that different patterns are cached separately.
func TestCompileRegexMultiplePatterns(t *testing.T) {
	// Clear cache before test
	regexCache = sync.Map{}

	patterns := []string{
		`^test_\w+$`,
		`^\d{4}-\d{2}-\d{2}$`,
		`^[A-Z][a-z]+$`,
		`\b\w+@\w+\.\w+\b`,
	}

	compiled := make([]*regexp.Regexp, len(patterns))

	// Compile all patterns
	for i, pattern := range patterns {
		re, err := compileRegex(pattern)
		if err != nil {
			t.Fatalf("Compile failed for pattern %s: %v", pattern, err)
		}
		compiled[i] = re
	}

	// Verify all are cached
	for i, pattern := range patterns {
		cached, ok := regexCache.Load(pattern)
		if !ok {
			t.Errorf("Pattern %s not in cache", pattern)
		}

		if cached.(*regexp.Regexp) != compiled[i] {
			t.Errorf("Cached regex for %s doesn't match compiled version", pattern)
		}
	}

	// All should be different objects
	for i := 0; i < len(compiled); i++ {
		for j := i + 1; j < len(compiled); j++ {
			if compiled[i] == compiled[j] {
				t.Errorf("Pattern %d and %d have same regex object", i, j)
			}
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
	regexCache = sync.Map{}

	pattern := `^\w+_[0-9]{3,5}_[a-zA-Z]+$`

	// Pre-populate cache
	_, _ = compileRegex(pattern)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = compileRegex(pattern)
	}
}

// BenchmarkCompileRegex_MixedPatterns benchmarks realistic workload with multiple patterns.
func BenchmarkCompileRegex_MixedPatterns(b *testing.B) {
	// Clear cache
	regexCache = sync.Map{}

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
		_, _ = compileRegex(pattern)
	}
}
