package providers

import (
	"strings"
	"time"

	"github.com/EngineerProjects/nexus-engine/internal/web/searxng"
)

// SearXNGProvider wraps the full-featured searxng.Client and satisfies SearchProvider.
// It ports all key behaviours from the mcp-searxng TypeScript server:
//   - HTML fallback when the instance does not serve JSON (always enabled)
//   - Basic auth (SEARXNG_AUTH_USERNAME / AUTH_USERNAME)
//   - Full search params (language, time_range, safesearch, categories, engines)
//   - Custom User-Agent and proxy support
//   - Configurable timeout (SEARXNG_TIMEOUT_MS)
type SearXNGProvider struct {
	client *searxng.Client
}

// NewSearXNGProvider creates a provider configured from environment variables.
// Reads SEARXNG_URL or SEARXNG_BASE_URL for the instance base URL.
func NewSearXNGProvider() *SearXNGProvider {
	return &SearXNGProvider{client: searxng.NewClient()}
}

// NewSearXNGProviderWithBaseURL creates a provider targeting a specific URL.
func NewSearXNGProviderWithBaseURL(baseURL string) *SearXNGProvider {
	return &SearXNGProvider{client: searxng.NewClientWithURL(baseURL)}
}

// NewSearXNGProviderWithConfig creates a provider with explicit URL and optional Basic Auth credentials.
// username and password are empty when the SearXNG instance has no HTTP Basic Auth.
func NewSearXNGProviderWithConfig(baseURL, username, password string) *SearXNGProvider {
	return &SearXNGProvider{client: searxng.NewClientWithURLAndAuth(baseURL, username, password)}
}

func (p *SearXNGProvider) Name() string { return "searxng" }

func (p *SearXNGProvider) IsConfigured() bool { return p.client.IsConfigured() }

func (p *SearXNGProvider) Search(input SearchInput) (ProviderOutput, error) {
	start := time.Now()

	resp, err := p.client.Search(searxng.SearchInput{
		Query: input.Query,
	})
	if err != nil {
		return ProviderOutput{}, err
	}

	hits := make([]SearchHit, 0, len(resp.Results))
	for _, r := range resp.Results {
		// Apply domain allow/block filters (handled by the search service layer,
		// but we respect them here too for defence in depth).
		if len(input.AllowedDomains) > 0 && !matchesDomainList(r.URL, input.AllowedDomains) {
			continue
		}
		if matchesDomainList(r.URL, input.BlockedDomains) {
			continue
		}
		hits = append(hits, SearchHit{
			Title:       r.Title,
			URL:         r.URL,
			Description: r.Content,
			Source:      r.Engine,
		})
	}

	return ProviderOutput{
		Hits:            hits,
		ProviderName:    "searxng",
		DurationSeconds: time.Since(start).Seconds(),
	}, nil
}

// matchesDomainList reports whether rawURL's host matches any entry in the list.
func matchesDomainList(rawURL string, domains []string) bool {
	if len(domains) == 0 {
		return false
	}
	// Extract host from URL cheaply (avoid a full url.Parse on every hit).
	host := rawURL
	if idx := strings.Index(host, "://"); idx >= 0 {
		host = host[idx+3:]
	}
	if idx := strings.IndexByte(host, '/'); idx >= 0 {
		host = host[:idx]
	}
	if idx := strings.IndexByte(host, ':'); idx >= 0 {
		host = host[:idx] // strip port
	}
	host = strings.ToLower(host)

	for _, d := range domains {
		d = strings.ToLower(strings.TrimPrefix(d, "."))
		if host == d || strings.HasSuffix(host, "."+d) {
			return true
		}
	}
	return false
}
