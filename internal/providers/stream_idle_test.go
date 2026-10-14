package providers

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	providerretry "github.com/KPO-Tech/seshat-runtime/internal/providers/retry"
)

// A server that sends one event and then goes silent without closing the
// connection: before the idle timeout, the turn waited for ever.
func TestAStalledStreamFailsAfterTheIdleTimeout(t *testing.T) {
	t.Setenv(streamIdleTimeoutEnv, "1")
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, `data: {"choices":[{"delta":{"content":"hi"}}]}`+"\n\n")
		w.(http.Flusher).Flush()
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(server.Close)
	t.Cleanup(func() { close(release) }) // runs before server.Close

	start := time.Now()
	_, err := streamOnce(t, server)
	if err == nil {
		t.Fatal("a stream that went silent must fail")
	}
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Errorf("the idle timeout is 1 s, the request failed after %v", elapsed)
	}
	var idle streamIdleError
	if !errors.As(err, &idle) {
		t.Errorf("the error must say the stream went silent, got %v", err)
	}
}

func TestAStreamThatKeepsTalkingIsNotCut(t *testing.T) {
	t.Setenv(streamIdleTimeoutEnv, "1")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		// A keep-alive every 400 ms for 2.4 s: longer than the idle timeout, never silent for it.
		for i := 0; i < 6; i++ {
			_, _ = fmt.Fprint(w, ": keep-alive\n\n")
			flusher.Flush()
			time.Sleep(400 * time.Millisecond)
		}
		_, _ = fmt.Fprint(w, okOpenAIStream)
	}))
	t.Cleanup(server.Close)

	if _, err := streamOnce(t, server); err != nil {
		t.Fatalf("a stream that keeps sending bytes must not be cut: %v", err)
	}
}

func TestIdleTimeoutFromTheEnvironment(t *testing.T) {
	for value, want := range map[string]time.Duration{
		"":    defaultStreamIdleTimeout,
		"abc": defaultStreamIdleTimeout,
		"-5":  defaultStreamIdleTimeout,
		"0":   0,
		"30":  30 * time.Second,
	} {
		t.Setenv(streamIdleTimeoutEnv, value)
		if got := streamIdleTimeout(); got != want {
			t.Errorf("%s=%q: got %v, want %v", streamIdleTimeoutEnv, value, got, want)
		}
	}
}

func TestIdleBodyWithNoTimeoutIsTheBodyItself(t *testing.T) {
	body := io.NopCloser(strings.NewReader("x"))
	if got := newIdleBody(body, 0); got != body {
		t.Error("a zero idle timeout must not wrap the body")
	}
}

// The loop retries a network failure and not a client error: a silent stream
// must be the first kind.
func TestAnIdleStreamIsClassifiedAsATransientNetworkFailure(t *testing.T) {
	err := streamIdleError{idle: 3 * time.Minute}
	if got := providerretry.ClassifyHTTPError(err, 0); got != providerretry.RetryClassificationNetwork {
		t.Errorf("an idle stream must be a network failure the loop retries, got %v", got)
	}
}
