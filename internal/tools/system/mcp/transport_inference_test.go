package mcp

import "testing"

// These guard H7 in TODO.md: NewClient used to return "unsupported transport
// type: " for any ServerConfig that left Transport unset, which is exactly
// what every MCP example in README.md, docs/sdk.md and docs/mcp.md does -
// so copying those examples connected no server and raised no error.

func TestNewClient_InfersStdioTransportFromCommand(t *testing.T) {
	client, err := NewClient(ServerConfig{
		Name:    "test",
		Command: "npx",
		Args:    []string{"-y", "some-server"},
	})
	if err != nil {
		t.Fatalf("expected Transport to be inferred as stdio from Command, got error: %v", err)
	}
	if client.config.Transport != TransportTypeStdio {
		t.Errorf("expected inferred transport %q, got %q", TransportTypeStdio, client.config.Transport)
	}
}

func TestNewClient_InfersHTTPTransportFromURL(t *testing.T) {
	client, err := NewClient(ServerConfig{
		Name: "test",
		URL:  "https://example.com/mcp",
	})
	if err != nil {
		t.Fatalf("expected Transport to be inferred as http from URL, got error: %v", err)
	}
	if client.config.Transport != TransportTypeHTTP {
		t.Errorf("expected inferred transport %q, got %q", TransportTypeHTTP, client.config.Transport)
	}
}

// An explicit Transport is never overridden by inference.
func TestNewClient_ExplicitTransportNotOverridden(t *testing.T) {
	client, err := NewClient(ServerConfig{
		Name:      "test",
		URL:       "https://example.com/mcp",
		Transport: TransportTypeSSE,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if client.config.Transport != TransportTypeSSE {
		t.Errorf("expected explicit transport %q to be preserved, got %q", TransportTypeSSE, client.config.Transport)
	}
}

func TestNewClient_NeitherCommandNorURLStillErrors(t *testing.T) {
	_, err := NewClient(ServerConfig{Name: "test"})
	if err == nil {
		t.Fatal("expected an error when neither Command nor URL is set to infer a transport from")
	}
}
