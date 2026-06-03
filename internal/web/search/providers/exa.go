package providers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

// ExaProvider implements search using Exa API.
type ExaProvider struct {
	httpClient *http.Client
	apiKey     string
}

// NewExaProvider creates a new Exa search provider.
func NewExaProvider() *ExaProvider {
	return NewExaProviderWithAPIKey(os.Getenv("EXA_API_KEY"))
}

func NewExaProviderWithAPIKey(apiKey string) *ExaProvider {
	return &ExaProvider{
		apiKey:     strings.TrimSpace(apiKey),
		httpClient: &http.Client{Timeout: 20 * time.Second},
	}
}

// Name returns the provider name.
func (p *ExaProvider) Name() string {
	return "exa"
}

// IsConfigured checks if the provider is configured with API key.
func (p *ExaProvider) IsConfigured() bool {
	return p.apiKey != ""
}

// ExaResponse represents the Exa API response.
type ExaResponse struct {
	Results []struct {
		Title     string  `json:"title"`
		URL       string  `json:"url"`
		Snippet   string  `json:"snippet"`
		Published string  `json:"published"`
		Score     float64 `json:"score"`
	} `json:"results"`
}

// Search performs a web search using Exa.
func (p *ExaProvider) Search(input SearchInput) (ProviderOutput, error) {
	start := time.Now()

	if !p.IsConfigured() {
		return ProviderOutput{}, fmt.Errorf("Exa API key not configured (set EXA_API_KEY)")
	}

	req, err := http.NewRequest("GET", "https://api.exa.ai/search", nil)
	if err != nil {
		return ProviderOutput{}, err
	}

	q := req.URL.Query()
	q.Set("query", input.Query)
	q.Set("num_results", "10")
	req.URL.RawQuery = q.Encode()

	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("User-Agent", "NexusAI-WebSearch/1.0")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return ProviderOutput{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return ProviderOutput{}, fmt.Errorf("Exa API returned status %d", resp.StatusCode)
	}

	var result ExaResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return ProviderOutput{}, fmt.Errorf("failed to decode Exa response: %w", err)
	}

	hits := make([]SearchHit, 0, len(result.Results))
	for _, r := range result.Results {
		hits = append(hits, SearchHit{
			Title:       r.Title,
			URL:         r.URL,
			Description: r.Snippet,
			Source:      "exa",
		})
	}

	return ProviderOutput{
		Hits:            hits,
		ProviderName:    "exa",
		DurationSeconds: time.Since(start).Seconds(),
	}, nil
}
