package docling

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// defaultTimeout covers a typical single-digit-page document comfortably.
// It's too short for large or scan-heavy documents on a CPU-only
// docling-serve: layout detection runs on every page's rasterized image
// regardless of native text (see the standard PDF pipeline), so a
// several-hundred-page PDF can take well past two minutes with no GPU.
// Callers that route documents through unconditionally - e.g. RAG
// ingestion, which never skips docling the way an ephemeral read does -
// should pass WithTimeout with a budget sized for their real documents.
const defaultTimeout = 120 * time.Second
const defaultMaxResponseBytes = 128 * 1024 * 1024
const defaultHealthCacheTTL = 5 * time.Second

// Client calls a running docling-serve instance to convert documents to markdown.
type Client struct {
	baseURL          string
	httpClient       *http.Client
	apiKey           string
	tenantID         string
	userAgent        string
	maxResponseBytes int64
	retry            RetryConfig
	healthCacheTTL   time.Duration
	healthMu         sync.Mutex
	healthCached     bool
	healthCacheUntil time.Time
}

// Option configures a Client at construction time.
type Option func(*Client)

// RetryConfig controls best-effort retries for replayable docling-serve calls.
type RetryConfig struct {
	MaxAttempts int
	BaseDelay   time.Duration
	MaxDelay    time.Duration
}

// WithTimeout overrides the default 120s per-request HTTP timeout. Pass a
// larger budget for deployments that expect large or scan-heavy documents
// (see defaultTimeout's doc comment for why the default is often too
// short), or a smaller one for latency-sensitive callers that would rather
// fail fast than wait.
func WithTimeout(d time.Duration) Option {
	return func(c *Client) {
		c.httpClient.Timeout = d
	}
}

// WithHTTPClient replaces the HTTP client used for docling-serve calls.
func WithHTTPClient(client *http.Client) Option {
	return func(c *Client) {
		if client != nil {
			c.httpClient = client
		}
	}
}

// WithAPIKey sets the X-Api-Key header sent to docling-serve.
func WithAPIKey(key string) Option {
	return func(c *Client) {
		c.apiKey = strings.TrimSpace(key)
	}
}

// WithTenantID sets the X-Tenant-Id header sent to docling-serve.
func WithTenantID(id string) Option {
	return func(c *Client) {
		c.tenantID = strings.TrimSpace(id)
	}
}

// WithUserAgent sets the User-Agent header sent to docling-serve.
func WithUserAgent(userAgent string) Option {
	return func(c *Client) {
		c.userAgent = strings.TrimSpace(userAgent)
	}
}

// WithMaxResponseBytes adjusts the response body safety limit.
func WithMaxResponseBytes(n int64) Option {
	return func(c *Client) {
		if n > 0 {
			c.maxResponseBytes = n
		}
	}
}

// WithRetry enables retries for transient failures on replayable requests.
func WithRetry(maxAttempts int, baseDelay, maxDelay time.Duration) Option {
	return func(c *Client) {
		if maxAttempts < 1 {
			maxAttempts = 1
		}
		if baseDelay <= 0 {
			baseDelay = 250 * time.Millisecond
		}
		if maxDelay <= 0 {
			maxDelay = 2 * time.Second
		}
		c.retry = RetryConfig{
			MaxAttempts: maxAttempts,
			BaseDelay:   baseDelay,
			MaxDelay:    maxDelay,
		}
	}
}

// WithHealthCacheTTL caches IsAvailable results for the given duration.
func WithHealthCacheTTL(ttl time.Duration) Option {
	return func(c *Client) {
		if ttl >= 0 {
			c.healthCacheTTL = ttl
		}
	}
}

