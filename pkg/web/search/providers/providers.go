package providers

import internalproviders "github.com/EngineerProjects/nexus-engine/internal/web/search/providers"

type (
	ProviderMode   = internalproviders.ProviderMode
	ProviderOutput = internalproviders.ProviderOutput
	SearchHit      = internalproviders.SearchHit
	SearchInput    = internalproviders.SearchInput
	SearchProvider = internalproviders.SearchProvider
)

const (
	ProviderModeAuto       = internalproviders.ProviderModeAuto
	ProviderModeCustom     = internalproviders.ProviderModeCustom
	ProviderModeTavily     = internalproviders.ProviderModeTavily
	ProviderModeDuckDuckGo = internalproviders.ProviderModeDuckDuckGo
	ProviderModeSearXNG    = internalproviders.ProviderModeSearXNG
	ProviderModeJina       = internalproviders.ProviderModeJina
	ProviderModeExa        = internalproviders.ProviderModeExa
	ProviderModeLangSearch = internalproviders.ProviderModeLangSearch
	ProviderModeFirecrawl  = internalproviders.ProviderModeFirecrawl
	ProviderModeBing       = internalproviders.ProviderModeBing
	ProviderModeYou        = internalproviders.ProviderModeYou
	ProviderModeLinkup     = internalproviders.ProviderModeLinkup
	ProviderModeMojeek     = internalproviders.ProviderModeMojeek
)

func IsValidMode(mode string) bool {
	return internalproviders.IsValidMode(mode)
}

func ProviderModeFromEnv() ProviderMode {
	return internalproviders.ProviderModeFromEnv()
}

func Search(input SearchInput) (ProviderOutput, error) {
	return internalproviders.Search(input)
}

func NewTavilyProvider() *internalproviders.TavilyProvider {
	return internalproviders.NewTavilyProvider()
}

func NewTavilyProviderWithAPIKey(apiKey string) *internalproviders.TavilyProvider {
	return internalproviders.NewTavilyProviderWithAPIKey(apiKey)
}

func NewExaProvider() *internalproviders.ExaProvider {
	return internalproviders.NewExaProvider()
}

func NewExaProviderWithAPIKey(apiKey string) *internalproviders.ExaProvider {
	return internalproviders.NewExaProviderWithAPIKey(apiKey)
}

func NewJinaProvider() *internalproviders.JinaProvider {
	return internalproviders.NewJinaProvider()
}

func NewJinaProviderWithAPIKey(apiKey string) *internalproviders.JinaProvider {
	return internalproviders.NewJinaProviderWithAPIKey(apiKey)
}

func NewLangSearchProvider() *internalproviders.LangSearchProvider {
	return internalproviders.NewLangSearchProvider()
}

func NewLangSearchProviderWithAPIKey(apiKey string) *internalproviders.LangSearchProvider {
	return internalproviders.NewLangSearchProviderWithAPIKey(apiKey)
}

func NewSearXNGProvider() *internalproviders.SearXNGProvider {
	return internalproviders.NewSearXNGProvider()
}

func NewSearXNGProviderWithBaseURL(baseURL string) *internalproviders.SearXNGProvider {
	return internalproviders.NewSearXNGProviderWithBaseURL(baseURL)
}

func NewDuckDuckGoProvider() *internalproviders.DuckDuckGoProvider {
	return internalproviders.NewDuckDuckGoProvider()
}
