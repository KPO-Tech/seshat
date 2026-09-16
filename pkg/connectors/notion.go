// Notion connector: Notion workspaces (read-only, pages only) via Notion's
// REST API. No official Go SDK is vendored in this module (matching
// sharepoint_graph.go/confluence_atlassian.go's own reasoning) - a small
// hand-rolled client sending the required Notion-Version header on every
// call.
//
// No native delta/changes API exists, and no per-page sharing/ACL endpoint
// either - unlike Drive, an integration only ever sees what it's been
// explicitly shared with, and there's no way to enumerate individual
// viewers of a given page, so SyncItem.AccessControl stays nil here, the
// same accepted precedent as S3/Azure Blob/Confluence.
//
// Unlike Confluence, there's no per-account "cloud ID" (Notion has one
// fixed base URL) and no refresh-token rotation to track (Notion's OAuth
// tokens aren't rotated per call) - structurally closer to gdrive.go/
// sharepoint.go there.
package connectors

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/oauth2"
)

// notionAPIVersion is the Notion-Version header every call must send - kept
// in sync manually with the same literal used by fetchNotionIdentity
// (seshat-server/internal/server/connectors/notion.go, a different Go
// module so it can't share this constant directly).
const notionAPIVersion = "2022-06-28"

// notionBaseURL is a var, not a const, so tests can point it at a local
// fake server instead of the real Notion API - see
// SetNotionBaseURLForTesting.
var notionBaseURL = "https://api.notion.com/v1"

// SetNotionBaseURLForTesting points this package's Notion API base URL at a
// test server, returning a function that restores the real value. Never
// call this outside a test.
func SetNotionBaseURLForTesting(baseURL string) (restore func()) {
	original := notionBaseURL
	notionBaseURL = baseURL
	return func() { notionBaseURL = original }
}

// maxNotionResponseBytes bounds any single API response this connector
// reads - same defensive cap used throughout this package.
const maxNotionResponseBytes = 20 * 1024 * 1024 // 20MB

// notionDefaultRateLimitBackoff is used when a 429 response carries no
// Retry-After header - matches the same conservative default used for
// Microsoft Graph/Confluence/Drive (this package's other rate-limit sites).
const notionDefaultRateLimitBackoff = 30 * time.Second

// maxNotionBlockDepth/maxNotionBlocks bound fetchPageText's recursive block
// walk, so one pathologically large/deeply-nested page can't runaway or
// produce an unbounded sync - hitting either cap stops the walk and returns
// whatever text was gathered so far, not an error.
const (
	maxNotionBlockDepth = 6
	maxNotionBlocks     = 2000
)

// NotionConnector implements Notion Discover/Sync.
type NotionConnector struct {
	oauthConfig *oauth2.Config
	extractText TextExtractorFunc
}

func NewNotionConnector(oauthConfig *oauth2.Config) *NotionConnector {
	return &NotionConnector{oauthConfig: oauthConfig}
}

func (c *NotionConnector) WithTextExtractor(fn TextExtractorFunc) *NotionConnector {
	c.extractText = fn
	return c
}

func (c *NotionConnector) client(ctx context.Context, secret Secret) (*notionClient, error) {
	if c.oauthConfig == nil {
		return nil, fmt.Errorf("notion oauth is not configured")
	}
	token := &oauth2.Token{
		AccessToken:  secret.AccessToken,
		RefreshToken: secret.RefreshToken,
		Expiry:       secret.ExpiresAt,
	}
	return &notionClient{http: c.oauthConfig.Client(ctx, token)}, nil
}

// Discover lists every page the connected integration can see, without
// downloading content - a lightweight preview, mirroring
// ConfluenceConnector.Discover.
func (c *NotionConnector) Discover(ctx context.Context, secret Secret) ([]ResourceRef, error) {
	cl, err := c.client(ctx, secret)
	if err != nil {
		return nil, err
	}
	var refs []ResourceRef
	err = cl.searchPages(ctx, false, func(page notionPage) bool {
		refs = append(refs, ResourceRef{ID: page.ID, Name: notionPageTitle(page), Kind: "file", Modified: page.LastEditedTime})
		return true
	})
	if err != nil {
		return nil, err
	}
	return refs, nil
}

// Sync bootstraps (cursor == "") or continues (cursor is a TimeCursor,
// types.go) - no change-feed API exists, so incrementality here means
// "search sorted newest-edited-first, stop once past the cursor boundary,"
// the same shape confluence.go/s3.go use for the same reason. The returned
// cursor is captured before the search starts, so a page edited mid-sync
// isn't missed by the next incremental run.
func (c *NotionConnector) Sync(ctx context.Context, secret Secret, cursor string) ([]SyncItem, string, error) {
	cl, err := c.client(ctx, secret)
	if err != nil {
		return nil, "", err
	}
	since, err := DecodeTimeCursor(cursor)
	if err != nil {
		return nil, "", err
	}
	nextCursor := NewTimeCursorNow()

	var items []SyncItem
	err = cl.searchPages(ctx, true, func(page notionPage) bool {
		if !since.Since.IsZero() && !page.LastEditedTime.After(since.Since) {
			// Sorted newest-edited-first - everything from here on is also
			// at or before the cursor, so the rest of the search can be
			// skipped rather than walked and filtered out one by one.
			return false
		}
		if item, ok := c.syncOnePage(ctx, cl, page); ok {
			items = append(items, item)
		}
		return true
	})
	if err != nil {
		return nil, "", err
	}
	return items, nextCursor.Encode(), nil
}

