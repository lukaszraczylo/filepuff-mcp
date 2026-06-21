// Package search provides text search functionality using ripgrep.
package search

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"path/filepath"
	"strings"

	json "github.com/goccy/go-json"
	"github.com/lukaszraczylo/mcp-filepuff/internal/config"
	"github.com/lukaszraczylo/mcp-filepuff/internal/util"
	"github.com/lukaszraczylo/mcp-filepuff/pkg/errors"
	"github.com/lukaszraczylo/mcp-filepuff/pkg/protocol"
)

// Searcher provides text search functionality using ripgrep.
type Searcher struct {
	cfg    *config.Config
	logger *slog.Logger
	rgPath string
}

// Request represents a search request.
type Request struct {
	Pattern        string
	Paths          []string
	FileTypes      []string
	ContextLines   int
	MaxResults     int
	IgnoreCase     bool
	Regex          bool
	IncludeHidden  bool
	FollowSymlinks bool
}

// Result represents a single search result.
type Result struct {
	File      string            `json:"file"`
	MatchText string            `json:"match_text"`
	Language  protocol.Language `json:"language"`
	Context   ContextLines      `json:"context"`
	Line      int               `json:"line"`
	Column    int               `json:"column"`
}

// ContextLines holds lines before and after a match.
type ContextLines struct {
	Before []string `json:"before"`
	After  []string `json:"after"`
}

// SearchResults holds the complete search results.
type SearchResults struct {
	Results   []Result `json:"results"`
	Truncated bool     `json:"truncated"`
	Total     int      `json:"total"`
}

// ripgrep JSON output types
type rgMessage struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

type rgMatch struct {
	Path struct {
		Text string `json:"text"`
	} `json:"path"`
	Lines struct {
		Text string `json:"text"`
	} `json:"lines"`
	Submatches []struct {
		Match struct {
			Text string `json:"text"`
		} `json:"match"`
		Start int `json:"start"`
		End   int `json:"end"`
	} `json:"submatches"`
	LineNumber     int `json:"line_number"`
	AbsoluteOffset int `json:"absolute_offset"`
}

type rgContext struct {
	Path struct {
		Text string `json:"text"`
	} `json:"path"`
	Lines struct {
		Text string `json:"text"`
	} `json:"lines"`
	LineNumber int `json:"line_number"`
}

type rgSummary struct {
	ElapsedTotal struct {
		Secs  int `json:"secs"`
		Nanos int `json:"nanos"`
	} `json:"elapsed_total"`
	Stats struct {
		Searches          int   `json:"searches"`
		SearchesWithMatch int   `json:"searches_with_match"`
		BytesSearched     int64 `json:"bytes_searched"`
		BytesPrinted      int64 `json:"bytes_printed"`
		MatchedLines      int   `json:"matched_lines"`
		Matches           int   `json:"matches"`
	} `json:"stats"`
}

// New creates a new Searcher instance.
func New(cfg *config.Config, logger *slog.Logger) (*Searcher, error) {
	// Detect ripgrep binary
	rgPath, err := exec.LookPath("rg")
	if err != nil {
		return nil, errors.NewRipgrepNotFound()
	}

	return &Searcher{
		cfg:    cfg,
		logger: logger,
		rgPath: rgPath,
	}, nil
}

// Search executes a search and returns results.
func (s *Searcher) Search(ctx context.Context, req *Request) (*SearchResults, error) {
	if req.Pattern == "" {
		return nil, errors.New(errors.ErrInvalidPattern, "pattern cannot be empty").
			WithRemediation("Provide a non-empty search pattern")
	}

	// Validate that at least one provided path is allowed
	if err := s.validatePaths(req.Paths); err != nil {
		return nil, err
	}

	// Build ripgrep command
	args := s.buildArgs(req)

	s.logger.Debug("executing ripgrep", "args", args)

	// Create command with timeout
	ctx, cancel := context.WithTimeout(ctx, s.cfg.SearchTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, s.rgPath, args...) // #nosec G204 - rgPath is validated at initialization

	// Set working directory to workspace root
	cmd.Dir = s.cfg.WorkspaceRoot

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Run command - ripgrep returns exit code 1 for no matches, which is not an error
	err := cmd.Run()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, errors.NewSearchTimeout(req.Pattern, s.cfg.SearchTimeout.String())
		}
		// Exit code 1 means no matches, which is fine
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return &SearchResults{Results: []Result{}, Total: 0}, nil
		}
		// Exit code 2 means error
		if stderr.Len() > 0 {
			return nil, errors.Wrap(errors.ErrSearchFailed, "ripgrep search failed", err).
				WithContext("pattern", req.Pattern).
				WithContext("stderr", stderr.String()).
				WithRemediation("Check search pattern syntax and ensure files are readable")
		}
		return nil, errors.Wrap(errors.ErrSearchFailed, "ripgrep search failed", err).
			WithContext("pattern", req.Pattern).
			WithRemediation("Check search pattern syntax and ensure ripgrep is functioning correctly")
	}

	// Parse JSON output
	return s.parseOutput(&stdout, req.MaxResults)
}

