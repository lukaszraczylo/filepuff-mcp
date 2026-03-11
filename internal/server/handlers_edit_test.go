package server

import "testing"

func TestUnescapeNewlines(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "no escapes",
			input:    "func test() {\n\treturn true;\n}",
			expected: "func test() {\n\treturn true;\n}",
		},
		{
			name:     "literal backslash n",
			input:    "func test() {\\n\\treturn true;\\n}",
			expected: "func test() {\n\treturn true;\n}",
		},
		{
			name:     "literal backslash t",
			input:    "func test() {\\n\\treturn true;\\n}",
			expected: "func test() {\n\treturn true;\n}",
		},
		{
			name:     "literal quotes",
			input:    `func test() {\n\treturn \"true\";\n}`,
			expected: "func test() {\n\treturn \"true\";\n}",
		},

		{
			name:     "mixed content",
			input:    "line1\\nline2\\tindented\\nline3",
			expected: "line1\nline2\tindented\nline3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := unescapeNewlines(tt.input)
			if result != tt.expected {
				t.Errorf("unescapeNewlines(%q) = %q, expected %q", tt.input, result, tt.expected)
			}
		})
	}
}
