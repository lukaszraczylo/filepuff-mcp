package parser

import (
	"context"
	"testing"
)

func TestFindNodeAtPosition(t *testing.T) {
	r := NewRegistry()
	content := []byte("package main\n\nfunc Hello() string { return \"hi\" }\n")
	res, err := r.Parse(context.Background(), "test.go", content)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	t.Run("nil tree", func(t *testing.T) {
		if FindNodeAtPosition(nil, 3, 6) != nil {
			t.Error("nil tree must return nil")
		}
	})

	// Guard against the uint32 underflow: non-positive coordinates must return
	// nil, never wrap to a huge Row/Column.
	t.Run("non-positive coordinates", func(t *testing.T) {
		for _, c := range []struct{ line, col int }{{0, 0}, {0, 5}, {3, 0}, {-1, 4}} {
			if got := FindNodeAtPosition(res.Tree, c.line, c.col); got != nil {
				t.Errorf("FindNodeAtPosition(tree, %d, %d) = %v, want nil", c.line, c.col, got.Type())
			}
		}
	})

	t.Run("out of range line returns nil", func(t *testing.T) {
		if got := FindNodeAtPosition(res.Tree, 99999, 1); got != nil {
			t.Errorf("far-out-of-range line should return nil, got %v", got.Type())
		}
	})

	t.Run("valid position returns a node", func(t *testing.T) {
		// Line 3, column 6 lands inside "Hello".
		got := FindNodeAtPosition(res.Tree, 3, 6)
		if got == nil {
			t.Fatal("expected a node at a valid in-range position, got nil")
		}
	})
}