// buildArgs builds the ripgrep command arguments.
func (s *Searcher) buildArgs(req *Request) []string {
	args := []string{"--json"}

	// Add context lines
	if req.ContextLines > 0 {
		args = append(args, fmt.Sprintf("--context=%d", req.ContextLines))
	}

	// File type filtering
	for _, ft := range req.FileTypes {
		args = append(args, "--type", ft)
	}

	// Case sensitivity
	if req.IgnoreCase {
		args = append(args, "--ignore-case")
	}

	// Fixed strings (non-regex)
	if !req.Regex {
		args = append(args, "--fixed-strings")
	}

	// Follow symlinks
	if req.FollowSymlinks || s.cfg.FollowSymlinks {
		args = append(args, "--follow")
	}

	// Include hidden files
	if req.IncludeHidden {
		args = append(args, "--hidden")
	}

	// Respect .gitignore (default behavior for rg)
	if !s.cfg.RespectGitignore {
		args = append(args, "--no-ignore")
	}

	// Result cap enforced in-process by parseOutput. rg has no cross-file
	// total-count flag in stable releases, so we don't pass one; --max-count is
	// per-file and would miss results unevenly.

	// Add pattern
	args = append(args, "--", req.Pattern)

	// Add paths (default to current directory which is workspace root)
	if len(req.Paths) > 0 {
		for _, p := range req.Paths {
			// Validate path is within workspace
			if s.cfg.IsPathAllowed(p) {
				args = append(args, p)
			}
		}
	} else {
		args = append(args, ".")
	}

	return args
}

// validatePaths checks that at least one caller-provided path is allowed.
// Returns an error if paths were provided but none passed IsPathAllowed.
func (s *Searcher) validatePaths(paths []string) error {
	if len(paths) == 0 {
		return nil // no explicit paths — will default to workspace root
	}
	for _, p := range paths {
		if s.cfg.IsPathAllowed(p) {
			return nil
		}
	}
	return errors.New(errors.ErrPathNotAllowed, "all provided search paths are outside the workspace root").
		WithContext("paths", fmt.Sprintf("%v", paths)).
		WithRemediation("Provide paths within the workspace root")
}

// resultPathAllowed reports whether a ripgrep result path is within the
// workspace root. ripgrep runs with Dir=WorkspaceRoot and emits paths relative
// to it, so a relative path is joined to the root before the check; IsPathAllowed
// then resolves symlinks, rejecting any path that --follow led outside the root.
func (s *Searcher) resultPathAllowed(p string) bool {
	if !filepath.IsAbs(p) {
		p = filepath.Join(s.cfg.WorkspaceRoot, p)
	}
	return s.cfg.IsPathAllowed(p)
}

// parseOutput parses ripgrep JSON output.
func (s *Searcher) parseOutput(output *bytes.Buffer, maxResults int) (*SearchResults, error) {
	results := &SearchResults{
		Results: []Result{},
	}

	// Track before-context lines linearly: accumulate context lines until the next match consumes them.
	var pendingBefore []string
	pendingFile := ""

	scanner := bufio.NewScanner(output)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var msg rgMessage
		if err := json.Unmarshal(line, &msg); err != nil {
			s.logger.Debug("failed to parse ripgrep output line", "error", err, "line", string(line))
			continue
		}

		switch msg.Type {
		case "match":
			var match rgMatch
			if err := json.Unmarshal(msg.Data, &match); err != nil {
				continue
			}

			// Defense in depth: never surface a file outside the workspace root.
			// ripgrep with --follow (default on) can traverse a symlink whose
			// target lives outside the root; dropping those matches keeps search
			// from leaking external file content.
			if !s.resultPathAllowed(match.Path.Text) {
				continue
			}

			// Check max results
			if maxResults > 0 && len(results.Results) >= maxResults {
				results.Truncated = true
				continue
			}

			result := Result{
				File:      match.Path.Text,
				Line:      match.LineNumber,
				MatchText: strings.TrimRight(match.Lines.Text, "\n\r"),
				Language:  protocol.DetectLanguage(match.Path.Text),
			}

			// Add column from first submatch
			if len(match.Submatches) > 0 {
				result.Column = match.Submatches[0].Start + 1 // 1-indexed
			}

			// Attach pending before-context if it belongs to this file
			if pendingFile == match.Path.Text && len(pendingBefore) > 0 {
				result.Context.Before = pendingBefore
			}
			pendingBefore = nil
			pendingFile = ""

			results.Results = append(results.Results, result)

		case "context":
			var ctx rgContext
			if err := json.Unmarshal(msg.Data, &ctx); err != nil {
				continue
			}

			lineText := strings.TrimRight(ctx.Lines.Text, "\n\r")

			isAfter := false
			if len(results.Results) > 0 {
				last := &results.Results[len(results.Results)-1]
				if last.File == ctx.Path.Text && ctx.LineNumber > last.Line {
					last.Context.After = append(last.Context.After, lineText)
					isAfter = true
				}
			}
			if !isAfter {
				if pendingFile != ctx.Path.Text {
					pendingBefore = nil
					pendingFile = ctx.Path.Text
				}
				pendingBefore = append(pendingBefore, lineText)
			}

		case "summary":
			var summary rgSummary
			if err := json.Unmarshal(msg.Data, &summary); err != nil {
				continue
			}
			results.Total = summary.Stats.Matches
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading ripgrep output: %w", err)
	}

	return results, nil
}

