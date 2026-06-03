package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// HTTPTransport implements the HTTP transport for MCP
type HTTPTransport struct {
	// config is the transport configuration
	config HTTPTransportConfig

	// httpClient is the HTTP client
	httpClient *http.Client

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
}

// rejectMCPHTTPURL validates the URL for an MCP HTTP transport.
// It blocks cloud metadata endpoints and credential-bearing URLs while
// intentionally allowing localhost/private addresses (local MCP servers are valid).
func rejectMCPHTTPURL(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid MCP HTTP URL: %w", err)
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("MCP HTTP URL scheme must be http or https, got %q", scheme)
	}
	if parsed.User != nil {
		return fmt.Errorf("MCP HTTP URL must not contain embedded credentials")
	}
	// Block the most dangerous SSRF targets: cloud instance-metadata endpoints.
	host := strings.ToLower(parsed.Hostname())
	switch host {
	case "169.254.169.254", // AWS/GCP/Azure IMDSv1
		"fd00:ec2::254",            // AWS IMDSv6
		"metadata.google.internal", // GCP metadata
		"metadata.goog",            // GCP metadata alias
		"169.254.170.2":            // ECS task metadata
		return fmt.Errorf("MCP HTTP URL targets a cloud metadata endpoint, which is not allowed")
	}
	return nil
}

// NewHTTPTransport creates a new HTTP transport
func NewHTTPTransport(config HTTPTransportConfig) (*HTTPTransport, error) {
	if err := rejectMCPHTTPURL(config.URL); err != nil {
		return nil, err
	}
	timeout := config.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &HTTPTransport{
		config: config,
		httpClient: &http.Client{
			Timeout: timeout,
			// Re-validate every redirect target through the same SSRF guard used
			// for the initial URL. Without this, a malicious server could issue a
			// 302 to http://169.254.169.254/ and bypass rejectMCPHTTPURL.
			CheckRedirect: func(req *http.Request, _ []*http.Request) error {
				return rejectMCPHTTPURL(req.URL.String())
			},
		},
		responseChan: make(map[int64]chan *JSONRPCResponse),
	}, nil
}

// Start starts the HTTP transport
func (t *HTTPTransport) Start(ctx context.Context) error {
	if t.started {
		return fmt.Errorf("transport already started")
	}

	t.started = true
	return nil
}

// Send sends a JSON-RPC request via HTTP
func (t *HTTPTransport) Send(ctx context.Context, req *JSONRPCRequest) (*JSONRPCResponse, error) {
	if !t.started {
		return nil, fmt.Errorf("transport not started")
	}

	if t.closed {
		return nil, fmt.Errorf("transport closed")
	}

	// Marshal request
	reqData, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, "POST", t.config.URL, bytes.NewReader(reqData))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// Set headers
	httpReq.Header.Set("Content-Type", "application/json")
	for k, v := range t.config.Headers {
		httpReq.Header.Set(k, v)
	}

	// Send request
	httpResp, err := t.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer httpResp.Body.Close()

	// Check status code
	if httpResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP error: %s", httpResp.Status)
	}

	// Parse response
	var resp JSONRPCResponse
	if err := json.NewDecoder(httpResp.Body).Decode(&resp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &resp, nil
}

// SendNotification sends a JSON-RPC notification via HTTP
func (t *HTTPTransport) SendNotification(ctx context.Context, notification *JSONRPCNotification) error {
	if !t.started {
		return fmt.Errorf("transport not started")
	}

	if t.closed {
		return fmt.Errorf("transport closed")
	}

	// Marshal notification
	reqData, err := json.Marshal(notification)
	if err != nil {
		return fmt.Errorf("failed to marshal notification: %w", err)
	}

	// Create HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, "POST", t.config.URL, bytes.NewReader(reqData))
	if err != nil {
		return fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// Set headers
	httpReq.Header.Set("Content-Type", "application/json")
	for k, v := range t.config.Headers {
		httpReq.Header.Set(k, v)
	}

	// Send request (don't wait for response)
	_, err = t.httpClient.Do(httpReq)
	return err
}

// Close closes the HTTP transport
func (t *HTTPTransport) Close() error {
	if t.closed {
		return nil
	}

	t.closed = true
	t.httpClient.CloseIdleConnections()
	return nil
}

// SetNotificationsHandler sets the handler for incoming notifications
func (t *HTTPTransport) SetNotificationsHandler(handler NotificationHandler) {
	t.notificationsHandler = handler
}

// SetMethodsHandler sets the handler for incoming method calls
func (t *HTTPTransport) SetMethodsHandler(handler MethodHandler) {
	t.methodsHandler = handler
}