// syncOnePage flattens one page's block tree into plain text. ok=false is a
// normal skip (fetch failure, empty extracted text), not an error.
func (c *NotionConnector) syncOnePage(ctx context.Context, cl *notionClient, page notionPage) (SyncItem, bool) {
	title := notionPageTitle(page)
	body, err := fetchPageText(ctx, cl, page.ID)
	if err != nil || strings.TrimSpace(body) == "" {
		return SyncItem{}, false
	}
	text := body
	if c.extractText != nil {
		text = c.extractText(ctx, []byte(body), "text/plain", title)
		if strings.TrimSpace(text) == "" {
			return SyncItem{}, false
		}
	}
	return SyncItem{
		ResourceRef: ResourceRef{ID: page.ID, Name: title, Kind: "file", Modified: page.LastEditedTime},
		Data:        []byte(body),
		ContentType: "text/plain",
		Text:        text,
		// No per-page ACL: Notion's API exposes no page-sharing/permissions
		// endpoint - see the package doc comment above.
		AccessControl: nil,
	}, true
}

// notionPageTitle scans a page's properties for the one entry whose type
// is "title" - its map key varies (a database row's title property can be
// named "Name", "Title", anything the database schema defines), only the
// type is stable.
func notionPageTitle(page notionPage) string {
	for _, prop := range page.Properties {
		if prop.Type != "title" {
			continue
		}
		return joinRichText(prop.Title)
	}
	return ""
}

// fetchPageText walks a page's block tree (GET /v1/blocks/{id}/children,
// paginated) and flattens every text-bearing block's rich_text into plain
// text (one block per line), recursing into any block with has_children.
func fetchPageText(ctx context.Context, cl *notionClient, pageID string) (string, error) {
	var b strings.Builder
	count := 0
	var walk func(blockID string, depth int) error
	walk = func(blockID string, depth int) error {
		if depth > maxNotionBlockDepth {
			return nil
		}
		startCursor := ""
		for {
			if count >= maxNotionBlocks {
				return nil
			}
			page, err := cl.listBlockChildren(ctx, blockID, startCursor)
			if err != nil {
				return err
			}
			for _, block := range page.Results {
				if count >= maxNotionBlocks {
					return nil
				}
				count++
				if text := notionBlockText(block); text != "" {
					b.WriteString(text)
					b.WriteString("\n")
				}
				if block.HasChildren {
					if err := walk(block.ID, depth+1); err != nil {
						return err
					}
				}
			}
			if !page.HasMore || page.NextCursor == "" {
				return nil
			}
			startCursor = page.NextCursor
		}
	}
	if err := walk(pageID, 0); err != nil {
		return "", err
	}
	return b.String(), nil
}

// notionBlockText extracts the flattened rich_text for the text-bearing
// block types this connector understands - structural/media blocks
// (images, files, embeds, dividers, table rows whose cell text isn't
// exposed the same way) are skipped rather than guessed at.
func notionBlockText(block notionBlock) string {
	var container *notionRichTextContainer
	switch block.Type {
	case "paragraph":
		container = block.Paragraph
	case "heading_1":
		container = block.Heading1
	case "heading_2":
		container = block.Heading2
	case "heading_3":
		container = block.Heading3
	case "bulleted_list_item":
		container = block.BulletedListItem
	case "numbered_list_item":
		container = block.NumberedListItem
	case "to_do":
		container = block.ToDo
	case "quote":
		container = block.Quote
	case "callout":
		container = block.Callout
	case "code":
		container = block.Code
	default:
		return ""
	}
	if container == nil {
		return ""
	}
	return joinRichText(container.RichText)
}

func joinRichText(rt []notionRichText) string {
	var b strings.Builder
	for _, r := range rt {
		b.WriteString(r.PlainText)
	}
	return b.String()
}

// notionPage is a search result / block-children's-parent page object.
type notionPage struct {
	Object         string                        `json:"object"`
	ID             string                        `json:"id"`
	LastEditedTime time.Time                     `json:"last_edited_time"`
	Properties     map[string]notionPageProperty `json:"properties"`
}

type notionPageProperty struct {
	Type  string           `json:"type"`
	Title []notionRichText `json:"title"`
}

type notionRichText struct {
	PlainText string `json:"plain_text"`
}

type notionRichTextContainer struct {
	RichText []notionRichText `json:"rich_text"`
}