// FormatOptions controls how search results are rendered.
type FormatOptions struct {
	Cluster    bool   // coalesce consecutive matches into line-range blocks
	CursorLine string // if non-empty, appended as a footer line
	Verbose    bool   // if true, emit "Found N matches in M files:" preamble (opt-in)
}

// FormatResults formats search results for display (backward-compat wrapper).
func (s *Searcher) FormatResults(results *SearchResults) string {
	return s.FormatResultsWithOptions(results, FormatOptions{})
}

// FormatResultsWithOptions formats search results with configurable output.
// By default the "Found N matches in M files:" preamble is omitted; set opts.Verbose=true to restore it.
func (s *Searcher) FormatResultsWithOptions(results *SearchResults, opts FormatOptions) string {
	if len(results.Results) == 0 {
		return "No matches found."
	}

	var sb strings.Builder

	// Group results by file
	fileResults := make(map[string][]Result)
	var fileOrder []string
	for _, r := range results.Results {
		if _, exists := fileResults[r.File]; !exists {
			fileOrder = append(fileOrder, r.File)
		}
		fileResults[r.File] = append(fileResults[r.File], r)
	}

	// Write preamble only when Verbose is requested.
	if opts.Verbose {
		totalMatches := len(results.Results)
		fileCount := len(fileResults)
		fmt.Fprintf(&sb, "Found %d matches in %d files", totalMatches, fileCount)
		if results.Truncated {
			fmt.Fprintf(&sb, " (truncated, total: %d)", results.Total)
		}
		sb.WriteString(":\n\n")
	} else if results.Truncated && opts.CursorLine == "" {
		// When a cursor footer is present it already reports the remaining count,
		// so the prose truncation line would be redundant.
		fmt.Fprintf(&sb, "(truncated, showing subset of %d total matches)\n\n", results.Total)
	}

	// Write results grouped by file
	for _, file := range fileOrder {
		// Make path relative to workspace root if possible
		relPath := file
		if absPath, err := filepath.Abs(file); err == nil {
			if rel, err := filepath.Rel(s.cfg.WorkspaceRoot, absPath); err == nil && !strings.HasPrefix(rel, "..") {
				relPath = rel
			}
		}

		fmt.Fprintf(&sb, "**%s**\n", relPath)

		if opts.Cluster {
			writeClusteredResults(&sb, fileResults[file])
		} else {
			writeVerboseResults(&sb, fileResults[file])
		}
		sb.WriteString("\n")
	}

	if opts.CursorLine != "" {
		sb.WriteString(opts.CursorLine)
		sb.WriteString("\n")
	}

	return sb.String()
}

// writeVerboseResults writes results in the standard verbose format.
func writeVerboseResults(sb *strings.Builder, results []Result) {
	for _, r := range results {
		// Write context before
		for _, ctx := range r.Context.Before {
			fmt.Fprintf(sb, "   │ %s\n", truncateLine(ctx, 200))
		}
		// Write match line
		fmt.Fprintf(sb, "L%d│ %s\n", r.Line, truncateLine(r.MatchText, 200))
		// Write context after
		for _, ctx := range r.Context.After {
			fmt.Fprintf(sb, "   │ %s\n", truncateLine(ctx, 200))
		}
	}
}

// writeClusteredResults coalesces consecutive or adjacent match lines into
// a single "L12-14│ <first-match-text>" entry. Context lines are dropped
// in cluster mode to maximise information density.
func writeClusteredResults(sb *strings.Builder, results []Result) {
	if len(results) == 0 {
		return
	}

	type clusterEntry struct {
		startLine int
		endLine   int
		firstText string
	}

	var clusters []clusterEntry
	cur := clusterEntry{
		startLine: results[0].Line,
		endLine:   results[0].Line,
		firstText: results[0].MatchText,
	}

	for _, r := range results[1:] {
		// Merge if adjacent (within 1 line gap)
		if r.Line <= cur.endLine+1 {
			if r.Line > cur.endLine {
				cur.endLine = r.Line
			}
		} else {
			clusters = append(clusters, cur)
			cur = clusterEntry{startLine: r.Line, endLine: r.Line, firstText: r.MatchText}
		}
	}
	clusters = append(clusters, cur)

	for _, c := range clusters {
		text := truncateLine(c.firstText, 200)
		if c.startLine == c.endLine {
			fmt.Fprintf(sb, "L%d│ %s\n", c.startLine, text)
		} else {
			fmt.Fprintf(sb, "L%d-%d│ %s\n", c.startLine, c.endLine, text)
		}
	}
}

// truncateLine truncates a line if it exceeds maxLen bytes, backing off to a
// rune boundary so a multibyte character is never split into invalid UTF-8.
func truncateLine(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:util.RuneBoundary(s, maxLen-3)] + "..."
}
