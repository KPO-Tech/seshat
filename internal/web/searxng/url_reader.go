package searxng

// URLReader fetches a URL and returns its content converted to readable text.
// Mirrors fetchAndConvertToMarkdown from url-reader.ts.
//
// Key behaviours ported from the MCP:
//   - In-process TTL cache (DefaultURLCache)
//   - Manual redirect following with loop detection
//   - Basic auth and User-Agent headers
//   - Proxy support (via the shared http.Client from Client)
//   - go-readability for article extraction (replaces node-html-markdown)
//   - Section / paragraph / character-level pagination options

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	readability "github.com/go-shiori/go-readability"
)

const (
	defaultURLReadTimeout  = 10 * time.Second
	defaultMaxContentBytes = 5 * 1024 * 1024 // 5 MB, matching DEFAULT_MAX_CONTENT_LENGTH_BYTES
	maxRedirects           = 5
)

// PaginationOptions controls how extracted content is sliced.
// Mirrors the PaginationOptions interface in url-reader.ts.
type PaginationOptions struct {
	// StartChar is the character offset to start from (0-based).
	StartChar int
	// MaxLength caps the number of characters returned. 0 means no cap.
	MaxLength int
	// Section returns only the content under the heading matching this string.
	Section string
	// ParagraphRange selects specific paragraphs: "3", "1-5", "10-".
	ParagraphRange string
	// ReadHeadings returns only the heading lines instead of full content.
	ReadHeadings bool
}

// URLReader fetches and converts URLs to readable text.
type URLReader struct {
	client    *http.Client
	auth      string
	userAgent string
	cache     *URLCache
}

// NewURLReader creates a URLReader sharing the same auth / proxy config as the searxng Client.
func NewURLReader() *URLReader {
	timeoutMs := parseIntEnv("SEARXNG_TIMEOUT_MS", int(defaultURLReadTimeout.Milliseconds()))
	return &URLReader{
		client: &http.Client{
			Timeout:   time.Duration(timeoutMs) * time.Millisecond,
			Transport: buildTransport(),
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse // we handle redirects manually
			},
		},
		auth:      buildBasicAuth(),
		userAgent: firstNonEmpty(os.Getenv("URL_READER_USER_AGENT"), os.Getenv("USER_AGENT"), "NexusAI-URLReader/2.0"),
		cache:     DefaultURLCache,
	}
}

// FetchMarkdown fetches the URL and returns its content as readable text.
// The content is cached (DefaultURLCache) for subsequent calls.
func (r *URLReader) FetchMarkdown(ctx context.Context, rawURL string, opts PaginationOptions) (string, error) {
	// Cache hit
	if md, ok := r.cache.Get(rawURL); ok {
		return applyPagination(md, opts), nil
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("invalid URL %q: %w", rawURL, err)
	}

	maxBytes := parseIntEnv("URL_READ_MAX_CONTENT_LENGTH_BYTES", defaultMaxContentBytes)

	// Follow redirects manually (mirrors the manual redirect loop in url-reader.ts).
	current := parsed
	var resp *http.Response
	for i := 0; i <= maxRedirects; i++ {
		resp, err = r.doRequest(ctx, current.String())
		if err != nil {
			return "", err
		}

		if !isRedirect(resp.StatusCode) {
			break
		}
		loc := resp.Header.Get("Location")
		resp.Body.Close()
		if loc == "" {
			break
		}
		if i == maxRedirects {
			return "", fmt.Errorf("too many redirects fetching %s", rawURL)
		}
		next, parseErr := url.Parse(loc)
		if parseErr != nil {
			return "", fmt.Errorf("invalid redirect location %q: %w", loc, parseErr)
		}
		current = current.ResolveReference(next)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("server returned %d for %s", resp.StatusCode, rawURL)
	}

	htmlBytes, readErr := io.ReadAll(io.LimitReader(resp.Body, int64(maxBytes)))
	if readErr != nil {
		return "", fmt.Errorf("reading response body: %w", readErr)
	}
	if len(htmlBytes) == 0 {
		return "", fmt.Errorf("server returned empty content for %s", rawURL)
	}

	// Extract readable article content using go-readability.
	article, parseErr := readability.FromReader(strings.NewReader(string(htmlBytes)), current)
	if parseErr != nil {
		return "", fmt.Errorf("extracting readable content from %s: %w", rawURL, parseErr)
	}

	// Use TextContent for clean plain text (readability already strips ads/nav).
	md := strings.TrimSpace(article.TextContent)
	if md == "" {
		// Fallback: return raw text content if readability found nothing.
		md = strings.TrimSpace(string(htmlBytes))
	}

	r.cache.Set(rawURL, string(htmlBytes), md)
	return applyPagination(md, opts), nil
}

