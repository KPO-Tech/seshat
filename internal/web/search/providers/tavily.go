package providers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

// TavilyProvider implements search using Tavily API.
type TavilyProvider struct {
	httpClient *http.Client
	apiKey     string
}

// NewTavilyProvider creates a new Tavily search provider.
func NewTavilyProvider() *TavilyProvider {
	return NewTavilyProviderWithAPIKey(os.Getenv("TAVILY_API_KEY"))
}

func NewTavilyProviderWithAPIKey(apiKey string) *TavilyProvider {
	return &TavilyProvider{
		apiKey:     strings.TrimSpace(apiKey),
		httpClient: &http.Client{Timeout: 20 * time.Second},
	}
}

// Name returns the provider name.
func (p *TavilyProvider) Name() string {
	return "tavily"
}

// IsConfigured checks if the provider is configured with API key.
func (p *TavilyProvider) IsConfigured() bool {
	return p.apiKey != ""
}

// TavilyResponse represents the Tavily API response.
type TavilyResponse struct {
	Results []struct {
		Title       string `json:"title"`
		URL         string `json:"url"`
		Content     string `json:"content"`
		PublishedOn string `json:"published_on"`
	} `json:"results"`
}

// Search performs a web search using Tavily.
func (p *TavilyProvider) Search(input SearchInput) (ProviderOutput, error) {
	start := time.Now()

	if !p.IsConfigured() {
		return ProviderOutput{}, fmt.Errorf("Tavily API key not configured (set TAVILY_API_KEY)")
	}

	payload := map[string]any{
		"query":               input.Query,
		"topic":               "general",
		"search_depth":        "basic",
		"max_results":         10,
		"include_answer":      false,
		"include_raw_content": false,
	}
	if len(input.AllowedDomains) > 0 {
		payload["include_domains"] = input.AllowedDomains
	}
	if len(input.BlockedDomains) > 0 {
		payload["exclude_domains"] = input.BlockedDomains
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return ProviderOutput{}, fmt.Errorf("failed to encode Tavily request: %w", err)
	}

	req, err := http.NewRequest("POST", "https://api.tavily.com/search", bytes.NewReader(body))
	if err != nil {
		return ProviderOutput{}, err
	}

	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "NexusAI-WebSearch/1.0")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return ProviderOutput{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return ProviderOutput{}, fmt.Errorf("Tavily API returned status %d", resp.StatusCode)
	}

	var result TavilyResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return ProviderOutput{}, fmt.Errorf("failed to decode Tavily response: %w", err)
	}

	hits := make([]SearchHit, 0, len(result.Results))
	for _, r := range result.Results {
		hits = append(hits, SearchHit{
			Title:       r.Title,
			URL:         r.URL,
			Description: r.Content,
			Source:      "tavily",
		})
	}

	return ProviderOutput{
		Hits:            hits,
		ProviderName:    "tavily",
		DurationSeconds: time.Since(start).Seconds(),
	}, nil
}
