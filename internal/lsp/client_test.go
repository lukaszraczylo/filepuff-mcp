package lsp

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// TestClientMarksNotRunningOnProcessExit verifies crash detection: when the
// underlying server process exits (stdout EOF), readLoop must flip the client to
// not-running so later Calls fail fast instead of blocking until their timeout.
func TestClientMarksNotRunningOnProcessExit(t *testing.T) {
	cmd := exec.Command("sh", "-c", "exit 0") // exits immediately, closing stdout
	c, err := NewClient(cmd)
	if err != nil {
		t.Skipf("cannot start helper process: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	deadline := time.Now().Add(2 * time.Second)
	for c.IsRunning() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if c.IsRunning() {
		t.Fatal("client should be marked not running after its process exits")
	}

	// A Call on the dead client must fail fast (the running guard), not hang.
	done := make(chan error, 1)
	go func() {
		_, callErr := c.Call(context.Background(), "test/method", nil)
		done <- callErr
	}()
	select {
	case callErr := <-done:
		if callErr == nil {
			t.Error("Call on a dead client should return an error")
		}
	case <-time.After(2 * time.Second):
		t.Error("Call on a dead client hung instead of failing fast")
	}
}

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