// NewClient creates a client pointing at a docling-serve base URL.
// baseURL is typically "http://localhost:5001". Defaults to a 120s
// per-request timeout - pass WithTimeout to override it.
func NewClient(baseURL string, opts ...Option) *Client {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = "http://localhost:5001"
	}
	c := &Client{
		baseURL:          strings.TrimRight(baseURL, "/"),
		userAgent:        "seshat-docling-client",
		maxResponseBytes: defaultMaxResponseBytes,
		retry: RetryConfig{
			MaxAttempts: 1,
			BaseDelay:   250 * time.Millisecond,
			MaxDelay:    2 * time.Second,
		},
		healthCacheTTL: defaultHealthCacheTTL,
		httpClient: &http.Client{
			Timeout: defaultTimeout,
		},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// APIError is returned for non-2xx docling-serve responses.
type APIError struct {
	Status int
	Body   string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("docling-serve returned %d: %s", e.Status, truncate(e.Body, 512))
}

// ConversionResult is what we get back from docling-serve for a single file.
type ConversionResult struct {
	// Markdown is the full document rendered as Markdown.
	Markdown string

	// Images are the pictures extracted from the document (inline figures, diagrams).
	// Each image carries a base64-encoded data URI so the agent can view them.
	Images []ExtractedImage

	// PageCount is the number of pages (0 if unknown).
	PageCount int
}

// ConvertOptions tunes docling-serve conversion. Zero values keep server
// defaults, which is the safest behavior across docling-serve versions.
type ConvertOptions struct {
	FromFormats        []string
	ToFormats          []string
	DoOCR              *bool
	ForceOCR           *bool
	DoTableStructure   *bool
	TableMode          string
	PDFBackend         string
	Pipeline           string
	OCRLang            []string
	ImageExportMode    string
	IncludePageImages  *bool
	IncludeImages      *bool
	AbortOnError       *bool
	DocumentTimeoutSec *float64
	ImagesScale        *float64
}

// ChunkOptions tunes docling-serve's hybrid chunker.
type ChunkOptions struct {
	MaxTokens         int
	Tokenizer         string
	MergePeers        *bool
	UseMarkdownTables *bool
	UseMarkdownImages *bool
	ImagePlaceholder  string
	IncludeRawText    *bool
	ConvertOptions    ConvertOptions
}

// Chunk is one docling-serve document-aware chunk.
type Chunk struct {
	Filename    string         `json:"filename"`
	ChunkIndex  int            `json:"chunk_index"`
	Text        string         `json:"text"`
	RawText     string         `json:"raw_text"`
	NumTokens   *int           `json:"num_tokens"`
	Headings    []string       `json:"headings"`
	Captions    []string       `json:"captions"`
	PageNumbers []int          `json:"page_numbers"`
	DocItems    []string       `json:"doc_items"`
	Metadata    map[string]any `json:"metadata"`
}

type chunkResponse struct {
	Chunks []Chunk `json:"chunks"`
}

// ExtractedImage is one picture found inside the converted document.
type ExtractedImage struct {
	// Filename is the suggested on-disk name (e.g. "image_0001.png").
	Filename string
	// MimeType is the image MIME type (e.g. "image/png").
	MimeType string
	// Base64 is the raw base64-encoded image bytes (no data-URI prefix).
	Base64 string
}

// ConvertFile sends filePath to docling-serve and returns the markdown + images.
func (c *Client) ConvertFile(ctx context.Context, filePath string) (*ConversionResult, error) {
	return c.ConvertFileWithOptions(ctx, filePath, ConvertOptions{})
}

// ConvertFileWithOptions sends filePath to docling-serve with conversion options.
func (c *Client) ConvertFileWithOptions(ctx context.Context, filePath string, opts ConvertOptions) (*ConversionResult, error) {
	rawBody, err := c.postMultipartReplayable(ctx, "/v1/convert/file", filepath.Base(filePath), func() (io.ReadCloser, error) {
		return os.Open(filePath)
	}, convertFields(opts, ""))
	if err != nil {
		return nil, err
	}
	return parseResponse(rawBody)
}

// ConvertBytes converts an in-memory document without writing it to disk first.
// filename is used only to hint the MIME type to docling (e.g. "report.pdf").
func (c *Client) ConvertBytes(ctx context.Context, data []byte, filename string) (*ConversionResult, error) {
	return c.ConvertBytesWithOptions(ctx, data, filename, ConvertOptions{})
}

// ConvertBytesWithOptions converts an in-memory document with conversion options.
func (c *Client) ConvertBytesWithOptions(ctx context.Context, data []byte, filename string, opts ConvertOptions) (*ConversionResult, error) {
	rawBody, err := c.postMultipartReplayable(ctx, "/v1/convert/file", filename, func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(data)), nil
	}, convertFields(opts, ""))
	if err != nil {
		return nil, err
	}
	return parseResponse(rawBody)
}

// ChunkHybridFile sends filePath to docling-serve's hybrid chunk endpoint.
func (c *Client) ChunkHybridFile(ctx context.Context, filePath string, opts ChunkOptions) ([]Chunk, error) {
	rawBody, err := c.postMultipartReplayable(ctx, "/v1/chunk/hybrid/file", filepath.Base(filePath), func() (io.ReadCloser, error) {
		return os.Open(filePath)
	}, chunkFields(opts))
	if err != nil {
		return nil, err
	}
	return parseChunkResponse(rawBody)
}

