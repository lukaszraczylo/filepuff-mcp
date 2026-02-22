package parser

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/cespare/xxhash/v2"
)

// BenchmarkParse benchmarks parsing files of various sizes.
func BenchmarkParse(b *testing.B) {
	registry := NewRegistry()
	defer registry.Close()
	ctx := context.Background()

	benchmarks := []struct {
		name    string
		content string
	}{
		{
			name:    "small_file_100_lines",
			content: generateGoCode(100),
		},
		{
			name:    "medium_file_1000_lines",
			content: generateGoCode(1000),
		},
		{
			name:    "large_file_5000_lines",
			content: generateGoCode(5000),
		},
		{
			name:    "very_large_file_10000_lines",
			content: generateGoCode(10000),
		},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			content := []byte(bm.content)
			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				_, err := registry.Parse(ctx, "test.go", content)
				if err != nil {
					b.Fatalf("Parse failed: %v", err)
				}
			}
		})
	}
}

// BenchmarkParseCacheHit benchmarks cache hit performance.
func BenchmarkParseCacheHit(b *testing.B) {
	registry := NewRegistry()
	defer registry.Close()
	ctx := context.Background()

	content := []byte(generateGoCode(1000))

	// Warm up the cache
	_, err := registry.Parse(ctx, "test.go", content)
	if err != nil {
		b.Fatalf("initial parse failed: %v", err)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := registry.Parse(ctx, "test.go", content)
		if err != nil {
			b.Fatalf("Parse failed: %v", err)
		}
	}
}

// BenchmarkParseCacheMiss benchmarks cache miss performance.
func BenchmarkParseCacheMiss(b *testing.B) {
	registry := NewRegistry()
	defer registry.Close()
	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Use different content each time to force cache miss
		content := []byte(generateGoCodeWithSuffix(1000, i))
		_, err := registry.Parse(ctx, "test.go", content)
		if err != nil {
			b.Fatalf("Parse failed: %v", err)
		}
	}
}

// BenchmarkParseLanguages benchmarks parsing different language files.
func BenchmarkParseLanguages(b *testing.B) {
	registry := NewRegistry()
	defer registry.Close()
	ctx := context.Background()

	languages := []struct {
		name     string
		filename string
		content  string
	}{
		{
			name:     "go",
			filename: "test.go",
			content:  generateGoCode(500),
		},
		{
			name:     "typescript",
			filename: "test.ts",
			content:  generateTypeScriptCode(500),
		},
		{
			name:     "python",
			filename: "test.py",
			content:  generatePythonCode(500),
		},
		{
			name:     "javascript",
			filename: "test.js",
			content:  generateJavaScriptCode(500),
		},
	}

	for _, lang := range languages {
		b.Run(lang.name, func(b *testing.B) {
			content := []byte(lang.content)
			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				_, err := registry.Parse(ctx, lang.filename, content)
				if err != nil {
					b.Fatalf("Parse failed: %v", err)
				}
			}
		})
	}
}

// BenchmarkParseComplexity benchmarks parsing files with varying complexity.
func BenchmarkParseComplexity(b *testing.B) {
	registry := NewRegistry()
	defer registry.Close()
	ctx := context.Background()

	benchmarks := []struct {
		name    string
		content string
	}{
		{
			name:    "simple_functions",
			content: generateSimpleFunctions(100),
		},
		{
			name:    "nested_structures",
			content: generateNestedStructures(50),
		},
		{
			name:    "complex_types",
			content: generateComplexTypes(50),
		},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			content := []byte(bm.content)
			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				_, err := registry.Parse(ctx, "test.go", content)
				if err != nil {
					b.Fatalf("Parse failed: %v", err)
				}
			}
		})
	}
}

// BenchmarkContentHash benchmarks the content hashing function.
func BenchmarkContentHash(b *testing.B) {
	sizes := []int{100, 1000, 10000, 100000}

	for _, size := range sizes {
		b.Run(formatSize(size), func(b *testing.B) {
			content := []byte(strings.Repeat("a", size))
			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				_ = fmt.Sprintf("%016x", xxhash.Sum64(content))
			}
		})
	}
}

// BenchmarkIsBinary benchmarks the binary detection function.
func BenchmarkIsBinary(b *testing.B) {
	sizes := []int{100, 1000, 8000, 10000}

	for _, size := range sizes {
		b.Run(formatSize(size)+"_text", func(b *testing.B) {
			content := []byte(strings.Repeat("Hello, World!\n", size/14))
			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				_ = isBinary(content)
			}
		})

		b.Run(formatSize(size)+"_binary", func(b *testing.B) {
			content := make([]byte, size)
			for j := 0; j < size; j++ {
				content[j] = byte(j % 256)
			}
			content[size/2] = 0 // Add null byte
			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				_ = isBinary(content)
			}
		})
	}
}

// BenchmarkParseWithMaxSize benchmarks parsing with different max size limits.
func BenchmarkParseWithMaxSize(b *testing.B) {
	ctx := context.Background()

	limits := []int64{
		10 * 1024,        // 10KB
		100 * 1024,       // 100KB
		1024 * 1024,      // 1MB
		10 * 1024 * 1024, // 10MB
	}

	content := []byte(generateGoCode(500))

	for _, limit := range limits {
		b.Run(formatBytes(limit), func(b *testing.B) {
			// Skip if content is larger than limit
			if int64(len(content)) > limit {
				b.Skipf("content size %d exceeds limit %d", len(content), limit)
			}

			registry := NewRegistryWithSize(limit)
			defer registry.Close()

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				_, err := registry.Parse(ctx, "test.go", content)
				if err != nil {
					b.Fatalf("Parse failed: %v", err)
				}
			}
		})
	}
}

