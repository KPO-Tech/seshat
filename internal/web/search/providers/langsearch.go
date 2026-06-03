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

// LangSearchProvider implements search using the LangSearch API.
// Free tier requires an API key — register at langsearch.com (no credit card).
type LangSearchProvider struct {
	httpClient *http.Client
	apiKey     string
}

func NewLangSearchProvider() *LangSearchProvider {
	return NewLangSearchProviderWithAPIKey(os.Getenv("LANGSEARCH_API_KEY"))
}

func NewLangSearchProviderWithAPIKey(apiKey string) *LangSearchProvider {
	return &LangSearchProvider{
		apiKey:     strings.TrimSpace(apiKey),
		httpClient: &http.Client{Timeout: 20 * time.Second},
	}
}

func (p *LangSearchProvider) Name() string { return "langsearch" }

func (p *LangSearchProvider) IsConfigured() bool { return p.apiKey != "" }

func (p *LangSearchProvider) Search(input SearchInput) (ProviderOutput, error) {
	start := time.Now()
	if !p.IsConfigured() {
		return ProviderOutput{}, fmt.Errorf("LangSearch API key not configured (set LANGSEARCH_API_KEY)")
	}

	payload := map[string]any{
		"query": input.Query,
		"count": 10,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return ProviderOutput{}, fmt.Errorf("failed to encode LangSearch request: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, "https://api.langsearch.com/v1/web-search", bytes.NewReader(body))
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
		return ProviderOutput{}, fmt.Errorf("LangSearch API returned status %d", resp.StatusCode)
	}

	var result struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			WebPages struct {
				Value []struct {
					Name    string `json:"name"`
					URL     string `json:"url"`
					Snippet string `json:"snippet"`
					Summary string `json:"summary"`
				} `json:"value"`
			} `json:"webPages"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return ProviderOutput{}, fmt.Errorf("failed to decode LangSearch response: %w", err)
	}

	hits := make([]SearchHit, 0, len(result.Data.WebPages.Value))
	for _, item := range result.Data.WebPages.Value {
		desc := strings.TrimSpace(item.Snippet)
		if desc == "" {
			desc = strings.TrimSpace(item.Summary)
		}
		hits = append(hits, SearchHit{
			Title:       strings.TrimSpace(item.Name),
			URL:         strings.TrimSpace(item.URL),
			Description: desc,
			Source:      "langsearch",
		})
	}
	return ProviderOutput{
		Hits:            hits,
		ProviderName:    "langsearch",
		DurationSeconds: time.Since(start).Seconds(),
	}, nil
}
