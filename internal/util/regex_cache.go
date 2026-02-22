// Package util provides shared utility functions and caches.
package util

import (
	"fmt"
	"regexp"
	"sync"
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
// Uses sync.RWMutex with a regular map so that ClearRegexCache can atomically
// clear the map and reset the count in a single lock acquisition.
var (
	cacheMu    sync.RWMutex
	regexCache = make(map[string]*regexp.Regexp)
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
// Thread-safe: uses RWMutex to prevent race conditions.
// Returns the compiled regex or an error if the pattern is invalid or unsafe.
func CompileRegex(pattern string) (*regexp.Regexp, error) {
	// Validate pattern first
	if err := ValidatePattern(pattern); err != nil {
		return nil, err
	}

	// Check cache first (read lock)
	cacheMu.RLock()
	if cached, ok := regexCache[pattern]; ok {
		cacheMu.RUnlock()
		return cached, nil
	}
	cacheMu.RUnlock()

	// Compile regex outside the lock to avoid holding it during compilation
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, &RegexError{
			Pattern: truncatePattern(pattern),
			Reason:  "invalid regex syntax",
			Err:     err,
		}
	}

	// Write lock to store in cache
	cacheMu.Lock()
	// Re-check in case another goroutine stored it while we were compiling
	if cached, ok := regexCache[pattern]; ok {
		cacheMu.Unlock()
		return cached, nil
	}

	// Check cache size and clear if too large
	if len(regexCache) >= MaxCacheSize {
		regexCache = make(map[string]*regexp.Regexp)
	}

	regexCache[pattern] = re
	cacheMu.Unlock()
	return re, nil
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
// Atomically replaces the map under a single write lock.
func ClearRegexCache() {
	cacheMu.Lock()
	regexCache = make(map[string]*regexp.Regexp)
	cacheMu.Unlock()
}

// CacheStats returns the current number of cached patterns.
func CacheStats() int64 {
	cacheMu.RLock()
	n := int64(len(regexCache))
	cacheMu.RUnlock()
	return n
}

// truncatePattern truncates a pattern for display in error messages.
func truncatePattern(pattern string) string {
	if len(pattern) > 50 {
		return pattern[:47] + "..."
	}
	return pattern
}
