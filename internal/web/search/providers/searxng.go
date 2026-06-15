package providers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const searxngMaxRetries = 2

// SearXNGProvider supports self-hosted/open-source metasearch backends.
type SearXNGProvider struct {
	baseURL    string
	httpClient *http.Client
}

func NewSearXNGProvider() *SearXNGProvider {
	return NewSearXNGProviderWithBaseURL(os.Getenv("SEARXNG_BASE_URL"))
}

func NewSearXNGProviderWithBaseURL(baseURL string) *SearXNGProvider {
	return &SearXNGProvider{
		baseURL:    strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		httpClient: &http.Client{Timeout: 20 * time.Second},
	}
}

func (p *SearXNGProvider) Name() string {
	return "searxng"
}

func (p *SearXNGProvider) IsConfigured() bool {
	return p.baseURL != ""
}

func (p *SearXNGProvider) Search(input SearchInput) (ProviderOutput, error) {
	start := time.Now()
	if !p.IsConfigured() {
		return ProviderOutput{}, fmt.Errorf("SearXNG base URL not configured (set SEARXNG_BASE_URL)")
	}

	var lastErr error
	for attempt := range searxngMaxRetries {
		if attempt > 0 {
			time.Sleep(500 * time.Millisecond)
		}
		out, err := p.searchOnce(input)
		if err == nil {
			out.DurationSeconds = time.Since(start).Seconds()
			return out, nil
		}
		lastErr = err
	}
	return ProviderOutput{}, lastErr
}

func (p *SearXNGProvider) searchOnce(input SearchInput) (ProviderOutput, error) {
	req, err := http.NewRequest(http.MethodGet, p.baseURL+"/search", nil)
	if err != nil {
		return ProviderOutput{}, err
	}
	q := req.URL.Query()
	q.Set("q", input.Query)
	q.Set("format", "json")
	req.URL.RawQuery = q.Encode()
	req.Header.Set("User-Agent", "NexusAI-WebSearch/1.0")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return ProviderOutput{}, fmt.Errorf("could not reach SearXNG at %s: %w", p.baseURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return ProviderOutput{}, fmt.Errorf("SearXNG returned status %d — is the instance running at %s?", resp.StatusCode, p.baseURL)
	}

	// Guard against HTML error pages that some instances return instead of JSON.
	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "json") {
		preview, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return ProviderOutput{}, fmt.Errorf("SearXNG returned non-JSON response (%s) — ensure format=json is enabled on the instance: %q", ct, string(preview))
	}

	var payload struct {
		Results []struct {
			Title   string `json:"title"`
			URL     string `json:"url"`
			Content string `json:"content"`
			Engine  string `json:"engine"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return ProviderOutput{}, fmt.Errorf("failed to decode SearXNG response: %w", err)
	}

	hits := make([]SearchHit, 0, len(payload.Results))
	for _, item := range payload.Results {
		hits = append(hits, SearchHit{
			Title:       strings.TrimSpace(item.Title),
			URL:         strings.TrimSpace(item.URL),
			Description: strings.TrimSpace(item.Content),
			Source:      strings.TrimSpace(item.Engine),
		})
	}
	return ProviderOutput{
		Hits:         hits,
		ProviderName: "searxng",
	}, nil
}
