// Package imageproviders exposes seshat's image.Generation implementations
// for direct use by a host application, independent of the agent/tool loop.
package imageproviders

import (
	"net/http"

	internalproviders "github.com/KPO-Tech/seshat-runtime/internal/image/providers"
)

type (
	// OpenAIClient generates images through an OpenAI-compatible endpoint:
	// the GPT Image models (gpt-image-2.5-flare, ...) and DALL-E.
	OpenAIClient = internalproviders.OpenAIClient
	// OpenAIOption configures an OpenAIClient.
	OpenAIOption = internalproviders.OpenAIOption

	// OpenRouterClient generates images through the OpenRouter image API.
	OpenRouterClient = internalproviders.OpenRouterClient
	// OpenRouterOption configures an OpenRouterClient.
	OpenRouterOption = internalproviders.OpenRouterOption

	// Profile describes an image provider: protocol, endpoint, key variables
	// and models (the first model is the default).
	Profile = internalproviders.Profile

	// GeminiClient generates images with the Gemini API (Nano Banana models,
	// or Imagen with an imagen-* model).
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

// WithOpenAIModel sets the model (e.g. "gpt-image-2.5-flare", or "dall-e-3").
func WithOpenAIModel(model string) OpenAIOption { return internalproviders.WithOpenAIModel(model) }

// WithOpenAISize sets the image dimensions (e.g. "1024x1024", "1792x1024").
func WithOpenAISize(size string) OpenAIOption { return internalproviders.WithOpenAISize(size) }

// WithOpenAIQuality sets the quality: low, medium, high or auto for GPT Image
// models, standard or hd for dall-e-3. A value of the other family is replaced
// by that family's default.
func WithOpenAIQuality(q string) OpenAIOption { return internalproviders.WithOpenAIQuality(q) }

// WithOpenAIBaseURL overrides the default endpoint (self-hosted server, or a proxy).
func WithOpenAIBaseURL(url string) OpenAIOption { return internalproviders.WithOpenAIBaseURL(url) }

// WithOpenAIHTTPClient injects a custom HTTP client.
func WithOpenAIHTTPClient(hc *http.Client) OpenAIOption {
	return internalproviders.WithOpenAIHTTPClient(hc)
}

// NewGemini creates a Gemini image generation client.
// apiKey is a Google AI Studio API key (AIza...).
func NewGemini(apiKey string, opts ...GeminiOption) *GeminiClient {
	return internalproviders.NewGemini(apiKey, opts...)
}

// WithGeminiModel sets the model (default gemini-nano-banana-2.1; an imagen-*
// model uses the Imagen predict call).
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

// Profiles returns the image providers this build knows (OpenAI, Gemini, OpenRouter).
func Profiles() []Profile { return internalproviders.Profiles() }

// ProfileFor returns the image profile of a provider ID.
func ProfileFor(id string) (Profile, bool) { return internalproviders.ProfileFor(id) }

// WithOpenAIProviderName sets the name an OpenAI-compatible client reports from Provider().
func WithOpenAIProviderName(name string) OpenAIOption {
	return internalproviders.WithOpenAIProviderName(name)
}

// NewOpenRouter creates an OpenRouter image generation client.
func NewOpenRouter(apiKey string, opts ...OpenRouterOption) *OpenRouterClient {
	return internalproviders.NewOpenRouter(apiKey, opts...)
}

// WithOpenRouterModel sets the model (for example "black-forest-labs/flux-3-image").
func WithOpenRouterModel(model string) OpenRouterOption {
	return internalproviders.WithOpenRouterModel(model)
}

// WithOpenRouterAspectRatio sets the aspect ratio ("1:1", "16:9", ...).
func WithOpenRouterAspectRatio(ratio string) OpenRouterOption {
	return internalproviders.WithOpenRouterAspectRatio(ratio)
}

// WithOpenRouterBaseURL overrides the API root.
func WithOpenRouterBaseURL(url string) OpenRouterOption {
	return internalproviders.WithOpenRouterBaseURL(url)
}
