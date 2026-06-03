package mcp

import (
	"context"
	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func newRegistry(t *testing.T) *tool.Registry {
	t.Helper()
	return tool.NewRegistry()
}

// badServerConfig returns a ServerConfig that will always fail to connect
// (the command does not exist).
func badServerConfig(name string) ServerConfig {
	return ServerConfig{
		Name:      name,
		Command:   "__nexus_mcp_no_such_binary__",
		Args:      nil,
		Transport: TransportTypeStdio,
	}
}

// ---------------------------------------------------------------------------
// Unit tests
// ---------------------------------------------------------------------------

// When a server fails to connect, its ServerResult must carry the error.
// The top-level MCPTools slice must be empty and Error must be set.
func TestIntegrateMCPServers_SingleFailure_CapturedInServerResult(t *testing.T) {
	reg := newRegistry(t)
	cfg := []ServerConfig{badServerConfig("bad-server")}

	result := IntegrateMCPServersWithOptions(context.Background(), reg, cfg, nil)

	if result == nil {
		t.Fatal("expected non-nil IntegrationResult")
	}
	if len(result.ServerResults) != 1 {
		t.Fatalf("expected 1 ServerResult, got %d", len(result.ServerResults))
	}
	sr := result.ServerResults[0]
	if sr.Name != "bad-server" {
		t.Errorf("ServerResult.Name = %q, want %q", sr.Name, "bad-server")
	}
	if sr.Error == nil {
		t.Error("ServerResult.Error should be non-nil for a server that failed to connect")
	}
	if sr.ToolsRegistered != 0 {
		t.Errorf("ToolsRegistered should be 0 for a failed server, got %d", sr.ToolsRegistered)
	}
	if len(result.MCPTools) != 0 {
		t.Errorf("MCPTools should be empty when all servers fail, got %d", len(result.MCPTools))
	}
	if result.Error == nil {
		t.Error("top-level Error should be set when no tools were registered")
	}
}

// Multiple servers configured — all fail — ServerResults must have one entry
// per configured server.
func TestIntegrateMCPServers_MultipleFailures_AllCaptured(t *testing.T) {
	reg := newRegistry(t)
	cfg := []ServerConfig{
		badServerConfig("srv-1"),
		badServerConfig("srv-2"),
		badServerConfig("srv-3"),
	}

	result := IntegrateMCPServersWithOptions(context.Background(), reg, cfg, nil)

	if len(result.ServerResults) != 3 {
		t.Fatalf("expected 3 ServerResults, got %d", len(result.ServerResults))
	}
	for i, sr := range result.ServerResults {
		if sr.Error == nil {
			t.Errorf("ServerResults[%d] (%q): Error should be non-nil", i, sr.Name)
		}
	}
	if result.Error == nil {
		t.Error("top-level Error should be set when all servers fail")
	}
}

// Zero servers configured: result must be non-nil, empty, and Error must be nil
// (not a "total failure" because nothing was attempted).
func TestIntegrateMCPServers_EmptyConfig_NoError(t *testing.T) {
	reg := newRegistry(t)

	result := IntegrateMCPServersWithOptions(context.Background(), reg, nil, nil)

	if result == nil {
		t.Fatal("expected non-nil IntegrationResult even for empty config")
	}
	if result.Error != nil {
		t.Errorf("Error should be nil for empty config, got: %v", result.Error)
	}
	if len(result.ServerResults) != 0 {
		t.Errorf("ServerResults should be empty for empty config, got %d entries", len(result.ServerResults))
	}
	if len(result.MCPTools) != 0 {
		t.Errorf("MCPTools should be empty for empty config, got %d", len(result.MCPTools))
	}
}

// Each failed server must have a distinct Name in its ServerResult.
func TestIntegrateMCPServers_ServerResultNames_MatchConfig(t *testing.T) {
	reg := newRegistry(t)
	cfg := []ServerConfig{
		badServerConfig("alpha"),
		badServerConfig("beta"),
	}

	result := IntegrateMCPServersWithOptions(context.Background(), reg, cfg, nil)

	names := make(map[string]bool)
	for _, sr := range result.ServerResults {
		names[sr.Name] = true
	}
	for _, want := range []string{"alpha", "beta"} {
		if !names[want] {
			t.Errorf("ServerResult for %q not found in results", want)
		}
	}
}

type fakeTransport struct {
	closeCalls int
}

func (t *fakeTransport) Start(ctx context.Context) error { return nil }
func (t *fakeTransport) Send(ctx context.Context, req *JSONRPCRequest) (*JSONRPCResponse, error) {
	return nil, nil
}
func (t *fakeTransport) SendNotification(ctx context.Context, notification *JSONRPCNotification) error {
	return nil
}
func (t *fakeTransport) Close() error {
	t.closeCalls++
	return nil
}
func (t *fakeTransport) SetNotificationsHandler(handler NotificationHandler) {}
func (t *fakeTransport) SetMethodsHandler(handler MethodHandler)             {}

func TestIntegrationResultCloseClosesIntegratedClientsOnce(t *testing.T) {
	firstTransport := &fakeTransport{}
	secondTransport := &fakeTransport{}
	result := &IntegrationResult{
		clients: []*Client{
			{transport: firstTransport},
			{transport: secondTransport},
		},
	}

	if err := result.Close(); err != nil {
		t.Fatalf("first Close failed: %v", err)
	}
	if err := result.Close(); err != nil {
		t.Fatalf("second Close failed: %v", err)
	}
	if firstTransport.closeCalls != 1 {
		t.Fatalf("expected first client to close once, got %d", firstTransport.closeCalls)
	}
	if secondTransport.closeCalls != 1 {
		t.Fatalf("expected second client to close once, got %d", secondTransport.closeCalls)
	}
}

func TestWrapperWrapToolReturnsErrorForInvalidDefinition(t *testing.T) {
	wrapper := NewWrapper(nil, "server", nil)

	_, err := wrapper.WrapTool(Tool{
		Name:        "",
		Description: "invalid tool with empty name",
	})
	if err == nil {
		t.Fatal("expected invalid MCP tool wrapper to return an error")
	}
}

// ---------------------------------------------------------------------------
// MCP HTTP SSRF tests
// ---------------------------------------------------------------------------

func TestRejectMCPHTTPURL_ValidURLs(t *testing.T) {
	valid := []string{
		"http://localhost:8080/mcp",
		"http://127.0.0.1:3000",
		"http://192.168.1.100:9000",
		"https://my-mcp-server.example.com/rpc",
		"http://10.0.0.1:8080",
	}
	for _, u := range valid {
		if err := rejectMCPHTTPURL(u); err != nil {
			t.Errorf("expected %q to be allowed, got: %v", u, err)
		}
	}
}

func TestRejectMCPHTTPURL_BlockedSchemes(t *testing.T) {
	blocked := []string{
		"file:///etc/passwd",
		"ftp://example.com/data",
		"ws://example.com/mcp",
	}
	for _, u := range blocked {
		if err := rejectMCPHTTPURL(u); err == nil {
			t.Errorf("expected %q to be blocked, but was allowed", u)
		}
	}
}

func TestRejectMCPHTTPURL_BlockedCredentials(t *testing.T) {
	blocked := []string{
		"http://user:pass@localhost/mcp",
		"https://admin:secret@example.com/rpc",
	}
	for _, u := range blocked {
		if err := rejectMCPHTTPURL(u); err == nil {
			t.Errorf("expected %q (with credentials) to be blocked", u)
		} else if !strings.Contains(err.Error(), "credential") {
			t.Errorf("unexpected error message for credential URL %q: %v", u, err)
		}
	}
}

func TestRejectMCPHTTPURL_CloudMetadataBlocked(t *testing.T) {
	blocked := []string{
		"http://169.254.169.254/latest/meta-data/",
		"http://169.254.169.254/",
		"http://metadata.google.internal/computeMetadata/v1/",
		"http://metadata.goog/",
		"http://169.254.170.2/v2/credentials/",
	}
	for _, u := range blocked {
		err := rejectMCPHTTPURL(u)
		if err == nil {
			t.Errorf("expected cloud metadata URL %q to be blocked", u)
		} else if !strings.Contains(err.Error(), "metadata") {
			t.Errorf("unexpected error for metadata URL %q: %v", u, err)
		}
	}
}

func TestRejectMCPHTTPURL_InvalidURL(t *testing.T) {
	if err := rejectMCPHTTPURL("://not-a-url"); err == nil {
		t.Error("expected invalid URL to be rejected")
	}
}

func TestNewHTTPTransport_BlocksMetadata(t *testing.T) {
	_, err := NewHTTPTransport(HTTPTransportConfig{URL: "http://169.254.169.254/"})
	if err == nil {
		t.Error("expected NewHTTPTransport to reject cloud metadata URL")
	}
}

func TestNewHTTPTransport_AllowsLocalhost(t *testing.T) {
	transport, err := NewHTTPTransport(HTTPTransportConfig{URL: "http://localhost:8080/mcp"})
	if err != nil {
		t.Errorf("NewHTTPTransport should allow localhost: %v", err)
	}
	if transport != nil {
		_ = transport.Close()
	}
}

// ---------------------------------------------------------------------------
// MCP HTTP SSRF-via-redirect tests
// ---------------------------------------------------------------------------

// TestHTTPTransport_BlocksRedirectToMetadata verifies that a server-issued
// redirect to a cloud metadata endpoint is blocked even though the initial URL
// was valid. This covers the gap where rejectMCPHTTPURL only ran on the first
// URL but http.Client would silently follow the 302.
func TestHTTPTransport_BlocksRedirectToMetadata(t *testing.T) {
	redirectTargets := []string{
		"http://169.254.169.254/latest/meta-data/",
		"http://metadata.google.internal/computeMetadata/v1/",
		"http://169.254.170.2/v2/credentials/",
	}

	for _, target := range redirectTargets {
		t.Run(target, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Redirect(w, r, target, http.StatusFound)
			}))
			defer srv.Close()

			transport, err := NewHTTPTransport(HTTPTransportConfig{URL: srv.URL + "/mcp"})
			if err != nil {
				t.Fatalf("NewHTTPTransport failed: %v", err)
			}
			defer transport.Close()

			if err := transport.Start(context.Background()); err != nil {
				t.Fatalf("Start failed: %v", err)
			}

			req := &JSONRPCRequest{JSONRPC: "2.0", Method: "initialize", ID: 1}
			_, err = transport.Send(context.Background(), req)
			if err == nil {
				t.Errorf("expected Send to fail when server redirects to %q, but got no error", target)
			}
		})
	}
}

