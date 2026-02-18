package util

import (
	"errors"
	"strings"
	"sync"
	"testing"
)

func TestValidatePattern(t *testing.T) {
	tests := []struct {
		name      string
		pattern   string
		expectErr bool
	}{
		{
			name:      "valid short pattern",
			pattern:   "^hello.*world$",
			expectErr: false,
		},
		{
			name:      "valid empty pattern",
			pattern:   "",
			expectErr: false,
		},
		{
			name:      "valid pattern at max length",
			pattern:   strings.Repeat("a", MaxPatternLength),
			expectErr: false,
		},
		{
			name:      "pattern too long",
			pattern:   strings.Repeat("a", MaxPatternLength+1),
			expectErr: true,
		},
		{
			name:      "very long pattern",
			pattern:   strings.Repeat("x", MaxPatternLength*2),
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePattern(tt.pattern)
			if tt.expectErr && err == nil {
				t.Error("expected error but got nil")
			}
			if !tt.expectErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestCompileRegex(t *testing.T) {
	// Clear cache before each test
	ClearRegexCache()

	t.Run("valid pattern is compiled and cached", func(t *testing.T) {
		ClearRegexCache()

		pattern := "^test.*pattern$"
		re1, err := CompileRegex(pattern)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if re1 == nil {
			t.Fatal("expected non-nil regex")
		}

		// Second call should return cached version
		re2, err := CompileRegex(pattern)
		if err != nil {
			t.Fatalf("unexpected error on second call: %v", err)
		}

		// Should be the same pointer
		if re1 != re2 {
			t.Error("expected same regex instance from cache")
		}

		// Cache should have one entry
		if stats := CacheStats(); stats != 1 {
			t.Errorf("expected cache size 1, got %d", stats)
		}
	})

	t.Run("invalid pattern returns error", func(t *testing.T) {
		ClearRegexCache()

		pattern := "[invalid(regex"
		_, err := CompileRegex(pattern)
		if err == nil {
			t.Fatal("expected error for invalid regex")
		}

		var regexErr *RegexError
		if !errors.As(err, &regexErr) {
			t.Errorf("expected RegexError, got %T", err)
		}
	})

	t.Run("pattern too long returns error", func(t *testing.T) {
		ClearRegexCache()

		pattern := strings.Repeat("a", MaxPatternLength+1)
		_, err := CompileRegex(pattern)
		if err == nil {
			t.Fatal("expected error for long pattern")
		}

		var regexErr *RegexError
		if !errors.As(err, &regexErr) {
			t.Errorf("expected RegexError, got %T", err)
		}
	})

	t.Run("different patterns are cached separately", func(t *testing.T) {
		ClearRegexCache()

		re1, _ := CompileRegex("pattern1")
		re2, _ := CompileRegex("pattern2")

		if re1 == re2 {
			t.Error("different patterns should produce different regex instances")
		}

		if stats := CacheStats(); stats != 2 {
			t.Errorf("expected cache size 2, got %d", stats)
		}
	})

	t.Run("regex matches correctly", func(t *testing.T) {
		ClearRegexCache()

		re, err := CompileRegex("^hello\\s+world$")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !re.MatchString("hello world") {
			t.Error("expected match for 'hello world'")
		}
		if !re.MatchString("hello   world") {
			t.Error("expected match for 'hello   world'")
		}
		if re.MatchString("helloworld") {
			t.Error("unexpected match for 'helloworld'")
		}
	})
}

func TestCompileRegexUncached(t *testing.T) {
	ClearRegexCache()

	t.Run("valid pattern compiles without caching", func(t *testing.T) {
		initialSize := CacheStats()

		re, err := CompileRegexUncached("^uncached.*pattern$")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if re == nil {
			t.Fatal("expected non-nil regex")
		}

		// Cache size should not change
		if stats := CacheStats(); stats != initialSize {
			t.Errorf("cache size changed from %d to %d", initialSize, stats)
		}
	})

	t.Run("invalid pattern returns error", func(t *testing.T) {
		_, err := CompileRegexUncached("[invalid")
		if err == nil {
			t.Fatal("expected error for invalid regex")
		}
	})

	t.Run("pattern too long returns error", func(t *testing.T) {
		pattern := strings.Repeat("x", MaxPatternLength+1)
		_, err := CompileRegexUncached(pattern)
		if err == nil {
			t.Fatal("expected error for long pattern")
		}
	})
}

func TestClearRegexCache(t *testing.T) {
	// Add some patterns
	_, _ = CompileRegex("pattern1")
	_, _ = CompileRegex("pattern2")
	_, _ = CompileRegex("pattern3")

	if stats := CacheStats(); stats < 3 {
		t.Fatalf("expected at least 3 cached patterns, got %d", stats)
	}

	ClearRegexCache()

	if stats := CacheStats(); stats != 0 {
		t.Errorf("expected cache size 0 after clear, got %d", stats)
	}
}

func TestCacheStats(t *testing.T) {
	ClearRegexCache()

	if stats := CacheStats(); stats != 0 {
		t.Errorf("expected initial cache size 0, got %d", stats)
	}

	_, _ = CompileRegex("a")
	if stats := CacheStats(); stats != 1 {
		t.Errorf("expected cache size 1, got %d", stats)
	}

	_, _ = CompileRegex("b")
	if stats := CacheStats(); stats != 2 {
		t.Errorf("expected cache size 2, got %d", stats)
	}

	// Same pattern should not increase cache size
	_, _ = CompileRegex("a")
	if stats := CacheStats(); stats != 2 {
		t.Errorf("expected cache size 2 after duplicate, got %d", stats)
	}
}
func TestConcurrentAccess(t *testing.T) {
	ClearRegexCache()

	var wg sync.WaitGroup
	numGoroutines := 100
	numPatterns := 10

	// Generate some patterns
	patterns := make([]string, numPatterns)
	for i := range patterns {
		patterns[i] = strings.Repeat("p", i+1)
	}

	// Concurrent compilation of same patterns
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			pattern := patterns[id%numPatterns]
			re, err := CompileRegex(pattern)
			if err != nil {
				t.Errorf("goroutine %d: unexpected error: %v", id, err)
				return
			}
			if re == nil {
				t.Errorf("goroutine %d: nil regex returned", id)
			}
		}(i)
	}

	wg.Wait()

	// Should have exactly numPatterns cached
	if stats := CacheStats(); stats != int64(numPatterns) {
		t.Errorf("expected cache size %d, got %d", numPatterns, stats)
	}
}

func TestRegexError(t *testing.T) {
	t.Run("error message with underlying error", func(t *testing.T) {
		underlying := errors.New("underlying error")
		err := &RegexError{
			Pattern: "test.*",
			Reason:  "test reason",
			Err:     underlying,
		}

		msg := err.Error()
		if !strings.Contains(msg, "test.*") {
			t.Error("error message should contain pattern")
		}
		if !strings.Contains(msg, "test reason") {
			t.Error("error message should contain reason")
		}
		if !strings.Contains(msg, "underlying error") {
			t.Error("error message should contain underlying error")
		}
	})

	t.Run("error message without underlying error", func(t *testing.T) {
		err := &RegexError{
			Pattern: "test.*",
			Reason:  "test reason",
			Err:     nil,
		}

		msg := err.Error()
		if !strings.Contains(msg, "test.*") {
			t.Error("error message should contain pattern")
		}
		if !strings.Contains(msg, "test reason") {
			t.Error("error message should contain reason")
		}
	})

	t.Run("error unwrap", func(t *testing.T) {
		underlying := errors.New("underlying")
		err := &RegexError{
			Pattern: "test",
			Reason:  "reason",
			Err:     underlying,
		}

		if errors.Unwrap(err) != underlying {
			t.Error("Unwrap should return underlying error")
		}
	})
}

func TestTruncatePattern(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "short pattern unchanged",
			input:    "short",
			expected: "short",
		},
		{
			name:     "exactly 50 chars unchanged",
			input:    strings.Repeat("x", 50),
			expected: strings.Repeat("x", 50),
		},
		{
			name:     "long pattern truncated",
			input:    strings.Repeat("x", 60),
			expected: strings.Repeat("x", 47) + "...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncatePattern(tt.input)
			if got != tt.expected {
				t.Errorf("truncatePattern() = %q (len %d), want %q (len %d)",
					got, len(got), tt.expected, len(tt.expected))
			}
		})
	}
}

// BenchmarkCompileRegex benchmarks regex compilation with caching
func BenchmarkCompileRegex(b *testing.B) {
	ClearRegexCache()
	pattern := "^test.*pattern\\d+$"

	// First call to populate cache
	_, _ = CompileRegex(pattern)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = CompileRegex(pattern)
	}
}

// BenchmarkCompileRegexUncached benchmarks regex compilation without caching
func BenchmarkCompileRegexUncached(b *testing.B) {
	pattern := "^test.*pattern\\d+$"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = CompileRegexUncached(pattern)
	}
}
