package fetch

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	webcore "github.com/EngineerProjects/nexus-engine/internal/web"
)

// HTTPClient defines the HTTP client interface
type HTTPClient interface {
	// Do executes an HTTP request
	Do(req *http.Request) (*http.Response, error)
}

// DefaultHTTPClient creates a default HTTP client
func DefaultHTTPClient(resolver webcore.HostResolver) *http.Client {
	dialer := &net.Dialer{Timeout: 30 * time.Second}
	return &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: func(ctx context.Context, network string, address string) (net.Conn, error) {
				if err := webcore.RejectLocalDialTarget(ctx, address, resolver); err != nil {
					return nil, err
				}
				return dialer.DialContext(ctx, network, address)
			},
		},
	}
}

// fetchViaHTTP keeps the fast path lean and cache-backed for the common case of static or server-rendered pages.
func (s *Service) fetchViaHTTP(ctx context.Context, urlStr string) (FetchedContent, error) {
	if entry, ok := s.cache.Get(urlStr); ok {
		return FetchedContent{
			Content:            entry.Content,
			Bytes:              entry.Bytes,
			Code:               entry.Code,
			CodeText:           entry.CodeText,
			ContentType:        entry.ContentType,
			FinalURL:           entry.FinalURL,
			PersistedPath:      entry.PersistedPath,
			PersistedSize:      entry.PersistedSize,
			BrowserRecommended: entry.BrowserRecommended,
		}, nil
	}

	timeout := s.config.Timeout
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	requestCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	response, finalURL, redirect, err := s.doRequest(requestCtx, urlStr)
	if err != nil {
		return FetchedContent{}, err
	}
	if redirect != nil {
		return FetchedContent{
			FinalURL: finalURL,
			Redirect: redirect,
		}, nil
	}

	body, err := ReadBody(response)
	if err != nil {
		return FetchedContent{}, fmt.Errorf("read body: %w", err)
	}
	originalBytes := len(body)

	maxLen := s.config.MaxContentLength
	if maxLen <= 0 {
		maxLen = 10 * 1024 * 1024
	}
	truncated := false
	if len(body) > maxLen {
		body = body[:maxLen]
		truncated = true
	}

	contentType := response.Header.Get("Content-Type")
	browserRecommended := false
	content := string(body)
	persistedPath := ""
	persistedSize := 0
	if strings.Contains(strings.ToLower(contentType), "text/html") {
		browserRecommended = shouldRecommendBrowser(string(body))
		content = HTMLToMarkdown(content, finalURL)
	} else if !isTextualContentType(contentType) {
		persistedPath, persistedSize, err = s.persistArtifact(requestCtx, finalURL, contentType, body)
		if err != nil {
			return FetchedContent{}, err
		}
		content = fmt.Sprintf("Fetched binary content (%s, %d bytes).", strings.TrimSpace(contentType), originalBytes)
		if persistedPath != "" {
			content += fmt.Sprintf(" Stored at %s.", persistedPath)
		}
	}
	if truncated {
		content += "\n\n[Content truncated due to length...]"
	}

	fetched := FetchedContent{
		Content:            content,
		Bytes:              originalBytes,
		Code:               response.StatusCode,
		CodeText:           response.Status,
		ContentType:        contentType,
		FinalURL:           finalURL,
		PersistedPath:      persistedPath,
		PersistedSize:      persistedSize,
		BrowserRecommended: browserRecommended,
	}

	s.cache.Set(urlStr, CacheEntry{
		Content:            fetched.Content,
		Bytes:              fetched.Bytes,
		Code:               fetched.Code,
		CodeText:           fetched.CodeText,
		ContentType:        fetched.ContentType,
		FinalURL:           fetched.FinalURL,
		PersistedPath:      fetched.PersistedPath,
		PersistedSize:      fetched.PersistedSize,
		BrowserRecommended: fetched.BrowserRecommended,
	})

	return fetched, nil
}

