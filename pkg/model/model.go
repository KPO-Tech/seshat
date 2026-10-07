// Package model re-exports internal/model's LLM model-metadata registry
// (context windows, pricing, capability flags such as vision support) for
// external consumers that need to query or extend it directly - e.g.
// pkg/pdfsmart/vision.Config.Registry, which uses this to gate the
// vision-LLM fallback behind the configured model actually supporting
// image content blocks.
package model

import internalmodel "github.com/KPO-Tech/seshat/internal/model"

type (
	// Capabilities describes what a specific model variant can do.
	Capabilities = internalmodel.Capabilities
	// Pricing holds per-token costs in USD per 1 million tokens (MTok).
	// Zero values mean pricing is unknown or not applicable.
	Pricing = internalmodel.Pricing
	// ContextWindow describes the token budget for a model.
	ContextWindow = internalmodel.ContextWindow
	// Metadata aggregates all model-level metadata.
	Metadata = internalmodel.Metadata
	// Registry is a lookup table from (provider, modelID) → Metadata.
	// It is safe for concurrent reads; writes happen only at init time via
	// Register or the package-level Global variable.
	Registry = internalmodel.Registry
)

// NewRegistry creates an empty registry.
func NewRegistry() *Registry {
	return internalmodel.NewRegistry()
}

// Global is the package-level registry populated at startup by
// internal/providers - the same instance internal/pdfsmart/vision.Config
// defaults to when Registry is left nil.
var Global = internalmodel.Global
