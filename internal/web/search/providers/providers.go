package providers

// AllProviders returns all available search providers in registration order.
// Note: custom provider is intentionally excluded from the auto chain.
func AllProviders() []SearchProvider {
	return []SearchProvider{
		NewTavilyProvider(),
		NewExaProvider(),
		NewJinaProvider(),
		NewLangSearchProvider(),
		NewSearXNGProvider(),
		NewDuckDuckGoProvider(),
	}
}

// autoProviderPriority defines the fallback order in auto mode.
// Lower = tried first. Paid/high-quality APIs first, free-key next,
// self-hosted last, scraping-based fallback at the very end.
func autoProviderPriority(name string) int {
	switch name {
	case "tavily":
		return 10
	case "exa":
		return 20
	case "jina":
		return 30
	case "langsearch":
		return 35 // free API key, clean JSON, AI-optimised results
	case "searxng":
		return 40 // self-hosted, no third-party key required
	case "ddg":
		return 50 // scraping fallback, always available
	default:
		return 100
	}
}

// GetConfiguredProviders returns only the providers that are configured.
func GetConfiguredProviders() []SearchProvider {
	var configured []SearchProvider
	for _, p := range AllProviders() {
		if p.IsConfigured() {
			configured = append(configured, p)
		}
	}
	return configured
}

// Search performs a search using the configured provider mode.
func Search(input SearchInput) (ProviderOutput, error) {
	mode := ProviderModeFromEnv()
	chain := GetProviderChain(mode, AllProviders())
	return RunSearch(input, chain, mode)
}

// ProviderDebugInfo returns debug information about provider configuration.
func ProviderDebugInfo() map[string]bool {
	info := make(map[string]bool)
	for _, p := range AllProviders() {
		info[p.Name()] = p.IsConfigured()
	}
	info["mode"] = true
	info["current_mode"] = true
	return info
}

// GetProviderMode returns the current provider mode.
func GetCurrentMode() ProviderMode {
	return ProviderModeFromEnv()
}

// IsProviderAvailable checks if any provider is configured.
func IsProviderAvailable() bool {
	return len(GetConfiguredProviders()) > 0
}
