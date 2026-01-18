// Package errors provides structured error handling with error codes and context.
package errors

import (
	"fmt"
	"runtime"
	"strings"
)

// ErrorCode represents a specific error condition.
type ErrorCode int

// Error codes organized by category
const (
	// Search errors (1000-1099)
	ErrRipgrepNotFound ErrorCode = 1001
	ErrRipgrepTimeout  ErrorCode = 1002
	ErrInvalidPattern  ErrorCode = 1003
	ErrSearchFailed    ErrorCode = 1004
	ErrNoResults       ErrorCode = 1005

	// Parser errors (1100-1199)
	ErrParserNotFound  ErrorCode = 1101
	ErrParseFailed     ErrorCode = 1102
	ErrInvalidLanguage ErrorCode = 1103
	ErrFileTooBig      ErrorCode = 1104
	ErrInvalidSyntax   ErrorCode = 1105

	// LSP errors (1200-1299)
	ErrLSPServerNotFound ErrorCode = 1201
	ErrLSPInitFailed     ErrorCode = 1202
	ErrLSPTimeout        ErrorCode = 1203
	ErrLSPCommunication  ErrorCode = 1204
	ErrNoHoverInfo       ErrorCode = 1205
	ErrNoDefinition      ErrorCode = 1206
	ErrNoReferences      ErrorCode = 1207

	// Edit errors (1300-1399)
	ErrEditFailed       ErrorCode = 1301
	ErrInvalidEdit      ErrorCode = 1302
	ErrFileNotFound     ErrorCode = 1303
	ErrFileNotReadable  ErrorCode = 1304
	ErrFileNotWritable  ErrorCode = 1305
	ErrNodeNotFound     ErrorCode = 1306
	ErrValidationFailed ErrorCode = 1307
	ErrInvalidSelection ErrorCode = 1308

	// Query errors (1400-1499)
	ErrInvalidQuery ErrorCode = 1401
	ErrQueryTimeout ErrorCode = 1402
	ErrNoMatches    ErrorCode = 1403
	ErrQueryCompile ErrorCode = 1404

	// Config errors (1500-1599)
	ErrInvalidConfig   ErrorCode = 1501
	ErrPathNotAllowed  ErrorCode = 1502
	ErrWorkspaceNotSet ErrorCode = 1503

	// Internal errors (1900-1999)
	ErrInternal    ErrorCode = 1900
	ErrCacheFailed ErrorCode = 1901
	ErrConcurrency ErrorCode = 1902
)

// StructuredError represents an error with rich context and remediation info.
type StructuredError struct {
	Cause       error
	Context     map[string]any
	Message     string
	Remediation string
	Stack       string
	Code        ErrorCode
}

// Error implements the error interface.
func (e *StructuredError) Error() string {
	var sb strings.Builder

	// Error code and message
	sb.WriteString(fmt.Sprintf("[%d] %s", e.Code, e.Message))

	// Context if available
	if len(e.Context) > 0 {
		sb.WriteString("\nContext:")
		for k, v := range e.Context {
			sb.WriteString(fmt.Sprintf("\n  %s: %v", k, v))
		}
	}

	// Remediation if available
	if e.Remediation != "" {
		sb.WriteString(fmt.Sprintf("\nHow to fix: %s", e.Remediation))
	}

	// Underlying cause if available
	if e.Cause != nil {
		sb.WriteString(fmt.Sprintf("\nCaused by: %v", e.Cause))
	}

	return sb.String()
}

// Unwrap returns the underlying cause for error chain support.
func (e *StructuredError) Unwrap() error {
	return e.Cause
}

// WithContext adds context to the error.
func (e *StructuredError) WithContext(key string, value any) *StructuredError {
	if e.Context == nil {
		e.Context = make(map[string]any)
	}
	e.Context[key] = value
	return e
}

// WithRemediation sets the remediation message.
func (e *StructuredError) WithRemediation(msg string) *StructuredError {
	e.Remediation = msg
	return e
}

// New creates a new structured error with stack trace.
func New(code ErrorCode, message string) *StructuredError {
	return &StructuredError{
		Code:    code,
		Message: message,
		Context: make(map[string]interface{}),
		Stack:   captureStack(2),
	}
}

// Wrap wraps an existing error with structured error information.
func Wrap(code ErrorCode, message string, cause error) *StructuredError {
	return &StructuredError{
		Code:    code,
		Message: message,
		Context: make(map[string]interface{}),
		Cause:   cause,
		Stack:   captureStack(2),
	}
}

// Is checks if an error matches the given error code.
func Is(err error, code ErrorCode) bool {
	if err == nil {
		return false
	}

	if se, ok := err.(*StructuredError); ok {
		return se.Code == code
	}

	return false
}

// GetCode extracts the error code from an error, or returns 0 if not a structured error.
func GetCode(err error) ErrorCode {
	if err == nil {
		return 0
	}

	if se, ok := err.(*StructuredError); ok {
		return se.Code
	}

	return 0
}

