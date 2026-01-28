// Package util provides shared utility functions and caches.
package util

import (
	"regexp"
	"sync"
)

// regexCache is a global thread-safe cache for compiled regular expressions.
// Caching regex compilation provides 10-50x speedup for repeated patterns.
var regexCache sync.Map // string -> *regexp.Regexp

// CompileRegex compiles a regex pattern with caching for performance.
// Thread-safe: uses LoadOrStore to prevent race conditions.
// Returns the compiled regex or an error if the pattern is invalid.
func CompileRegex(pattern string) (*regexp.Regexp, error) {
	// Check cache first
	if cached, ok := regexCache.Load(pattern); ok {
		return cached.(*regexp.Regexp), nil
	}

	// Compile regex
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}

	// Try to store - if another goroutine already stored it, use theirs
	// This prevents race conditions where multiple goroutines compile the same pattern
	actual, _ := regexCache.LoadOrStore(pattern, re)
	return actual.(*regexp.Regexp), nil
}

// ClearRegexCache clears all cached compiled regular expressions.
// Useful for testing or when memory usage needs to be reduced.
func ClearRegexCache() {
	regexCache.Range(func(key, value interface{}) bool {
		regexCache.Delete(key)
		return true
	})
}