func (s *Service) doRequest(ctx context.Context, urlStr string) (*http.Response, string, *RedirectInfo, error) {
	currentURL := urlStr
	maxRedirects := s.config.MaxRedirects
	if maxRedirects <= 0 {
		maxRedirects = 10
	}

	for redirects := 0; ; redirects++ {
		parsedCurrent, parseErr := url.Parse(currentURL)
		if parseErr != nil {
			return nil, currentURL, nil, fmt.Errorf("parse request URL: %w", parseErr)
		}
		if err := webcore.ResolveAndRejectLocalNetworkTarget(ctx, parsedCurrent, s.resolver); err != nil {
			return nil, currentURL, nil, err
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, currentURL, nil)
		if err != nil {
			return nil, currentURL, nil, fmt.Errorf("create request: %w", err)
		}
		req.Header.Set("User-Agent", "NexusAI/1.0")
		req.Header.Set("Accept", "text/markdown, text/html, text/plain, */*")

		resp, err := s.httpClient.Do(req)
		if err != nil {
			if ctx.Err() == context.DeadlineExceeded {
				return nil, currentURL, nil, ErrTimeout
			}
			if ctx.Err() == context.Canceled {
				return nil, currentURL, nil, ErrAborted
			}
			return nil, currentURL, nil, err
		}

		if resp.StatusCode < 300 || resp.StatusCode > 399 {
			return resp, currentURL, nil, nil
		}

		location := resp.Header.Get("Location")
		if location == "" {
			resp.Body.Close()
			return nil, currentURL, nil, fmt.Errorf("redirect missing Location header")
		}
		redirectURL := resolveURL(currentURL, location)
		redirectParsed, redirectParseErr := url.Parse(redirectURL)
		if redirectParseErr == nil {
			if err := webcore.ResolveAndRejectLocalNetworkTarget(ctx, redirectParsed, s.resolver); err != nil {
				resp.Body.Close()
				return nil, currentURL, nil, err
			}
		}
		originalParsed, originalErr := url.Parse(currentURL)
		redirectParsed, redirectErr := url.Parse(redirectURL)
		if originalErr == nil && redirectErr == nil && !sameRedirectHost(originalParsed, redirectParsed) {
			resp.Body.Close()
			return nil, currentURL, &RedirectInfo{
				Type:        "redirect",
				OriginalURL: currentURL,
				RedirectURL: redirectURL,
				StatusCode:  resp.StatusCode,
			}, nil
		}
		resp.Body.Close()
		if redirects >= maxRedirects {
			return nil, currentURL, nil, ErrTooManyRedirects
		}
		currentURL = redirectURL
	}
}

// FetchWithRedirect performs HTTP request with custom redirect handling
func FetchWithRedirect(
	client HTTPClient,
	method, url string,
	headers map[string]string,
	redirectChecker func(original, redirect string) bool,
	maxRedirects int,
) (resp *http.Response, err error) {
	currentURL := url
	redirectCount := 0

	for {
		req, err := http.NewRequest(method, currentURL, nil)
		if err != nil {
			return nil, err
		}

		for k, v := range headers {
			req.Header.Set(k, v)
		}
		req.Header.Set("User-Agent", "NexusAI/1.0")

		resp, err = client.Do(req)
		if err != nil {
			return nil, err
		}

		// Check for redirect
		if resp.StatusCode >= 300 && resp.StatusCode <= 399 {
			location := resp.Header.Get("Location")
			if location == "" {
				resp.Body.Close()
				return nil, Err("redirect missing Location header")
			}

			// Resolve relative redirect
			redirectURL := resolveURL(currentURL, location)

			if redirectChecker != nil && !redirectChecker(currentURL, redirectURL) {
				resp.Body.Close()
				return nil, nil // Signal redirect needed
			}

			resp.Body.Close()
			redirectCount++

			if redirectCount > maxRedirects {
				return nil, ErrTooManyRedirects
			}

			currentURL = redirectURL
			continue
		}

		return resp, nil
	}
}

func resolveURL(base, location string) string {
	parsed, _ := url.Parse(location)
	if parsed != nil && parsed.IsAbs() {
		return location
	}
	baseURL, _ := url.Parse(base)
	if baseURL != nil && parsed != nil {
		return baseURL.ResolveReference(parsed).String()
	}
	return location
}

func sameRedirectHost(original, redirect *url.URL) bool {
	stripWww := func(hostname string) string {
		return strings.TrimPrefix(hostname, "www.")
	}
	return stripWww(original.Hostname()) == stripWww(redirect.Hostname())
}

func shouldRecommendBrowser(rawHTML string) bool {
	lower := strings.ToLower(rawHTML)
	scriptCount := strings.Count(lower, "<script")

	if strings.Contains(lower, "enable javascript") ||
		strings.Contains(lower, "javascript is required") ||
		strings.Contains(lower, "please enable javascript") ||
		strings.Contains(lower, "<meta http-equiv=\"refresh\"") {
		return true
	}

	if scriptCount >= 8 {
		for _, marker := range []string{
			"id=\"__next\"",
			"id=\"root\"",
			"id=\"app\"",
			"data-reactroot",
			"window.__next_data__",
			"window.__apollo_state__",
			"__nuxt",
			"ng-version",
		} {
			if strings.Contains(lower, marker) {
				return true
			}
		}
	}

	return false
}

// ReadBody reads and closes the response body
func ReadBody(resp *http.Response) ([]byte, error) {
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}