func (r *URLReader) doRequest(ctx context.Context, rawURL string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("building request for %s: %w", rawURL, err)
	}
	req.Header.Set("User-Agent", r.userAgent)
	if r.auth != "" {
		req.Header.Set("Authorization", "Basic "+r.auth)
	}
	resp, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("network error fetching %s: %w", rawURL, err)
	}
	return resp, nil
}

func isRedirect(status int) bool {
	switch status {
	case 301, 302, 303, 307, 308:
		return true
	}
	return false
}

// applyPagination slices the content according to PaginationOptions.
// Mirrors applyPaginationOptions from url-reader.ts.
func applyPagination(content string, opts PaginationOptions) string {
	if opts.ReadHeadings {
		return extractHeadings(content)
	}

	result := content

	if opts.Section != "" {
		extracted := extractSection(result, opts.Section)
		if extracted == "" {
			return fmt.Sprintf("Section %q not found in the content.", opts.Section)
		}
		result = extracted
	}

	if opts.ParagraphRange != "" {
		extracted := extractParagraphRange(result, opts.ParagraphRange)
		if extracted == "" {
			return fmt.Sprintf("Paragraph range %q is invalid or out of bounds.", opts.ParagraphRange)
		}
		result = extracted
	}

	if opts.StartChar > 0 || opts.MaxLength > 0 {
		result = applyCharPagination(result, opts.StartChar, opts.MaxLength)
	}

	return result
}

func applyCharPagination(s string, start, maxLen int) string {
	if start >= len(s) {
		return ""
	}
	if start > 0 {
		s = s[start:]
	}
	if maxLen > 0 && len(s) > maxLen {
		s = s[:maxLen]
	}
	return s
}

func extractHeadings(content string) string {
	var headings []string
	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(line, "# ") ||
			strings.HasPrefix(line, "## ") ||
			strings.HasPrefix(line, "### ") ||
			strings.HasPrefix(line, "#### ") ||
			strings.HasPrefix(line, "##### ") ||
			strings.HasPrefix(line, "###### ") {
			headings = append(headings, line)
		}
	}
	if len(headings) == 0 {
		return "No headings found in the content."
	}
	return strings.Join(headings, "\n")
}

func extractSection(content, heading string) string {
	lines := strings.Split(content, "\n")
	lower := strings.ToLower(heading)

	startIdx := -1
	level := 0
	for i, line := range lines {
		if !strings.HasPrefix(line, "#") {
			continue
		}
		if strings.Contains(strings.ToLower(line), lower) {
			// Count leading #
			for level = 0; level < len(line) && line[level] == '#'; level++ {
			}
			startIdx = i
			break
		}
	}
	if startIdx < 0 {
		return ""
	}

	endIdx := len(lines)
	for i := startIdx + 1; i < len(lines); i++ {
		line := lines[i]
		if !strings.HasPrefix(line, "#") {
			continue
		}
		thisLevel := 0
		for thisLevel < len(line) && line[thisLevel] == '#' {
			thisLevel++
		}
		if thisLevel <= level {
			endIdx = i
			break
		}
	}

	return strings.Join(lines[startIdx:endIdx], "\n")
}

func extractParagraphRange(content, rangeStr string) string {
	paragraphs := splitParagraphs(content)

	// Parse "3", "1-5", "10-"
	dashIdx := strings.Index(rangeStr, "-")
	if dashIdx < 0 {
		// Single paragraph
		n, err := parseInt(rangeStr)
		if err != nil || n < 1 || n > len(paragraphs) {
			return ""
		}
		return paragraphs[n-1]
	}

	startStr := rangeStr[:dashIdx]
	endStr := rangeStr[dashIdx+1:]

	start, err := parseInt(startStr)
	if err != nil || start < 1 {
		return ""
	}
	start-- // 0-based

	if endStr == "" {
		// "10-" — to end
		if start >= len(paragraphs) {
			return ""
		}
		return strings.Join(paragraphs[start:], "\n\n")
	}

	end, err := parseInt(endStr)
	if err != nil || end < start+1 {
		return ""
	}
	if end > len(paragraphs) {
		end = len(paragraphs)
	}
	return strings.Join(paragraphs[start:end], "\n\n")
}

func splitParagraphs(content string) []string {
	raw := strings.Split(content, "\n\n")
	var out []string
	for _, p := range raw {
		if strings.TrimSpace(p) != "" {
			out = append(out, p)
		}
	}
	return out
}

func parseInt(s string) (int, error) {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("not an integer: %s", s)
		}
		n = n*10 + int(c-'0')
	}
	return n, nil
}
