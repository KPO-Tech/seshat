package providers

import (
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
)

// ProviderModeFromEnv returns the configured provider mode from environment.
func ProviderModeFromEnv() ProviderMode {
	raw := strings.TrimSpace(os.Getenv("WEB_SEARCH_PROVIDER"))
	if raw == "" {
		return ProviderModeAuto
	}
	if IsValidMode(raw) {
		return ProviderMode(raw)
	}
	return ProviderModeAuto
}

// GetProviderChain returns the list of providers to try based on the mode.
func GetProviderChain(mode ProviderMode, allProviders []SearchProvider) []SearchProvider {
	if mode == ProviderModeAuto {
		var configured []SearchProvider
		for _, p := range allProviders {
			if p.IsConfigured() {
				configured = append(configured, p)
			}
		}
		sort.SliceStable(configured, func(i, j int) bool {
			return autoProviderPriority(configured[i].Name()) < autoProviderPriority(configured[j].Name())
		})
		return configured
	}

	// Specific mode - return single provider if configured
	for _, p := range allProviders {
		if strings.EqualFold(p.Name(), string(mode)) && p.IsConfigured() {
			return []SearchProvider{p}
		}
	}
	return nil
}

func shouldFallbackOnEmpty(provider SearchProvider) bool {
	switch provider.Name() {
	case "ddg", "searxng":
		return true
	default:
		return false
	}
}

// RunSearch executes a search using the provider chain.
// In auto mode, it tries each provider in order and falls through on failure.
// In specific mode, it fails immediately if the provider is not configured.
func RunSearch(input SearchInput, chain []SearchProvider, mode ProviderMode) (ProviderOutput, error) {
	if len(chain) == 0 {
		return ProviderOutput{}, fmt.Errorf("no search providers available for mode %q", mode)
	}

	var lastError error

	for i, provider := range chain {
		output, err := provider.Search(input)
		if err == nil {
			if mode == ProviderModeAuto && len(output.Hits) == 0 && shouldFallbackOnEmpty(provider) && i < len(chain)-1 {
				log.Printf("[web-search] %s returned 0 hits, trying next provider...", provider.Name())
				continue
			}
			return output, nil
		}

		lastError = err

		// In specific mode (not auto), fail immediately
		if mode != ProviderModeAuto {
			return ProviderOutput{}, fmt.Errorf("search provider %q failed: %w", provider.Name(), err)
		}

		// In auto mode, try next provider
		if i < len(chain)-1 {
			log.Printf("[web-search] %s failed: %v, trying next provider...", provider.Name(), err)
		}
	}

	// All providers failed
	if lastError != nil {
		return ProviderOutput{}, fmt.Errorf("all search providers failed, last error: %w", lastError)
	}
	return ProviderOutput{}, fmt.Errorf("all search providers failed with no error details")
}
