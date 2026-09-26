package documentreader

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ConvertParser translates a server's raw convert-endpoint response body
// into seshat's own ConversionResult shape. Client's own parseResponse
// (docling-serve's JSON shape) is one example; a different server gets its
// own parser doing the equivalent field-by-field translation.
type ConvertParser func(rawBody []byte) (*ConversionResult, error)

// ChunkParser translates a server's raw chunk-endpoint response body into
// seshat's own []Chunk shape. Client's own parseChunkResponse is one
// example.
type ChunkParser func(rawBody []byte) ([]Chunk, error)

// GenericClient is a configurable client for any document-intelligence HTTP
// server that accepts a multipart file upload and returns JSON - the shape
// docling-serve, seshat-intelligence, and similar services all share, even
// though each names its own endpoints and JSON fields differently.
//
// Unlike Client (docling-serve-specific: hardcoded paths, a fixed JSON
// shape, docling-serve's own conversion/chunking option names), every
// protocol detail here is supplied by the caller at construction time via
// GenericConfig: paths, the multipart file field name, and functions
// translating that server's own JSON into the same ConversionResult/Chunk
// types Client produces. The rest of seshat
// (internal/rag.HybridDocumentChunker, internal/pdfsmart, the file-read/convert
// tools) depends only on the Converter/HybridChunker
// interfaces, so it works unchanged regardless of which concrete backend a
// host configures.
//
// A concrete backend's own adapter is typically small: paths + a parser
// function mapping its response fields onto ConversionResult/Chunk, then
// NewGenericClient. It doesn't need to live in this package, or even this
// repo - GenericClient (and the ConvertParser/ChunkParser it's built from)
// are exported specifically so a private downstream repo (e.g.
// seshat-backend, for its own seshat-intelligence deployment) can build
// one without needing a change here first.
type GenericClient struct {
	transport *httpTransport

	fileField    string
	convertPath  string
	chunkPath    string
	healthPath   string
	parseConvert ConvertParser
	parseChunk   ChunkParser
}

// GenericConfig configures a GenericClient. BaseURL and FileField are
// always required; ConvertPath/ParseConvert and ChunkPath/ParseChunk are
// each optional as a pair - configure whichever capabilities the target
// server actually offers (see NewGenericClient's validation).
type GenericConfig struct {
	// BaseURL is the server's base URL, e.g. "http://localhost:5100" for a
	// local seshat-intelligence instance. Required.
	BaseURL string

	// FileField is the multipart form field name the server expects the
	// uploaded file under. docling-serve uses "files"; seshat-intelligence
	// uses "file" (FastAPI's UploadFile parameter is named `file` in its
	// route signature, and FastAPI binds the multipart field name to the
	// parameter name). Required.
	FileField string

	// ConvertPath is the endpoint ConvertFile/ConvertBytes POST the file
	// to, e.g. "/v1/documents" for seshat-intelligence (vs docling-serve's
	// "/v1/convert/file"). Leave empty (with ParseConvert nil) to leave
	// conversion unsupported - ConvertFile/ConvertBytes/ConvertURL then
	// return a clear error instead of attempting a request.
	ConvertPath  string
	ParseConvert ConvertParser

	// ChunkPath is the endpoint ChunkHybridBytes/ChunkHybridFile POST the
	// file to, e.g. "/v1/documents/chunks" for seshat-intelligence (vs
	// docling-serve's "/v1/chunk/hybrid/file"). Leave empty (with
	// ParseChunk nil) to leave chunking unsupported.
	ChunkPath  string
	ParseChunk ChunkParser

	// HealthPath is the endpoint IsAvailable GETs. Defaults to "/health" -
	// true for both docling-serve and seshat-intelligence today, but
	// overridable for a server that names it differently.
	HealthPath string

	// The remaining fields mirror Client's own Options (see their doc
	// comments in client.go) - a plain config struct here instead of the
	// same functional-options pattern, since a concrete backend's adapter
	// typically builds one GenericConfig value up front rather than
	// composing it incrementally.
	HTTPClient       *http.Client
	Timeout          time.Duration
	APIKey           string
	TenantID         string
	UserAgent        string
	MaxResponseBytes int64
	Retry            RetryConfig

	// HealthCacheTTL caches IsAvailable results for this duration, mirroring
	// WithHealthCacheTTL. nil (the zero value - easy to get by accident by
	// simply not setting this field) keeps Client's own default (5s), not
	// "disabled" - a pointer, not a plain time.Duration, specifically so an
	// unset field can't be silently misread as an explicit request to
	// disable caching. Pass a pointer to 0 to actually disable it.
	HealthCacheTTL *time.Duration
}

