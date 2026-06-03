package retry

import (
	"errors"
	"testing"

	"github.com/EngineerProjects/nexus-engine/internal/types"
)

// TestShouldRetryWithStrategy_NilError verifies nil always returns false.
func TestShouldRetryWithStrategy_NilError(t *testing.T) {
	if shouldRetryWithStrategy(nil, nil) {
		t.Error("expected false for nil error")
	}
}

// TestShouldRetryWithStrategy_PermanentEngineErrors verifies permanent EngineErrors
// are not retried regardless of network conditions.
func TestShouldRetryWithStrategy_PermanentEngineErrors(t *testing.T) {
	permanent := []types.ErrorCode{
		types.ErrCodeAPIAuth,
		types.ErrCodeAPIInvalid,
		types.ErrCodePermissionDenied,
		types.ErrCodeToolNotFound,
		types.ErrCodeToolInvalidInput,
		types.ErrCodeInvalidInput,
	}
	for _, code := range permanent {
		err := types.NewError(code, "permanent failure")
		if shouldRetryWithStrategy(err, nil) {
			t.Errorf("shouldRetryWithStrategy(%s) = true, want false (permanent error)", code)
		}
	}
}

// TestShouldRetryWithStrategy_RetryableEngineErrors verifies retryable EngineErrors
// are retried (rate limit and timeout are canonical retryable codes).
func TestShouldRetryWithStrategy_RetryableEngineErrors(t *testing.T) {
	retryable := []types.ErrorCode{
		types.ErrCodeAPIRateLimit,
		types.ErrCodeAPITimeout,
	}
	for _, code := range retryable {
		err := types.NewError(code, "transient failure")
		if !shouldRetryWithStrategy(err, nil) {
			t.Errorf("shouldRetryWithStrategy(%s) = false, want true (retryable error)", code)
		}
	}
}

// TestShouldRetryWithStrategy_AmbiguousEngineErrors verifies EngineErrors that are
// neither explicitly permanent nor explicitly retryable (e.g. api_request, internal)
// are still retried — they may be transient server errors.
func TestShouldRetryWithStrategy_AmbiguousEngineErrors(t *testing.T) {
	ambiguous := []types.ErrorCode{
		types.ErrCodeAPIRequest,
		types.ErrCodeAPIResponse,
		types.ErrCodeInternal,
	}
	for _, code := range ambiguous {
		err := types.NewError(code, "ambiguous failure")
		if !shouldRetryWithStrategy(err, nil) {
			t.Errorf("shouldRetryWithStrategy(%s) = false, want true (not permanently excluded)", code)
		}
	}
}

// TestShouldRetryWithStrategy_NonEngineErrorRetried verifies plain Go errors
// (network errors, etc.) are retried.
func TestShouldRetryWithStrategy_NonEngineErrorRetried(t *testing.T) {
	if !shouldRetryWithStrategy(errors.New("connection reset by peer"), nil) {
		t.Error("expected true for plain network error")
	}
}
