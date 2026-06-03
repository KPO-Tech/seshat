package providers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

// JinaProvider implements search using Jina API.
type JinaProvider struct {
	httpClient *http.Client
	apiKey     string
}

// NewJinaProvider creates a new Jina search provider.
func NewJinaProvider() *JinaProvider {
	return NewJinaProviderWithAPIKey(os.Getenv("JINA_API_KEY"))
}

func NewJinaProviderWithAPIKey(apiKey string) *JinaProvider {
	return &JinaProvider{
		apiKey:     strings.TrimSpace(apiKey),
		httpClient: &http.Client{Timeout: 20 * time.Second},
	}
}

// Name returns the provider name.
func (p *JinaProvider) Name() string {
	return "jina"
}

// IsConfigured checks if the provider is configured with API key.
func (p *JinaProvider) IsConfigured() bool {
	return p.apiKey != ""
}

// JinaResponse represents the Jina API response.
type JinaResponse struct {
	Code    int `json:"code"`
	Message struct {
		Results []struct {
			Title       string `json:"title"`
			URL         string `json:"url"`
			Description string `json:"description"`
			Icon        string `json:"icon"`
		} `json:"data"`
	} `json:"message"`
}

// Search performs a web search using Jina.
func (p *JinaProvider) Search(input SearchInput) (ProviderOutput, error) {
	start := time.Now()

	if !p.IsConfigured() {
		return ProviderOutput{}, fmt.Errorf("Jina API key not configured (set JINA_API_KEY)")
	}

	reqURL := "https://api.jina.ai/v1/search"
	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return ProviderOutput{}, err
	}

	q := req.URL.Query()
	q.Set("query", input.Query)
	q.Set("limit", "10")
	req.URL.RawQuery = q.Encode()

	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("User-Agent", "NexusAI-WebSearch/1.0")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return ProviderOutput{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return ProviderOutput{}, fmt.Errorf("Jina API returned status %d", resp.StatusCode)
	}

	var result JinaResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return ProviderOutput{}, fmt.Errorf("failed to decode Jina response: %w", err)
	}

	hits := make([]SearchHit, 0, len(result.Message.Results))
	for _, r := range result.Message.Results {
		hits = append(hits, SearchHit{
			Title:       r.Title,
			URL:         r.URL,
			Description: r.Description,
			Source:      "jina",
		})
	}

	return ProviderOutput{
		Hits:            hits,
		ProviderName:    "jina",
		DurationSeconds: time.Since(start).Seconds(),
	}, nil
}
