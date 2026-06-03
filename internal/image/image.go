// Package image provides a provider-agnostic interface for image generation.
// The Generation interface follows the same pluggable-provider philosophy as
// internal/runtime/memory.CompactionStrategy and internal/providers — configure
// once, generate many.
package image

import (
	"context"
	"errors"
)

// ErrStreamingNotSupported is returned by GenerateImageStream when the
// underlying provider does not support incremental image delivery.
var ErrStreamingNotSupported = errors.New("image: streaming not supported by this provider")

// GenerationResult holds a single generated image, either as a URL or as
// base64-encoded bytes. Providers set exactly one of the two fields.
type GenerationResult struct {
	// ImageURL is the publicly accessible URL of the generated image.
	ImageURL string

	// ImageBase64 is the raw base64-encoded image data (no data-URI prefix).
	ImageBase64 string

	// MIMEType is the MIME type of the image (e.g. "image/png").
	MIMEType string

	// RevisedPrompt is the prompt the provider actually used after any internal
	// rewriting (e.g. DALL-E 3 always rewrites prompts).
	RevisedPrompt string
}

// GenerationResponse groups one or more results from a single generation call.
type GenerationResponse struct {
	// Images contains the generated images in request order.
	Images []GenerationResult

	// Model is the model identifier that produced the response.
	Model string

	// PromptTokens is the number of tokens consumed by the prompt.
	PromptTokens int
}

// Generation is the core interface for image generation providers.
// Implementations must be safe for concurrent use.
type Generation interface {
	// GenerateImage generates one or more images from the provided text prompt.
	GenerateImage(ctx context.Context, prompt string) (*GenerationResponse, error)

	// Provider returns a human-readable provider name (e.g. "openai", "gemini").
	Provider() string
}
