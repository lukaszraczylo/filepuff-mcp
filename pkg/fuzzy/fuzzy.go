// Package fuzzy provides fuzzy string matching using Levenshtein distance.
package fuzzy

import (
	"sort"
	"strings"
	"unicode"
)

// Match represents a fuzzy match result.
type Match struct {
	Text       string
	Distance   int
	Similarity float64
	Score      float64
}

// Matcher provides fuzzy matching capabilities.
type Matcher struct {
	threshold int
}

// New creates a new fuzzy matcher with the given threshold.
// Threshold is the maximum edit distance to consider a match (typically 1-3).
func New(threshold int) *Matcher {
	return &Matcher{
		threshold: threshold,
	}
}

// Match performs fuzzy matching of query against candidates.
func (m *Matcher) Match(query string, candidates []string) []Match {
	if query == "" {
		return nil
	}

	matches := make([]Match, 0, len(candidates)/10)
	queryLower := strings.ToLower(query)

	for _, candidate := range candidates {
		candidateLower := strings.ToLower(candidate)

		// Calculate Levenshtein distance
		dist := levenshteinDistance(queryLower, candidateLower)

		// Skip if distance exceeds threshold
		if dist > m.threshold {
			// Check if it's a substring match (important for identifiers)
			if !strings.Contains(candidateLower, queryLower) {
				continue
			}
			// Allow substring matches even if edit distance is high
		}

		// Calculate similarity (0.0 to 1.0)
		maxLen := max(len(query), len(candidate))
		similarity := 1.0 - float64(dist)/float64(maxLen)

		// Calculate composite score
		score := m.calculateScore(queryLower, candidateLower, dist, similarity)

		matches = append(matches, Match{
			Text:       candidate,
			Distance:   dist,
			Similarity: similarity,
			Score:      score,
		})
	}

	// Sort by score descending
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].Score > matches[j].Score
	})

	return matches
}

// calculateScore computes a composite score considering multiple factors.
func (m *Matcher) calculateScore(query, candidate string, dist int, similarity float64) float64 {
	score := similarity

	// Bonus for exact match
	if query == candidate {
		score += 2.0
	}

	// Bonus for prefix match (important for identifier search)
	if strings.HasPrefix(candidate, query) {
		score += 1.0
	}

	// Bonus for word boundary matches (e.g., "getName" matches "get")
	if containsWordBoundary(candidate, query) {
		score += 0.5
	}

	// Penalty for length difference (prefer similar-length matches)
	lenDiff := abs(len(candidate) - len(query))
	score -= float64(lenDiff) * 0.01

	// Penalty for edit distance
	score -= float64(dist) * 0.1

	return score
}

// levenshteinDistance computes the Levenshtein distance between two strings.
// Uses the Wagner-Fischer algorithm with space optimization O(min(m,n)).
func levenshteinDistance(s1, s2 string) int {
	if s1 == s2 {
		return 0
	}
	if len(s1) == 0 {
		return len(s2)
	}
	if len(s2) == 0 {
		return len(s1)
	}

	// Ensure s1 is the shorter string for space optimization
	if len(s1) > len(s2) {
		s1, s2 = s2, s1
	}

	// Use rune slices to handle Unicode properly
	r1 := []rune(s1)
	r2 := []rune(s2)
	len1 := len(r1)
	len2 := len(r2)

	// Only need two rows of the matrix
	previous := make([]int, len2+1)
	current := make([]int, len2+1)

	// Initialize first row
	for j := 0; j <= len2; j++ {
		previous[j] = j
	}

	// Calculate edit distance
	for i := 1; i <= len1; i++ {
		current[0] = i

		for j := 1; j <= len2; j++ {
			cost := 1
			if r1[i-1] == r2[j-1] {
				cost = 0
			}

			current[j] = min(
				previous[j]+1,      // deletion
				current[j-1]+1,     // insertion
				previous[j-1]+cost, // substitution
			)
		}

		// Swap rows
		previous, current = current, previous
	}

	return previous[len2]
}

