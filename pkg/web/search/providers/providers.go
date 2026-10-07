package providers

import internalproviders "github.com/KPO-Tech/seshat/internal/web/search/providers"

type (
	// ProviderMode defines the search provider selection mode.
	ProviderMode = internalproviders.ProviderMode
	// ProviderOutput represents the result of a provider search.
	ProviderOutput = internalproviders.ProviderOutput
	// SearchHit represents a single search result.
	SearchHit = internalproviders.SearchHit
	// SearchInput represents the input for a search operation.
	SearchInput = internalproviders.SearchInput
	// SearchProvider defines the interface for search backends.
	SearchProvider = internalproviders.SearchProvider
)

const (
	ProviderModeAuto       = internalproviders.ProviderModeAuto
	ProviderModeCustom     = internalproviders.ProviderModeCustom
	ProviderModeTavily     = internalproviders.ProviderModeTavily
	ProviderModeSearXNG    = internalproviders.ProviderModeSearXNG
	ProviderModeJina       = internalproviders.ProviderModeJina
	ProviderModeExa        = internalproviders.ProviderModeExa
	ProviderModeLangSearch = internalproviders.ProviderModeLangSearch
	ProviderModeFirecrawl  = internalproviders.ProviderModeFirecrawl
	ProviderModeBing       = internalproviders.ProviderModeBing
	ProviderModeYou        = internalproviders.ProviderModeYou
	ProviderModeLinkup     = internalproviders.ProviderModeLinkup
	ProviderModeMojeek     = internalproviders.ProviderModeMojeek
	ProviderModeDuckDuckGo = internalproviders.ProviderModeDuckDuckGo
)

// IsValidMode checks if the given mode is valid.
func IsValidMode(mode string) bool {
	return internalproviders.IsValidMode(mode)
}

// ProviderModeFromEnv returns the configured provider mode from environment.
func ProviderModeFromEnv() ProviderMode {
	return internalproviders.ProviderModeFromEnv()
}

// Search performs a search using the configured provider mode.
func Search(input SearchInput) (ProviderOutput, error) {
	return internalproviders.Search(input)
}

// NewTavilyProvider creates a new Tavily search provider.
func NewTavilyProvider() *internalproviders.TavilyProvider {
	return internalproviders.NewTavilyProvider()
}

// NewTavilyProviderWithAPIKey creates a Tavily search provider that uses apiKey.
func NewTavilyProviderWithAPIKey(apiKey string) *internalproviders.TavilyProvider {
	return internalproviders.NewTavilyProviderWithAPIKey(apiKey)
}

// NewExaProvider creates a new Exa search provider.
func NewExaProvider() *internalproviders.ExaProvider {
	return internalproviders.NewExaProvider()
}

// NewExaProviderWithAPIKey creates an Exa search provider that uses apiKey.
func NewExaProviderWithAPIKey(apiKey string) *internalproviders.ExaProvider {
	return internalproviders.NewExaProviderWithAPIKey(apiKey)
}

// NewJinaProvider creates a new Jina search provider.
func NewJinaProvider() *internalproviders.JinaProvider {
	return internalproviders.NewJinaProvider()
}

// NewJinaProviderWithAPIKey creates a Jina search provider that uses apiKey.
func NewJinaProviderWithAPIKey(apiKey string) *internalproviders.JinaProvider {
	return internalproviders.NewJinaProviderWithAPIKey(apiKey)
}

// NewLangSearchProvider creates a LangSearch provider that uses the key in the LANGSEARCH_API_KEY environment variable.
func NewLangSearchProvider() *internalproviders.LangSearchProvider {
	return internalproviders.NewLangSearchProvider()
}

// NewLangSearchProviderWithAPIKey creates a LangSearch provider that uses apiKey.
func NewLangSearchProviderWithAPIKey(apiKey string) *internalproviders.LangSearchProvider {
	return internalproviders.NewLangSearchProviderWithAPIKey(apiKey)
}

// NewSearXNGProvider creates a provider configured from environment variables.
// Reads SEARXNG_URL or SEARXNG_BASE_URL for the instance base URL.
func NewSearXNGProvider() *internalproviders.SearXNGProvider {
	return internalproviders.NewSearXNGProvider()
}

// NewSearXNGProviderWithBaseURL creates a provider targeting a specific URL.
func NewSearXNGProviderWithBaseURL(baseURL string) *internalproviders.SearXNGProvider {
	return internalproviders.NewSearXNGProviderWithBaseURL(baseURL)
}

// NewSearXNGProviderWithConfig creates a provider with explicit URL and optional Basic Auth credentials.
// username and password are empty when the SearXNG instance has no HTTP Basic Auth.
func NewSearXNGProviderWithConfig(baseURL, username, password string) *internalproviders.SearXNGProvider {
	return internalproviders.NewSearXNGProviderWithConfig(baseURL, username, password)
}

// NewDuckDuckGoProvider creates a no-API-key search provider (DuckDuckGo's
// HTML endpoint) - always available, no configuration required.
func NewDuckDuckGoProvider() *internalproviders.DuckDuckGoProvider {
	return internalproviders.NewDuckDuckGoProvider()
}
