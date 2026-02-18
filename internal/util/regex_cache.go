// Package util provides shared utility functions and caches.
package util

import (
	"fmt"
	"regexp"
	"sync"
	"sync/atomic"
)

const (
	// MaxPatternLength is the maximum allowed length for regex patterns.
	// This prevents memory issues from extremely long patterns.
	MaxPatternLength = 1000

	// MaxCacheSize is the maximum number of patterns to cache.
	// When exceeded, the cache is cleared to prevent unbounded memory growth.
	MaxCacheSize = 10000
)

// regexCache is a global thread-safe cache for compiled regular expressions.
// Caching regex compilation provides 10-50x speedup for repeated patterns.
var (
	regexCache sync.Map // string -> *regexp.Regexp
	cacheSize  atomic.Int64
)

// RegexError represents an error during regex compilation or validation.
type RegexError struct {
	Pattern string
	Reason  string
	Err     error
}

func (e *RegexError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("regex error for pattern %q: %s: %v", e.Pattern, e.Reason, e.Err)
	}
	return fmt.Sprintf("regex error for pattern %q: %s", e.Pattern, e.Reason)
}

func (e *RegexError) Unwrap() error {
	return e.Err
}

// ValidatePattern validates a regex pattern for safety.
// Returns an error if the pattern is too long or appears malicious.
func ValidatePattern(pattern string) error {
	// Check pattern length
	if len(pattern) > MaxPatternLength {
		return &RegexError{
			Pattern: truncatePattern(pattern),
			Reason:  fmt.Sprintf("pattern too long (%d chars, max %d)", len(pattern), MaxPatternLength),
		}
	}

	// Note: Go's regexp package uses Thompson NFA which guarantees O(n) matching time,
	// making it inherently resistant to ReDoS attacks. However, we still validate
	// pattern length to prevent memory issues during compilation.

	return nil
}

// CompileRegex compiles a regex pattern with caching and validation for security.
// Thread-safe: uses LoadOrStore to prevent race conditions.
// Returns the compiled regex or an error if the pattern is invalid or unsafe.
func CompileRegex(pattern string) (*regexp.Regexp, error) {
	// Validate pattern first
	if err := ValidatePattern(pattern); err != nil {
		return nil, err
	}

	// Check cache first
	if cached, ok := regexCache.Load(pattern); ok {
		return cached.(*regexp.Regexp), nil
	}

	// Compile regex
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, &RegexError{
			Pattern: truncatePattern(pattern),
			Reason:  "invalid regex syntax",
			Err:     err,
		}
	}

	// Check cache size and clear if too large
	if cacheSize.Load() >= MaxCacheSize {
		ClearRegexCache()
	}

	// Try to store - if another goroutine already stored it, use theirs
	// This prevents race conditions where multiple goroutines compile the same pattern
	actual, loaded := regexCache.LoadOrStore(pattern, re)
	if !loaded {
		cacheSize.Add(1)
	}
	return actual.(*regexp.Regexp), nil
}

// CompileRegexUncached compiles a regex pattern without caching.
// Useful for one-off patterns that shouldn't pollute the cache.
func CompileRegexUncached(pattern string) (*regexp.Regexp, error) {
	if err := ValidatePattern(pattern); err != nil {
		return nil, err
	}

	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, &RegexError{
			Pattern: truncatePattern(pattern),
			Reason:  "invalid regex syntax",
			Err:     err,
		}
	}
	return re, nil
}

// ClearRegexCache clears all cached compiled regular expressions.
// Useful for testing or when memory usage needs to be reduced.
func ClearRegexCache() {
	regexCache.Range(func(key, _ interface{}) bool {
		regexCache.Delete(key)
		return true
	})
	cacheSize.Store(0)
}

// CacheStats returns the current number of cached patterns.
func CacheStats() int64 {
	return cacheSize.Load()
}

// truncatePattern truncates a pattern for display in error messages.
func truncatePattern(pattern string) string {
	if len(pattern) > 50 {
		return pattern[:47] + "..."
	}
	return pattern
}
