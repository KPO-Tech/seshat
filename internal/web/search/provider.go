package search

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/EngineerProjects/nexus-engine/internal/web/search/providers"
)

// HTTPProvider is a configurable HTTP search provider.
type HTTPProvider struct {
	name       string
	endpoint   string
	httpClient *http.Client
}

// NewHTTPProvider creates a provider from environment configuration.
func NewHTTPProvider() *HTTPProvider {
	return &HTTPProvider{
		name:       providerNameFromEnv(),
		endpoint:   strings.TrimSpace(os.Getenv("WEB_SEARCH_API")),
		httpClient: &http.Client{Timeout: 20 * time.Second},
	}
}

func providerNameFromEnv() string {
	if name := strings.TrimSpace(os.Getenv("WEB_SEARCH_PROVIDER")); name != "" {
		return name
	}
	return "custom"
}

func (p *HTTPProvider) Name() string {
	return p.name
}

func (p *HTTPProvider) IsConfigured() bool {
	return p.endpoint != ""
}

func (p *HTTPProvider) Search(input providers.SearchInput) (providers.ProviderOutput, error) {
	start := time.Now()
	if !p.IsConfigured() {
		return providers.ProviderOutput{}, fmt.Errorf("WEB_SEARCH_API is not configured")
	}

	endpoint, err := url.Parse(p.endpoint)
	if err != nil {
		return providers.ProviderOutput{}, fmt.Errorf("invalid WEB_SEARCH_API: %w", err)
	}
	query := endpoint.Query()
	query.Set("q", input.Query)
	for _, domain := range input.AllowedDomains {
		query.Add("allowed_domain", domain)
	}
	for _, domain := range input.BlockedDomains {
		query.Add("blocked_domain", domain)
	}
	endpoint.RawQuery = query.Encode()

	req, err := http.NewRequest(http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return providers.ProviderOutput{}, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "NexusAI-WebSearch/1.0")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return providers.ProviderOutput{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return providers.ProviderOutput{}, fmt.Errorf("search provider returned %d", resp.StatusCode)
	}

	var payload any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return providers.ProviderOutput{}, err
	}

	hits := normalizePayloadToHits(payload)
	hits = applyDomainFilters(hits, input)

	return providers.ProviderOutput{
		Hits:            hits,
		ProviderName:    p.name,
		DurationSeconds: time.Since(start).Seconds(),
	}, nil
}
