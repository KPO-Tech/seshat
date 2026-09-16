package connectors

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// confluenceDefaultRateLimitBackoff is used when a 429 response carries no
// Retry-After header - Atlassian's documented throttling guidance is to
// back off "at least a few seconds"; 30s matches the same conservative
// default used for Microsoft Graph (sharepoint_graph.go).
const confluenceDefaultRateLimitBackoff = 30 * time.Second

// confluenceSearchResponse is GET /wiki/rest/api/search's response shape -
// a minimal hand-rolled subset, no official Go SDK is vendored in this
// module (matching sharepoint_graph.go's own reasoning).
type confluenceSearchResponse struct {
	Results []confluenceSearchResult `json:"results"`
	Links   struct {
		Next string `json:"next"`
	} `json:"_links"`
}

type confluenceSearchResult struct {
	Content struct {
		ID   string `json:"id"`
		Type string `json:"type"`
	} `json:"content"`
	Title        string `json:"title"`
	LastModified string `json:"lastModified"`
}

// confluencePageResponse is GET /wiki/api/v2/pages/{id}?body-format=export_view's
// response shape.
type confluencePageResponse struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Body  struct {
		ExportView struct {
			Value string `json:"value"`
		} `json:"export_view"`
	} `json:"body"`
}

type confluenceAccessibleResource struct {
	ID string `json:"id"`
}

// confluenceClient is a minimal hand-rolled Atlassian REST client, scoped
// to one account's cloud ID - see ConfluenceConnector.client's doc comment
// for why that's resolved fresh per call rather than cached.
type confluenceClient struct {
	http    *http.Client
	baseURL string
}

type confluenceError struct {
	Status      int
	Body        string
	retryAfter  time.Duration
	isRateLimit bool
}

func (e *confluenceError) Error() string {
	return fmt.Sprintf("confluence api returned %d: %s", e.Status, e.Body)
}

// RetryAfter implements RateLimited (types.go) - isRateLimit is only true
// for a 429 response; every other confluenceError reports (0, false).
func (e *confluenceError) RetryAfter() (time.Duration, bool) {
	if !e.isRateLimit {
		return 0, false
	}
	return e.retryAfter, true
}

var _ RateLimited = (*confluenceError)(nil)

func (cl *confluenceClient) get(ctx context.Context, requestURL string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return err
	}
	resp, err := cl.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		cerr := &confluenceError{Status: resp.StatusCode, Body: string(body)}
		if resp.StatusCode == http.StatusTooManyRequests {
			cerr.isRateLimit = true
			if delay, ok := parseRetryAfter(resp.Header); ok {
				cerr.retryAfter = delay
			} else {
				cerr.retryAfter = confluenceDefaultRateLimitBackoff
			}
		}
		return cerr
	}
	if out != nil {
		if err := json.NewDecoder(io.LimitReader(resp.Body, maxConfluenceResponseBytes)).Decode(out); err != nil {
			return fmt.Errorf("decode confluence response: %w", err)
		}
	}
	return nil
}

// resolveConfluenceCloudID finds which Atlassian site this token grants
// access to - every subsequent API call is scoped under
// https://api.atlassian.com/ex/confluence/{cloudID}, unlike Graph's fixed
// base URL. A token granting access to more than one site (rare for a
// single connected account) picks the first - stated, not silently
// resolved by accident.
func resolveConfluenceCloudID(ctx context.Context, httpClient *http.Client) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.atlassian.com/oauth/token/accessible-resources", nil)
	if err != nil {
		return "", err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("list accessible atlassian resources: %d: %s", resp.StatusCode, body)
	}
	var resources []confluenceAccessibleResource
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxConfluenceResponseBytes)).Decode(&resources); err != nil {
		return "", fmt.Errorf("decode accessible resources: %w", err)
	}
	if len(resources) == 0 {
		return "", fmt.Errorf("no accessible atlassian sites for this account")
	}
	return resources[0].ID, nil
}

// searchPages walks every page of a CQL search, invoking visit for each
// result - visit returns false to stop early (Sync uses this once it
// passes its cursor boundary), true to keep paginating.
func (cl *confluenceClient) searchPages(ctx context.Context, cql string, visit func(confluenceSearchResult) bool) error {
	requestURL := cl.baseURL + "/wiki/rest/api/search?limit=25&cql=" + url.QueryEscape(cql)
	for requestURL != "" {
		var page confluenceSearchResponse
		if err := cl.get(ctx, requestURL, &page); err != nil {
			return fmt.Errorf("search confluence pages: %w", err)
		}
		for _, item := range page.Results {
			if !visit(item) {
				return nil
			}
		}
		next := page.Links.Next
		if next == "" {
			return nil
		}
		if strings.HasPrefix(next, "http") {
			requestURL = next
		} else {
			requestURL = cl.baseURL + next
		}
	}
	return nil
}

// getPageContent fetches one page's title and rendered HTML content.
// export_view (not raw storage format) is requested deliberately - see
// ConfluenceConnector's package doc comment.
func (cl *confluenceClient) getPageContent(ctx context.Context, pageID string) (title, html string, err error) {
	requestURL := cl.baseURL + "/wiki/api/v2/pages/" + url.PathEscape(pageID) + "?body-format=export_view"
	var page confluencePageResponse
	if err := cl.get(ctx, requestURL, &page); err != nil {
		return "", "", fmt.Errorf("get confluence page %s: %w", pageID, err)
	}
	return page.Title, page.Body.ExportView.Value, nil
}
