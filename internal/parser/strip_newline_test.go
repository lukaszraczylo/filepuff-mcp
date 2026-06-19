package parser

import (
	"strings"
	"testing"

	"github.com/lukaszraczylo/mcp-filepuff/pkg/protocol"
)

// An inline block comment (code before it on the same line) must not cause the following
// line to be merged onto it — the line's terminator must survive.
func TestStripBlockCommentInlineNoLineMerge(t *testing.T) {
	got := StripContent("a := 1 /* note */\nb := 2\n", []StripFlag{StripBlockComments}, protocol.LangGo)
	want := "a := 1 \nb := 2\n"
	if got.Content != want {
		t.Fatalf("inline block comment must not merge lines.\nwant: %q\ngot:  %q", want, got.Content)
	}
}

// A standalone block-comment line (only whitespace before it) is removed in full,
// including its indentation and terminator — no stray blank/whitespace line left behind.
func TestStripBlockCommentStandaloneRemovesLine(t *testing.T) {
	got := StripContent("x\n\t/* c */\ny\n", []StripFlag{StripBlockComments}, protocol.LangGo)
	want := "x\ny\n"
	if got.Content != want {
		t.Fatalf("standalone block comment line must be removed cleanly.\nwant: %q\ngot:  %q", want, got.Content)
	}
}

// On a CRLF file, removing a standalone block-comment line must consume the full \r\n
// terminator rather than leaving a stray blank (bare-CR) line.
func TestStripBlockCommentCRLFNoStrayBlank(t *testing.T) {
	got := StripContent("code\r\n/* c */\r\nmore\r\n", []StripFlag{StripBlockComments}, protocol.LangGo)
	want := "code\r\nmore\r\n"
	if got.Content != want {
		t.Fatalf("CRLF standalone block comment must not leave a stray blank line.\nwant: %q\ngot:  %q", want, got.Content)
	}
}

// Stripping a hash-style license header must not greedily swallow the blank separator
// line that follows it.
func TestStripLicensePythonPreservesSeparatorBlank(t *testing.T) {
	got := StripContent("# Copyright 2024\n# License MIT\n\ncode\n", []StripFlag{StripLicense}, protocol.LangPython)
	want := "\ncode\n"
	if got.Content != want {
		t.Fatalf("python license strip must keep the blank separator.\nwant: %q\ngot:  %q", want, got.Content)
	}
}

// A multi-line TS named import must be removed in full, not just its first line.
func TestStripTSImportsMultiLine(t *testing.T) {
	src := "import {\n  a,\n  b,\n} from 'x';\nconst y = 1;\n"
	got := StripContent(src, []StripFlag{StripImports}, protocol.LangTypeScript)
	want := "const y = 1;\n"
	if got.Content != want {
		t.Fatalf("multi-line import must be fully removed.\nwant: %q\ngot:  %q", want, got.Content)
	}
}

// A side-effect import without a semicolon must not consume following code.
func TestStripTSImportsSideEffectNoRunaway(t *testing.T) {
	src := "import './setup'\nconst y = 1\n"
	got := StripContent(src, []StripFlag{StripImports}, protocol.LangTypeScript)
	want := "const y = 1\n"
	if got.Content != want {
		t.Fatalf("side-effect import must not swallow code.\nwant: %q\ngot:  %q", want, got.Content)
	}
}

// Blank/underscore/dot/aliased single Go imports must all be stripped.
func TestStripGoImportsSingleSpecForms(t *testing.T) {
	src := "package main\n\nimport _ \"embed\"\nimport . \"math\"\nimport m \"strings\"\n\nfunc main() {}\n"
	got := StripContent(src, []StripFlag{StripImports}, protocol.LangGo)
	for _, frag := range []string{`"embed"`, `"math"`, `"strings"`} {
		if containsStr(got.Content, frag) {
			t.Fatalf("single import %s not stripped, got:\n%s", frag, got.Content)
		}
	}
	if !containsStr(got.Content, "func main") {
		t.Fatalf("code removed, got:\n%s", got.Content)
	}
}

// A Go // line-comment license after build constraints: the license is removed
// while the build constraint is preserved above it.
func TestStripLicenseGoLineCommentAfterBuildTag(t *testing.T) {
	src := "//go:build linux\n\n// Copyright 2024 Acme\n// SPDX-License-Identifier: MIT\n\npackage main\n"
	got := StripContent(src, []StripFlag{StripLicense}, protocol.LangGo)
	if containsStr(got.Content, "Copyright") {
		t.Fatalf("line-comment license not stripped, got:\n%q", got.Content)
	}
	if !containsStr(got.Content, "//go:build linux") {
		t.Fatalf("build constraint must be preserved, got:\n%q", got.Content)
	}
	if !containsStr(got.Content, "package main") {
		t.Fatalf("code removed, got:\n%q", got.Content)
	}
}

func containsStr(s, sub string) bool { return strings.Contains(s, sub) }
