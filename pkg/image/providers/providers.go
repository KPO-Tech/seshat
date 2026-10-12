// Package imageproviders exposes seshat's image.Generation implementations
// for direct use by a host application, independent of the agent/tool loop.
package imageproviders

import (
	"net/http"

	internalproviders "github.com/KPO-Tech/seshat-runtime/internal/image/providers"
)

type (
	// OpenAIClient generates images using OpenAI DALL-E (dall-e-3, dall-e-2).
	OpenAIClient = internalproviders.OpenAIClient
	// OpenAIOption configures an OpenAIClient.
	OpenAIOption = internalproviders.OpenAIOption

	// GeminiClient generates images using Google Imagen via the Gemini API.
	GeminiClient = internalproviders.GeminiClient
	// GeminiOption configures a GeminiClient.
	GeminiOption = internalproviders.GeminiOption
)

// NewOpenAI creates an OpenAI DALL-E image generation client. Pass
// WithOpenAIBaseURL to point at a self-hosted, OpenAI-API-compatible image
// generation server instead of the real OpenAI cloud.
func NewOpenAI(apiKey string, opts ...OpenAIOption) *OpenAIClient {
	return internalproviders.NewOpenAI(apiKey, opts...)
}

// WithOpenAIModel sets the DALL-E model (e.g. "dall-e-3", "dall-e-2").
func WithOpenAIModel(model string) OpenAIOption { return internalproviders.WithOpenAIModel(model) }

// WithOpenAISize sets the image dimensions (e.g. "1024x1024", "1792x1024").
func WithOpenAISize(size string) OpenAIOption { return internalproviders.WithOpenAISize(size) }

// WithOpenAIQuality sets the quality ("standard" or "hd").
func WithOpenAIQuality(q string) OpenAIOption { return internalproviders.WithOpenAIQuality(q) }

// WithOpenAIBaseURL overrides the default endpoint (self-hosted server, or a proxy).
func WithOpenAIBaseURL(url string) OpenAIOption { return internalproviders.WithOpenAIBaseURL(url) }

// WithOpenAIHTTPClient injects a custom HTTP client.
func WithOpenAIHTTPClient(hc *http.Client) OpenAIOption {
	return internalproviders.WithOpenAIHTTPClient(hc)
}

// NewGemini creates a Google Imagen image generation client.
// apiKey is a Google AI Studio API key (AIza...).
func NewGemini(apiKey string, opts ...GeminiOption) *GeminiClient {
	return internalproviders.NewGemini(apiKey, opts...)
}

// WithGeminiModel sets the Imagen model.
func WithGeminiModel(model string) GeminiOption { return internalproviders.WithGeminiModel(model) }

// WithGeminiCount sets the number of images to generate (1-4).
func WithGeminiCount(n int) GeminiOption { return internalproviders.WithGeminiCount(n) }

// WithGeminiAspectRatio sets the aspect ratio ("1:1", "3:4", "4:3", "9:16", "16:9").
func WithGeminiAspectRatio(ar string) GeminiOption {
	return internalproviders.WithGeminiAspectRatio(ar)
}

// WithGeminiBaseURL overrides the default Google AI Studio endpoint.
func WithGeminiBaseURL(url string) GeminiOption { return internalproviders.WithGeminiBaseURL(url) }

// WithGeminiHTTPClient injects a custom HTTP client.
func WithGeminiHTTPClient(hc *http.Client) GeminiOption {
	return internalproviders.WithGeminiHTTPClient(hc)
}
