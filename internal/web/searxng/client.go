// Package searxng provides a full-featured SearXNG search client for Go,
// mirroring the mcp-searxng TypeScript server (src/search.ts).
//
// Key behaviours ported from the MCP:
//   - Full search parameter support (language, time_range, safesearch,
//     categories, engines, pageno, num_results, min_score)
//   - HTML fallback: always active — when the instance returns 403/404 or a
//     non-JSON body, the client retries without format=json and parses the HTML DOM
//   - Basic authentication via SEARXNG_AUTH_USERNAME / SEARXNG_AUTH_PASSWORD
//     (or AUTH_USERNAME / AUTH_PASSWORD for MCP compatibility)
//   - Custom User-Agent via SEARXNG_USER_AGENT / USER_AGENT
//   - HTTP proxy via SEARXNG_HTTP_PROXY / HTTP_PROXY / HTTPS_PROXY
//   - Configurable timeout via SEARXNG_TIMEOUT_MS (default 10 000 ms)
//   - Result truncation via SEARXNG_MAX_RESULTS (operator cap 1–20)
//   - Per-result char limit via SEARXNG_MAX_RESULT_CHARS
package searxng

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultSearchTimeoutMS = 10_000
	defaultMaxResults      = 20
)

// Client is a stateless SearXNG search client.
// Create one with NewClient and reuse it across requests.
type Client struct {
	baseURL    *url.URL
	httpClient *http.Client
	auth       string // base64-encoded "user:pass", empty when not configured
	userAgent  string
}

// NewClient creates a Client configured from environment variables.
//
// Environment variables (in priority order):
//   - SEARXNG_URL or SEARXNG_BASE_URL — base URL of the SearXNG instance (required)
//   - SEARXNG_TIMEOUT_MS              — request timeout in milliseconds (default 10 000)
//   - SEARXNG_AUTH_USERNAME / AUTH_USERNAME — basic auth username
//   - SEARXNG_AUTH_PASSWORD / AUTH_PASSWORD — basic auth password
//   - SEARXNG_USER_AGENT / USER_AGENT       — custom User-Agent header
//   - SEARXNG_HTTP_PROXY / HTTP_PROXY / HTTPS_PROXY — proxy URL
func NewClient() *Client {
	raw := firstNonEmpty(
		os.Getenv("SEARXNG_URL"),
		os.Getenv("SEARXNG_BASE_URL"),
	)
	raw = strings.TrimRight(strings.TrimSpace(raw), "/")

	var base *url.URL
	if raw != "" {
		if u, err := url.Parse(raw); err == nil {
			base = u
		}
	}

	timeoutMs := parseIntEnv("SEARXNG_TIMEOUT_MS", defaultSearchTimeoutMS)
	transport := buildTransport()

	return &Client{
		baseURL: base,
		httpClient: &http.Client{
			Timeout:   time.Duration(timeoutMs) * time.Millisecond,
			Transport: transport,
		},
		auth:      buildBasicAuth(),
		userAgent: firstNonEmpty(os.Getenv("SEARXNG_USER_AGENT"), os.Getenv("USER_AGENT"), "NexusAI-WebSearch/2.0"),
	}
}

// NewClientWithURL creates a Client pointing at a specific base URL.
// All other settings are read from environment variables.
func NewClientWithURL(rawURL string) *Client {
	c := NewClient()
	rawURL = strings.TrimRight(strings.TrimSpace(rawURL), "/")
	if u, err := url.Parse(rawURL); err == nil {
		c.baseURL = u
	}
	return c
}

// NewClientWithURLAndAuth creates a Client with an explicit base URL and Basic Auth credentials.
// username and password are empty strings when no auth is needed.
func NewClientWithURLAndAuth(rawURL, username, password string) *Client {
	c := NewClientWithURL(rawURL)
	if username != "" && password != "" {
		c.auth = base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
	}
	return c
}

// NewClientForTest creates a minimal Client with a custom http.RoundTripper.
// Intended for unit tests only; skips env-var reading for transport/auth.
func NewClientForTest(rawURL string, transport http.RoundTripper) *Client {
	rawURL = strings.TrimRight(strings.TrimSpace(rawURL), "/")
	var base *url.URL
	if rawURL != "" {
		if u, err := url.Parse(rawURL); err == nil {
			base = u
		}
	}
	return &Client{
		baseURL: base,
		httpClient: &http.Client{
			Timeout:   10 * time.Second,
			Transport: transport,
		},
		userAgent: "NexusAI-Test/1.0",
	}
}

