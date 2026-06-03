package providers

import (
	"bytes"
	"context"
	"fmt"
	providerretry "github.com/EngineerProjects/nexus-engine/internal/providers/retry"
	"io"
	"net/http"
	"time"

	"github.com/EngineerProjects/nexus-engine/internal/types"
)

// sendMessageWithRetry sends a message request with retry logic
// This implements actual retry with exponential backoff, circuit breaker, and rate limit handling
func (c *Client) sendMessageWithRetry(ctx context.Context, req types.APIRequest) (*http.Response, error) {
	return providerretry.RetryHTTP(
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
	)
}

// calculateBackoff calculates the next backoff delay with jitter
func calculateBackoff(baseDelay, maxBackoff int64, multiplier float64, attempt int64) time.Duration {
	delay := baseDelay
	for i := int64(1); i < attempt; i++ {
		delay = int64(float64(delay) * multiplier)
		if delay > maxBackoff {
			delay = maxBackoff
			break
		}
	}

	// Add jitter (0-25%)
	jitter := float64(delay) * 0.25 * float64(time.Now().UnixNano()%1000) / 1000.0
	delay = delay + int64(jitter)

	if delay > maxBackoff {
		delay = maxBackoff
	}

	return time.Duration(delay) * time.Millisecond
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

func (c *Client) calculateNextBackoff(current int64) int64 {
	next := float64(current) * c.retryConfig.BackoffMultiplier
	max := float64(c.retryConfig.MaxBackoff)
	if next > max {
		return c.retryConfig.MaxBackoff
	}
	return int64(next)
}

func (c *Client) waitDuration(duration int64) <-chan time.Time {
	return time.After(time.Duration(duration) * time.Millisecond)
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

// powFloat64 calculates base^exponent for float64 values
func powFloat64(base, exponent float64) float64 {
	if exponent == 0 {
		return 1
	}
	if exponent == 1 {
		return base
	}

	result := base
	for i := 2; i <= int(exponent); i++ {
		result *= base
	}

	return result
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
		resp, err := c.sendMessage(ctx, req)
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
