package providers

import (
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// DuckDuckGoProvider implements search using DuckDuckGo HTML API.
type DuckDuckGoProvider struct {
	httpClient *http.Client
}

// NewDuckDuckGoProvider creates a new DuckDuckGo search provider.
func NewDuckDuckGoProvider() *DuckDuckGoProvider {
	return &DuckDuckGoProvider{
		httpClient: &http.Client{Timeout: 20 * time.Second},
	}
}

// Name returns the provider name.
func (p *DuckDuckGoProvider) Name() string {
	return "ddg"
}

// IsConfigured checks if the provider is available (always true for DDG).
func (p *DuckDuckGoProvider) IsConfigured() bool {
	return true
}

// Search performs a web search using DuckDuckGo.
func (p *DuckDuckGoProvider) Search(input SearchInput) (ProviderOutput, error) {
	start := time.Now()

	req, err := http.NewRequest(http.MethodGet, "https://html.duckduckgo.com/html/", nil)
	if err != nil {
		return ProviderOutput{}, err
	}

	q := req.URL.Query()
	q.Set("q", input.Query)
	req.URL.RawQuery = q.Encode()

	req.Header.Set("User-Agent", "NexusAI-WebSearch/1.0 (compatible; bot)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return ProviderOutput{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return ProviderOutput{}, fmt.Errorf("DuckDuckGo returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ProviderOutput{}, fmt.Errorf("failed to read DuckDuckGo response: %w", err)
	}
	hits := parseDuckDuckGoHTML(string(body))

	return ProviderOutput{
		Hits:            hits,
		ProviderName:    "duckduckgo",
		DurationSeconds: time.Since(start).Seconds(),
	}, nil
}

var (
	ddgResultRe  = regexp.MustCompile(`(?is)<a[^>]*class="[^"]*result__a[^"]*"[^>]*href="([^"]+)"[^>]*>(.*?)</a>`)
	ddgSnippetRe = regexp.MustCompile(`(?is)<a[^>]*class="[^"]*result__snippet[^"]*"[^>]*>(.*?)</a>|<div[^>]*class="[^"]*result__snippet[^"]*"[^>]*>(.*?)</div>`)
	ddgTagRe     = regexp.MustCompile(`(?is)<[^>]+>`)
)

func parseDuckDuckGoHTML(body string) []SearchHit {
	matches := ddgResultRe.FindAllStringSubmatch(body, 10)
	snippets := ddgSnippetRe.FindAllStringSubmatch(body, 10)
	hits := make([]SearchHit, 0, len(matches))
	for index, match := range matches {
		title := sanitizeHTMLText(match[2])
		url := strings.TrimSpace(match[1])
		description := ""
		if index < len(snippets) {
			description = sanitizeHTMLText(snippets[index][1] + " " + snippets[index][2])
		}
		if title == "" || url == "" {
			continue
		}
		hits = append(hits, SearchHit{
			Title:       title,
			URL:         url,
			Description: description,
			Source:      "duckduckgo",
		})
	}
	return hits
}

func sanitizeHTMLText(value string) string {
	value = ddgTagRe.ReplaceAllString(value, " ")
	return strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
}