// captureStack captures the stack trace.
func captureStack(skip int) string {
	const depth = 16
	var pcs [depth]uintptr
	n := runtime.Callers(skip+1, pcs[:])

	var sb strings.Builder
	frames := runtime.CallersFrames(pcs[:n])

	for {
		frame, more := frames.Next()
		if !strings.Contains(frame.File, "runtime/") {
			sb.WriteString(fmt.Sprintf("\n  %s:%d %s", frame.File, frame.Line, frame.Function))
		}
		if !more {
			break
		}
	}

	return sb.String()
}

// Common error constructors for convenience

// NewRipgrepNotFound creates an error for missing ripgrep binary.
func NewRipgrepNotFound() *StructuredError {
	os := runtime.GOOS
	install := "brew install ripgrep"

	switch os {
	case "linux":
		install = "apt-get install ripgrep (Debian/Ubuntu) or yum install ripgrep (RHEL/CentOS)"
	case "windows":
		install = "choco install ripgrep or scoop install ripgrep"
	}

	return New(ErrRipgrepNotFound, "ripgrep (rg) binary not found in system PATH").
		WithContext("os", os).
		WithRemediation(fmt.Sprintf("Install ripgrep: %s", install))
}

// NewLSPServerNotFound creates an error for missing LSP server.
func NewLSPServerNotFound(language, serverName string) *StructuredError {
	return New(ErrLSPServerNotFound, fmt.Sprintf("LSP server '%s' not found for language %s", serverName, language)).
		WithContext("language", language).
		WithContext("server", serverName).
		WithRemediation(fmt.Sprintf("Install the %s LSP server to enable IDE features for %s", serverName, language))
}

// NewFileTooLarge creates an error for files exceeding size limit.
func NewFileTooLarge(path string, size, limit int64) *StructuredError {
	return New(ErrFileTooBig, "file exceeds maximum size limit").
		WithContext("file", path).
		WithContext("size_bytes", size).
		WithContext("limit_bytes", limit).
		WithRemediation(fmt.Sprintf("File size (%d bytes) exceeds limit (%d bytes). Consider processing smaller files or increasing the limit.", size, limit))
}

// NewParseError creates an error for parsing failures.
func NewParseError(language, file string, cause error) *StructuredError {
	return Wrap(ErrParseFailed, fmt.Sprintf("failed to parse %s file", language), cause).
		WithContext("language", language).
		WithContext("file", file).
		WithRemediation("Check file syntax and ensure it's valid source code for the specified language")
}

// NewSearchTimeout creates an error for search timeouts.
func NewSearchTimeout(pattern string, duration string) *StructuredError {
	return New(ErrRipgrepTimeout, "search operation timed out").
		WithContext("pattern", pattern).
		WithContext("duration", duration).
		WithRemediation("Try narrowing the search scope, using more specific patterns, or increasing the timeout limit")
}

// NewEditValidationError creates an error for edit validation failures.
func NewEditValidationError(file string, cause error) *StructuredError {
	return Wrap(ErrValidationFailed, "edit validation failed - syntax errors detected", cause).
		WithContext("file", file).
		WithRemediation("Review the edit operation and ensure it produces valid syntax. The file was not modified.")
}

// NewFileNotFoundError creates an error for missing files.
func NewFileNotFoundError(file string) *StructuredError {
	return New(ErrFileNotFound, fmt.Sprintf("file not found: %s", file)).
		WithContext("file", file).
		WithRemediation("Verify the file path is correct and the file exists")
}

// NewFileNotReadableError creates an error for unreadable files.
func NewFileNotReadableError(file string, cause error) *StructuredError {
	return Wrap(ErrFileNotReadable, fmt.Sprintf("cannot read file: %s", file), cause).
		WithContext("file", file).
		WithRemediation("Check file permissions and ensure the file is not locked by another process")
}

// NewFileNotWritableError creates an error for write failures.
func NewFileNotWritableError(file string, cause error) *StructuredError {
	return Wrap(ErrFileNotWritable, fmt.Sprintf("cannot write to file: %s", file), cause).
		WithContext("file", file).
		WithRemediation("Check file permissions, disk space, and ensure the file is not locked by another process")
}

// NewNodeNotFoundError creates an error when AST node selector finds no matches.
func NewNodeNotFoundError(selector string) *StructuredError {
	return New(ErrNodeNotFound, "no AST nodes match the selector criteria").
		WithContext("selector", selector).
		WithRemediation("Verify the selector criteria (kind, name, pattern, line) match an existing code structure")
}

// NewInvalidSelectionError creates an error for ambiguous or invalid selectors.
func NewInvalidSelectionError(message string) *StructuredError {
	return New(ErrInvalidSelection, message).
		WithRemediation("Refine the selector to be more specific or provide a selector_index to choose between multiple matches")
}

// NewInvalidEditError creates an error for invalid edit operations.
func NewInvalidEditError(message string) *StructuredError {
	return New(ErrInvalidEdit, message).
		WithRemediation("Review the edit request and ensure all required fields are provided with valid values")
}
