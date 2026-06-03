package fetch

import "errors"

var (
	// ErrInvalidURL indicates an invalid URL
	ErrInvalidURL = errors.New("invalid URL")

	// ErrDomainBlocked indicates domain is blocked
	ErrDomainBlocked = errors.New("domain is blocked by security policy")

	// ErrDomainCheckFailed indicates domain check failure
	ErrDomainCheckFailed = errors.New("unable to verify domain safety")

	// ErrTooManyRedirects indicates too many redirects
	ErrTooManyRedirects = errors.New("too many redirects")

	// ErrContentTooLarge indicates content exceeds limit
	ErrContentTooLarge = errors.New("content exceeds maximum size")

	// ErrTimeout indicates request timeout
	ErrTimeout = errors.New("request timeout")

	// ErrAborted indicates request was aborted
	ErrAborted = errors.New("request aborted")
)

// Err creates a new error with message
func Err(msg string) error {
	return errors.New(msg)
}