// DamerauLevenshteinDistance computes Damerau-Levenshtein distance (includes transpositions).
// This is more accurate for typos where adjacent characters are swapped.
func DamerauLevenshteinDistance(s1, s2 string) int {
	if s1 == s2 {
		return 0
	}
	if len(s1) == 0 {
		return len(s2)
	}
	if len(s2) == 0 {
		return len(s1)
	}

	r1 := []rune(s1)
	r2 := []rune(s2)
	len1 := len(r1)
	len2 := len(r2)

	// Create distance matrix
	d := make([][]int, len1+1)
	for i := range d {
		d[i] = make([]int, len2+1)
	}

	// Initialize first row and column
	for i := 0; i <= len1; i++ {
		d[i][0] = i
	}
	for j := 0; j <= len2; j++ {
		d[0][j] = j
	}

	// Calculate distances
	for i := 1; i <= len1; i++ {
		for j := 1; j <= len2; j++ {
			cost := 1
			if r1[i-1] == r2[j-1] {
				cost = 0
			}

			d[i][j] = min(
				d[i-1][j]+1,      // deletion
				d[i][j-1]+1,      // insertion
				d[i-1][j-1]+cost, // substitution
			)

			// Check for transposition
			if i > 1 && j > 1 && r1[i-1] == r2[j-2] && r1[i-2] == r2[j-1] {
				d[i][j] = min(d[i][j], d[i-2][j-2]+cost)
			}
		}
	}

	return d[len1][len2]
}

// JaroWinklerSimilarity computes Jaro-Winkler similarity (0.0 to 1.0).
// Better for short strings and names.
func JaroWinklerSimilarity(s1, s2 string) float64 {
	if s1 == s2 {
		return 1.0
	}

	r1 := []rune(s1)
	r2 := []rune(s2)

	if len(r1) == 0 || len(r2) == 0 {
		return 0.0
	}

	// Calculate Jaro similarity first
	jaro := jaroSimilarity(r1, r2)

	// Calculate common prefix length (up to 4 characters)
	prefixLen := 0
	for i := 0; i < min(min(len(r1), len(r2)), 4); i++ {
		if r1[i] == r2[i] {
			prefixLen++
		} else {
			break
		}
	}

	// Jaro-Winkler adds bonus for common prefix
	const p = 0.1
	return jaro + float64(prefixLen)*p*(1.0-jaro)
}

// jaroSimilarity computes Jaro similarity.
func jaroSimilarity(r1, r2 []rune) float64 {
	len1 := len(r1)
	len2 := len(r2)

	// Maximum allowed distance
	matchDist := max(len1, len2)/2 - 1
	if matchDist < 0 {
		matchDist = 0
	}

	matched1 := make([]bool, len1)
	matched2 := make([]bool, len2)

	matches := 0
	transpositions := 0

	// Find matches
	for i := range len1 {
		start := max(0, i-matchDist)
		end := min(i+matchDist+1, len2)

		for j := start; j < end; j++ {
			if matched2[j] || r1[i] != r2[j] {
				continue
			}
			matched1[i] = true
			matched2[j] = true
			matches++
			break
		}
	}

	if matches == 0 {
		return 0.0
	}

	// Count transpositions
	k := 0
	for i := range len1 {
		if !matched1[i] {
			continue
		}
		for !matched2[k] {
			k++
		}
		if r1[i] != r2[k] {
			transpositions++
		}
		k++
	}

	return (float64(matches)/float64(len1) +
		float64(matches)/float64(len2) +
		float64(matches-transpositions/2)/float64(matches)) / 3.0
}

// containsWordBoundary checks if query appears at word boundaries in text.
func containsWordBoundary(text, query string) bool {
	textLower := strings.ToLower(text)
	queryLower := strings.ToLower(query)

	idx := strings.Index(textLower, queryLower)
	if idx == -1 {
		return false
	}

	// Check if match is at start
	if idx == 0 {
		return true
	}

	// Check for underscore or non-alphanumeric boundary
	prevRune := rune(text[idx-1])
	if !unicode.IsLetter(prevRune) && !unicode.IsDigit(prevRune) {
		return true
	}

	// Check for camelCase boundary (lowercase before uppercase)
	if idx > 0 && len(text) > idx {
		curr := rune(text[idx])
		prev := rune(text[idx-1])
		if unicode.IsLower(prev) && unicode.IsUpper(curr) {
			return true
		}
	}

	return false
}

// Helper functions

func min(values ...int) int {
	if len(values) == 0 {
		return 0
	}
	m := values[0]
	for _, v := range values[1:] {
		if v < m {
			m = v
		}
	}
	return m
}

func max(values ...int) int {
	if len(values) == 0 {
		return 0
	}
	m := values[0]
	for _, v := range values[1:] {
		if v > m {
			m = v
		}
	}
	return m
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