// notionBlock covers only the text-bearing block types notionBlockText
// reads - every other type (image, divider, table, ...) simply decodes
// with all of these fields absent/nil, which is fine: notionBlockText's
// switch returns "" for any Type it doesn't recognize.
type notionBlock struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	HasChildren bool   `json:"has_children"`

	Paragraph        *notionRichTextContainer `json:"paragraph,omitempty"`
	Heading1         *notionRichTextContainer `json:"heading_1,omitempty"`
	Heading2         *notionRichTextContainer `json:"heading_2,omitempty"`
	Heading3         *notionRichTextContainer `json:"heading_3,omitempty"`
	BulletedListItem *notionRichTextContainer `json:"bulleted_list_item,omitempty"`
	NumberedListItem *notionRichTextContainer `json:"numbered_list_item,omitempty"`
	ToDo             *notionRichTextContainer `json:"to_do,omitempty"`
	Quote            *notionRichTextContainer `json:"quote,omitempty"`
	Callout          *notionRichTextContainer `json:"callout,omitempty"`
	Code             *notionRichTextContainer `json:"code,omitempty"`
}

type notionSearchResponse struct {
	Results    []notionPage `json:"results"`
	HasMore    bool         `json:"has_more"`
	NextCursor string       `json:"next_cursor"`
}

type notionBlockChildrenResponse struct {
	Results    []notionBlock `json:"results"`
	HasMore    bool          `json:"has_more"`
	NextCursor string        `json:"next_cursor"`
}

// notionError carries the HTTP status so a 429 can be surfaced as
// RateLimited (types.go) - every other status reports RetryAfter's ok as
// false.
type notionError struct {
	Status      int
	Body        string
	retryAfter  time.Duration
	isRateLimit bool
}

func (e *notionError) Error() string {
	return fmt.Sprintf("notion api returned %d: %s", e.Status, e.Body)
}

func (e *notionError) RetryAfter() (time.Duration, bool) {
	if !e.isRateLimit {
		return 0, false
	}
	return e.retryAfter, true
}

var _ RateLimited = (*notionError)(nil)

// notionClient is a minimal hand-rolled Notion REST client - no official Go
// SDK is vendored in this module, and this connector only needs three call
// shapes (search, list block children, and their shared error handling).
type notionClient struct {
	http *http.Client
}

func (cl *notionClient) do(ctx context.Context, method, requestURL string, body any, out any) error {
	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode notion request: %w", err)
		}
		reqBody = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, requestURL, reqBody)
	if err != nil {
		return err
	}
	req.Header.Set("Notion-Version", notionAPIVersion)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := cl.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		nerr := &notionError{Status: resp.StatusCode, Body: string(respBody)}
		if resp.StatusCode == http.StatusTooManyRequests {
			nerr.isRateLimit = true
			if delay, ok := parseRetryAfter(resp.Header); ok {
				nerr.retryAfter = delay
			} else {
				nerr.retryAfter = notionDefaultRateLimitBackoff
			}
		}
		return nerr
	}
	if out != nil {
		if err := json.NewDecoder(io.LimitReader(resp.Body, maxNotionResponseBytes)).Decode(out); err != nil {
			return fmt.Errorf("decode notion response: %w", err)
		}
	}
	return nil
}

// searchPages walks every page of POST /v1/search (filtered to
// object=page - databases themselves are schemas/containers with no
// readable content; each row inside one is already its own page object),
// invoking visit for each result - visit returns false to stop early (Sync
// uses this once it passes its cursor boundary), true to keep paginating.
func (cl *notionClient) searchPages(ctx context.Context, sortByLastEdited bool, visit func(notionPage) bool) error {
	startCursor := ""
	for {
		reqBody := map[string]any{
			"filter":    map[string]any{"property": "object", "value": "page"},
			"page_size": 100,
		}
		if sortByLastEdited {
			reqBody["sort"] = map[string]any{"direction": "descending", "timestamp": "last_edited_time"}
		}
		if startCursor != "" {
			reqBody["start_cursor"] = startCursor
		}
		var page notionSearchResponse
		if err := cl.do(ctx, http.MethodPost, notionBaseURL+"/search", reqBody, &page); err != nil {
			return fmt.Errorf("search notion pages: %w", err)
		}
		for _, item := range page.Results {
			if item.Object != "page" {
				continue
			}
			if !visit(item) {
				return nil
			}
		}
		if !page.HasMore || page.NextCursor == "" {
			return nil
		}
		startCursor = page.NextCursor
	}
}

func (cl *notionClient) listBlockChildren(ctx context.Context, blockID, startCursor string) (notionBlockChildrenResponse, error) {
	requestURL := notionBaseURL + "/blocks/" + url.PathEscape(blockID) + "/children?page_size=100"
	if startCursor != "" {
		requestURL += "&start_cursor=" + url.QueryEscape(startCursor)
	}
	var page notionBlockChildrenResponse
	if err := cl.do(ctx, http.MethodGet, requestURL, nil, &page); err != nil {
		return notionBlockChildrenResponse{}, fmt.Errorf("list notion block children %s: %w", blockID, err)
	}
	return page, nil
}
