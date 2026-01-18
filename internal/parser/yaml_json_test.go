package parser

import (
	"context"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/lukaszraczylo/mcp-filepuff/pkg/protocol"
)

func TestParseYAML(t *testing.T) {
	registry := NewRegistry()
	defer registry.Close()

	tests := []struct {
		name        string
		content     string
		shouldError bool
	}{
		{
			name: "valid simple YAML",
			content: `name: test
version: 1.0.0
enabled: true`,
			shouldError: false,
		},
		{
			name: "valid nested YAML",
			content: `metadata:
  name: test-app
  namespace: default
spec:
  replicas: 3
  selector:
    matchLabels:
      app: test`,
			shouldError: false,
		},
		{
			name: "valid list YAML",
			content: `items:
  - name: item1
    value: 100
  - name: item2
    value: 200`,
			shouldError: false,
		},
		{
			name:        "invalid YAML - bad syntax",
			content:     `name: test\n  bad: indent\n wrong: [unclosed`,
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := registry.ParseYAML(context.Background(), "test.yaml", []byte(tt.content))

			if tt.shouldError {
				if err == nil {
					t.Error("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if result == nil {
				t.Error("expected result but got nil")
				return
			}

			if result.Language != protocol.LangYAML {
				t.Errorf("expected language YAML, got %s", result.Language)
			}

			if len(result.Errors) > 0 {
				t.Errorf("expected no syntax errors, got %d", len(result.Errors))
			}
		})
	}
}

func TestParseJSON(t *testing.T) {
	registry := NewRegistry()
	defer registry.Close()

	tests := []struct {
		name        string
		content     string
		shouldError bool
	}{
		{
			name:        "valid simple JSON",
			content:     `{"name": "test", "version": "1.0.0", "enabled": true}`,
			shouldError: false,
		},
		{
			name: "valid nested JSON",
			content: `{
				"metadata": {
					"name": "test-app",
					"namespace": "default"
				},
				"spec": {
					"replicas": 3,
					"selector": {
						"matchLabels": {
							"app": "test"
						}
					}
				}
			}`,
			shouldError: false,
		},
		{
			name:        "valid array JSON",
			content:     `[{"name": "item1", "value": 100}, {"name": "item2", "value": 200}]`,
			shouldError: false,
		},
		{
			name:        "invalid JSON - unclosed brace",
			content:     `{"name": "test", "value": 100`,
			shouldError: true,
		},
		{
			name:        "invalid JSON - trailing comma",
			content:     `{"name": "test", "value": 100,}`,
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := registry.ParseJSON(context.Background(), "test.json", []byte(tt.content))

			if tt.shouldError {
				if err == nil {
					t.Error("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if result == nil {
				t.Error("expected result but got nil")
				return
			}

			if result.Language != protocol.LangJSON {
				t.Errorf("expected language JSON, got %s", result.Language)
			}

			if len(result.Errors) > 0 {
				t.Errorf("expected no syntax errors, got %d", len(result.Errors))
			}
		})
	}
}

func TestRegistryParse_YAML_JSON(t *testing.T) {
	registry := NewRegistry()
	defer registry.Close()

	yamlContent := []byte(`name: test
version: 1.0.0`)

	jsonContent := []byte(`{"name": "test", "version": "1.0.0"}`)

	// Test YAML through main Parse method
	yamlResult, err := registry.Parse(context.Background(), "config.yaml", yamlContent)
	if err != nil {
		t.Errorf("failed to parse YAML: %v", err)
	}
	if yamlResult.Language != protocol.LangYAML {
		t.Errorf("expected YAML language, got %s", yamlResult.Language)
	}

	// Test JSON through main Parse method
	jsonResult, err := registry.Parse(context.Background(), "config.json", jsonContent)
	if err != nil {
		t.Errorf("failed to parse JSON: %v", err)
	}
	if jsonResult.Language != protocol.LangJSON {
		t.Errorf("expected JSON language, got %s", jsonResult.Language)
	}

	// Test .yml extension
	ymlResult, err := registry.Parse(context.Background(), "config.yml", yamlContent)
	if err != nil {
		t.Errorf("failed to parse .yml: %v", err)
	}
	if ymlResult.Language != protocol.LangYAML {
		t.Errorf("expected YAML language for .yml extension, got %s", ymlResult.Language)
	}
}

func TestWalkYAML(t *testing.T) {
	content := []byte(`metadata:
  name: test
  labels:
    app: myapp
    env: prod`)

	var root yaml.Node
	if err := yaml.Unmarshal(content, &root); err != nil {
		t.Fatalf("failed to parse YAML: %v", err)
	}

	nodeCount := 0
	WalkYAML(&root, func(node *yaml.Node) bool {
		nodeCount++
		return true
	})

	if nodeCount == 0 {
		t.Error("expected to visit nodes, but count is 0")
	}
}

func TestValidateYAML(t *testing.T) {
	tests := []struct {
		name        string
		content     []byte
		shouldError bool
	}{
		{
			name:        "valid YAML",
			content:     []byte("name: test\nvalue: 100"),
			shouldError: false,
		},
		{
			name:        "invalid YAML",
			content:     []byte("name: test\n  bad:\n[unclosed"),
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateYAML(tt.content)
			if (err != nil) != tt.shouldError {
				t.Errorf("ValidateYAML() error = %v, shouldError = %v", err, tt.shouldError)
			}
		})
	}
}

func TestValidateJSON(t *testing.T) {
	tests := []struct {
		name        string
		content     []byte
		shouldError bool
	}{
		{
			name:        "valid JSON",
			content:     []byte(`{"name": "test", "value": 100}`),
			shouldError: false,
		},
		{
			name:        "invalid JSON",
			content:     []byte(`{"name": "test", "value": 100`),
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateJSON(tt.content)
			if (err != nil) != tt.shouldError {
				t.Errorf("ValidateJSON() error = %v, shouldError = %v", err, tt.shouldError)
			}
		})
	}
}
