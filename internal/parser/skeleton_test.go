package parser

import (
	"context"
	"strings"
	"testing"
)

// A decorated Python function must keep BOTH its decorator(s) and its def
// signature in the skeleton — previously only the first decorator survived and
// the signature was dropped.
func TestSkeletonPythonDecoratedFunction(t *testing.T) {
	r := NewRegistry()
	defer r.Close()

	src := "@app.route(\"/\")\n" +
		"def home():\n" +
		"    return \"hi\"\n"

	out, _, err := SkeletonFile(context.Background(), r, "app.py", []byte(src))
	if err != nil {
		t.Fatalf("SkeletonFile error: %v", err)
	}
	if !strings.Contains(out, "@app.route(\"/\")") {
		t.Errorf("skeleton lost decorator, got:\n%s", out)
	}
	if !strings.Contains(out, "def home():") {
		t.Errorf("skeleton lost def signature, got:\n%s", out)
	}
	if strings.Contains(out, "return \"hi\"") {
		t.Errorf("skeleton should elide the body, got:\n%s", out)
	}
}

// A plain (undecorated) Python function still renders its signature + body stub.
func TestSkeletonPythonPlainFunction(t *testing.T) {
	r := NewRegistry()
	defer r.Close()

	src := "def add(a, b):\n    return a + b\n"
	out, _, err := SkeletonFile(context.Background(), r, "m.py", []byte(src))
	if err != nil {
		t.Fatalf("SkeletonFile error: %v", err)
	}
	if !strings.Contains(out, "def add(a, b):") {
		t.Errorf("skeleton lost signature, got:\n%s", out)
	}
}
