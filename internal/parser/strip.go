package parser

import (
	"strings"

	"github.com/lukaszraczylo/mcp-filepuff/pkg/protocol"
)

// StripFlag names the categories of content to remove.
type StripFlag string

const (
	StripImports       StripFlag = "imports"
	StripLicense       StripFlag = "license"
	StripBlockComments StripFlag = "block_comments"
)

// StripResult holds the stripped content and which flags actually removed content.
type StripResult struct {
	Content  string
	Stripped []StripFlag
}

// StripContent applies requested strip operations to content, in order:
// license → imports → block_comments.
// lang is used to pick language-specific heuristics.
func StripContent(content string, flags []StripFlag, lang protocol.Language) StripResult {
	flagSet := make(map[StripFlag]bool, len(flags))
	for _, f := range flags {
		flagSet[f] = true
	}

	var stripped []StripFlag

	if flagSet[StripLicense] {
		next, removed := stripLicense(content)
		if removed {
			content = next
			stripped = append(stripped, StripLicense)
		}
	}

	if flagSet[StripImports] {
		next, removed := stripImports(content, lang)
		if removed {
			content = next
			stripped = append(stripped, StripImports)
		}
	}

	if flagSet[StripBlockComments] {
		next, removed := stripBlockComments(content, lang)
		if removed {
			content = next
			stripped = append(stripped, StripBlockComments)
		}
	}

	return StripResult{Content: content, Stripped: stripped}
}

// stripLicense removes a leading block comment that looks like a license header.
// A comment qualifies if it contains "copyright", "license", or "spdx-license-identifier" (case-insensitive).
func stripLicense(content string) (string, bool) {
	trimmed := strings.TrimLeft(content, " \t\n\r")

	// C-style block comment at top
	if strings.HasPrefix(trimmed, "/*") {
		end := strings.Index(trimmed, "*/")
		if end >= 0 {
			candidate := trimmed[:end+2]
			lower := strings.ToLower(candidate)
			if strings.Contains(lower, "copyright") ||
				strings.Contains(lower, "license") ||
				strings.Contains(lower, "spdx-license-identifier") {
				rest := trimmed[end+2:]
				// Consume trailing newline(s)
				rest = strings.TrimLeft(rest, "\r\n")
				return rest, true
			}
		}
	}

	// Python/hash-style leading comment block. Only contiguous "#" lines belong to the
	// header; a blank line ends it and is preserved as a separator (rather than being
	// greedily swallowed and collapsed away).
	if strings.HasPrefix(trimmed, "#") {
		lines := strings.Split(trimmed, "\n")
		var commentLines, rest []string
		for i, l := range lines {
			if strings.HasPrefix(l, "#") {
				commentLines = append(commentLines, l)
				continue
			}
			rest = lines[i:]
			break
		}
		lower := strings.ToLower(strings.Join(commentLines, "\n"))
		if strings.Contains(lower, "copyright") ||
			strings.Contains(lower, "license") ||
			strings.Contains(lower, "spdx-license-identifier") {
			return strings.Join(rest, "\n"), true
		}
	}

	return content, false
}

// stripImports removes top-of-file import blocks, language-specific.
func stripImports(content string, lang protocol.Language) (string, bool) {
	switch lang {
	case protocol.LangGo:
		return stripGoImports(content)
	case protocol.LangTypeScript, protocol.LangJavaScript:
		return stripTSImports(content)
	case protocol.LangPython:
		return stripPythonImports(content)
	case protocol.LangRust:
		return stripRustImports(content)
	default:
		return content, false
	}
}

// stripGoImports removes Go import(...) or single import "..." declarations.
func stripGoImports(content string) (string, bool) {
	lines := strings.Split(content, "\n")
	var out []string
	removed := false
	i := 0
	for i < len(lines) {
		trimLine := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trimLine, "import (") || trimLine == "import (" {
			// multi-line import block
			removed = true
			i++ // skip "import ("
			for i < len(lines) {
				if strings.TrimSpace(lines[i]) == ")" {
					i++ // skip closing ")"
					break
				}
				i++
			}
			// skip one blank line after
			if i < len(lines) && strings.TrimSpace(lines[i]) == "" {
				i++
			}
			continue
		}
		if strings.HasPrefix(trimLine, `import "`) || strings.HasPrefix(trimLine, "import `") {
			removed = true
			i++
			continue
		}
		out = append(out, lines[i])
		i++
	}
	if !removed {
		return content, false
	}
	return strings.Join(out, "\n"), true
}

// stripTSImports removes TypeScript/JavaScript "import ... from ..." and "require(...)" lines.
func stripTSImports(content string) (string, bool) {
	lines := strings.Split(content, "\n")
	var out []string
	removed := false
	for _, l := range lines {
		trimLine := strings.TrimSpace(l)
		if strings.HasPrefix(trimLine, "import ") || strings.HasPrefix(trimLine, "const {") && strings.Contains(trimLine, "require(") {
			removed = true
			continue
		}
		out = append(out, l)
	}
	if !removed {
		return content, false
	}
	return strings.Join(out, "\n"), true
}