// IsConfigured reports whether a base URL has been set.
func (c *Client) IsConfigured() bool {
	return c.baseURL != nil && c.baseURL.Host != ""
}

// Search executes a search and returns the response.
// It transparently falls back to HTML parsing when the instance does not
// support the JSON format endpoint.
func (c *Client) Search(input SearchInput) (*SearchResponse, error) {
	if !c.IsConfigured() {
		return nil, fmt.Errorf("SearXNG base URL not configured (set SEARXNG_URL or SEARXNG_BASE_URL)")
	}

	// Operator cap: SEARXNG_MAX_RESULTS limits what callers can request.
	operatorMax := parseIntEnv("SEARXNG_MAX_RESULTS", 0)
	effectiveMax := input.NumResults
	if operatorMax > 0 {
		if effectiveMax == 0 || effectiveMax > operatorMax {
			effectiveMax = operatorMax
		}
	}

	maxResultChars := parseIntEnv("SEARXNG_MAX_RESULT_CHARS", 0)

	// --- JSON attempt ---
	jsonURL := c.buildSearchURL(input, true)
	resp, err := c.doRequest(jsonURL)
	if err != nil {
		return nil, err
	}

	var data *SearchResponse

	if !resp.ok {
		if c.shouldFallbackForStatus(resp.status) {
			data, err = c.htmlFallback(input)
			if err != nil {
				return nil, err
			}
		} else {
			return nil, fmt.Errorf("SearXNG returned status %d — is the instance running at %s?", resp.status, c.baseURL)
		}
	} else {
		data, err = c.parseJSONOrHTMLFallback(resp, input)
		if err != nil {
			return nil, err
		}
	}

	if data.Results == nil {
		return nil, fmt.Errorf("SearXNG returned an unexpected response structure (no results field)")
	}

	// --- Apply min_score filter ---
	filtered := data.Results
	if input.MinScore > 0 {
		var keep []WebResult
		for _, r := range filtered {
			if r.Score == 0 || r.Score >= input.MinScore {
				keep = append(keep, r)
			}
		}
		filtered = keep
	}

	// --- Cap results ---
	if effectiveMax > 0 && len(filtered) > effectiveMax {
		filtered = filtered[:effectiveMax]
	}

	// --- Truncate long content ---
	if maxResultChars > 0 {
		for i := range filtered {
			if len(filtered[i].Content) > maxResultChars {
				filtered[i].Content = filtered[i].Content[:maxResultChars] + "…"
			}
		}
	}

	data.Results = filtered
	return data, nil
}

// FormatText formats a SearchResponse into human-readable text,
// matching the text output path of performWebSearch in search.ts.
func FormatText(data *SearchResponse) string {
	var b strings.Builder

	// Leading metadata (answers, corrections, suggestions, infoboxes)
	meta := formatMetadata(data)
	if data.SourceFormat == "html" {
		if meta != "" {
			b.WriteString(meta)
			b.WriteString("\n\n")
		}
		b.WriteString("Note: Results parsed from SearXNG HTML fallback; metadata is limited.\n\n---\n\n")
	} else if meta != "" {
		b.WriteString(meta)
		b.WriteString("\n\n---\n\n")
	}

	if len(data.Results) == 0 {
		b.WriteString(fmt.Sprintf("No results found for query: %q", data.Query))
		return b.String()
	}

	for i, r := range data.Results {
		if i > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString("Title: ")
		b.WriteString(r.Title)
		b.WriteString("\nDescription: ")
		b.WriteString(r.Content)
		b.WriteString("\nURL: ")
		b.WriteString(r.URL)
		if r.Score > 0 {
			b.WriteString(fmt.Sprintf("\nRelevance Score: %.3f", r.Score))
		}
	}

	return b.String()
}

// --- internal helpers ---

type rawResponse struct {
	status int
	body   []byte
	ok     bool
}

