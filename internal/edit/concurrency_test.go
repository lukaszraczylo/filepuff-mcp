package edit

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/lukaszraczylo/mcp-filepuff/internal/parser"
)

// TestConcurrentEditLocking tests that concurrent edits to the same file are properly serialized.
func TestConcurrentEditLocking(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.go")

	// Create initial file
	initialContent := `package main

func main() {
	println("hello")
}
`
	if err := os.WriteFile(testFile, []byte(initialContent), 0600); err != nil {
		t.Fatal(err)
	}

	registry := parser.NewRegistry()
	engine := NewEngine(registry)

	// Run 10 concurrent edits
	const numEdits = 10
	var wg sync.WaitGroup
	wg.Add(numEdits)

	errors := make(chan error, numEdits)

	for i := 0; i < numEdits; i++ {
		i := i
		go func() {
			defer wg.Done()

			edit := &ASTEdit{
				File:      testFile,
				Operation: EditReplace,
				Selector: ASTSelector{
					Kind: "function_declaration",
					Name: "main",
				},
				NewContent: `func main() {
	println("edit ` + string(rune(i)) + `")
}`,
			}

			ctx := context.Background()
			result, err := engine.Apply(ctx, edit)
			if err != nil {
				errors <- err
				return
			}

			if !result.Success {
				errors <- err
				return
			}
		}()
	}

	wg.Wait()
	close(errors)

	// Check for errors
	for err := range errors {
		if err != nil {
			t.Errorf("Concurrent edit failed: %v", err)
		}
	}

	// Verify file wasn't corrupted
	finalContent, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatal(err)
	}

	// Parse to ensure it's still valid Go
	_, err = registry.Parse(context.Background(), testFile, finalContent)
	if err != nil {
		t.Errorf("File corrupted after concurrent edits: %v\nContent:\n%s", err, string(finalContent))
	}
}

// TestConcurrentEditDifferentFiles tests that concurrent edits to different files don't block each other.
func TestConcurrentEditDifferentFiles(t *testing.T) {
	tmpDir := t.TempDir()

	registry := parser.NewRegistry()
	engine := NewEngine(registry)

	const numFiles = 5
	var wg sync.WaitGroup
	wg.Add(numFiles)

	startBarrier := make(chan struct{})

	for i := 0; i < numFiles; i++ {
		i := i
		testFile := filepath.Join(tmpDir, fmt.Sprintf("test%d.go", i))

		// Create initial file
		initialContent := `package main

func test() {
	println("initial")
}
`
		if err := os.WriteFile(testFile, []byte(initialContent), 0600); err != nil {
			t.Fatal(err)
		}

		go func() {
			defer wg.Done()

			// Wait for all goroutines to be ready
			<-startBarrier

			edit := &ASTEdit{
				File:      testFile,
				Operation: EditReplace,
				Selector: ASTSelector{
					Kind: "function_declaration",
					Name: "test",
				},
				NewContent: `func test() {
	println("modified")
}`,
			}

			ctx := context.Background()
			result, err := engine.Apply(ctx, edit)
			if err != nil {
				t.Errorf("Edit failed for %s: %v", testFile, err)
				return
			}

			if !result.Success {
				t.Errorf("Edit unsuccessful for %s: %s", testFile, result.Error)
			}
		}()
	}

	// Release all goroutines simultaneously
	close(startBarrier)

	wg.Wait()
}

// TestFileLockRelease tests that file locks are properly released after edits.
func TestFileLockRelease(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.go")

	initialContent := `package main

func test() {}
`
	if err := os.WriteFile(testFile, []byte(initialContent), 0600); err != nil {
		t.Fatal(err)
	}

	registry := parser.NewRegistry()
	engine := NewEngine(registry)

	edit := &ASTEdit{
		File:      testFile,
		Operation: EditReplace,
		Selector: ASTSelector{
			Kind: "function_declaration",
			Name: "test",
		},
		NewContent: `func test() { println("updated") }`,
	}

	// First edit
	ctx := context.Background()
	result1, err := engine.Apply(ctx, edit)
	if err != nil {
		t.Fatal(err)
	}
	if !result1.Success {
		t.Fatalf("First edit failed: %s", result1.Error)
	}

	// Second edit should succeed (lock was released)
	result2, err := engine.Apply(ctx, edit)
	if err != nil {
		t.Fatal(err)
	}
	if !result2.Success {
		t.Fatalf("Second edit failed (lock not released?): %s", result2.Error)
	}
}