// ChunkHybridBytes chunks an in-memory document without writing it to disk.
func (c *Client) ChunkHybridBytes(ctx context.Context, data []byte, filename string, opts ChunkOptions) ([]Chunk, error) {
	rawBody, err := c.postMultipartReplayable(ctx, "/v1/chunk/hybrid/file", filename, func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(data)), nil
	}, chunkFields(opts))
	if err != nil {
		return nil, err
	}
	return parseChunkResponse(rawBody)
}

// ChunkHybrid sends a document to docling-serve's hybrid chunk endpoint.
func (c *Client) ChunkHybrid(ctx context.Context, filename string, r io.Reader, opts ChunkOptions) ([]Chunk, error) {
	rawBody, err := c.postMultipart(ctx, "/v1/chunk/hybrid/file", filename, r, chunkFields(opts))
	if err != nil {
		return nil, err
	}
	return parseChunkResponse(rawBody)
}

// ConvertURL fetches and converts a remote document (e.g. an arXiv PDF URL).
func (c *Client) ConvertURL(ctx context.Context, docURL string) (*ConversionResult, error) {
	payload, err := json.Marshal(map[string]any{
		"sources": []map[string]any{
			{"kind": "http", "url": docURL},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("marshal url request: %w", err)
	}
	req, err := c.newRequest(ctx, http.MethodPost, "/v1/convert/source", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("build url request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	rawBody, err := c.doBytes(req)
	if err != nil {
		return nil, err
	}
	return parseResponse(rawBody)
}

// convert is the shared multipart sender used by ConvertFile and ConvertBytes.
func (c *Client) convert(ctx context.Context, filename string, r io.Reader, opts ConvertOptions) (*ConversionResult, error) {
	rawBody, err := c.postMultipart(ctx, "/v1/convert/file", filename, r, convertFields(opts, ""))
	if err != nil {
		return nil, err
	}

	return parseResponse(rawBody)
}

// IsAvailable does a cheap health check against the running service.
func (c *Client) IsAvailable(ctx context.Context) bool {
	if cached, ok := c.cachedHealth(); ok {
		return cached
	}
	reqCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	req, err := c.newRequest(reqCtx, http.MethodGet, "/health", nil)
	if err != nil {
		c.cacheHealth(false)
		return false
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.cacheHealth(false)
		return false
	}
	resp.Body.Close()
	available := resp.StatusCode < 500
	c.cacheHealth(available)
	return available
}

// ── internal response parsing ─────────────────────────────────────────────────

type multipartField struct {
	name  string
	value string
}

func (c *Client) cachedHealth() (bool, bool) {
	if c.healthCacheTTL <= 0 {
		return false, false
	}
	c.healthMu.Lock()
	defer c.healthMu.Unlock()
	if time.Now().Before(c.healthCacheUntil) {
		return c.healthCached, true
	}
	return false, false
}

func (c *Client) cacheHealth(available bool) {
	if c.healthCacheTTL <= 0 {
		return
	}
	c.healthMu.Lock()
	defer c.healthMu.Unlock()
	c.healthCached = available
	c.healthCacheUntil = time.Now().Add(c.healthCacheTTL)
}

func (c *Client) newRequest(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if c.userAgent != "" {
		req.Header.Set("User-Agent", c.userAgent)
	}
	if c.apiKey != "" {
		req.Header.Set("X-Api-Key", c.apiKey)
	}
	if c.tenantID != "" {
		req.Header.Set("X-Tenant-Id", c.tenantID)
	}
	return req, nil
}

func (c *Client) doBytes(req *http.Request) ([]byte, error) {
	attempts := c.retry.MaxAttempts
	if attempts < 1 {
		attempts = 1
	}
	if req.Body != nil && req.GetBody == nil {
		attempts = 1
	}
	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		tryReq := req
		if attempt > 1 {
			tryReq = req.Clone(req.Context())
			if req.GetBody != nil {
				body, err := req.GetBody()
				if err != nil {
					return nil, err
				}
				tryReq.Body = body
			}
		}
		rawBody, status, err := c.doBytesOnce(tryReq)
		if err == nil {
			return rawBody, nil
		}
		lastErr = err
		if !c.shouldRetry(status, err) || attempt == attempts {
			break
		}
		if err := sleepWithContext(req.Context(), c.retryDelay(attempt)); err != nil {
			return nil, err
		}
	}
	return nil, lastErr
}

func (c *Client) doBytesOnce(req *http.Request) ([]byte, int, error) {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("docling-serve request: %w", err)
	}
	defer resp.Body.Close()
	limit := c.maxResponseBytes
	if limit <= 0 {
		limit = defaultMaxResponseBytes
	}
	rawBody, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("read response: %w", err)
	}
	if int64(len(rawBody)) > limit {
		return nil, resp.StatusCode, fmt.Errorf("docling-serve response exceeded %d bytes", limit)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, resp.StatusCode, &APIError{Status: resp.StatusCode, Body: string(rawBody)}
	}
	return rawBody, resp.StatusCode, nil
}

func (c *Client) postMultipart(ctx context.Context, path, filename string, r io.Reader, fields []multipartField) ([]byte, error) {
	pr, pw := io.Pipe()
	mw := multipart.NewWriter(pw)

	go func() {
		var err error
		defer func() {
			if closeErr := mw.Close(); err == nil {
				err = closeErr
			}
			_ = pw.CloseWithError(err)
		}()

		for _, field := range fields {
			if strings.TrimSpace(field.name) == "" {
				continue
			}
			if err = mw.WriteField(field.name, field.value); err != nil {
				return
			}
		}
		var fw io.Writer
		fw, err = mw.CreateFormFile("files", filepath.Base(filename))
		if err != nil {
			return
		}
		_, err = io.Copy(fw, r)
	}()

	req, err := c.newRequest(ctx, http.MethodPost, path, pr)
	if err != nil {
		_ = pr.Close()
		_ = pw.Close()
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return c.doBytes(req)
}

func (c *Client) postMultipartReplayable(ctx context.Context, path, filename string, open func() (io.ReadCloser, error), fields []multipartField) ([]byte, error) {
	attempts := c.retry.MaxAttempts
	if attempts < 1 {
		attempts = 1
	}
	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		reader, err := open()
		if err != nil {
			return nil, fmt.Errorf("open file: %w", err)
		}
		rawBody, err := c.postMultipart(ctx, path, filename, reader, fields)
		_ = reader.Close()
		if err == nil {
			return rawBody, nil
		}
		lastErr = err
		status := 0
		var apiErr *APIError
		if errors.As(err, &apiErr) {
			status = apiErr.Status
		}
		if !c.shouldRetry(status, err) || attempt == attempts {
			break
		}
		if err := sleepWithContext(ctx, c.retryDelay(attempt)); err != nil {
			return nil, err
		}
	}
	return nil, lastErr
}

func chunkFields(opts ChunkOptions) []multipartField {
	fields := make([]multipartField, 0, 16)
	fields = append(fields, convertFields(opts.ConvertOptions, "convert_")...)
	if opts.MaxTokens > 0 {
		fields = append(fields, multipartField{name: "chunking_max_tokens", value: fmt.Sprint(opts.MaxTokens)})
	}
	if strings.TrimSpace(opts.Tokenizer) != "" {
		fields = append(fields, multipartField{name: "chunking_tokenizer", value: strings.TrimSpace(opts.Tokenizer)})
	}
	if opts.MergePeers != nil {
		fields = append(fields, multipartField{name: "chunking_merge_peers", value: fmt.Sprint(*opts.MergePeers)})
	}
	if opts.UseMarkdownTables != nil {
		fields = append(fields, multipartField{name: "chunking_use_markdown_tables", value: fmt.Sprint(*opts.UseMarkdownTables)})
	}
	if opts.UseMarkdownImages != nil {
		fields = append(fields, multipartField{name: "chunking_use_markdown_images", value: fmt.Sprint(*opts.UseMarkdownImages)})
	}
	if strings.TrimSpace(opts.ImagePlaceholder) != "" {
		fields = append(fields, multipartField{name: "chunking_image_placeholder", value: opts.ImagePlaceholder})
	}
	if opts.IncludeRawText != nil {
		fields = append(fields, multipartField{name: "chunking_include_raw_text", value: fmt.Sprint(*opts.IncludeRawText)})
	}
	return fields
}

func parseChunkResponse(rawBody []byte) ([]Chunk, error) {
	var out chunkResponse
	if err := json.Unmarshal(rawBody, &out); err != nil {
		return nil, fmt.Errorf("decode docling chunk response: %w", err)
	}
	return out.Chunks, nil
}

func convertFields(opts ConvertOptions, prefix string) []multipartField {
	fields := make([]multipartField, 0, 16)
	add := func(name, value string) {
		if strings.TrimSpace(value) != "" {
			fields = append(fields, multipartField{name: prefix + name, value: value})
		}
	}
	addBool := func(name string, value *bool) {
		if value != nil {
			fields = append(fields, multipartField{name: prefix + name, value: fmt.Sprint(*value)})
		}
	}
	addFloat := func(name string, value *float64) {
		if value != nil {
			fields = append(fields, multipartField{name: prefix + name, value: fmt.Sprint(*value)})
		}
	}
	for _, format := range opts.FromFormats {
		add("from_formats", format)
	}
	for _, format := range opts.ToFormats {
		add("to_formats", format)
	}
	for _, lang := range opts.OCRLang {
		add("ocr_lang", lang)
	}
	addBool("do_ocr", opts.DoOCR)
	addBool("force_ocr", opts.ForceOCR)
	addBool("do_table_structure", opts.DoTableStructure)
	addBool("include_page_images", opts.IncludePageImages)
	addBool("include_images", opts.IncludeImages)
	addBool("abort_on_error", opts.AbortOnError)
	addFloat("document_timeout", opts.DocumentTimeoutSec)
	addFloat("images_scale", opts.ImagesScale)
	add("table_mode", opts.TableMode)
	add("pdf_backend", opts.PDFBackend)
	add("pipeline", opts.Pipeline)
	add("image_export_mode", opts.ImageExportMode)
	return fields
}

func (c *Client) shouldRetry(status int, err error) bool {
	if err == nil {
		return false
	}
	if status == http.StatusRequestTimeout || status == http.StatusTooManyRequests || status >= 500 {
		return true
	}
	return status == 0
}

func (c *Client) retryDelay(attempt int) time.Duration {
	delay := c.retry.BaseDelay
	if delay <= 0 {
		delay = 250 * time.Millisecond
	}
	for i := 1; i < attempt; i++ {
		delay *= 2
	}
	maxDelay := c.retry.MaxDelay
	if maxDelay <= 0 {
		maxDelay = 2 * time.Second
	}
	if delay > maxDelay {
		return maxDelay
	}
	return delay
}

func sleepWithContext(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// doclingResponse mirrors the docling-serve v1alpha/convert/source JSON shape.
type doclingResponse struct {
	Status   string           `json:"status"`
	Errors   []string         `json:"errors"`
	Document *doclingDocument `json:"document"`
}

type doclingDocument struct {
	MdContent string           `json:"md_content"`
	Pages     []any            `json:"pages"`
	Pictures  []doclingPicture `json:"pictures"`
}

type doclingPicture struct {
	Image *doclingImage `json:"image"`
}

type doclingImage struct {
	MimeType string `json:"mimetype"`
	URI      string `json:"uri"` // "data:<mime>;base64,<b64>"
	Size     *struct {
		Width  int `json:"width"`
		Height int `json:"height"`
	} `json:"size"`
}

func parseResponse(raw []byte) (*ConversionResult, error) {
	var dr doclingResponse
	if err := json.Unmarshal(raw, &dr); err != nil {
		return nil, fmt.Errorf("decode docling response: %w", err)
	}
	if dr.Status != "success" && dr.Status != "partial_success" {
		msg := strings.Join(dr.Errors, "; ")
		if msg == "" {
			msg = "unknown failure"
		}
		return nil, fmt.Errorf("docling conversion failed (%s): %s", dr.Status, msg)
	}
	if dr.Document == nil {
		return nil, fmt.Errorf("docling returned no document")
	}

	result := &ConversionResult{
		Markdown:  dr.Document.MdContent,
		PageCount: len(dr.Document.Pages),
	}

	for i, pic := range dr.Document.Pictures {
		if pic.Image == nil || pic.Image.URI == "" {
			continue
		}
		b64, mime := extractDataURI(pic.Image.URI)
		if b64 == "" {
			continue
		}
		if mime == "" && pic.Image.MimeType != "" {
			mime = pic.Image.MimeType
		}
		if mime == "" {
			mime = "image/png"
		}
		ext := mimeToExt(mime)
		result.Images = append(result.Images, ExtractedImage{
			Filename: fmt.Sprintf("image_%04d%s", i+1, ext),
			MimeType: mime,
			Base64:   b64,
		})
	}

	return result, nil
}

// extractDataURI splits "data:<mime>;base64,<data>" into (base64, mime).
func extractDataURI(uri string) (b64, mime string) {
	if !strings.HasPrefix(uri, "data:") {
		return "", ""
	}
	uri = uri[len("data:"):]
	semi := strings.Index(uri, ";base64,")
	if semi < 0 {
		return "", ""
	}
	return uri[semi+len(";base64,"):], uri[:semi]
}

func mimeToExt(mime string) string {
	switch mime {
	case "image/jpeg":
		return ".jpg"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	default:
		return ".png"
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
