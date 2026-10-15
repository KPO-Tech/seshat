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
	// Gemini image models (Nano Banana) are called through generateContent;
	// an imagen-* model keeps the older predict call.
	geminiDefaultModel   = "gemini-nano-banana-2.1"
	geminiDefaultBaseURL = "https://generativelanguage.googleapis.com/v1beta"
	geminiDefaultCount   = 1
	geminiDefaultAspect  = "1:1"
)

// GeminiClient generates images with the Gemini API: the Gemini image models
// (gemini-nano-banana-2.1, gemini-3.1-flash-image, ...) through generateContent,
// and an imagen-* model through the Imagen predict call.
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
		httpClient:  &http.Client{Timeout: requestTimeout},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Provider implements image.Generation.
func (c *GeminiClient) Provider() string { return "gemini" }

// GenerateImage generates an image: generateContent for a Gemini image model,
// the Imagen predict call for an imagen-* model.
func (c *GeminiClient) GenerateImage(ctx context.Context, prompt string) (*image.GenerationResponse, error) {
	if !strings.HasPrefix(strings.ToLower(c.model), "imagen") {
		return c.generateContent(ctx, prompt)
	}
	return c.predict(ctx, prompt)
}

// generateContent calls POST /models/{model}:generateContent. The key goes in
// the x-goog-api-key header, not the URL, so it never lands in a logged URL.
func (c *GeminiClient) generateContent(ctx context.Context, prompt string) (*image.GenerationResponse, error) {
	generation := map[string]any{"responseModalities": []string{"IMAGE"}}
	if c.aspectRatio != "" {
		generation["imageConfig"] = map[string]any{"aspectRatio": c.aspectRatio}
	}
	body, err := json.Marshal(map[string]any{
		"contents":         []map[string]any{{"parts": []map[string]any{{"text": prompt}}}},
		"generationConfig": generation,
	})
	if err != nil {
		return nil, fmt.Errorf("gemini image: marshal request: %w", err)
	}
	endpoint := fmt.Sprintf("%s/models/%s:generateContent", c.baseURL, c.model)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("gemini image: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", c.apiKey)

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
		Candidates []struct {
			Content struct {
				Parts []struct {
					InlineData *struct {
						MimeType string `json:"mimeType"`
						Data     string `json:"data"`
					} `json:"inlineData"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(raw, &apiResp); err != nil {
		return nil, fmt.Errorf("gemini image: decode response: %w", err)
	}
	var results []image.GenerationResult
	for _, cand := range apiResp.Candidates {
		for _, part := range cand.Content.Parts {
			if part.InlineData == nil || part.InlineData.Data == "" {
				continue
			}
			mime := part.InlineData.MimeType
			if mime == "" {
				mime = "image/png"
			}
			results = append(results, image.GenerationResult{ImageBase64: part.InlineData.Data, MIMEType: mime})
		}
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("gemini image: the response held no image (the prompt may have been blocked): %s", string(raw))
	}
	return &image.GenerationResponse{Images: results, Model: c.model}, nil
}

// predict calls the Imagen predict API.
// Endpoint: POST /v1beta/models/{model}:predict?key=API_KEY
func (c *GeminiClient) predict(ctx context.Context, prompt string) (*image.GenerationResponse, error) {
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
