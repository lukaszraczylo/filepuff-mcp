// Package lsp provides a generic LSP client implementation.
package lsp

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	json "github.com/goccy/go-json"
)

// Client represents an LSP client connection.
type Client struct {
	stdin         io.WriteCloser
	stdout        io.ReadCloser
	stderr        io.ReadCloser
	cmd           *exec.Cmd
	pending       map[int64]chan *Response
	done          chan struct{}
	notifications chan *Notification
	requestID     atomic.Int64
	runningMu     sync.RWMutex
	stopOnce      sync.Once
	mu            sync.Mutex
	running       bool
}

// Request represents a JSON-RPC request.
type Request struct {
	Params  interface{} `json:"params,omitempty"`
	JSONRPC string      `json:"jsonrpc"`
	Method  string      `json:"method"`
	ID      int64       `json:"id"`
}

// Response represents a JSON-RPC response.
type Response struct {
	Error   *ResponseError  `json:"error,omitempty"`
	JSONRPC string          `json:"jsonrpc"`
	Result  json.RawMessage `json:"result,omitempty"`
	ID      int64           `json:"id"`
}

// ResponseError represents a JSON-RPC error.
type ResponseError struct {
	Data    interface{} `json:"data,omitempty"`
	Message string      `json:"message"`
	Code    int         `json:"code"`
}

func (e *ResponseError) Error() string {
	return fmt.Sprintf("LSP error %d: %s", e.Code, e.Message)
}

// Notification represents a JSON-RPC notification.
type Notification struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// NewClient creates a new LSP client from a command.
func NewClient(cmd *exec.Cmd) (*Client, error) {
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to get stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("failed to get stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		return nil, fmt.Errorf("failed to get stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		_ = stderr.Close()
		return nil, fmt.Errorf("failed to start LSP server: %w", err)
	}

	c := &Client{
		cmd:           cmd,
		stdin:         stdin,
		stdout:        stdout,
		stderr:        stderr,
		pending:       make(map[int64]chan *Response),
		done:          make(chan struct{}),
		running:       true,
		notifications: make(chan *Notification, 100),
	}

	// Start reader goroutine
	go c.readLoop()

	return c, nil
}

// Call sends a request and waits for a response.
func (c *Client) Call(ctx context.Context, method string, params interface{}) (*Response, error) {
	c.runningMu.RLock()
	if !c.running {
		c.runningMu.RUnlock()
		return nil, fmt.Errorf("client is not running")
	}
	c.runningMu.RUnlock()

	id := c.requestID.Add(1)
	req := &Request{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}

	// Create response channel
	respChan := make(chan *Response, 1)
	c.mu.Lock()
	c.pending[id] = respChan
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
	}()

	// Send request
	if err := c.send(req); err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	// Wait for response
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-c.done:
		return nil, fmt.Errorf("client closed")
	case resp := <-respChan:
		if resp.Error != nil {
			return nil, resp.Error
		}
		return resp, nil
	}
}

// Notify sends a notification (no response expected).
func (c *Client) Notify(method string, params interface{}) error {
	c.runningMu.RLock()
	if !c.running {
		c.runningMu.RUnlock()
		return fmt.Errorf("client is not running")
	}
	c.runningMu.RUnlock()

	notif := struct {
		Params  interface{} `json:"params,omitempty"`
		JSONRPC string      `json:"jsonrpc"`
		Method  string      `json:"method"`
	}{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	}

	return c.send(notif)
}

// Notifications returns a channel for receiving server notifications.
func (c *Client) Notifications() <-chan *Notification {
	return c.notifications
}

// Close shuts down the client and the LSP server.
func (c *Client) Close() error {
	var err error
	c.stopOnce.Do(func() {
		c.runningMu.Lock()
		c.running = false
		c.runningMu.Unlock()

		close(c.done)

		// Close stdin to signal the server
		_ = c.stdin.Close()

		// Wait for process to exit with timeout
		done := make(chan struct{})
		go func() {
			_ = c.cmd.Wait()
			close(done)
		}()

		select {
		case <-done:
			// Clean exit
		case <-time.After(5 * time.Second):
			// Force kill
			_ = c.cmd.Process.Kill()
		}

		close(c.notifications)
	})
	return err
}

// send writes a JSON-RPC message to the server.
func (c *Client) send(msg interface{}) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	// Format with Content-Length header
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(data))
	_, err = c.stdin.Write([]byte(header))
	if err != nil {
		return fmt.Errorf("failed to write header: %w", err)
	}

	_, err = c.stdin.Write(data)
	if err != nil {
		return fmt.Errorf("failed to write body: %w", err)
	}

	return nil
}

// readLoop reads and dispatches messages from the server.
func (c *Client) readLoop() {
	reader := bufio.NewReader(c.stdout)

	for {
		select {
		case <-c.done:
			return
		default:
		}

		// Read headers
		contentLength := -1
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				return
			}
			line = strings.TrimSpace(line)
			if line == "" {
				break
			}
			if strings.HasPrefix(line, "Content-Length:") {
				lengthStr := strings.TrimSpace(strings.TrimPrefix(line, "Content-Length:"))
				contentLength, _ = strconv.Atoi(lengthStr)
			}
		}

		if contentLength <= 0 {
			continue
		}

		// Read body
		body := make([]byte, contentLength)
		_, err := io.ReadFull(reader, body)
		if err != nil {
			return
		}

		// Try to parse as response first
		var resp Response
		if err := json.Unmarshal(body, &resp); err == nil && resp.ID != 0 {
			c.mu.Lock()
			if ch, ok := c.pending[resp.ID]; ok {
				ch <- &resp
			}
			c.mu.Unlock()
			continue
		}

		// Try to parse as notification
		var notif Notification
		if err := json.Unmarshal(body, &notif); err == nil && notif.Method != "" {
			select {
			case c.notifications <- &notif:
			default:
				// Drop notification if channel is full
			}
		}
	}
}

// IsRunning returns whether the client is running.
func (c *Client) IsRunning() bool {
	c.runningMu.RLock()
	defer c.runningMu.RUnlock()
	return c.running
}
