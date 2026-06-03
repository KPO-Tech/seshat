package transport

import (
	"net/http"
	"time"
)

// HTTPRoundTripper is the transport-level round tripper contract used by provider clients.
type HTTPRoundTripper = http.RoundTripper

// NewHTTPClient creates an HTTP client with an optional custom round tripper.
func NewHTTPClient(roundTripper HTTPRoundTripper, timeout time.Duration) *http.Client {
	client := &http.Client{Timeout: timeout}
	if roundTripper != nil {
		client.Transport = roundTripper
	}
	return client
}
