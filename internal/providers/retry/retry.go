package retry

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/EngineerProjects/nexus-engine/internal/types"
)

func Is529Error(resp *http.Response, err error) bool {
	if err != nil {
		errMsg := strings.ToLower(err.Error())
		if strings.Contains(errMsg, "overloaded") || strings.Contains(errMsg, "529") {
			return true
		}
	}
	if resp != nil {
		return resp.StatusCode == 529
	}
	return false
}

func IsRateLimitError(resp *http.Response) bool {
	return resp != nil && resp.StatusCode == http.StatusTooManyRequests
}

func IsQuotaExhaustedError(err error, resp *http.Response) bool {
	if err == nil {
		return false
	}
	errMsg := strings.ToLower(err.Error())
	return resp != nil && resp.StatusCode == http.StatusTooManyRequests &&
		(strings.Contains(errMsg, "limit: 0") || strings.Contains(errMsg, "exceeded your current quota"))
}

// IsModelDeprecatedError returns true when the provider signals that the
// requested model is no longer available (retired, renamed, or removed).
func IsModelDeprecatedError(err error, resp *http.Response) bool {
	if err != nil {
		msg := strings.ToLower(err.Error())
		return strings.Contains(msg, "model_not_found") ||
			strings.Contains(msg, "model not found") ||
			strings.Contains(msg, "model_deprecated") ||
			strings.Contains(msg, "model has been deprecated") ||
			strings.Contains(msg, "no longer supported") ||
			strings.Contains(msg, "model is not available") ||
			strings.Contains(msg, "model was deprecated")
	}
	if resp != nil && resp.StatusCode == http.StatusNotFound {
		return true
	}
	return false
}

// IsRegionallyUnavailableError returns true when the service is reachable but
// not available in the current region or endpoint configuration.
func IsRegionallyUnavailableError(err error, resp *http.Response) bool {
	if err != nil {
		msg := strings.ToLower(err.Error())
		return strings.Contains(msg, "region") && strings.Contains(msg, "unavailable") ||
			strings.Contains(msg, "not available in your region") ||
			strings.Contains(msg, "access denied in this region") ||
			strings.Contains(msg, "geographic restriction")
	}
	if resp != nil && resp.StatusCode == http.StatusForbidden {
		// Forbidden with no auth error hint can indicate region-level denial.
		return false // handled separately in ClassifyHTTPError via header sniffing
	}
	return false
}

// RecoveryHint returns a human-readable suggestion for the operator based on
// the classification. Returns empty string for classifications that have no
// actionable hint beyond retrying.
func RecoveryHint(classification RetryClassification, modelID string) string {
	switch classification {
	case RetryClassificationModelDeprecated:
		if modelID != "" {
			return fmt.Sprintf("Model %q is no longer available. Update your provider setting to use a current model.", modelID)
		}
		return "The requested model is no longer available. Update your provider setting."
	case RetryClassificationRegionallyUnavailable:
		return "The service is not available in the current region. Check your region/endpoint configuration or switch provider."
	case RetryClassificationQuotaExhausted:
		return "Account quota exhausted. Add billing, wait for the quota reset, or switch to a different provider."
	case RetryClassificationAuthError:
		return "Authentication failed. Verify your API key is valid and has not expired."
	case RetryClassificationRateLimit:
		return "Rate limit reached. The request will be retried with exponential backoff."
	default:
		return ""
	}
}

func ParseContextOverflowError(err error) (inputTokens, maxTokens, contextLimit int, ok bool) {
	if err == nil {
		return 0, 0, 0, false
	}
	errMsg := err.Error()

	var foundTokens []int

	if strings.Contains(errMsg, "input length") && strings.Contains(errMsg, "exceed context limit") {
		var numbers []int
		for _, part := range strings.Split(errMsg, " ") {
			var num int
			if _, err := fmt.Sscanf(part, "%d", &num); err == nil && num > 0 {
				numbers = append(numbers, num)
			}
		}
		if len(numbers) >= 3 {
			foundTokens = numbers
		}
	} else if strings.Contains(errMsg, "exceeds context limit") {
		var input, max, limit int
		if _, err := fmt.Sscanf(errMsg, "Input tokens (%d) + max_tokens (%d) exceeds context limit (%d)", &input, &max, &limit); err == nil {
			foundTokens = []int{input, max, limit}
		}
	}

	if len(foundTokens) >= 3 && foundTokens[2] > 0 {
		return foundTokens[0], foundTokens[1], foundTokens[2], true
	}
	return 0, 0, 0, false
}

