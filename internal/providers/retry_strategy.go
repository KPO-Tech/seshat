package providers

import (
	"context"
	"net/http"

	providerretry "github.com/EngineerProjects/nexus-engine/internal/providers/retry"
	"github.com/EngineerProjects/nexus-engine/internal/types"
)

type RetryStrategy = providerretry.RetryStrategy
type RetryState = providerretry.RetryState
type RequestFunc = providerretry.RequestFunc
type RetryClassification = providerretry.RetryClassification

const (
	RetryClassificationNetwork               = providerretry.RetryClassificationNetwork
	RetryClassificationRateLimit             = providerretry.RetryClassificationRateLimit
	RetryClassificationServerOverload        = providerretry.RetryClassificationServerOverload
	RetryClassificationTimeout               = providerretry.RetryClassificationTimeout
	RetryClassificationServerError           = providerretry.RetryClassificationServerError
	RetryClassificationClientError           = providerretry.RetryClassificationClientError
	RetryClassificationAuthError             = providerretry.RetryClassificationAuthError
	RetryClassificationUnknown               = providerretry.RetryClassificationUnknown
	RetryClassificationModelDeprecated       = providerretry.RetryClassificationModelDeprecated
	RetryClassificationRegionallyUnavailable = providerretry.RetryClassificationRegionallyUnavailable
	RetryClassificationQuotaExhausted        = providerretry.RetryClassificationQuotaExhausted
)

func IsModelDeprecatedError(err error, resp *http.Response) bool {
	return providerretry.IsModelDeprecatedError(err, resp)
}

func IsRegionallyUnavailableError(err error, resp *http.Response) bool {
	return providerretry.IsRegionallyUnavailableError(err, resp)
}

func RecoveryHint(classification RetryClassification, modelID string) string {
	return providerretry.RecoveryHint(classification, modelID)
}

func DefaultRetryStrategy() *RetryStrategy {
	return providerretry.DefaultRetryStrategy()
}

func NewRetryState() *RetryState {
	return providerretry.NewRetryState()
}

func Is529Error(resp *http.Response, err error) bool {
	return providerretry.Is529Error(resp, err)
}

func IsRateLimitError(resp *http.Response) bool {
	return providerretry.IsRateLimitError(resp)
}

func IsQuotaExhaustedError(err error, resp *http.Response) bool {
	return providerretry.IsQuotaExhaustedError(err, resp)
}

func ParseContextOverflowError(err error) (inputTokens, maxTokens, contextLimit int, ok bool) {
	return providerretry.ParseContextOverflowError(err)
}

func RetryWithStrategy(
	ctx context.Context,
	strategy *RetryStrategy,
	model types.ModelIdentifier,
	maxTokens int,
	fn RequestFunc,
) (*types.APIResponse, error) {
	return providerretry.RetryWithStrategy(ctx, strategy, model, maxTokens, fn)
}

func IsRetryableStatus(statusCode int) bool {
	return providerretry.IsRetryableStatus(statusCode)
}

func ClassifyHTTPError(err error, statusCode int) RetryClassification {
	return providerretry.ClassifyHTTPError(err, statusCode)
}
