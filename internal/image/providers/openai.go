// Package imageproviders contains concrete implementations of image.Generation
// for each supported provider. Add a new file here (e.g. stabilityai.go) to
// support additional image generation backends — no other package needs changing.
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
	openAIDefaultBaseURL     = "https://api.openai.com/v1"
	openAIDefaultModel       = "dall-e-3"
	openAIDefaultSize        = "1024x1024"
	openAIDefaultQuality     = "standard"
	openAIDefaultResponseFmt = "b64_json"
)

// OpenAIClient generates images using OpenAI DALL-E (dall-e-3, dall-e-2).
type OpenAIClient struct {
	apiKey     string
	baseURL    string
	model      string
	size       string
	quality    string
	httpClient *http.Client
}

// OpenAIOption configures an OpenAIClient.
type OpenAIOption func(*OpenAIClient)

// WithOpenAIModel sets the DALL-E model (e.g. "dall-e-3", "dall-e-2").
func WithOpenAIModel(model string) OpenAIOption { return func(c *OpenAIClient) { c.model = model } }

// WithOpenAISize sets the image dimensions (e.g. "1024x1024", "1792x1024").
func WithOpenAISize(size string) OpenAIOption { return func(c *OpenAIClient) { c.size = size } }

// WithOpenAIQuality sets the quality ("standard" or "hd").
func WithOpenAIQuality(q string) OpenAIOption { return func(c *OpenAIClient) { c.quality = q } }

// WithOpenAIBaseURL overrides the default endpoint (useful for proxies).
func WithOpenAIBaseURL(url string) OpenAIOption { return func(c *OpenAIClient) { c.baseURL = url } }

// WithOpenAIHTTPClient injects a custom HTTP client.
func WithOpenAIHTTPClient(hc *http.Client) OpenAIOption {
	return func(c *OpenAIClient) { c.httpClient = hc }
}

// NewOpenAI creates an OpenAI DALL-E image generation client.
func NewOpenAI(apiKey string, opts ...OpenAIOption) *OpenAIClient {
	c := &OpenAIClient{
		apiKey:     apiKey,
		baseURL:    openAIDefaultBaseURL,
		model:      openAIDefaultModel,
		size:       openAIDefaultSize,
		quality:    openAIDefaultQuality,
		httpClient: &http.Client{},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Provider implements image.Generation.
func (c *OpenAIClient) Provider() string { return "openai" }

// GenerateImage calls the OpenAI Images API and returns the result.
func (c *OpenAIClient) GenerateImage(ctx context.Context, prompt string) (*image.GenerationResponse, error) {
	body, err := json.Marshal(map[string]any{
		"model":           c.model,
		"prompt":          prompt,
		"size":            c.size,
		"quality":         c.quality,
		"response_format": openAIDefaultResponseFmt,
		"n":               1,
	})
	if err != nil {
		return nil, fmt.Errorf("openai image: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/images/generations", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("openai image: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai image: http: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("openai image: read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openai image: API error %d: %s", resp.StatusCode, string(raw))
	}

	var apiResp struct {
		Data []struct {
			B64JSON       string `json:"b64_json"`
			URL           string `json:"url"`
			RevisedPrompt string `json:"revised_prompt"`
		} `json:"data"`
		Model string `json:"model"`
	}
	if err := json.Unmarshal(raw, &apiResp); err != nil {
		return nil, fmt.Errorf("openai image: decode response: %w", err)
	}

	results := make([]image.GenerationResult, 0, len(apiResp.Data))
	for _, d := range apiResp.Data {
		r := image.GenerationResult{RevisedPrompt: d.RevisedPrompt, MIMEType: "image/png"}
		if d.B64JSON != "" {
			r.ImageBase64 = d.B64JSON
		} else {
			r.ImageURL = d.URL
		}
		results = append(results, r)
	}

	model := apiResp.Model
	if model == "" {
		model = c.model
	}
	return &image.GenerationResponse{Images: results, Model: model}, nil
}

var _ image.Generation = (*OpenAIClient)(nil)
