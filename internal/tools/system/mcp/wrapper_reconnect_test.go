package mcp

import (
	"context"
	"testing"
)

// TestActiveClientReturnsHealthyClientUnchanged guards the fast path: a
// healthy (connected) client must be returned as-is, with no reconnect
// attempt - reconnecting is only for a client that's actually broken.
func TestActiveClientReturnsHealthyClientUnchanged(t *testing.T) {
	transport := &fakeTransport{}
	client := &Client{transport: transport}
	client.setStatus(ClientStatusConnected, nil)

	w := NewWrapper(client, ServerConfig{Name: "srv"}, nil)

	got := w.activeClient(context.Background())
	if got != client {
		t.Fatal("expected the same healthy client instance to be returned unchanged")
	}
	if transport.closeCalls != 0 {
		t.Fatalf("expected no reconnect (and therefore no Close) for a healthy client, got %d Close call(s)", transport.closeCalls)
	}
}

// TestActiveClientReconnectFailureFallsBackToBrokenClient is the regression
// test for the fix this file's production code implements: when a cached
// client's MCP connection has died (status Failed/Closed/Expired - see
// activeClient's doc comment on why this matters once seshat-backend caches
// the whole sdk.Client for up to 30 minutes), activeClient must attempt one
// reconnect using the original ServerConfig. Here the config points at a
// nonexistent binary, so the reconnect attempt itself fails - this verifies
// that failure path is handled gracefully (no panic, no hang) and falls
// back to returning the same still-broken client so the caller gets a clear
// error instead of a nil client or a crash.
func TestActiveClientReconnectFailureFallsBackToBrokenClient(t *testing.T) {
	transport := &fakeTransport{}
	client := &Client{transport: transport}
	client.setStatus(ClientStatusFailed, nil)

	w := NewWrapper(client, badServerConfig("srv"), nil)

	got := w.activeClient(context.Background())
	if got != client {
		t.Fatal("expected the original (still-broken) client back when reconnect itself fails")
	}
	if got.Metadata().Status != ClientStatusFailed {
		t.Fatalf("expected status to remain Failed after a failed reconnect attempt, got %q", got.Metadata().Status)
	}
}
