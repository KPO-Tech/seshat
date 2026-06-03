package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"
)

// stdioMaxLineBytes is the maximum JSON-RPC line size we accept from a stdio MCP server.
// 64 MB is generous; the default bufio.Scanner limit of 64 KB is too small for real payloads.
const stdioMaxLineBytes = 64 * 1024 * 1024

// StdioTransport implements the stdio transport for MCP
type StdioTransport struct {
	// config is the transport configuration
	config StdioTransportConfig

	// cmd is the running command
	cmd *exec.Cmd

	// stdin is the process stdin
	stdin io.WriteCloser

	// stdout is the process stdout
	stdout io.ReadCloser

	// sendMu serialises writes to stdin so concurrent Send/SendNotification calls
	// do not interleave partial JSON lines.
	sendMu sync.Mutex

	// responseChan channels responses by request ID
	responseChan map[int64]chan *JSONRPCResponse

	// responseChanMu protects responseChan
	responseChanMu sync.RWMutex

	// notificationsHandler handles incoming notifications
	notificationsHandler NotificationHandler

	// methodsHandler handles incoming method calls
	methodsHandler MethodHandler

	// started indicates if the transport has been started
	started bool

	// closed indicates if the transport is closed
	closed bool

	// closeChan is used to signal shutdown
	closeChan chan struct{}
}

// NewStdioTransport creates a new stdio transport
func NewStdioTransport(config StdioTransportConfig) (*StdioTransport, error) {
	return &StdioTransport{
		config:       config,
		responseChan: make(map[int64]chan *JSONRPCResponse),
		closeChan:    make(chan struct{}),
	}, nil
}

// Start starts the stdio transport
func (t *StdioTransport) Start(ctx context.Context) error {
	if t.started {
		return fmt.Errorf("transport already started")
	}

	// Create command
	cmd := exec.CommandContext(ctx, t.config.Command, t.config.Args...)

	// Set environment
	if t.config.Env != nil {
		env := []string{}
		for k, v := range t.config.Env {
			env = append(env, fmt.Sprintf("%s=%s", k, v))
		}
		cmd.Env = env
	}

	// Get stdin/stdout
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdin: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdout: %w", err)
	}

	// Use provided streams for testing.
	// Config Stdin/Stdout fields override the subprocess pipes in test scenarios.
	if t.config.Stdin != nil {
		if wc, ok := t.config.Stdin.(io.WriteCloser); ok {
			stdin = wc
		}
	}
	if t.config.Stdout != nil {
		if rc, ok := t.config.Stdout.(io.ReadCloser); ok {
			stdout = rc
		}
	}

	t.cmd = cmd
	t.stdin = stdin
	t.stdout = stdout

	// Drain stderr so the child process never blocks on a full pipe.
	// If a test provides a Stderr writer, use it; otherwise discard.
	stderrDst := io.Discard
	if t.config.Stderr != nil {
		stderrDst = t.config.Stderr
	}
	if cmd.Stderr == nil {
		stderrPipe, err := cmd.StderrPipe()
		if err == nil {
			go io.Copy(stderrDst, stderrPipe) //nolint:errcheck
		}
	}

	// Start command
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start command: %w", err)
	}

	t.started = true

	// Start reading responses in background
	go t.readResponses()

	return nil
}

