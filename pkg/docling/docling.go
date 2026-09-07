package docling

import (
	"net/http"
	"time"

	internaldocling "github.com/KPO-Tech/seshat/internal/docling"
)

type (
	APIError         = internaldocling.APIError
	Client           = internaldocling.Client
	ConversionResult = internaldocling.ConversionResult
	Chunk            = internaldocling.Chunk
	ChunkOptions     = internaldocling.ChunkOptions
	ConvertOptions   = internaldocling.ConvertOptions
	ExtractedImage   = internaldocling.ExtractedImage
	Option           = internaldocling.Option
	RetryConfig      = internaldocling.RetryConfig
)

// NewClient creates a client pointing at a docling-serve base URL.
// baseURL is typically "http://localhost:5001". Defaults to a 120s
// per-request timeout - pass WithTimeout to override it for deployments
// that expect large or scan-heavy documents.
func NewClient(baseURL string, opts ...Option) *Client {
	return internaldocling.NewClient(baseURL, opts...)
}

// WithTimeout overrides the default 120s per-request HTTP timeout.
func WithTimeout(d time.Duration) Option {
	return internaldocling.WithTimeout(d)
}

// WithHTTPClient replaces the HTTP client used for docling-serve calls.
func WithHTTPClient(client *http.Client) Option {
	return internaldocling.WithHTTPClient(client)
}

// WithAPIKey sets the X-Api-Key header sent to docling-serve.
func WithAPIKey(key string) Option {
	return internaldocling.WithAPIKey(key)
}

// WithTenantID sets the X-Tenant-Id header sent to docling-serve.
func WithTenantID(id string) Option {
	return internaldocling.WithTenantID(id)
}

// WithUserAgent sets the User-Agent header sent to docling-serve.
func WithUserAgent(userAgent string) Option {
	return internaldocling.WithUserAgent(userAgent)
}

// WithMaxResponseBytes adjusts the response body safety limit.
func WithMaxResponseBytes(n int64) Option {
	return internaldocling.WithMaxResponseBytes(n)
}

// WithRetry enables retries for transient failures on replayable requests.
func WithRetry(maxAttempts int, baseDelay, maxDelay time.Duration) Option {
	return internaldocling.WithRetry(maxAttempts, baseDelay, maxDelay)
}

// WithHealthCacheTTL caches IsAvailable results for the given duration.
func WithHealthCacheTTL(ttl time.Duration) Option {
	return internaldocling.WithHealthCacheTTL(ttl)
}