// stripPythonImports removes Python "import ..." and "from ... import ..." lines.
func stripPythonImports(content string) (string, bool) {
	lines := strings.Split(content, "\n")
	var out []string
	removed := false
	for _, l := range lines {
		trimLine := strings.TrimSpace(l)
		if strings.HasPrefix(trimLine, "import ") || strings.HasPrefix(trimLine, "from ") {
			removed = true
			continue
		}
		out = append(out, l)
	}
	if !removed {
		return content, false
	}
	return strings.Join(out, "\n"), true
}

// stripRustImports removes Rust "use ..." declarations.
func stripRustImports(content string) (string, bool) {
	lines := strings.Split(content, "\n")
	var out []string
	removed := false
	inMulti := false
	for _, l := range lines {
		trimLine := strings.TrimSpace(l)
		if inMulti {
			// look for semicolon terminating multi-line use
			if strings.Contains(trimLine, ";") {
				inMulti = false
			}
			removed = true
			continue
		}
		if strings.HasPrefix(trimLine, "use ") {
			removed = true
			if !strings.HasSuffix(trimLine, ";") {
				inMulti = true
			}
			continue
		}
		out = append(out, l)
	}
	if !removed {
		return content, false
	}
	return strings.Join(out, "\n"), true
}

// stripBlockComments removes /* ... */ block comments (Go/TS/C/Rust)
// and Python triple-quoted docstrings.
func stripBlockComments(content string, lang protocol.Language) (string, bool) {
	if lang == protocol.LangPython {
		return stripPythonDocstrings(content)
	}
	return stripCStyleBlockComments(content)
}

// trimTrailingLineWhitespace drops trailing spaces/tabs from out (back to, but not past,
// the previous newline). Used when a standalone comment line is removed so its leading
// indentation does not linger as a whitespace-only line.
func trimTrailingLineWhitespace(out []byte) []byte {
	for len(out) > 0 && (out[len(out)-1] == ' ' || out[len(out)-1] == '\t') {
		out = out[:len(out)-1]
	}
	return out
}

// skipLineTail advances i over trailing spaces/tabs and a CR, then a single LF — i.e. the
// remainder of a line after a standalone comment's closer, including its \n or \r\n
// terminator. Returns the new index.
func skipLineTail(content string, i int) int {
	for i < len(content) && (content[i] == ' ' || content[i] == '\t' || content[i] == '\r') {
		i++
	}
	if i < len(content) && content[i] == '\n' {
		i++
	}
	return i
}

// stripCStyleBlockComments removes /* ... */ comments. A comment that occupies a whole
// line (only whitespace before it) is removed together with that line's indentation and
// terminator; an inline comment (code precedes it) is removed in place, leaving the
// surrounding line — and crucially its terminator — intact so lines are never merged.
func stripCStyleBlockComments(content string) (string, bool) {
	removed := false
	out := make([]byte, 0, len(content))
	lineHasNonSpace := false
	i := 0
	for i < len(content) {
		if i+1 < len(content) && content[i] == '/' && content[i+1] == '*' {
			if end := strings.Index(content[i+2:], "*/"); end >= 0 {
				removed = true
				standalone := !lineHasNonSpace
				i = i + 2 + end + 2 // advance past closing */
				if standalone {
					out = trimTrailingLineWhitespace(out)
					i = skipLineTail(content, i)
					lineHasNonSpace = false
				}
				continue
			}
		}
		c := content[i]
		switch c {
		case '\n':
			lineHasNonSpace = false
		case ' ', '\t', '\r':
			// whitespace: does not mark the line as having content
		default:
			lineHasNonSpace = true
		}
		out = append(out, c)
		i++
	}
	if !removed {
		return content, false
	}
	return string(out), true
}

// stripPythonDocstrings removes triple-quoted strings (""" and ”'). As with block
// comments, a standalone docstring line is removed along with its indentation and
// terminator, while an inline triple-quoted string leaves its line's terminator intact.
func stripPythonDocstrings(content string) (string, bool) {
	removed := false
	out := make([]byte, 0, len(content))
	lineHasNonSpace := false
	i := 0
	for i < len(content) {
		if i+2 < len(content) {
			triple := content[i : i+3]
			if triple == `"""` || triple == `'''` {
				if end := strings.Index(content[i+3:], triple); end >= 0 {
					removed = true
					standalone := !lineHasNonSpace
					i = i + 3 + end + 3
					if standalone {
						out = trimTrailingLineWhitespace(out)
						i = skipLineTail(content, i)
						lineHasNonSpace = false
					}
					continue
				}
			}
		}
		c := content[i]
		switch c {
		case '\n':
			lineHasNonSpace = false
		case ' ', '\t', '\r':
			// whitespace: does not mark the line as having content
		default:
			lineHasNonSpace = true
		}
		out = append(out, c)
		i++
	}
	if !removed {
		return content, false
	}
	return string(out), true
}