// NewGenericClient validates cfg and builds a GenericClient. Returns an
// error rather than panicking or silently producing an unusable client,
// since a misconfigured backend adapter (e.g. forgetting ParseConvert)
// would otherwise only fail at first use, far from the actual mistake.
func NewGenericClient(cfg GenericConfig) (*GenericClient, error) {
	if strings.TrimSpace(cfg.BaseURL) == "" {
		return nil, fmt.Errorf("documentreader: GenericClient requires BaseURL")
	}
	if strings.TrimSpace(cfg.FileField) == "" {
		return nil, fmt.Errorf("documentreader: GenericClient requires FileField (the multipart field name the server expects the file under)")
	}
	if cfg.ConvertPath != "" && cfg.ParseConvert == nil {
		return nil, fmt.Errorf("documentreader: GenericClient has ConvertPath set but no ParseConvert")
	}
	if cfg.ConvertPath == "" && cfg.ParseConvert != nil {
		return nil, fmt.Errorf("documentreader: GenericClient has ParseConvert set but no ConvertPath")
	}
	if cfg.ChunkPath != "" && cfg.ParseChunk == nil {
		return nil, fmt.Errorf("documentreader: GenericClient has ChunkPath set but no ParseChunk")
	}
	if cfg.ChunkPath == "" && cfg.ParseChunk != nil {
		return nil, fmt.Errorf("documentreader: GenericClient has ParseChunk set but no ChunkPath")
	}
	if cfg.ConvertPath == "" && cfg.ChunkPath == "" {
		return nil, fmt.Errorf("documentreader: GenericClient needs at least one of ConvertPath or ChunkPath configured")
	}

	transport := newDefaultTransport(cfg.BaseURL, cfg.UserAgent)
	if cfg.HTTPClient != nil {
		transport.httpClient = cfg.HTTPClient
	}
	if cfg.Timeout > 0 {
		transport.httpClient.Timeout = cfg.Timeout
	}
	transport.apiKey = strings.TrimSpace(cfg.APIKey)
	transport.tenantID = strings.TrimSpace(cfg.TenantID)
	if cfg.MaxResponseBytes > 0 {
		transport.maxResponseBytes = cfg.MaxResponseBytes
	}
	if cfg.Retry.MaxAttempts > 0 {
		transport.retry = cfg.Retry
	}
	if cfg.HealthCacheTTL != nil && *cfg.HealthCacheTTL >= 0 {
		transport.healthCacheTTL = *cfg.HealthCacheTTL
	}

	healthPath := cfg.HealthPath
	if strings.TrimSpace(healthPath) == "" {
		healthPath = "/health"
	}

	return &GenericClient{
		transport:    transport,
		fileField:    cfg.FileField,
		convertPath:  cfg.ConvertPath,
		chunkPath:    cfg.ChunkPath,
		healthPath:   healthPath,
		parseConvert: cfg.ParseConvert,
		parseChunk:   cfg.ParseChunk,
	}, nil
}

func (g *GenericClient) ConvertFile(ctx context.Context, filePath string) (*ConversionResult, error) {
	if g.convertPath == "" {
		return nil, fmt.Errorf("documentreader: this GenericClient has no ConvertPath configured")
	}
	rawBody, err := g.transport.postMultipartReplayable(ctx, g.convertPath, g.fileField, filepath.Base(filePath), func() (io.ReadCloser, error) {
		return os.Open(filePath)
	}, nil)
	if err != nil {
		return nil, err
	}
	return g.parseConvert(rawBody)
}

func (g *GenericClient) ConvertBytes(ctx context.Context, data []byte, filename string) (*ConversionResult, error) {
	if g.convertPath == "" {
		return nil, fmt.Errorf("documentreader: this GenericClient has no ConvertPath configured")
	}
	rawBody, err := g.transport.postMultipartReplayable(ctx, g.convertPath, g.fileField, filename, func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(data)), nil
	}, nil)
	if err != nil {
		return nil, err
	}
	return g.parseConvert(rawBody)
}

// ConvertURL is not supported by GenericClient - a URL-based conversion
// endpoint's request shape (JSON body vs. docling-serve's own
// {"sources":[...]}) is specific enough per server that it isn't worth
// generalizing here without a second real backend that needs it. Returns a
// clear error rather than silently failing a caller that assumed every
// Converter supports it.
func (g *GenericClient) ConvertURL(context.Context, string) (*ConversionResult, error) {
	return nil, fmt.Errorf("documentreader: GenericClient does not support ConvertURL")
}

// ChunkHybridFile mirrors Client's own convenience method of the same name
// (see ChunkHybridBytes's doc comment for why opts is unused here).
func (g *GenericClient) ChunkHybridFile(ctx context.Context, filePath string, _ ChunkOptions) ([]Chunk, error) {
	if g.chunkPath == "" {
		return nil, fmt.Errorf("documentreader: this GenericClient has no ChunkPath configured")
	}
	rawBody, err := g.transport.postMultipartReplayable(ctx, g.chunkPath, g.fileField, filepath.Base(filePath), func() (io.ReadCloser, error) {
		return os.Open(filePath)
	}, nil)
	if err != nil {
		return nil, err
	}
	return g.parseChunk(rawBody)
}

// ChunkHybridBytes satisfies HybridChunker. opts is accepted for
// interface compatibility but currently unused: unlike docling-serve,
// there is no generic way to map ChunkOptions onto an arbitrary server's
// own tuning knobs (or lack thereof - seshat-intelligence's own chunk
// endpoint takes none today). A backend whose server does support tuning
// can build its own field mapping ahead of NewGenericClient, or wrap
// GenericClient with that translation, once a real need for it shows up.
func (g *GenericClient) ChunkHybridBytes(ctx context.Context, data []byte, filename string, _ ChunkOptions) ([]Chunk, error) {
	if g.chunkPath == "" {
		return nil, fmt.Errorf("documentreader: this GenericClient has no ChunkPath configured")
	}
	rawBody, err := g.transport.postMultipartReplayable(ctx, g.chunkPath, g.fileField, filename, func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(data)), nil
	}, nil)
	if err != nil {
		return nil, err
	}
	return g.parseChunk(rawBody)
}

func (g *GenericClient) IsAvailable(ctx context.Context) bool {
	return g.transport.checkHealth(ctx, g.healthPath)
}

var (
	_ Converter     = (*GenericClient)(nil)
	_ HybridChunker = (*GenericClient)(nil)
)