func RetryWithStrategy(
	ctx context.Context,
	strategy *RetryStrategy,
	model types.ModelIdentifier,
	maxTokens int,
	fn RequestFunc,
) (*types.APIResponse, error) {
	if strategy == nil {
		strategy = DefaultRetryStrategy()
	}

	state := NewRetryState()
	maxRetries := strategy.MaxRetries
	currentModel := model

	for attempt := 1; attempt <= maxRetries+1; attempt++ {
		select {
		case <-ctx.Done():
			return nil, types.WrapError(types.ErrCodeAPITimeout, "request cancelled", ctx.Err())
		default:
		}

		if state.IsCircuitOpen(strategy.MaxConsecutiveFailures) {
			return nil, types.NewError(types.ErrCodeAPIRequest,
				fmt.Sprintf("circuit breaker open after %d consecutive failures", state.consecutiveFailures))
		}

		adjustedMaxTokens := maxTokens
		if state.adjustedMaxTokens > 0 {
			adjustedMaxTokens = state.adjustedMaxTokens
		}

		resp, err := fn(ctx, attempt, adjustedMaxTokens, currentModel)
		if err == nil {
			state.ResetConsecutiveFailures()
			return resp, nil
		}

		state.lastError = err

		if IsQuotaExhaustedError(err, nil) {
			return nil, types.NewError(types.ErrCodeAPIRateLimit,
				"API quota exhausted or not enabled. Enable billing or switch provider.")
		}

		if !shouldRetryWithStrategy(err, state) {
			return nil, err
		}

		state.IncrementFailure()

		if Is529Error(nil, err) {
			state.consecutive529Errors++
			if state.consecutive529Errors >= strategy.Max529Errors && strategy.FallbackModel != nil && !state.usedFallback {
				currentModel = *strategy.FallbackModel
				state.usedFallback = true
				state.consecutive529Errors = 0
				continue
			}
		}

		if strategy.EnableContextOverflowRetry {
			inputTokens, _, contextLimit, ok := ParseContextOverflowError(err)
			if ok && contextLimit > 0 {
				safetyBuffer := 1000
				availableContext := contextLimit - inputTokens - safetyBuffer
				if availableContext > 3000 {
					state.adjustedMaxTokens = availableContext
				}
			}
		}

		delay := calculateRetryDelay(attempt, strategy.BaseDelayMs, strategy.MaxBackoffMs)
		select {
		case <-ctx.Done():
			return nil, types.WrapError(types.ErrCodeAPITimeout, "request cancelled during retry backoff", ctx.Err())
		case <-time.After(delay):
		}
	}

	return nil, types.WrapError(types.ErrCodeAPIRequest,
		fmt.Sprintf("max retry attempts (%d) reached", maxRetries), state.lastError)
}

func shouldRetryWithStrategy(err error, _ *RetryState) bool {
	if err == nil {
		return false
	}
	if engineErr, ok := err.(*types.EngineError); ok {
		return !engineErr.IsPermanent()
	}
	return true
}

func calculateRetryDelay(attempt int, baseDelayMs int64, maxDelayMs int64) time.Duration {
	delay := baseDelayMs
	for i := 1; i < attempt; i++ {
		delay = delay * 2
		if delay > maxDelayMs {
			delay = maxDelayMs
			break
		}
	}

	jitter := float64(delay) * 0.25 * float64(time.Now().UnixNano()%1000) / 1000.0
	delay = delay + int64(jitter)

	if delay > maxDelayMs {
		delay = maxDelayMs
	}

	return time.Duration(delay) * time.Millisecond
}

func IsRetryableStatus(statusCode int) bool {
	switch statusCode {
	case http.StatusTooManyRequests:
		return true
	case http.StatusServiceUnavailable:
		return true
	case http.StatusGatewayTimeout:
		return true
	case http.StatusInternalServerError:
		return true
	case http.StatusBadGateway:
		return true
	case http.StatusRequestTimeout:
		return true
	case http.StatusConflict:
		return true
	default:
		return false
	}
}

func ClassifyHTTPError(err error, statusCode int) RetryClassification {
	if err != nil {
		// Check extended classifications before falling back to generic network.
		if IsModelDeprecatedError(err, nil) {
			return RetryClassificationModelDeprecated
		}
		if IsRegionallyUnavailableError(err, nil) {
			return RetryClassificationRegionallyUnavailable
		}
		if IsQuotaExhaustedError(err, &http.Response{StatusCode: http.StatusTooManyRequests}) {
			return RetryClassificationQuotaExhausted
		}
		return RetryClassificationNetwork
	}

	switch statusCode {
	case http.StatusTooManyRequests:
		return RetryClassificationRateLimit
	case http.StatusGatewayTimeout, http.StatusServiceUnavailable:
		return RetryClassificationServerOverload
	case http.StatusRequestTimeout:
		return RetryClassificationTimeout
	case http.StatusInternalServerError:
		return RetryClassificationServerError
	case http.StatusBadRequest:
		return RetryClassificationClientError
	case http.StatusNotFound:
		// 404 from a provider most likely means the model is gone.
		return RetryClassificationModelDeprecated
	case http.StatusUnauthorized, http.StatusForbidden:
		return RetryClassificationAuthError
	default:
		if statusCode >= 500 && statusCode < 600 {
			return RetryClassificationServerError
		}
		if statusCode >= 400 && statusCode < 500 {
			return RetryClassificationClientError
		}
		return RetryClassificationUnknown
	}
}