// TestHTTPTransport_AllowsLegitimateRedirect verifies that a redirect between
// two safe URLs (e.g. http → https on the same host) is not blocked.
func TestHTTPTransport_AllowsLegitimateRedirect(t *testing.T) {
	// Final destination: a trivial JSON-RPC response.
	dst := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{}}`))
	}))
	defer dst.Close()

	// Redirect source: points to dst (both are loopback — both are valid).
	src := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, dst.URL+"/mcp", http.StatusFound)
	}))
	defer src.Close()

	transport, err := NewHTTPTransport(HTTPTransportConfig{URL: src.URL + "/mcp"})
	if err != nil {
		t.Fatalf("NewHTTPTransport failed: %v", err)
	}
	defer transport.Close()

	if err := transport.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	req := &JSONRPCRequest{JSONRPC: "2.0", Method: "initialize", ID: 1}
	_, err = transport.Send(context.Background(), req)
	if err != nil {
		t.Errorf("expected legitimate redirect to succeed, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// MCP stdio framing tests
// ---------------------------------------------------------------------------

func TestStdioTransport_NewlineSeparation(t *testing.T) {
	// Verify dispatchLine handles lines both with and without trailing newline.
	transport := &StdioTransport{
		responseChan: make(map[int64]chan *JSONRPCResponse),
		closeChan:    make(chan struct{}),
	}

	// Register a pending request.
	respChan := make(chan *JSONRPCResponse, 1)
	transport.responseChanMu.Lock()
	transport.responseChan[42] = respChan
	transport.responseChanMu.Unlock()

	// Dispatch a line that ends with \n.
	transport.dispatchLine([]byte(`{"jsonrpc":"2.0","id":42,"result":{"ok":true}}` + "\n"))

	select {
	case resp := <-respChan:
		if resp == nil {
			t.Error("expected non-nil response")
		}
	default:
		t.Error("expected response to be dispatched")
	}
}

func TestStdioTransport_LargePayloadSizeConstant(t *testing.T) {
	// Verify the constant is large enough for realistic MCP payloads.
	const expectedMinBytes = 1 * 1024 * 1024 // 1 MB minimum
	if stdioMaxLineBytes < expectedMinBytes {
		t.Errorf("stdioMaxLineBytes=%d should be at least %d", stdioMaxLineBytes, expectedMinBytes)
	}
}

// ---------------------------------------------------------------------------
// AddMcpServer name validation tests
// ---------------------------------------------------------------------------

func TestAddMcpServer_ValidNames(t *testing.T) {
	valid := []string{
		"my-server",
		"my_server",
		"MyServer",
		"server123",
		"a",
		"A-B_C-123",
	}
	for _, name := range valid {
		if !validMCPServerNameRe.MatchString(name) {
			t.Errorf("expected %q to be a valid MCP server name", name)
		}
	}
}

func TestAddMcpServer_InvalidNames(t *testing.T) {
	invalid := []string{
		"",               // empty
		"server name",    // space
		"server/path",    // slash
		"server.name",    // dot
		"../traversal",   // path traversal
		"server\x00null", // null byte
		"[a-z]*",         // regex chars (the old bug: was accidentally allowed)
		"^inject$",       // regex chars
	}
	for _, name := range invalid {
		if validMCPServerNameRe.MatchString(name) {
			t.Errorf("expected %q to be an invalid MCP server name", name)
		}
	}
}
