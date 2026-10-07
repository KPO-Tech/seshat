package search

import (
	internalsearch "github.com/KPO-Tech/seshat/internal/web/search"
	internalproviders "github.com/KPO-Tech/seshat/internal/web/search/providers"
)

type (
	// Config configures the shared search service.
	Config = internalsearch.Config
	// Input represents a normalized search request for the shared search core.
	Input = internalsearch.Input
	// Output represents a normalized search response produced by the shared search core.
	Output = internalsearch.Output
	// Service wraps provider discovery and execution behind a stable shared search interface.
	Service = internalsearch.Service
)

// NewService creates a shared search service using the current environment-driven provider mode.
func NewService() *Service {
	return internalsearch.NewService()
}

// NewServiceWithConfig creates a shared search service with explicit cache overrides when needed.
func NewServiceWithConfig(config *Config) *Service {
	return internalsearch.NewServiceWithConfig(config)
}

// FinalizeOutput applies the shared normalization, filtering, dedupe, and ranking
// used by the core search service so backend callers can stay behaviorally aligned.
func FinalizeOutput(input Input, output internalproviders.ProviderOutput) Output {
	return internalsearch.FinalizeOutput(input, output)
}

// IsEnabled reports whether any search provider is currently configured.
func IsEnabled() bool {
	return internalsearch.IsEnabled()
}

// GetProviderMode returns the current provider mode from environment.
func GetProviderMode() string {
	return internalsearch.GetProviderMode()
}

// GetConfiguredProviders returns the configured provider names for diagnostics and UI.
func GetConfiguredProviders() []string {
	return internalsearch.GetConfiguredProviders()
}
