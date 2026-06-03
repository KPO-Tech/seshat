package providers

// SearchInput represents the input for a search operation.
type SearchInput struct {
	Query          string
	AllowedDomains []string
	BlockedDomains []string
}

// SearchHit represents a single search result.
type SearchHit struct {
	Title       string
	URL         string
	Description string
	Source      string
}

// ProviderOutput represents the result of a provider search.
type ProviderOutput struct {
	Hits            []SearchHit
	ProviderName    string
	DurationSeconds float64
}

// SearchProvider defines the interface for search backends.
type SearchProvider interface {
	Name() string
	IsConfigured() bool
	Search(input SearchInput) (ProviderOutput, error)
}

// ProviderMode defines the search provider selection mode.
type ProviderMode string

const (
	ProviderModeAuto       ProviderMode = "auto"
	ProviderModeCustom     ProviderMode = "custom"
	ProviderModeTavily     ProviderMode = "tavily"
	ProviderModeDuckDuckGo ProviderMode = "ddg"
	ProviderModeSearXNG    ProviderMode = "searxng"
	ProviderModeJina       ProviderMode = "jina"
	ProviderModeExa        ProviderMode = "exa"
	ProviderModeLangSearch ProviderMode = "langsearch"
	ProviderModeFirecrawl  ProviderMode = "firecrawl"
	ProviderModeBing       ProviderMode = "bing"
	ProviderModeYou        ProviderMode = "you"
	ProviderModeLinkup     ProviderMode = "linkup"
	ProviderModeMojeek     ProviderMode = "mojeek"
)

// IsValidMode checks if the given mode is valid.
func IsValidMode(mode string) bool {
	validModes := map[string]bool{
		"auto":       true,
		"custom":     true,
		"tavily":     true,
		"ddg":        true,
		"searxng":    true,
		"jina":       true,
		"exa":        true,
		"langsearch": true,
		"firecrawl":  true,
		"bing":       true,
		"you":        true,
		"linkup":     true,
		"mojeek":     true,
	}
	return validModes[mode]
}