// BenchmarkConcurrentParse benchmarks concurrent parsing operations.
func BenchmarkConcurrentParse(b *testing.B) {
	registry := NewRegistry()
	defer registry.Close()
	ctx := context.Background()

	content := []byte(generateGoCode(500))

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := registry.Parse(ctx, "test.go", content)
			if err != nil {
				b.Fatalf("Parse failed: %v", err)
			}
		}
	})
}

// Helper functions to generate test code

func generateGoCode(lines int) string {
	var sb strings.Builder
	sb.WriteString("package main\n\n")

	for i := 0; i < lines/10; i++ {
		sb.WriteString("func Function")
		sb.WriteString(itoa(i))
		sb.WriteString("(a, b int) int {\n")
		sb.WriteString("\tif a > b {\n")
		sb.WriteString("\t\treturn a + b\n")
		sb.WriteString("\t}\n")
		sb.WriteString("\treturn a - b\n")
		sb.WriteString("}\n\n")
	}

	return sb.String()
}

func generateGoCodeWithSuffix(lines int, suffix int) string {
	code := generateGoCode(lines)
	return code + "// Suffix: " + itoa(suffix) + "\n"
}

func generateTypeScriptCode(lines int) string {
	var sb strings.Builder

	for i := 0; i < lines/8; i++ {
		sb.WriteString("function function")
		sb.WriteString(itoa(i))
		sb.WriteString("(a: number, b: number): number {\n")
		sb.WriteString("  if (a > b) {\n")
		sb.WriteString("    return a + b;\n")
		sb.WriteString("  }\n")
		sb.WriteString("  return a - b;\n")
		sb.WriteString("}\n\n")
	}

	return sb.String()
}

func generatePythonCode(lines int) string {
	var sb strings.Builder

	for i := 0; i < lines/6; i++ {
		sb.WriteString("def function")
		sb.WriteString(itoa(i))
		sb.WriteString("(a, b):\n")
		sb.WriteString("    if a > b:\n")
		sb.WriteString("        return a + b\n")
		sb.WriteString("    return a - b\n\n")
	}

	return sb.String()
}

func generateJavaScriptCode(lines int) string {
	var sb strings.Builder

	for i := 0; i < lines/8; i++ {
		sb.WriteString("function function")
		sb.WriteString(itoa(i))
		sb.WriteString("(a, b) {\n")
		sb.WriteString("  if (a > b) {\n")
		sb.WriteString("    return a + b;\n")
		sb.WriteString("  }\n")
		sb.WriteString("  return a - b;\n")
		sb.WriteString("}\n\n")
	}

	return sb.String()
}

func generateSimpleFunctions(count int) string {
	var sb strings.Builder
	sb.WriteString("package main\n\n")

	for i := 0; i < count; i++ {
		sb.WriteString("func Func")
		sb.WriteString(itoa(i))
		sb.WriteString("() { }\n\n")
	}

	return sb.String()
}

func generateNestedStructures(depth int) string {
	var sb strings.Builder
	sb.WriteString("package main\n\n")

	for i := 0; i < depth; i++ {
		sb.WriteString("type Struct")
		sb.WriteString(itoa(i))
		sb.WriteString(" struct {\n")
		sb.WriteString("\tField1 int\n")
		sb.WriteString("\tField2 string\n")
		if i > 0 {
			sb.WriteString("\tNested Struct")
			sb.WriteString(itoa(i - 1))
			sb.WriteString("\n")
		}
		sb.WriteString("}\n\n")
	}

	return sb.String()
}

func generateComplexTypes(count int) string {
	var sb strings.Builder
	sb.WriteString("package main\n\n")

	for i := 0; i < count; i++ {
		sb.WriteString("type Type")
		sb.WriteString(itoa(i))
		sb.WriteString(" interface {\n")
		sb.WriteString("\tMethod1() error\n")
		sb.WriteString("\tMethod2(a int, b string) (int, error)\n")
		sb.WriteString("\tMethod3() chan interface{}\n")
		sb.WriteString("}\n\n")
	}

	return sb.String()
}

func formatSize(size int) string {
	if size < 1000 {
		return itoa(size) + "B"
	}
	return itoa(size/1000) + "KB"
}

func formatBytes(bytes int64) string {
	if bytes < 1024 {
		return itoa(int(bytes)) + "B"
	}
	if bytes < 1024*1024 {
		return itoa(int(bytes/1024)) + "KB"
	}
	return itoa(int(bytes/(1024*1024))) + "MB"
}

// Simple integer to string conversion without importing strconv
func itoa(n int) string {
	if n == 0 {
		return "0"
	}

	negative := n < 0
	if negative {
		n = -n
	}

	var buf [20]byte
	i := len(buf) - 1

	for n > 0 {
		buf[i] = byte('0' + n%10)
		n /= 10
		i--
	}

	if negative {
		buf[i] = '-'
		i--
	}

	return string(buf[i+1:])
}
