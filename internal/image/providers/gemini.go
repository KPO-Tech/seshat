package imageproviders

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/EngineerProjects/nexus-engine/internal/image"
)

const (
	// Imagen 3 is Google's dedicated high-quality image generation model.
	// Gemini 2.0 Flash also supports inline image generation via generateContent
	// but Imagen produces superior results for pure image-gen use cases.
	geminiDefaultModel   = "imagen-3.0-generate-002"
	geminiDefaultBaseURL = "https://generativelanguage.googleapis.com/v1beta"
	geminiDefaultCount   = 1
	geminiDefaultAspect  = "1:1"
)

// GeminiClient generates images using Google Imagen via the Gemini API.
// Supported models: imagen-3.0-generate-002, imagen-3.0-fast-generate-001,
// imagegeneration@006 (Vertex AI only).
type GeminiClient struct {
	apiKey      string
	baseURL     string
	model       string
	count       int
	aspectRatio string
	httpClient  *http.Client
}

// GeminiOption configures a GeminiClient.
type GeminiOption func(*GeminiClient)

// WithGeminiModel sets the Imagen model.
func WithGeminiModel(model string) GeminiOption { return func(c *GeminiClient) { c.model = model } }

// WithGeminiCount sets the number of images to generate (1–4).
func WithGeminiCount(n int) GeminiOption { return func(c *GeminiClient) { c.count = n } }

// WithGeminiAspectRatio sets the aspect ratio ("1:1", "3:4", "4:3", "9:16", "16:9").
func WithGeminiAspectRatio(ar string) GeminiOption {
	return func(c *GeminiClient) { c.aspectRatio = ar }
}

// WithGeminiBaseURL overrides the default Google AI Studio endpoint.
func WithGeminiBaseURL(url string) GeminiOption { return func(c *GeminiClient) { c.baseURL = url } }

// WithGeminiHTTPClient injects a custom HTTP client.
func WithGeminiHTTPClient(hc *http.Client) GeminiOption {
	return func(c *GeminiClient) { c.httpClient = hc }
}

// NewGemini creates a Google Imagen image generation client.
// apiKey is a Google AI Studio API key (AIza...).
func NewGemini(apiKey string, opts ...GeminiOption) *GeminiClient {
	c := &GeminiClient{
		apiKey:      apiKey,
		baseURL:     geminiDefaultBaseURL,
		model:       geminiDefaultModel,
		count:       geminiDefaultCount,
		aspectRatio: geminiDefaultAspect,
		httpClient:  &http.Client{},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Provider implements image.Generation.
func (c *GeminiClient) Provider() string { return "gemini" }

// GenerateImage calls the Imagen predict API and returns the result.
// Endpoint: POST /v1beta/models/{model}:predict?key=API_KEY
func (c *GeminiClient) GenerateImage(ctx context.Context, prompt string) (*image.GenerationResponse, error) {
	body, err := json.Marshal(map[string]any{
		"instances": []map[string]any{
			{"prompt": prompt},
		},
		"parameters": map[string]any{
			"sampleCount": c.count,
			"aspectRatio": c.aspectRatio,
			// Block only clearly problematic content — tune via WithGeminiSafetyFilter if needed.
			"safetyFilterLevel": "block_some",
		},
	})
	if err != nil {
		return nil, fmt.Errorf("gemini image: marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/models/%s:predict?key=%s", c.baseURL, c.model, c.apiKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("gemini image: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gemini image: http: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("gemini image: read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gemini image: API error %d: %s", resp.StatusCode, string(raw))
	}

	var apiResp struct {
		Predictions []struct {
			BytesBase64Encoded string `json:"bytesBase64Encoded"`
			MimeType           string `json:"mimeType"`
		} `json:"predictions"`
	}
	if err := json.Unmarshal(raw, &apiResp); err != nil {
		return nil, fmt.Errorf("gemini image: decode response: %w", err)
	}

	results := make([]image.GenerationResult, 0, len(apiResp.Predictions))
	for _, p := range apiResp.Predictions {
		mime := p.MimeType
		if mime == "" {
			mime = "image/png"
		}
		results = append(results, image.GenerationResult{
			ImageBase64: p.BytesBase64Encoded,
			MIMEType:    mime,
		})
	}

	return &image.GenerationResponse{Images: results, Model: c.model}, nil
}

var _ image.Generation = (*GeminiClient)(nil)
