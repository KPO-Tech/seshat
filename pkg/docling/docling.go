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

	// DocumentConverterBackend is anything that can convert documents to
	// markdown - Client (docling-serve) is the default implementation. See
	// internaldocling.DocumentConverterBackend's own doc comment.
	DocumentConverterBackend = internaldocling.DocumentConverterBackend

	// HybridChunkBackend is anything that can produce document-aware hybrid
	// chunks - Client (docling-serve) is the default implementation. See
	// internaldocling.HybridChunkBackend's own doc comment.
	HybridChunkBackend = internaldocling.HybridChunkBackend

	// GenericClient is a configurable client for any document-intelligence
	// HTTP server that accepts a multipart file upload and returns JSON -
	// unlike Client (docling-serve-specific), every protocol detail
	// (paths, the multipart file field name, response parsing) is supplied
	// via GenericConfig. See internaldocling.GenericClient's own doc
	// comment for the full rationale and how a concrete backend's own
	// adapter (e.g. for a seshat-intelligence deployment) is meant to use
	// this from a downstream repo.
	GenericClient = internaldocling.GenericClient

	// GenericConfig configures a GenericClient.
	GenericConfig = internaldocling.GenericConfig

	// ConvertParser translates a server's raw convert-endpoint response
	// body into ConversionResult.
	ConvertParser = internaldocling.ConvertParser

	// ChunkParser translates a server's raw chunk-endpoint response body
	// into []Chunk.
	ChunkParser = internaldocling.ChunkParser
)

// NewGenericClient validates cfg and builds a GenericClient - see
// GenericConfig's own field docs for what's required.
func NewGenericClient(cfg GenericConfig) (*GenericClient, error) {
	return internaldocling.NewGenericClient(cfg)
}

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
