// Package model re-exports internal/model's LLM model-metadata registry
// (context windows, pricing, capability flags such as vision support) for
// external consumers that need to query or extend it directly - e.g.
// pkg/pdfsmart/vision.Config.Registry, which uses this to gate the
// vision-LLM fallback behind the configured model actually supporting
// image content blocks.
package model

import internalmodel "github.com/KPO-Tech/seshat/internal/model"

type (
	Capabilities  = internalmodel.Capabilities
	Pricing       = internalmodel.Pricing
	ContextWindow = internalmodel.ContextWindow
	Metadata      = internalmodel.Metadata
	Registry      = internalmodel.Registry
)

// NewRegistry creates an empty registry.
func NewRegistry() *Registry {
	return internalmodel.NewRegistry()
}

// Global is the package-level registry populated at startup by
// internal/providers - the same instance internal/pdfsmart/vision.Config
// defaults to when Registry is left nil.
var Global = internalmodel.Global
