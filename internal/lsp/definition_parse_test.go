package lsp

import (
	"encoding/json"
	"testing"
)

// TestParseDefinitionResult covers every LSP definition result shape: single
// Location, Location[], single LocationLink, LocationLink[], plus empty/error
// cases. The LocationLink shapes are the regression target — we advertise
// LinkSupport, so gopls/rust-analyzer return targetUri/targetRange, which a
// naive []Location unmarshal would silently turn into zero-value locations.
func TestParseDefinitionResult(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		wantURIs []string
		wantErr  bool
	}{
		{
			name:     "single Location",
			raw:      `{"uri":"file:///a.go","range":{"start":{"line":4,"character":2},"end":{"line":4,"character":8}}}`,
			wantURIs: []string{"file:///a.go"},
		},
		{
			name:     "Location array",
			raw:      `[{"uri":"file:///a.go","range":{"start":{"line":1,"character":0},"end":{"line":1,"character":3}}},{"uri":"file:///b.go","range":{"start":{"line":2,"character":0},"end":{"line":2,"character":3}}}]`,
			wantURIs: []string{"file:///a.go", "file:///b.go"},
		},
		{
			name:     "single LocationLink",
			raw:      `{"targetUri":"file:///c.go","targetRange":{"start":{"line":9,"character":0},"end":{"line":12,"character":1}}}`,
			wantURIs: []string{"file:///c.go"},
		},
		{
			name:     "LocationLink array",
			raw:      `[{"targetUri":"file:///d.go","targetRange":{"start":{"line":0,"character":0},"end":{"line":0,"character":5}}}]`,
			wantURIs: []string{"file:///d.go"},
		},
		{
			name:     "empty array",
			raw:      `[]`,
			wantURIs: []string{},
		},
		{
			name:    "garbage",
			raw:     `"not-a-location"`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			locs, err := parseDefinitionResult(json.RawMessage(tt.raw))
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got locations %+v", locs)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(locs) != len(tt.wantURIs) {
				t.Fatalf("got %d locations, want %d (%+v)", len(locs), len(tt.wantURIs), locs)
			}
			for i, want := range tt.wantURIs {
				if locs[i].URI != want {
					t.Errorf("location[%d].URI = %q, want %q", i, locs[i].URI, want)
				}
			}
			// LocationLink ranges must be carried over, not zeroed.
			if tt.name == "single LocationLink" && locs[0].Range.Start.Line != 9 {
				t.Errorf("LocationLink targetRange not preserved: got start line %d, want 9", locs[0].Range.Start.Line)
			}
		})
	}
}