// Send sends a JSON-RPC request
func (t *StdioTransport) Send(ctx context.Context, req *JSONRPCRequest) (*JSONRPCResponse, error) {
	if !t.started {
		return nil, fmt.Errorf("transport not started")
	}

	if t.closed {
		return nil, fmt.Errorf("transport closed")
	}

	// Create response channel before writing so we never miss a fast response.
	respChan := make(chan *JSONRPCResponse, 1)

	t.responseChanMu.Lock()
	t.responseChan[req.ID] = respChan
	t.responseChanMu.Unlock()

	// Marshal request
	data, err := json.Marshal(req)
	if err != nil {
		t.responseChanMu.Lock()
		delete(t.responseChan, req.ID)
		t.responseChanMu.Unlock()
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Append newline — JSON-RPC over stdio is newline-delimited.
	data = append(data, '\n')

	// Serialise writes so concurrent Send calls do not interleave JSON lines.
	t.sendMu.Lock()
	_, writeErr := t.stdin.Write(data)
	t.sendMu.Unlock()

	if writeErr != nil {
		t.responseChanMu.Lock()
		delete(t.responseChan, req.ID)
		t.responseChanMu.Unlock()
		return nil, fmt.Errorf("failed to write request: %w", writeErr)
	}

	// Wait for response
	select {
	case resp := <-respChan:
		return resp, nil
	case <-ctx.Done():
		t.responseChanMu.Lock()
		delete(t.responseChan, req.ID)
		t.responseChanMu.Unlock()
		return nil, ctx.Err()
	case <-t.closeChan:
		return nil, fmt.Errorf("transport closed")
	}
}

// SendNotification sends a JSON-RPC notification
func (t *StdioTransport) SendNotification(ctx context.Context, notification *JSONRPCNotification) error {
	if !t.started {
		return fmt.Errorf("transport not started")
	}

	if t.closed {
		return fmt.Errorf("transport closed")
	}

	// Marshal notification
	data, err := json.Marshal(notification)
	if err != nil {
		return fmt.Errorf("failed to marshal notification: %w", err)
	}

	// Append newline — JSON-RPC over stdio is newline-delimited.
	data = append(data, '\n')

	t.sendMu.Lock()
	_, err = t.stdin.Write(data)
	t.sendMu.Unlock()

	if err != nil {
		return fmt.Errorf("failed to write notification: %w", err)
	}
	return nil
}

// Close closes the transport
func (t *StdioTransport) Close() error {
	if t.closed {
		return nil
	}

	close(t.closeChan)
	t.closed = true

	// Closing stdin signals EOF to the child process.
	if t.stdin != nil {
		t.stdin.Close()
	}

	// Wait for the command to exit, with a hard deadline so Close never hangs.
	if t.cmd != nil && t.cmd.Process != nil {
		done := make(chan error, 1)
		go func() { done <- t.cmd.Wait() }()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.cmd.Process.Kill() //nolint:errcheck
			<-done
		}
	}

	return nil
}

// SetNotificationsHandler sets the handler for incoming notifications
func (t *StdioTransport) SetNotificationsHandler(handler NotificationHandler) {
	t.notificationsHandler = handler
}

// SetMethodsHandler sets the handler for incoming method calls
func (t *StdioTransport) SetMethodsHandler(handler MethodHandler) {
	t.methodsHandler = handler
}

// readResponses reads newline-delimited JSON-RPC messages from stdout.
// It uses a bufio.Reader with a 64 MB buffer to handle large MCP payloads
// without the 64 KB hard limit that bufio.Scanner imposes.
func (t *StdioTransport) readResponses() {
	reader := bufio.NewReaderSize(t.stdout, stdioMaxLineBytes)

	for {
		// ReadString('\n') returns the line including the '\n' delimiter.
		// On EOF (process exited) it may return a non-empty string with a non-nil error.
		lineStr, err := reader.ReadString('\n')
		if len(lineStr) > 0 {
			t.dispatchLine([]byte(lineStr))
		}
		if err != nil {
			// EOF or read error — process is gone.
			return
		}
	}
}

// dispatchLine parses a single JSON-RPC line and routes it to the right channel.
func (t *StdioTransport) dispatchLine(line []byte) {
	// Parse JSON-RPC message
	var raw map[string]any
	if err := json.Unmarshal(line, &raw); err != nil {
		// Invalid JSON, skip
		return
	}

	// Check if it's a response (has "id" field)
	if idRaw, ok := raw["id"]; ok {
		if idFloat, ok := idRaw.(float64); ok {
			id := int64(idFloat)

			t.responseChanMu.RLock()
			respChan, ok := t.responseChan[id]
			t.responseChanMu.RUnlock()

			if ok {
				var resp JSONRPCResponse
				if err := json.Unmarshal(line, &resp); err == nil {
					respChan <- &resp
				}

				t.responseChanMu.Lock()
				delete(t.responseChan, id)
				t.responseChanMu.Unlock()
			}
		}
	} else if method, ok := raw["method"].(string); ok {
		_ = method
		var notification JSONRPCNotification
		if err := json.Unmarshal(line, &notification); err == nil {
			t.handleNotification(&notification)
		}
	}
}

// handleNotification handles an incoming notification
func (t *StdioTransport) handleNotification(notification *JSONRPCNotification) {
	if t.notificationsHandler != nil {
		t.notificationsHandler(notification)
	}
}
