package parser

import (
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