func (c *Client) doRequest(u string) (rawResponse, error) {
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return rawResponse{}, fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("User-Agent", c.userAgent)
	if c.auth != "" {
		req.Header.Set("Authorization", "Basic "+c.auth)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return rawResponse{}, fmt.Errorf("network error reaching SearXNG at %s: %w", c.baseURL, err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
	return rawResponse{
		status: resp.StatusCode,
		body:   body,
		ok:     resp.StatusCode >= 200 && resp.StatusCode < 300,
	}, nil
}

func (c *Client) parseJSONOrHTMLFallback(resp rawResponse, input SearchInput) (*SearchResponse, error) {
	var data SearchResponse
	if err := json.Unmarshal(resp.body, &data); err != nil {
		// JSON decode failed (body is HTML or an error page) — always try HTML fallback.
		return c.htmlFallback(input)
	}
	data.SourceFormat = "json"
	return &data, nil
}

func (c *Client) htmlFallback(input SearchInput) (*SearchResponse, error) {
	htmlURL := c.buildSearchURL(input, false)
	resp, err := c.doRequest(htmlURL)
	if err != nil {
		return nil, err
	}
	if !resp.ok {
		return nil, fmt.Errorf("SearXNG HTML fallback returned status %d", resp.status)
	}
	return parseHTMLSearchResults(string(resp.body), input.Query)
}

// buildSearchURL constructs the /search URL with all parameters.
// withJSON controls whether format=json is included.
func (c *Client) buildSearchURL(input SearchInput, withJSON bool) string {
	ref := *c.baseURL
	ref.Path = strings.TrimRight(ref.Path, "/") + "/search"

	q := ref.Query()
	q.Set("q", input.Query)
	if withJSON {
		q.Set("format", "json")
	}

	pageno := input.PageNo
	if pageno < 1 {
		pageno = 1
	}
	q.Set("pageno", strconv.Itoa(pageno))

	lang := input.Language
	if lang == "" {
		lang = firstNonEmpty(os.Getenv("SEARXNG_DEFAULT_LANGUAGE"), "all")
	}
	if lang != "" && lang != "all" {
		q.Set("language", lang)
	}

	if input.TimeRange != "" && isValidTimeRange(input.TimeRange) {
		q.Set("time_range", input.TimeRange)
	}

	if isValidSafesearch(input.Safesearch) {
		q.Set("safesearch", strconv.Itoa(input.Safesearch))
	}

	if input.Categories != "" {
		q.Set("categories", input.Categories)
	}

	if input.Engines != "" {
		q.Set("engines", input.Engines)
	}

	ref.RawQuery = q.Encode()
	return ref.String()
}

func (c *Client) shouldFallbackForStatus(status int) bool {
	return status == 403 || status == 404
}

// buildTransport returns an http.Transport that respects HTTP_PROXY / HTTPS_PROXY
// environment variables. net/http's DefaultTransport already does this via
// ProxyFromEnvironment, so we clone it and check SEARXNG_HTTP_PROXY first.
func buildTransport() http.RoundTripper {
	proxyURL := firstNonEmpty(
		os.Getenv("SEARXNG_HTTP_PROXY"),
		os.Getenv("SEARXNG_HTTPS_PROXY"),
	)
	if proxyURL != "" {
		if u, err := url.Parse(proxyURL); err == nil {
			t := http.DefaultTransport.(*http.Transport).Clone()
			t.Proxy = http.ProxyURL(u)
			return t
		}
	}
	// Fall back to system proxy (reads HTTP_PROXY / HTTPS_PROXY / NO_PROXY).
	return http.DefaultTransport
}

func buildBasicAuth() string {
	user := firstNonEmpty(os.Getenv("SEARXNG_AUTH_USERNAME"), os.Getenv("AUTH_USERNAME"))
	pass := firstNonEmpty(os.Getenv("SEARXNG_AUTH_PASSWORD"), os.Getenv("AUTH_PASSWORD"))
	if user == "" || pass == "" {
		return ""
	}
	return base64.StdEncoding.EncodeToString([]byte(user + ":" + pass))
}

func formatMetadata(data *SearchResponse) string {
	var sections []string

	for _, ans := range data.Answers {
		sections = append(sections, "Direct answer: "+ans)
	}
	for _, corr := range data.Corrections {
		sections = append(sections, fmt.Sprintf("Spelling correction: did you mean %q?", corr))
	}
	if len(data.Suggestions) > 0 {
		sections = append(sections, "Suggestions: "+strings.Join(data.Suggestions, ", "))
	}
	for _, box := range data.Infoboxes {
		var lines []string
		lines = append(lines, "Infobox: "+box.Infobox)
		if box.Content != "" {
			lines = append(lines, box.Content)
		}
		for _, u := range box.URLs {
			lines = append(lines, u.Title+": "+u.URL)
		}
		sections = append(sections, strings.Join(lines, "\n"))
	}

	return strings.Join(sections, "\n\n")
}

func isValidTimeRange(s string) bool {
	switch s {
	case "day", "week", "month", "year":
		return true
	}
	return false
}

func isValidSafesearch(v int) bool {
	return v == 0 || v == 1 || v == 2
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func truncatePreview(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "…"
}
