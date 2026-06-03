package retry

import (
	"context"
	"net/http"

	"github.com/EngineerProjects/nexus-engine/internal/types"
)

// RetryStrategy defines advanced retry options.
type RetryStrategy struct {
	MaxRetries                 int
	MaxConsecutiveFailures     int
	FallbackModel              *types.ModelIdentifier
	Max529Errors               int
	EnableContextOverflowRetry bool
	BaseDelayMs                int64
	MaxBackoffMs               int64
}

// DefaultRetryStrategy returns the default retry strategy.
func DefaultRetryStrategy() *RetryStrategy {
	return &RetryStrategy{
		MaxRetries:                 10,
		MaxConsecutiveFailures:     3,
		FallbackModel:              nil,
		Max529Errors:               3,
		EnableContextOverflowRetry: true,
		BaseDelayMs:                500,
		MaxBackoffMs:               32000,
	}
}

// RetryState contains retry and circuit breaker state.
type RetryState struct {
	consecutiveFailures  int
	consecutive529Errors int
	lastError            error
	usedFallback         bool
	adjustedMaxTokens    int
}

// NewRetryState creates a new retry state.
func NewRetryState() *RetryState {
	return &RetryState{}
}

func (s *RetryState) IsCircuitOpen(maxFailures int) bool {
	return s.consecutiveFailures >= maxFailures
}

func (s *RetryState) ResetConsecutiveFailures() {
	s.consecutiveFailures = 0
}

func (s *RetryState) IncrementFailure() {
	s.consecutiveFailures++
}

// RequestFunc is the function executed under advanced retry control.
type RequestFunc func(ctx context.Context, attempt int, maxTokens int, model types.ModelIdentifier) (*types.APIResponse, error)

// RetryableResponseInspector allows packages to inject HTTP-specific retry logic.
type RetryableResponseInspector func(resp *http.Response, err error) bool

// RetryClassification represents the type of error for retry logic.
type RetryClassification string

const (
	RetryClassificationNetwork        RetryClassification = "network"
	RetryClassificationRateLimit      RetryClassification = "rate_limit"
	RetryClassificationServerOverload RetryClassification = "server_overload"
	RetryClassificationTimeout        RetryClassification = "timeout"
	RetryClassificationServerError    RetryClassification = "server_error"
	RetryClassificationClientError    RetryClassification = "client_error"
	RetryClassificationAuthError      RetryClassification = "auth_error"
	RetryClassificationUnknown        RetryClassification = "unknown"

	// Extended classifications for actionable recovery hints.

	// RetryClassificationModelDeprecated is returned when the provider rejects
	// the request because the model identifier is no longer available. The
	// caller should switch to an active fallback model and not retry with the
	// same model name.
	RetryClassificationModelDeprecated RetryClassification = "model_deprecated"

	// RetryClassificationRegionallyUnavailable is returned when the service is
	// reachable but not available in the configured region or endpoint. The
	// caller may retry with a different region or endpoint.
	RetryClassificationRegionallyUnavailable RetryClassification = "regionally_unavailable"

	// RetryClassificationQuotaExhausted is returned when the account has
	// exhausted its quota (distinct from a transient rate-limit). Retrying
	// immediately will not help; the caller should either wait until the
	// quota resets or switch to a different provider.
	RetryClassificationQuotaExhausted RetryClassification = "quota_exhausted"
)

// HTTPRequestFunc is the HTTP-level request function executed under retry control.
type HTTPRequestFunc func(ctx context.Context) (*http.Response, error)

// HTTPResponseErrorFunc translates a non-2xx HTTP response into an error.
// Implementations may consume and restore the response body as needed.
type HTTPResponseErrorFunc func(resp *http.Response) error

// CircuitOpenChecker reports whether an error comes from an open circuit breaker.
type CircuitOpenChecker func(err error) bool
