package providers

import (
	"context"

	internalproviders "github.com/KPO-Tech/seshat/internal/providers"
	internaltypes "github.com/KPO-Tech/seshat/internal/types"
)

type (
	Client          = internalproviders.Client
	Config          = internalproviders.Config
	DiscoveryResult = internalproviders.DiscoveryResult
	FetchedModel    = internalproviders.FetchedModel
	ModelInfo       = internalproviders.ModelInfo
	ProviderInfo    = internalproviders.ProviderInfo
)

func NewClient(apiKey string, providerType internaltypes.APIProvider) *Client {
	return internalproviders.NewClient(apiKey, providerType)
}

// NewClientWithConfig builds a provider client from an explicit Config
// (provider type, base URL, etc.) rather than just a provider type - needed
// by callers resolving a specific org/platform provider setting (custom
// base URL, self-hosted endpoint) instead of using provider defaults.
func NewClientWithConfig(apiKey string, config *Config) *Client {
	return internalproviders.NewClientWithConfig(apiKey, config)
}

func StaticModels(providerName string) []FetchedModel {
	return internalproviders.StaticModels(providerName)
}

func FetchModels(ctx context.Context, providerName, baseURL, apiKey string) ([]FetchedModel, error) {
	return internalproviders.FetchModels(ctx, providerName, baseURL, apiKey)
}

func DefaultBaseURL(providerName string) string {
	return internalproviders.DefaultBaseURL(providerName)
}

func NeedsCatalogReseed(providerName string, knownModelIDs []string) bool {
	return internalproviders.NeedsCatalogReseed(providerName, knownModelIDs)
}

func ResolveProviderFromString(s string) internaltypes.APIProvider {
	return internalproviders.ResolveProviderFromString(s)
}

func GetModelInfo(provider internaltypes.APIProvider, model string) (ModelInfo, bool) {
	return internalproviders.GetModelInfo(provider, model)
}

func AllProvidersInfo() map[internaltypes.APIProvider]ProviderInfo {
	return internalproviders.AllProvidersInfo()
}

func DiscoverProviders(ctx context.Context) []DiscoveryResult {
	return internalproviders.DiscoverProviders(ctx)
}

func GetProviderConfig(provider internaltypes.APIProvider) *Config {
	return internalproviders.GetProviderConfig(provider)
}
