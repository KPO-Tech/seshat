package mcp

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

// closeTracker reports whether the body of a response was closed.
type closeTracker struct {
	io.ReadCloser
	closed *atomic.Bool
}

func (c closeTracker) Close() error {
	c.closed.Store(true)
	return c.ReadCloser.Close()
}

type trackingTransport struct {
	next   http.RoundTripper
	closed atomic.Bool
}

func (t *trackingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.next.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	resp.Body = closeTracker{ReadCloser: resp.Body, closed: &t.closed}
	return resp, nil
}

// A notification has no answer to wait for, but the response of the server is still read and its body closed: left open, the
// connection never goes back to the pool and a long session of notifications piles up sockets.
func TestSendNotificationClosesTheResponseBody(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	t.Cleanup(server.Close)

	transport, err := NewHTTPTransport(HTTPTransportConfig{URL: server.URL})
	if err != nil {
		t.Fatalf("NewHTTPTransport: %v", err)
	}
	tracker := &trackingTransport{next: http.DefaultTransport}
	transport.httpClient.Transport = tracker
	if err := transport.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	if err := transport.SendNotification(context.Background(), &JSONRPCNotification{JSONRPC: "2.0", Method: "notifications/initialized"}); err != nil {
		t.Fatalf("SendNotification: %v", err)
	}
	if !tracker.closed.Load() {
		t.Error("the body of the response to a notification must be closed")
	}
}
