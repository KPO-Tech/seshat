package imageproviders

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/KPO-Tech/seshat-runtime/internal/image"
)

const (
	openRouterDefaultBaseURL = "https://openrouter.ai/api/v1"
	openRouterDefaultModel   = "google/gemini-nano-banana-2.1"
)

// OpenRouterClient generates images through the OpenRouter image API
// (POST /images), which fronts Google, OpenAI, Black Forest Labs, ByteDance,
// Recraft and others behind one key. The response carries base64 bytes.
type OpenRouterClient struct {
	apiKey      string
	baseURL     string
	model       string
	aspectRatio string // empty = the model default
	httpClient  *http.Client
}

// OpenRouterOption configures an OpenRouterClient.
type OpenRouterOption func(*OpenRouterClient)

// WithOpenRouterModel sets the model (for example "black-forest-labs/flux-3-image").
func WithOpenRouterModel(model string) OpenRouterOption {
	return func(c *OpenRouterClient) { c.model = model }
}

// WithOpenRouterAspectRatio sets the aspect ratio ("1:1", "16:9", ...).
func WithOpenRouterAspectRatio(ratio string) OpenRouterOption {
	return func(c *OpenRouterClient) { c.aspectRatio = ratio }
}

// WithOpenRouterBaseURL overrides the API root.
func WithOpenRouterBaseURL(url string) OpenRouterOption {
	return func(c *OpenRouterClient) { c.baseURL = strings.TrimRight(url, "/") }
}

// WithOpenRouterHTTPClient injects a custom HTTP client.
func WithOpenRouterHTTPClient(hc *http.Client) OpenRouterOption {
	return func(c *OpenRouterClient) { c.httpClient = hc }
}

// NewOpenRouter creates an OpenRouter image generation client.
func NewOpenRouter(apiKey string, opts ...OpenRouterOption) *OpenRouterClient {
	c := &OpenRouterClient{
		apiKey:     apiKey,
		baseURL:    openRouterDefaultBaseURL,
		model:      openRouterDefaultModel,
		httpClient: &http.Client{Timeout: requestTimeout},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Provider implements image.Generation.
func (c *OpenRouterClient) Provider() string { return "openrouter" }

// GenerateImage calls the OpenRouter image API.
func (c *OpenRouterClient) GenerateImage(ctx context.Context, prompt string) (*image.GenerationResponse, error) {
	payload := map[string]any{"model": c.model, "prompt": prompt, "n": 1}
	if c.aspectRatio != "" {
		payload["aspect_ratio"] = c.aspectRatio
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("openrouter image: marshal request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/images", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("openrouter image: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openrouter image: http: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("openrouter image: read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openrouter image: API error %d: %s", resp.StatusCode, string(raw))
	}

	var apiResp struct {
		Data []struct {
			B64JSON   string `json:"b64_json"`
			MediaType string `json:"media_type"`
			URL       string `json:"url"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &apiResp); err != nil {
		return nil, fmt.Errorf("openrouter image: decode response: %w", err)
	}
	results := make([]image.GenerationResult, 0, len(apiResp.Data))
	for _, d := range apiResp.Data {
		r := image.GenerationResult{MIMEType: d.MediaType}
		if r.MIMEType == "" {
			r.MIMEType = "image/png"
		}
		if d.B64JSON != "" {
			r.ImageBase64 = d.B64JSON
		} else {
			r.ImageURL = d.URL
		}
		results = append(results, r)
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("openrouter image: the response held no image")
	}
	return &image.GenerationResponse{Images: results, Model: c.model}, nil
}

var _ image.Generation = (*OpenRouterClient)(nil)
