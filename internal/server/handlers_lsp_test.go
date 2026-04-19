package server

import (
	"strings"
	"testing"

	"github.com/lukaszraczylo/mcp-filepuff/internal/lsp"
)

// makeLocation builds an lsp.Location with a file:// URI.
func makeLocation(file string, line, col int) lsp.Location {
	return lsp.Location{
		URI: "file://" + file,
		Range: lsp.Range{
			Start: lsp.Position{Line: line - 1, Character: col - 1},
			End:   lsp.Position{Line: line - 1, Character: col - 1},
		},
	}
}

// groupLocations is a helper that mirrors the grouping logic in lspReferences.
func groupLocations(locations []lsp.Location) (map[string][]lsp.Location, []string) {
	fileGroups := make(map[string][]lsp.Location)
	fileOrder := make([]string, 0)
	for _, loc := range locations {
		filePath := lsp.URIToFile(loc.URI)
		if _, seen := fileGroups[filePath]; !seen {
			fileOrder = append(fileOrder, filePath)
		}
		fileGroups[filePath] = append(fileGroups[filePath], loc)
	}
	return fileGroups, fileOrder
}

// TestFormatReferencesVerbose verifies the default (non-compact) format is unchanged.
func TestFormatReferencesVerbose(t *testing.T) {
	locs := []lsp.Location{
		makeLocation("/a/foo.go", 12, 5),
		makeLocation("/a/foo.go", 13, 8),
		makeLocation("/a/bar.go", 15, 1),
	}
	groups, order := groupLocations(locs)
	out := formatReferences(groups, order, len(locs), false, true)

	if !strings.Contains(out, "Found 3 reference(s):") {
		t.Errorf("missing header, got:\n%s", out)
	}
	if !strings.Contains(out, "**") {
		t.Errorf("verbose format should use **file** markers, got:\n%s", out)
	}
	if !strings.Contains(out, "L12:5") {
		t.Errorf("missing L12:5 in verbose output, got:\n%s", out)
	}
	if !strings.Contains(out, "L13:8") {
		t.Errorf("missing L13:8 in verbose output, got:\n%s", out)
	}
	if !strings.Contains(out, "L15:1") {
		t.Errorf("missing L15:1 in verbose output, got:\n%s", out)
	}
}

// TestFormatReferencesCompactBasic verifies compact output for distinct lines.
func TestFormatReferencesCompactBasic(t *testing.T) {
	locs := []lsp.Location{
		makeLocation("/a/foo.go", 12, 5),
		makeLocation("/a/foo.go", 13, 8),
		makeLocation("/a/foo.go", 15, 12),
	}
	groups, order := groupLocations(locs)
	out := formatReferences(groups, order, len(locs), true, true)

	if !strings.Contains(out, "Found 3 reference(s):") {
		t.Errorf("missing header, got:\n%s", out)
	}
	// Should contain "foo.go:[12:5, 13:8, 15:12]  (3)"
	if !strings.Contains(out, "12:5") {
		t.Errorf("missing 12:5 in compact output, got:\n%s", out)
	}
	if !strings.Contains(out, "13:8") {
		t.Errorf("missing 13:8 in compact output, got:\n%s", out)
	}
	if !strings.Contains(out, "15:12") {
		t.Errorf("missing 15:12 in compact output, got:\n%s", out)
	}
	if !strings.Contains(out, "(3)") {
		t.Errorf("missing (3) count, got:\n%s", out)
	}
	// Compact format must NOT use ** markers
	if strings.Contains(out, "**") {
		t.Errorf("compact format must not use ** markers, got:\n%s", out)
	}
	// Must not have L prefix
	if strings.Contains(out, "L12") {
		t.Errorf("compact format must not have L prefix, got:\n%s", out)
	}
}

// TestFormatReferencesCompactSameLineCollapse verifies same-line columns collapse.
func TestFormatReferencesCompactSameLineCollapse(t *testing.T) {
	locs := []lsp.Location{
		makeLocation("/a/foo.go", 12, 5),
		makeLocation("/a/foo.go", 12, 8),
		makeLocation("/a/foo.go", 15, 3),
	}
	groups, order := groupLocations(locs)
	out := formatReferences(groups, order, len(locs), true, false)

	// Line 12 has two refs → should be collapsed: 12:{5,8}
	if !strings.Contains(out, "12:{5,8}") {
		t.Errorf("same-line refs should collapse to 12:{5,8}, got:\n%s", out)
	}
	// Line 15 is single → 15:3
	if !strings.Contains(out, "15:3") {
		t.Errorf("single ref on line 15 should be 15:3, got:\n%s", out)
	}
}

// TestFormatReferencesCompactMultiFile verifies compact output across multiple files.
func TestFormatReferencesCompactMultiFile(t *testing.T) {
	locs := []lsp.Location{
		makeLocation("/a/foo.go", 5, 1),
		makeLocation("/b/bar.go", 10, 3),
		makeLocation("/b/bar.go", 10, 7),
	}
	groups, order := groupLocations(locs)
	out := formatReferences(groups, order, len(locs), true, true)

	if !strings.Contains(out, "Found 3 reference(s):") {
		t.Errorf("missing header, got:\n%s", out)
	}
	// foo.go: one ref
	if !strings.Contains(out, "5:1") {
		t.Errorf("missing 5:1 for foo.go, got:\n%s", out)
	}
	// bar.go: two refs on same line → collapsed
	if !strings.Contains(out, "10:{3,7}") {
		t.Errorf("missing 10:{3,7} collapse for bar.go, got:\n%s", out)
	}
}

// TestFormatReferencesCompactSingleRef verifies single-reference compact output.
func TestFormatReferencesCompactSingleRef(t *testing.T) {
	locs := []lsp.Location{
		makeLocation("/a/only.go", 7, 2),
	}
	groups, order := groupLocations(locs)
	out := formatReferences(groups, order, len(locs), true, false)

	if !strings.Contains(out, "7:2") {
		t.Errorf("missing 7:2, got:\n%s", out)
	}
	if !strings.Contains(out, "(1)") {
		t.Errorf("missing (1), got:\n%s", out)
	}
}

// TestFormatReferencesCompactNoLPrefix verifies the L prefix is absent in compact mode.
func TestFormatReferencesCompactNoLPrefix(t *testing.T) {
	locs := []lsp.Location{
		makeLocation("/a/x.go", 3, 4),
	}
	groups, order := groupLocations(locs)
	out := formatReferences(groups, order, len(locs), true, false)

	if strings.Contains(out, "L3") {
		t.Errorf("compact output must not contain L prefix, got:\n%s", out)
	}
}

// TestFormatReferencesVerboseNoChange verifies compact=false preserves old L-prefix format.
func TestFormatReferencesVerbosePreservesLPrefix(t *testing.T) {
	locs := []lsp.Location{
		makeLocation("/a/x.go", 3, 4),
	}
	groups, order := groupLocations(locs)
	out := formatReferences(groups, order, len(locs), false, true)

	if !strings.Contains(out, "L3:4") {
		t.Errorf("verbose output must contain L3:4, got:\n%s", out)
	}
}
