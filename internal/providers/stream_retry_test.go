package providers

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/KPO-Tech/seshat-runtime/internal/types"
)

const okOpenAIStream = `data: {"choices":[{"delta":{"content":"hi"}}]}` + "\n\n" +
	`data: {"choices":[{"delta":{},"finish_reason":"stop"}]}` + "\n\n" +
	"data: [DONE]\n\n"

// streamServer answers the first len(statuses) requests with those statuses
// (and body/headers from setup) and every later one with a good stream.
func streamServer(t *testing.T, statuses []int, headers map[string]string, body string) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := int(requests.Add(1))
		if n <= len(statuses) {
			for k, v := range headers {
				w.Header().Set(k, v)
			}
			w.WriteHeader(statuses[n-1])
			_, _ = fmt.Fprint(w, body)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, okOpenAIStream)
	}))
	t.Cleanup(server.Close)
	return server, &requests
}

func streamOnce(t *testing.T, server *httptest.Server) (*types.APIStreamResult, error) {
	t.Helper()
	client := NewClientWithConfig("test-key", &Config{Provider: types.APIProviderOpenAI, BaseURL: server.URL})
	client.SetHTTPClient(server.Client())
	client.SetRetryConfig(types.RetryConfig{MaxAttempts: 4, InitialBackoff: 5, MaxBackoff: 20, BackoffMultiplier: 1})
	return client.CreateMessageStreamResult(context.Background(), types.APIRequest{
		Model:     types.ModelIdentifier{Provider: types.APIProviderOpenAI, Model: "gpt-4o-mini"},
		MaxTokens: 64,
		Messages:  []types.Message{types.UserMessage("msg-1", "hello")},
	})
}

// Found by an audit: only a request without streaming was retried; every real
// turn streams, so a rate limit ended the turn after the loop's three quick
// attempts.
func TestAStreamIsRetriedAfterARateLimit(t *testing.T) {
	server, requests := streamServer(t, []int{http.StatusTooManyRequests, http.StatusTooManyRequests}, nil, `{"error":{"message":"slow down"}}`)
	result, err := streamOnce(t, server)
	if err != nil {
		t.Fatalf("a rate limit that clears must not fail the request: %v", err)
	}
	if len(result.Response.Content) == 0 {
		t.Error("the retried stream must be read")
	}
	if got := requests.Load(); got != 3 {
		t.Errorf("want 3 requests (2 limited, 1 good), got %d", got)
	}
}

func TestAStreamIsRetriedAfterAServerError(t *testing.T) {
	server, requests := streamServer(t, []int{http.StatusServiceUnavailable}, nil, `{"error":{"message":"overloaded"}}`)
	if _, err := streamOnce(t, server); err != nil {
		t.Fatalf("a 503 that clears must not fail the request: %v", err)
	}
	if got := requests.Load(); got != 2 {
		t.Errorf("want 2 requests, got %d", got)
	}
}

func TestAStreamWaitsForRetryAfter(t *testing.T) {
	server, requests := streamServer(t, []int{http.StatusTooManyRequests}, map[string]string{"Retry-After": "1"}, `{"error":{"message":"slow down"}}`)
	start := time.Now()
	if _, err := streamOnce(t, server); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The configured backoff is a few milliseconds: only Retry-After explains a wait.
	if elapsed := time.Since(start); elapsed < 900*time.Millisecond {
		t.Errorf("Retry-After: 1 must be waited for, the retry came after %v", elapsed)
	}
	if got := requests.Load(); got != 2 {
		t.Errorf("want 2 requests, got %d", got)
	}
}

func TestAStreamDoesNotWaitForAVeryLongRetryAfter(t *testing.T) {
	server, requests := streamServer(t, []int{http.StatusTooManyRequests, http.StatusTooManyRequests, http.StatusTooManyRequests, http.StatusTooManyRequests, http.StatusTooManyRequests},
		map[string]string{"Retry-After": "3600"}, `{"error":{"message":"slow down"}}`)
	start := time.Now()
	_, err := streamOnce(t, server)
	if err == nil {
		t.Fatal("a provider asking for an hour must fail the request, with its message")
	}
	if time.Since(start) > 5*time.Second {
		t.Error("the request must fail at once, not wait")
	}
	if got := requests.Load(); got != 1 {
		t.Errorf("want 1 request, got %d", got)
	}
}

func TestAStreamDoesNotRetryASpentQuota(t *testing.T) {
	for _, message := range []string{
		"Insufficient balance or no resource package. Please recharge.",
		"You exceeded your current quota, please check your plan and billing details.",
		"Rate limit exceeded: free-models-per-day",
	} {
		server, requests := streamServer(t, []int{http.StatusTooManyRequests, http.StatusTooManyRequests}, nil, fmt.Sprintf(`{"error":{"message":%q}}`, message))
		if _, err := streamOnce(t, server); err == nil {
			t.Errorf("%q: a spent quota must fail the request", message)
		}
		if got := requests.Load(); got != 1 {
			t.Errorf("%q: a spent quota must not be retried, got %d requests", message, got)
		}
	}
}

func TestAStreamDoesNotRetryABadRequest(t *testing.T) {
	server, requests := streamServer(t, []int{http.StatusBadRequest, http.StatusBadRequest}, nil, `{"error":{"message":"invalid"}}`)
	if _, err := streamOnce(t, server); err == nil {
		t.Fatal("a 400 must fail the request")
	}
	if got := requests.Load(); got != 1 {
		t.Errorf("a client error must not be retried, got %d requests", got)
	}
}
