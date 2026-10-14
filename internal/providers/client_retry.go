package providers

import (
	"bytes"
	"context"
	"fmt"
	providerretry "github.com/KPO-Tech/seshat-runtime/internal/providers/retry"
	"io"
	"net/http"
	"time"

	"github.com/KPO-Tech/seshat-runtime/internal/types"
)

// sendMessageWithRetry sends a message request with retry logic
// This implements actual retry with exponential backoff, circuit breaker, and rate limit handling
func (c *Client) sendMessageWithRetry(ctx context.Context, req types.APIRequest) (*http.Response, error) {
	return providerretry.RetryHTTPNotify(
		ctx,
		c.retryConfig,
		func(ctx context.Context) (*http.Response, error) {
			return c.executeWithCircuitBreaker(ctx, req)
		},
		func(resp *http.Response) error {
			bodyBytes, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			resp.Body = io.NopCloser(bytes.NewReader(bodyBytes))
			err := c.handleErrorResponse(resp, nil)
			resp.Body = io.NopCloser(bytes.NewReader(bodyBytes))
			return err
		},
		IsCircuitBreakerOpenError,
		retryNotifier(ctx),
	)
}

// parseRetryAfterHeader parses the Retry-After header from a response
func (c *Client) parseRetryAfterHeader(resp *http.Response) time.Duration {
	return providerretry.ParseRetryAfterHeader(resp)
}

func (c *Client) isRetryableStatus(statusCode int) bool {
	return providerretry.IsRetryableStatus(statusCode)
}

func (c *Client) shouldRetry(err error, resp *http.Response) bool {
	if err != nil {
		// Network errors are retryable
		return true
	}

	if resp != nil && c.isRetryableStatus(resp.StatusCode) {
		return true
	}

	return false
}

// calculateAdvancedBackoff calculates next retry delay with jitter
func (c *Client) calculateAdvancedBackoff(attempt int) time.Duration {
	return providerretry.CalculateHTTPBackoff(c.retryConfig, attempt)
}

// classifyError classifies errors for selective retry
func (c *Client) classifyError(err error, statusCode int) RetryClassification {
	return providerretry.ClassifyHTTPError(err, statusCode)
}

// shouldRetryAdvanced determines if an error should trigger a retry
func (c *Client) shouldRetryAdvanced(classification RetryClassification, attempt int) bool {
	return providerretry.ShouldRetryHTTP(c.retryConfig, classification, attempt)
}

// executeWithCircuitBreaker executes an HTTP request with circuit breaker protection
func (c *Client) executeWithCircuitBreaker(ctx context.Context, req types.APIRequest) (*http.Response, error) {
	if c.circuitBreaker == nil {
		// No circuit breaker configured, execute directly
		return c.sendMessage(ctx, req)
	}

	var result *http.Response
	var resultErr error
	var retryableErr error

	// Execute through circuit breaker
	err := c.circuitBreaker.Execute(func() error {
		resp, err := c.sendMessage(ctx, req) //nolint:bodyclose // the response is returned to the caller of this function, which owns the body
		result = resp
		resultErr = err
		if err != nil {
			return err
		}
		if c.isRetryableStatus(resp.StatusCode) {
			retryableErr = &retryableStatusError{statusCode: resp.StatusCode}
			return retryableErr
		}
		return nil
	})

	// Check if circuit breaker blocked the request
	if IsCircuitBreakerOpenError(err) {
		return nil, err
	}
	if retryableErr != nil {
		return result, nil
	}

	return result, resultErr
}

// SetCircuitBreaker sets the circuit breaker for this client
func (c *Client) SetCircuitBreaker(cb *CircuitBreaker) {
	c.circuitBreaker = cb
}

// GetCircuitBreaker returns the current circuit breaker
func (c *Client) GetCircuitBreaker() *CircuitBreaker {
	return c.circuitBreaker
}

// CircuitBreakerStats returns the circuit breaker statistics
func (c *Client) CircuitBreakerStats() CircuitBreakerStats {
	if c.circuitBreaker == nil {
		return CircuitBreakerStats{}
	}
	return c.circuitBreaker.Stats()
}

// EnableCircuitBreaker enables the circuit breaker with default configuration
func (c *Client) EnableCircuitBreaker() {
	if c.circuitBreaker == nil {
		c.circuitBreaker = NewCircuitBreaker()
	}
}

// EnableCircuitBreakerWithConfig enables the circuit breaker with custom configuration
func (c *Client) EnableCircuitBreakerWithConfig(config *CircuitBreakerConfig) {
	c.circuitBreaker = NewCircuitBreakerWithConfig(config)
}

// ResetCircuitBreaker resets the circuit breaker to closed state
func (c *Client) ResetCircuitBreaker() {
	if c.circuitBreaker != nil {
		c.circuitBreaker.Reset()
	}
}

type retryableStatusError struct {
	statusCode int
}

func (e *retryableStatusError) Error() string {
	return fmt.Sprintf("retryable status %d", e.statusCode)
}

// sendStreamRequest opens a streaming request. Until the first byte of the
// response the request is retried as a request without streaming is: a 429 or a
// 5xx, a refused or reset connection, with the wait the provider asks for in
// Retry-After or an exponential one. The streaming entry points called
// sendMessage directly, so only the retry of the loop (3 attempts, 250 ms, 500 ms
// and 1 s) stood between a rate limit and a failed turn, and Retry-After was
// never read. Once the response is open, the stream itself is not retried here:
// a body that breaks half way is the loop's to recover.
func (c *Client) sendStreamRequest(ctx context.Context, req types.APIRequest) (*http.Response, error) {
	resp, err := c.sendMessageWithRetry(ctx, req)
	if err != nil {
		if resp != nil && resp.Body != nil {
			resp.Body.Close()
		}
		return nil, err
	}
	// The body is bounded by the context only; a connection that goes silent
	// must not hold the turn for ever.
	resp.Body = newIdleBody(resp.Body, streamIdleTimeout())
	return resp, nil
}

// retryNotifier turns the waits of a retry into provider.retry runtime events,
// when the request runs inside a turn that listens for them (the emitter is on
// the context). Without it a rate limit looks like a model that is thinking.
func retryNotifier(ctx context.Context) providerretry.RetryNotifier {
	emit, ok := ctx.Value(types.RuntimeEventEmitterKey).(func(types.RuntimeEvent))
	if !ok || emit == nil {
		return nil
	}
	return func(notice providerretry.RetryNotice) {
		emit(types.RuntimeEvent{
			Type: types.RuntimeEventTypeProviderRetry,
			ProviderRetryEvent: &types.ProviderRetryRuntimeEvent{
				Attempt:     notice.Attempt,
				MaxAttempts: notice.MaxAttempts,
				WaitMs:      notice.Wait.Milliseconds(),
				Reason:      notice.Reason,
			},
		})
	}
}
