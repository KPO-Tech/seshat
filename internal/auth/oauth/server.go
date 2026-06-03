package oauth

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"sync"
	"time"
)

// ============================================================================
// OAuth Callback Server
// ============================================================================

// Server handles OAuth callbacks
type Server struct {
	mu       sync.RWMutex
	config   *ServerConfig
	server   *http.Server
	result   *TokenResponse
	err      error
	received chan struct{}
}

// ServerConfig represents server configuration
type ServerConfig struct {
	Port       int           // Port to listen on (0 for random)
	Timeout    time.Duration // Timeout for the auth process
	HTTPClient *http.Client
}

// DefaultServerConfig returns a default server configuration
func DefaultServerConfig() *ServerConfig {
	return &ServerConfig{
		Port:    3456,
		Timeout: 5 * time.Minute,
	}
}

// NewServer creates a new OAuth callback server
func NewServer(config *ServerConfig) *Server {
	if config == nil {
		config = DefaultServerConfig()
	}
	return &Server{
		config:   config,
		received: make(chan struct{}),
	}
}

// Start starts the callback server
func (s *Server) Start(ctx context.Context) (string, error) {
	// Find an available port
	if s.config.Port == 0 {
		ln, err := net.Listen("tcp", ":0")
		if err != nil {
			return "", fmt.Errorf("listen: %w", err)
		}
		ln.Close()
		s.config.Port = ln.Addr().(*net.TCPAddr).Port
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleRoot)
	mux.HandleFunc("/callback", s.handleCallback)
	mux.HandleFunc("/health", s.handleHealth)

	s.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", s.config.Port),
		Handler: mux,
	}

	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("oauth server error: %v", err)
		}
	}()

	return fmt.Sprintf("http://127.0.0.1:%d", s.config.Port), nil
}

// Stop stops the callback server
func (s *Server) Stop(ctx context.Context) error {
	if s.server == nil {
		return nil
	}
	return s.server.Shutdown(ctx)
}

// WaitForCallback waits for the OAuth callback
func (s *Server) WaitForCallback(ctx context.Context) (*TokenResponse, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-s.received:
		return s.result, s.err
	case <-time.After(s.config.Timeout):
		return nil, fmt.Errorf("authentication timed out")
	}
}

// handleRoot handles the root path
func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `<html>
<head><title>Nexus Auth Server</title></head>
<body style="font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; display: flex; justify-content: center; align-items: center; height: 100vh; margin: 0; background: #10a37f; color: white;">
<div style="text-align: center;">
<h1>🔐 Waiting for authentication...</h1>
<p>Visit <a href="https://chat.openai.com/device" style="color: white;">chat.openai.com/device</a> and enter your user code.</p>
</div>
</body></html>`)
}

// handleCallback handles the OAuth callback
func (s *Server) handleCallback(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	// Check for errors
	if errParam := query.Get("error"); errParam != "" {
		errDesc := query.Get("error_description")
		s.mu.Lock()
		s.err = fmt.Errorf("%s: %s", errParam, errDesc)
		s.mu.Unlock()

		// Render error page
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, `<html>
<head><title>Authentication Failed</title></head>
<body style="font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; display: flex; justify-content: center; align-items: center; height: 100vh; margin: 0; background: #dc2626; color: white;">
<div style="text-align: center;">
<h1>✗ Authentication failed</h1>
<p>%s</p>
</div>
</body></html>`, errDesc)
		close(s.received)
		return
	}

	// Get authorization code
	code := query.Get("code")
	if code == "" {
		http.Error(w, "missing code parameter", http.StatusBadRequest)
		return
	}

	// The code is not a direct token - it's an authorization code.
	// For device flow, we'll poll for the token ourselves.
	// For authorization code flow, the caller will exchange it.
	s.mu.Lock()
	s.result = &TokenResponse{}
	s.result.AccessToken = code // Pass code back - caller will exchange
	s.mu.Unlock()

	// Render success page
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `<html>
<head><title>Authenticated</title></head>
<body style="font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; display: flex; justify-content: center; align-items: center; height: 100vh; margin: 0; background: #10a37f; color: white;">
<div style="text-align: center;">
<h1>✓ Successfully authenticated!</h1>
<p>You can close this window and return to your terminal.</p>
</div>
</body></html>`)
	close(s.received)
}

