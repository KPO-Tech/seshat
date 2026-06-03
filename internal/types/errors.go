package types

import "fmt"

// ErrorCode represents a specific error type
type ErrorCode string

const (
	// API errors
	ErrCodeAPIRequest   ErrorCode = "api_request"
	ErrCodeAPIResponse  ErrorCode = "api_response"
	ErrCodeAPIRateLimit ErrorCode = "api_rate_limit"
	ErrCodeAPIAuth      ErrorCode = "api_auth"
	ErrCodeAPITimeout   ErrorCode = "api_timeout"
	ErrCodeAPIInvalid   ErrorCode = "api_invalid"

	// Tool errors
	ErrCodeToolNotFound     ErrorCode = "tool_not_found"
	ErrCodeToolExecution    ErrorCode = "tool_execution"
	ErrCodeToolPermission   ErrorCode = "tool_permission"
	ErrCodeToolTimeout      ErrorCode = "tool_timeout"
	ErrCodeToolInvalidInput ErrorCode = "tool_invalid_input"

	// Permission errors
	ErrCodePermissionDenied ErrorCode = "permission_denied"
	ErrCodePermissionError  ErrorCode = "permission_error"

	// Session errors
	ErrCodeSessionNotFound    ErrorCode = "session_not_found"
	ErrCodeSessionClosed      ErrorCode = "session_closed"
	ErrCodeSessionInterrupted ErrorCode = "session_interrupted"

	// Compact errors
	ErrCodeCompactFailed ErrorCode = "compact_failed"
	ErrCodeCompactFull   ErrorCode = "compact_full"

	// General errors
	ErrCodeInvalidInput ErrorCode = "invalid_input"
	ErrCodeInternal     ErrorCode = "internal"
	ErrCodeNotSupported ErrorCode = "not_supported"
)

// EngineError represents an error in the engine
type EngineError struct {
	// Code is the error code
	Code ErrorCode `json:"code"`

	// Message is a human-readable message
	Message string `json:"message"`

	// Err is the underlying error
	Err error `json:"-"`

	// Context contains additional error context
	Context map[string]any `json:"context,omitempty"`
}

// Error implements the error interface
func (e *EngineError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Unwrap returns the underlying error
func (e *EngineError) Unwrap() error {
	return e.Err
}

// NewError creates a new EngineError
func NewError(code ErrorCode, message string) *EngineError {
	return &EngineError{
		Code:    code,
		Message: message,
	}
}

// WrapError wraps an existing error
func WrapError(code ErrorCode, message string, err error) *EngineError {
	return &EngineError{
		Code:    code,
		Message: message,
		Err:     err,
	}
}

// WrapErrorWithContext wraps an error with additional context
func WrapErrorWithContext(code ErrorCode, message string, err error, context map[string]any) *EngineError {
	return &EngineError{
		Code:    code,
		Message: message,
		Err:     err,
		Context: context,
	}
}

// IsAPIError returns true if this is an API-related error
func (e *EngineError) IsAPIError() bool {
	switch e.Code {
	case ErrCodeAPIRequest, ErrCodeAPIResponse, ErrCodeAPIRateLimit,
		ErrCodeAPIAuth, ErrCodeAPITimeout, ErrCodeAPIInvalid:
		return true
	default:
		return false
	}
}

// IsRetryable returns true if the error is retryable
func (e *EngineError) IsRetryable() bool {
	switch e.Code {
	case ErrCodeAPIRateLimit, ErrCodeAPITimeout:
		return true
	default:
		return false
	}
}

// IsPermanent returns true if the error is permanent (not retryable)
func (e *EngineError) IsPermanent() bool {
	switch e.Code {
	case ErrCodeAPIAuth, ErrCodeAPIInvalid, ErrCodePermissionDenied,
		ErrCodeToolNotFound, ErrCodeToolInvalidInput, ErrCodeInvalidInput:
		return true
	default:
		return false
	}
}
