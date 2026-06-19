package lsp

import (
	"strings"
	"testing"
)

// drainStderr retains a bounded tail of server stderr for diagnostics; capture
// must keep recent bytes and never grow past StderrCaptureMax.
func TestClientStderrCapture(t *testing.T) {
	c := &Client{}

	if c.RecentStderr() != "" {
		t.Errorf("expected empty stderr initially, got %q", c.RecentStderr())
	}

	c.captureStderr([]byte("gopls: command not found\n"))
	if !strings.Contains(c.RecentStderr(), "command not found") {
		t.Errorf("expected captured stderr, got %q", c.RecentStderr())
	}

	// Exceed the cap: only the most recent StderrCaptureMax bytes are kept.
	big := strings.Repeat("x", StderrCaptureMax*2)
	c.captureStderr([]byte(big))
	if got := len(c.RecentStderr()); got > StderrCaptureMax {
		t.Errorf("stderr tail not bounded: len=%d, want <= %d", got, StderrCaptureMax)
	}
}