func ShouldRetryHTTP(config types.RetryConfig, classification RetryClassification, attempt int) bool {
	maxAttempts := config.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = types.DefaultRetryConfig().MaxAttempts
	}
	if attempt >= maxAttempts {
		return false
	}

	switch classification {
	case RetryClassificationClientError, RetryClassificationAuthError:
		return false
	case RetryClassificationRateLimit,
		RetryClassificationNetwork,
		RetryClassificationServerOverload,
		RetryClassificationServerError,
		RetryClassificationTimeout:
		return true
	default:
		return attempt < 3
	}
}

func CalculateHTTPBackoff(config types.RetryConfig, attempt int) time.Duration {
	initial := config.InitialBackoff
	if initial <= 0 {
		initial = types.DefaultRetryConfig().InitialBackoff
	}
	maxBackoff := config.MaxBackoff
	if maxBackoff <= 0 {
		maxBackoff = types.DefaultRetryConfig().MaxBackoff
	}
	multiplier := config.BackoffMultiplier
	if multiplier <= 0 {
		multiplier = types.DefaultRetryConfig().BackoffMultiplier
	}

	exponentialDelay := float64(initial) * powFloat64(multiplier, float64(attempt-1))
	if exponentialDelay > float64(maxBackoff) {
		exponentialDelay = float64(maxBackoff)
	}

	jitterFactor := 0.75 + (float64(time.Now().UnixNano()%1000) / 1000.0 * 0.5)
	jitteredDelay := exponentialDelay * jitterFactor
	if jitteredDelay < float64(initial) {
		jitteredDelay = float64(initial)
	}

	return time.Duration(int64(jitteredDelay)) * time.Millisecond
}

func ParseRetryAfterHeader(resp *http.Response) time.Duration {
	if resp == nil {
		return 0
	}
	retryAfter := resp.Header.Get("Retry-After")
	if retryAfter == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(retryAfter); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	if t, err := http.ParseTime(retryAfter); err == nil {
		until := time.Until(t)
		if until > 0 {
			return until
		}
	}
	return 0
}

func RetryHTTP(
	ctx context.Context,
	config types.RetryConfig,
	send HTTPRequestFunc,
	buildResponseError HTTPResponseErrorFunc,
	isCircuitOpen CircuitOpenChecker,
) (*http.Response, error) {
	maxAttempts := config.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = types.DefaultRetryConfig().MaxAttempts
	}

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		select {
		case <-ctx.Done():
			return nil, types.WrapError(types.ErrCodeAPITimeout, "request cancelled", ctx.Err())
		default:
		}

		resp, err := send(ctx)
		if err != nil {
			lastErr = err
			if isCircuitOpen != nil && isCircuitOpen(err) {
				return nil, err
			}
			classification := ClassifyHTTPError(err, 0)
			if ShouldRetryHTTP(config, classification, attempt) {
				delay := CalculateHTTPBackoff(config, attempt)
				select {
				case <-ctx.Done():
					return nil, types.WrapError(types.ErrCodeAPITimeout, "request cancelled during backoff", ctx.Err())
				case <-time.After(delay):
				}
				continue
			}
			return nil, err
		}

		if resp.StatusCode == http.StatusOK {
			return resp, nil
		}

		if buildResponseError != nil {
			lastErr = buildResponseError(resp)
		} else {
			lastErr = types.NewError(types.ErrCodeAPIRequest, fmt.Sprintf("http status %d", resp.StatusCode))
		}

		classification := ClassifyHTTPError(nil, resp.StatusCode)
		if resp.StatusCode == http.StatusTooManyRequests && ShouldRetryHTTP(config, classification, attempt) {
			delay := ParseRetryAfterHeader(resp)
			if delay <= 0 {
				delay = CalculateHTTPBackoff(config, attempt)
			}
			select {
			case <-ctx.Done():
				return nil, types.WrapError(types.ErrCodeAPITimeout, "request cancelled during backoff", ctx.Err())
			case <-time.After(delay):
			}
			continue
		}

		if ShouldRetryHTTP(config, classification, attempt) {
			delay := CalculateHTTPBackoff(config, attempt)
			select {
			case <-ctx.Done():
				return nil, types.WrapError(types.ErrCodeAPITimeout, "request cancelled during backoff", ctx.Err())
			case <-time.After(delay):
			}
			continue
		}

		if IsRetryableStatus(resp.StatusCode) {
			return nil, lastErr
		}
		return resp, lastErr
	}

	return nil, types.WrapError(types.ErrCodeAPIRequest, fmt.Sprintf("max retry attempts (%d) reached", maxAttempts), lastErr)
}

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
