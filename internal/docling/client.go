package docling

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const defaultTimeout = 120 * time.Second

// Client calls a running docling-serve instance to convert documents to markdown.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient creates a client pointing at a docling-serve base URL.
// baseURL is typically "http://localhost:5001".
func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: defaultTimeout,
		},
	}
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
	f, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("open file: %w", err)
	}
	defer f.Close()
	return c.convert(ctx, filepath.Base(filePath), f)
}

// ConvertBytes converts an in-memory document without writing it to disk first.
// filename is used only to hint the MIME type to docling (e.g. "report.pdf").
func (c *Client) ConvertBytes(ctx context.Context, data []byte, filename string) (*ConversionResult, error) {
	return c.convert(ctx, filename, bytes.NewReader(data))
}

// ConvertURL fetches and converts a remote document (e.g. an arXiv PDF URL).
func (c *Client) ConvertURL(ctx context.Context, docURL string) (*ConversionResult, error) {
	payload, err := json.Marshal(map[string]any{
		"http_source": map[string]string{"url": docURL},
	})
	if err != nil {
		return nil, fmt.Errorf("marshal url request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/v1alpha/convert/source", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("build url request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("docling-serve url request: %w", err)
	}
	defer resp.Body.Close()
	rawBody, err := io.ReadAll(io.LimitReader(resp.Body, 32*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("docling-serve returned %d: %s", resp.StatusCode, truncate(string(rawBody), 256))
	}
	return parseResponse(rawBody)
}

// convert is the shared multipart sender used by ConvertFile and ConvertBytes.
func (c *Client) convert(ctx context.Context, filename string, r io.Reader) (*ConversionResult, error) {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, err := mw.CreateFormFile("file", filename)
	if err != nil {
		return nil, fmt.Errorf("create form file: %w", err)
	}
	if _, err = io.Copy(fw, r); err != nil {
		return nil, fmt.Errorf("copy file: %w", err)
	}
	mw.Close()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/v1alpha/convert/source", &body)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("docling-serve request: %w", err)
	}
	defer resp.Body.Close()

	rawBody, err := io.ReadAll(io.LimitReader(resp.Body, 32*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("docling-serve returned %d: %s", resp.StatusCode, truncate(string(rawBody), 256))
	}

	return parseResponse(rawBody)
}

// IsAvailable does a cheap health check against the running service.
func (c *Client) IsAvailable(ctx context.Context) bool {
	reqCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, c.baseURL+"/health", nil)
	if err != nil {
		return false
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode < 500
}

// ── internal response parsing ─────────────────────────────────────────────────

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
