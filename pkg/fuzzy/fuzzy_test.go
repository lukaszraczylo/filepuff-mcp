package fuzzy

import (
	"testing"
)

func TestLevenshteinDistance(t *testing.T) {
	tests := []struct {
		s1       string
		s2       string
		expected int
	}{
		{"", "", 0},
		{"", "abc", 3},
		{"abc", "", 3},
		{"abc", "abc", 0},
		{"abc", "abd", 1},
		{"kitten", "sitting", 3},
		{"saturday", "sunday", 3},
		{"book", "back", 2},
		{"café", "cafe", 1}, // Unicode handling
	}

	for _, tt := range tests {
		got := levenshteinDistance(tt.s1, tt.s2)
		if got != tt.expected {
			t.Errorf("levenshteinDistance(%q, %q) = %d, want %d", tt.s1, tt.s2, got, tt.expected)
		}
	}
}

func TestDamerauLevenshteinDistance(t *testing.T) {
	tests := []struct {
		s1       string
		s2       string
		expected int
	}{
		{"abc", "abc", 0},
		{"abc", "acb", 1}, // Transposition
		{"ca", "abc", 3},  // Delete a, delete b, insert c = 3 operations
		{"", "abc", 3},
	}

	for _, tt := range tests {
		got := DamerauLevenshteinDistance(tt.s1, tt.s2)
		if got != tt.expected {
			t.Errorf("DamerauLevenshteinDistance(%q, %q) = %d, want %d", tt.s1, tt.s2, got, tt.expected)
		}
	}
}

func TestJaroWinklerSimilarity(t *testing.T) {
	tests := []struct {
		s1       string
		s2       string
		minScore float64 // Minimum expected similarity
	}{
		{"", "", 1.0},
		{"abc", "abc", 1.0},
		{"martha", "marhta", 0.96},  // High similarity for transposition
		{"dixon", "dicksonx", 0.76}, // Moderate similarity
		{"", "abc", 0.0},
	}

	for _, tt := range tests {
		got := JaroWinklerSimilarity(tt.s1, tt.s2)
		if got < tt.minScore {
			t.Errorf("JaroWinklerSimilarity(%q, %q) = %.2f, want >= %.2f", tt.s1, tt.s2, got, tt.minScore)
		}
	}
}

func TestMatcher_Match(t *testing.T) {
	m := New(2) // Allow edit distance up to 2

	candidates := []string{
		"getUserName",
		"getUsername",
		"get_user_name",
		"getUserId",
		"setUserName",
		"findUser",
		"userName",
		"usernameField",
	}

	tests := []struct {
		query     string
		topMatch  string
		expectMin int
	}{
		{
			query:     "getUserName",
			expectMin: 3, // Exact + similar variants
			topMatch:  "getUserName",
		},
		{
			query:     "getuser",
			expectMin: 2, // Should match getUserName, getUsername at minimum
			topMatch:  "getUserName",
		},
		{
			query:     "username",
			expectMin: 2, // Case-insensitive matches
			topMatch:  "userName",
		},
	}

	for _, tt := range tests {
		matches := m.Match(tt.query, candidates)

		if len(matches) < tt.expectMin {
			t.Errorf("Match(%q) returned %d matches, want at least %d", tt.query, len(matches), tt.expectMin)
		}

		if len(matches) > 0 {
			// Top match should have highest score
			if matches[0].Score < matches[len(matches)-1].Score {
				t.Errorf("Match(%q) results not sorted by score", tt.query)
			}
		}
	}
}

func TestMatcher_EmptyQuery(t *testing.T) {
	m := New(2)
	candidates := []string{"test", "example"}

	matches := m.Match("", candidates)
	if matches != nil {
		t.Errorf("Match with empty query should return nil, got %v", matches)
	}
}

func TestMatcher_PrefixBonus(t *testing.T) {
	m := New(2)
	candidates := []string{
		"getUserName",  // prefix match
		"findUserName", // contains but not prefix
	}

	matches := m.Match("get", candidates)

	if len(matches) < 1 {
		t.Fatal("Expected at least one match")
	}

	// Prefix match should score higher
	if matches[0].Text != "getUserName" {
		t.Errorf("Expected prefix match to rank first, got %q", matches[0].Text)
	}
}

func TestMatcher_ExactMatchBonus(t *testing.T) {
	m := New(2)
	candidates := []string{
		"test",
		"testing",
		"tester",
	}

	matches := m.Match("test", candidates)

	if len(matches) < 1 {
		t.Fatal("Expected at least one match")
	}

	// Exact match should rank first
	if matches[0].Text != "test" {
		t.Errorf("Expected exact match to rank first, got %q", matches[0].Text)
	}

	// Exact match should have highest score
	if matches[0].Score < 2.0 { // Should have exact match bonus
		t.Errorf("Exact match score too low: %.2f", matches[0].Score)
	}
}

func TestContainsWordBoundary(t *testing.T) {
	tests := []struct {
		text     string
		query    string
		expected bool
	}{
		{"getUserName", "get", true},    // At start
		{"getUserName", "user", true},   // After lowercase->uppercase boundary
		{"get_user_name", "user", true}, // After underscore
		{"getUserName", "Name", true},   // After lowercase->uppercase
		{"getUserName", "ser", false},   // Middle of word
		{"", "test", false},             // Empty text
	}

	for _, tt := range tests {
		got := containsWordBoundary(tt.text, tt.query)
		if got != tt.expected {
			t.Errorf("containsWordBoundary(%q, %q) = %v, want %v", tt.text, tt.query, got, tt.expected)
		}
	}
}

func TestMatcher_UnicodeHandling(t *testing.T) {
	m := New(2)
	candidates := []string{
		"café",
		"resume",
		"naïve",
	}

	// Test with Unicode characters
	matches := m.Match("cafe", candidates)
	if len(matches) == 0 {
		t.Error("Expected matches for Unicode strings")
	}

	// Should find café with small edit distance
	found := false
	for _, match := range matches {
		if match.Text == "café" && match.Distance <= 2 {
			found = true
			break
		}
	}

	if !found {
		t.Error("Failed to fuzzy match Unicode string 'café'")
	}
}

func BenchmarkLevenshteinDistance(b *testing.B) {
	s1 := "the quick brown fox jumps over the lazy dog"
	s2 := "the quikc brown fox jumps ovver the lazy dog"

	b.ResetTimer()
	for i := range b.N {
		_ = levenshteinDistance(s1, s2)
		_ = i // use i to avoid unused warning
	}
}

func BenchmarkDamerauLevenshteinDistance(b *testing.B) {
	s1 := "the quick brown fox jumps over the lazy dog"
	s2 := "the quikc brown fox jumps ovver the lazy dog"

	b.ResetTimer()
	for i := range b.N {
		_ = DamerauLevenshteinDistance(s1, s2)
		_ = i
	}
}

func BenchmarkJaroWinklerSimilarity(b *testing.B) {
	s1 := "martha"
	s2 := "marhta"

	b.ResetTimer()
	for i := range b.N {
		_ = JaroWinklerSimilarity(s1, s2)
		_ = i
	}
}

func BenchmarkMatcher_Match(b *testing.B) {
	m := New(2)
	candidates := []string{
		"getUserName", "getUsername", "get_user_name", "getUserId",
		"setUserName", "findUser", "userName", "usernameField",
		"userAccount", "accountUser", "userProfile", "profileUser",
	}

	b.ResetTimer()
	for i := range b.N {
		_ = m.Match("getuser", candidates)
		_ = i
	}
}
