package search

import (
	"context"

	webcore "github.com/EngineerProjects/nexus-engine/internal/web"
	"github.com/EngineerProjects/nexus-engine/internal/web/search/providers"
)

var _ webcore.SearchService = (*Service)(nil)

// Config configures the shared search service.
type Config struct {
	Cache *Cache
}

// Service wraps provider discovery and execution behind a stable shared search interface.
type Service struct {
	providerMode providers.ProviderMode
	cache        *Cache
}

// NewService creates a shared search service using the current environment-driven provider mode.
func NewService() *Service {
	return NewServiceWithConfig(nil)
}

// NewServiceWithConfig creates a shared search service with explicit cache overrides when needed.
func NewServiceWithConfig(config *Config) *Service {
	if config == nil {
		config = &Config{}
	}
	if config.Cache == nil {
		config.Cache = DefaultCache()
	}
	return &Service{
		providerMode: providers.ProviderModeFromEnv(),
		cache:        config.Cache,
	}
}

// Search executes the configured provider chain and returns the shared search response contract.
func (s *Service) Search(ctx context.Context, request webcore.SearchRequest) (webcore.SearchResponse, error) {
	input := Input{
		Query:          request.Query,
		AllowedDomains: append([]string(nil), request.AllowedDomains...),
		BlockedDomains: append([]string(nil), request.BlockedDomains...),
	}
	if err := input.Validate(); err != nil {
		return Output{}, err
	}

	cacheKey := searchCacheKey(input)
	if s.cache != nil {
		if cached, ok := s.cache.Get(cacheKey); ok {
			return cached, nil
		}
	}

	select {
	case <-ctx.Done():
		return Output{}, ctx.Err()
	default:
	}

	providerOutput, err := providers.Search(providers.SearchInput{
		Query:          input.Query,
		AllowedDomains: input.AllowedDomains,
		BlockedDomains: input.BlockedDomains,
	})
	if err != nil {
		return Output{}, err
	}

	output := FinalizeOutput(input, providerOutput)
	if s.cache != nil {
		s.cache.Set(cacheKey, output)
	}
	return output, nil
}

// ProviderMode reports the provider mode selected when the service was constructed.
func (s *Service) ProviderMode() string {
	return string(s.providerMode)
}

// IsEnabled reports whether any search provider is currently configured.
func IsEnabled() bool {
	return providers.IsProviderAvailable()
}

// GetProviderMode returns the current provider mode from environment.
func GetProviderMode() string {
	return string(providers.ProviderModeFromEnv())
}

// GetConfiguredProviders returns the configured provider names for diagnostics and UI.
func GetConfiguredProviders() []string {
	var names []string
	for _, p := range providers.GetConfiguredProviders() {
		names = append(names, p.Name())
	}
	return names
}