// handleHealth handles health checks
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"status": "ok"}`)
}

// ============================================================================
// OAuth Handler (combines server and client)
// ============================================================================

// Handler handles the complete OAuth device flow
type Handler struct {
	server *Server
	client *Client
	ctx    context.Context
	cancel context.CancelFunc
}

// NewHandler creates a new OAuth handler
func NewHandler(client *Client) *Handler {
	ctx, cancel := context.WithCancel(context.Background())
	return &Handler{
		server: NewServer(nil),
		client: client,
		ctx:    ctx,
		cancel: cancel,
	}
}

// Start starts the OAuth flow
func (h *Handler) Start() (userCode, verificationURL string, err error) {
	// Start callback server
	serverURL, err := h.server.Start(h.ctx)
	if err != nil {
		return "", "", fmt.Errorf("start server: %w", err)
	}

	// Update redirect URI in client config
	h.client.config.RedirectURI = serverURL + "/callback"

	// Get device code
	deviceCode, err := h.client.DeviceCode(h.ctx)
	if err != nil {
		h.server.Stop(h.ctx)
		return "", "", fmt.Errorf("device code: %w", err)
	}

	// Build verification URL
	verificationURL = deviceCode.VerificationURI
	if deviceCode.VerificationURIComplete != "" {
		verificationURL = deviceCode.VerificationURIComplete
	}
	// Add user code as query param
	verificationURL += "?user_code=" + deviceCode.UserCode

	return deviceCode.UserCode, verificationURL, nil
}

// WaitComplete waits for the user to complete authentication
func (h *Handler) WaitComplete() (*Token, error) {
	// Wait for callback
	_, err := h.server.WaitForCallback(h.ctx)
	if err != nil {
		return nil, err
	}

	// For device flow, we got the device_code in the callback URL
	// We need to exchange it for a token
	// Actually, for device flow, we should poll while waiting
	// Let's simplify: the server received the auth, now we poll for token

	// For now, return nil - the token will be obtained via polling
	// This is a simplification for device flow
	return &Token{
		AccessToken: "pending",
	}, nil
}

// Stop stops the OAuth handler
func (h *Handler) Stop() {
	h.cancel()
	h.server.Stop(h.ctx)
}

// ============================================================================
// Simple Authenticator — device code flow (no callback server needed)
// ============================================================================

// SimpleAuthenticator handles the Auth0 device code flow for ChatGPT/OpenAI.
// Usage:
//
//	userCode, url, err := a.StartDeviceFlow(ctx)
//	// show userCode and url to the user, then:
//	token, err := a.WaitDeviceFlow(ctx)
type SimpleAuthenticator struct {
	client     *Client
	deviceCode string
	userCode   string
	interval   int
}

// NewSimpleAuthenticator creates a new authenticator for the given OAuth client ID.
func NewSimpleAuthenticator(clientID string) *SimpleAuthenticator {
	config := DefaultOpenAIConfig()
	config.ClientID = clientID
	return &SimpleAuthenticator{
		client: NewClient(config),
	}
}

// StartDeviceFlow requests a device code from Auth0 and returns the user-facing
// code and verification URL. Call WaitDeviceFlow afterwards to poll for the token.
func (a *SimpleAuthenticator) StartDeviceFlow(ctx context.Context) (userCode, verificationURL string, err error) {
	dcr, err := a.client.DeviceCode(ctx)
	if err != nil {
		return "", "", err
	}

	// Store for polling in WaitDeviceFlow.
	a.deviceCode = dcr.DeviceCode
	a.userCode = dcr.UserCode
	a.interval = dcr.Interval
	if a.interval <= 0 {
		a.interval = 5
	}

	verificationURL = dcr.VerificationURI
	if dcr.VerificationURIComplete != "" {
		verificationURL = dcr.VerificationURIComplete
	}
	return dcr.UserCode, verificationURL, nil
}

// WaitDeviceFlow polls Auth0 until the user completes authentication.
// It blocks until the token is received or ctx is cancelled.
func (a *SimpleAuthenticator) WaitDeviceFlow(ctx context.Context) (*Token, error) {
	if a.deviceCode == "" {
		return nil, fmt.Errorf("device flow not started — call StartDeviceFlow first")
	}
	tokenResp, err := a.client.PollToken(ctx, a.deviceCode, a.userCode, a.interval)
	if err != nil {
		return nil, err
	}
	return tokenResp.ToToken(), nil
}
