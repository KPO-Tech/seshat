// Package image is the public, provider-agnostic image generation interface.
// It exists as a standalone client library separate from the generate_image
// agent tool (internal/tools/multimedia/imagegen.go) so a host application
// can call generation directly - e.g. a UI where a human types a prompt -
// without going through an LLM tool-calling loop for a deterministic action
// the human already decided to take. The same provider implementations
// (pkg/image/providers) power both call sites.
package image

import internalimage "github.com/KPO-Tech/seshat-runtime/internal/image"

type (
	// Generation is the core interface for image generation providers.
	Generation = internalimage.Generation

	// GenerationResult holds a single generated image (URL or base64).
	GenerationResult = internalimage.GenerationResult

	// GenerationResponse groups one or more results from a single call.
	GenerationResponse = internalimage.GenerationResponse
)

// ErrStreamingNotSupported is returned by a provider's streaming method (if
// it has one) when it doesn't support incremental image delivery.
var ErrStreamingNotSupported = internalimage.ErrStreamingNotSupported
