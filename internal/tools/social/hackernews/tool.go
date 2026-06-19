// Package hackernews provides tools for the Hacker News public API.
//
// No authentication required. Two APIs are used:
//   - Firebase REST: https://hacker-news.firebaseio.com/v0/
//     → top/new/best/ask/show/job story IDs, individual items
//   - Algolia Search: https://hn.algolia.com/api/v1/
//     → full-text search with filters
//
// Tools exposed:
//
//	hn_search   — search HN posts and comments (Algolia)
//	hn_stories  — fetch top/new/best/ask/show/job feed (Firebase)
//	hn_item     — fetch a single story or comment thread (Firebase)
package hackernews

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	"github.com/EngineerProjects/nexus-engine/internal/tools/schema"
	"github.com/EngineerProjects/nexus-engine/internal/types"
)

const (
	firebaseBase = "https://hacker-news.firebaseio.com/v0"
	algoliaBase  = "https://hn.algolia.com/api/v1"
	defaultLimit = 10
	maxLimit     = 30
)

// ─── Shared HTTP client ───────────────────────────────────────────────────────

var httpClient = &http.Client{Timeout: 15 * time.Second}

func get(ctx context.Context, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, rawURL)
	}
	return io.ReadAll(resp.Body)
}

// ─── HN item struct (Firebase) ────────────────────────────────────────────────

type hnItem struct {
	ID          int    `json:"id"`
	Type        string `json:"type"`
	By          string `json:"by"`
	Time        int64  `json:"time"`
	Title       string `json:"title"`
	URL         string `json:"url"`
	Text        string `json:"text"`
	Score       int    `json:"score"`
	Descendants int    `json:"descendants"`
	Kids        []int  `json:"kids"`
	Dead        bool   `json:"dead"`
	Deleted     bool   `json:"deleted"`
	Parent      int    `json:"parent"`
}

func (i hnItem) HNUrl() string {
	return fmt.Sprintf("https://news.ycombinator.com/item?id=%d", i.ID)
}

func (i hnItem) TimeStr() string {
	return time.Unix(i.Time, 0).UTC().Format("2006-01-02 15:04 UTC")
}

func fetchItem(ctx context.Context, id int) (*hnItem, error) {
	data, err := get(ctx, fmt.Sprintf("%s/item/%d.json", firebaseBase, id))
	if err != nil {
		return nil, err
	}
	var item hnItem
	if err := json.Unmarshal(data, &item); err != nil {
		return nil, err
	}
	return &item, nil
}

// ─── hn_stories tool ─────────────────────────────────────────────────────────

const (
	StoriesToolName    = "hn_stories"
	StoriesDisplayName = "Hacker News Stories"
	StoriesDescription = `Fetch stories from Hacker News feeds.

Parameters:
- feed:  "top" | "new" | "best" | "ask" | "show" | "job" (default: top)
- limit: number of stories to return, 1-30 (default: 10)`
)

type StoriesTool struct{}

func NewStoriesTool() *StoriesTool { return &StoriesTool{} }

func (t *StoriesTool) Definition() tool.Definition {
	return tool.Definition{
		Name:        StoriesToolName,
		DisplayName: StoriesDisplayName,
		Description: StoriesDescription,
		Category:    "social",
		InputSchema: schema.FromMap(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"feed":  map[string]any{"type": "string", "enum": []string{"top", "new", "best", "ask", "show", "job"}, "description": "Which feed to read (default: top)."},
				"limit": map[string]any{"type": "integer", "description": "Number of stories (1-30, default: 10)."},
			},
		}),
		IsReadOnly:         true,
		IsConcurrencySafe:  true,
		IsDestructive:      false,
		RequiresPermission: false,
	}
}

func (t *StoriesTool) Call(ctx context.Context, input tool.CallInput, _ types.CanUseToolFn) (tool.CallResult, error) {
	feed := "top"
	if v, ok := input.Parsed["feed"].(string); ok && v != "" {
		feed = v
	}
	limit := defaultLimit
	if v, ok := input.Parsed["limit"].(float64); ok && v > 0 {
		limit = int(v)
	}
	if limit > maxLimit {
		limit = maxLimit
	}

	validFeeds := map[string]bool{"top": true, "new": true, "best": true, "ask": true, "show": true, "job": true}
	if !validFeeds[feed] {
		return tool.NewErrorResult(fmt.Errorf("feed must be one of: top, new, best, ask, show, job")), nil
	}

	data, err := get(ctx, fmt.Sprintf("%s/%sstories.json", firebaseBase, feed))
	if err != nil {
		return tool.NewErrorResult(fmt.Errorf("fetch %s feed: %w", feed, err)), nil
	}
	var ids []int
	if err := json.Unmarshal(data, &ids); err != nil {
		return tool.NewErrorResult(fmt.Errorf("parse feed: %w", err)), nil
	}
	if len(ids) > limit {
		ids = ids[:limit]
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Hacker News — %s feed (top %d)\n\n", feed, len(ids)))

	for rank, id := range ids {
		item, err := fetchItem(ctx, id)
		if err != nil || item == nil || item.Dead || item.Deleted {
			continue
		}
		link := item.URL
		if link == "" {
			link = item.HNUrl()
		}
		sb.WriteString(fmt.Sprintf("%d. %s\n   by %s · %d points · %d comments · %s\n   %s\n   HN: %s\n\n",
			rank+1, item.Title, item.By, item.Score, item.Descendants,
			item.TimeStr(), link, item.HNUrl()))
	}

	return tool.NewTextResult(strings.TrimRight(sb.String(), "\n")), nil
}

func (t *StoriesTool) Description(_ context.Context) (string, error) { return StoriesDescription, nil }
func (t *StoriesTool) ValidateInput(_ context.Context, input map[string]any) (map[string]any, error) {
	return input, nil
}
func (t *StoriesTool) CheckPermissions(_ context.Context, input map[string]any, _ tool.ToolUseContext) types.PermissionResult {
	return types.Passthrough(input)
}
func (t *StoriesTool) IsConcurrencySafe(_ map[string]any) bool { return true }
func (t *StoriesTool) IsReadOnly(_ map[string]any) bool        { return true }
func (t *StoriesTool) IsEnabled() bool                         { return true }
func (t *StoriesTool) FormatResult(data any) string            { return fmt.Sprintf("%v", data) }
func (t *StoriesTool) BackfillInput(_ context.Context, input map[string]any) map[string]any {
	return input
}

// ─── hn_item tool ─────────────────────────────────────────────────────────────

const (
	ItemToolName    = "hn_item"
	ItemDisplayName = "Hacker News Item"
	ItemDescription = `Fetch a single Hacker News story or comment thread by ID.

Returns the item details and up to max_comments top-level comments.

Parameters:
- id:           HN item ID (required)
- max_comments: max top-level comments to fetch (default: 5, max: 20)`
)

type ItemTool struct{}

func NewItemTool() *ItemTool { return &ItemTool{} }

func (t *ItemTool) Definition() tool.Definition {
	return tool.Definition{
		Name:        ItemToolName,
		DisplayName: ItemDisplayName,
		Description: ItemDescription,
		Category:    "social",
		InputSchema: schema.FromMap(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":           map[string]any{"type": "integer", "description": "HN item ID."},
				"max_comments": map[string]any{"type": "integer", "description": "Max top-level comments to include (default: 5, max: 20)."},
			},
			"required": []string{"id"},
		}),
		IsReadOnly:         true,
		IsConcurrencySafe:  true,
		IsDestructive:      false,
		RequiresPermission: false,
	}
}

func (t *ItemTool) Call(ctx context.Context, input tool.CallInput, _ types.CanUseToolFn) (tool.CallResult, error) {
	idRaw, ok := input.Parsed["id"]
	if !ok {
		return tool.NewErrorResult(fmt.Errorf("id is required")), nil
	}
	id := 0
	switch v := idRaw.(type) {
	case float64:
		id = int(v)
	case int:
		id = v
	}
	if id <= 0 {
		return tool.NewErrorResult(fmt.Errorf("id must be a positive integer")), nil
	}

	maxComments := 5
	if v, ok := input.Parsed["max_comments"].(float64); ok && v > 0 {
		maxComments = int(v)
	}
	if maxComments > 20 {
		maxComments = 20
	}

	item, err := fetchItem(ctx, id)
	if err != nil {
		return tool.NewErrorResult(fmt.Errorf("fetch item %d: %w", id, err)), nil
	}
	if item.Dead || item.Deleted {
		return tool.NewTextResult(fmt.Sprintf("Item %d is dead or deleted.", id)), nil
	}

	var sb strings.Builder
	link := item.URL
	if link == "" {
		link = item.HNUrl()
	}
	sb.WriteString(fmt.Sprintf("## %s\n", item.Title))
	sb.WriteString(fmt.Sprintf("by %s · %d points · %d comments · %s\n", item.By, item.Score, item.Descendants, item.TimeStr()))
	sb.WriteString(fmt.Sprintf("URL: %s\nHN:  %s\n", link, item.HNUrl()))
	if item.Text != "" {
		sb.WriteString("\n" + stripHTML(item.Text) + "\n")
	}

	if maxComments > 0 && len(item.Kids) > 0 {
		sb.WriteString(fmt.Sprintf("\n--- Top comments (%d/%d) ---\n\n", min(maxComments, len(item.Kids)), len(item.Kids)))
		for i, kid := range item.Kids {
			if i >= maxComments {
				break
			}
			comment, err := fetchItem(ctx, kid)
			if err != nil || comment == nil || comment.Dead || comment.Deleted || comment.Text == "" {
				continue
			}
			sb.WriteString(fmt.Sprintf("[%s · %s]\n%s\n\n", comment.By, comment.TimeStr(), stripHTML(comment.Text)))
		}
	}

	return tool.NewTextResult(strings.TrimRight(sb.String(), "\n")), nil
}

func (t *ItemTool) Description(_ context.Context) (string, error) { return ItemDescription, nil }
func (t *ItemTool) ValidateInput(_ context.Context, input map[string]any) (map[string]any, error) {
	return input, nil
}
func (t *ItemTool) CheckPermissions(_ context.Context, input map[string]any, _ tool.ToolUseContext) types.PermissionResult {
	return types.Passthrough(input)
}
func (t *ItemTool) IsConcurrencySafe(_ map[string]any) bool { return true }
func (t *ItemTool) IsReadOnly(_ map[string]any) bool        { return true }
func (t *ItemTool) IsEnabled() bool                         { return true }
func (t *ItemTool) FormatResult(data any) string            { return fmt.Sprintf("%v", data) }
func (t *ItemTool) BackfillInput(_ context.Context, input map[string]any) map[string]any {
	return input
}

// ─── hn_search tool ───────────────────────────────────────────────────────────

const (
	SearchToolName    = "hn_search"
	SearchDisplayName = "Search Hacker News"
	SearchDescription = `Search Hacker News posts and comments using Algolia.

Parameters:
- query:  search terms (required)
- type:   "story" | "comment" | "all" (default: story)
- limit:  results to return, 1-30 (default: 10)
- sort:   "relevance" | "date" (default: relevance)`
)

type SearchTool struct{}

func NewSearchTool() *SearchTool { return &SearchTool{} }

func (t *SearchTool) Definition() tool.Definition {
	return tool.Definition{
		Name:        SearchToolName,
		DisplayName: SearchDisplayName,
		Description: SearchDescription,
		Category:    "social",
		InputSchema: schema.FromMap(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{"type": "string", "description": "Search query."},
				"type":  map[string]any{"type": "string", "enum": []string{"story", "comment", "all"}, "description": "Result type (default: story)."},
				"limit": map[string]any{"type": "integer", "description": "Results to return (1-30, default: 10)."},
				"sort":  map[string]any{"type": "string", "enum": []string{"relevance", "date"}, "description": "Sort order (default: relevance)."},
			},
			"required": []string{"query"},
		}),
		IsReadOnly:         true,
		IsConcurrencySafe:  true,
		IsDestructive:      false,
		RequiresPermission: false,
	}
}

type algoliaResponse struct {
	Hits []algoliaHit `json:"hits"`
}

type algoliaHit struct {
	ObjectID    string `json:"objectID"`
	Title       string `json:"title"`
	StoryTitle  string `json:"story_title"`
	Author      string `json:"author"`
	Points      int    `json:"points"`
	NumComments int    `json:"num_comments"`
	URL         string `json:"url"`
	StoryURL    string `json:"story_url"`
	CommentText string `json:"comment_text"`
	CreatedAt   string `json:"created_at"`
	Type        string `json:"_tags"` // not used directly
}

func (h algoliaHit) displayTitle() string {
	if h.Title != "" {
		return h.Title
	}
	return h.StoryTitle
}

func (h algoliaHit) hnURL() string {
	return fmt.Sprintf("https://news.ycombinator.com/item?id=%s", h.ObjectID)
}

func (t *SearchTool) Call(ctx context.Context, input tool.CallInput, _ types.CanUseToolFn) (tool.CallResult, error) {
	query, _ := input.Parsed["query"].(string)
	if strings.TrimSpace(query) == "" {
		return tool.NewErrorResult(fmt.Errorf("query is required")), nil
	}

	typ := "story"
	if v, ok := input.Parsed["type"].(string); ok && v != "" && v != "all" {
		typ = v
	}
	limit := defaultLimit
	if v, ok := input.Parsed["limit"].(float64); ok && v > 0 {
		limit = int(v)
	}
	if limit > maxLimit {
		limit = maxLimit
	}
	sortBy := "search"
	if v, ok := input.Parsed["sort"].(string); ok && v == "date" {
		sortBy = "search_by_date"
	}

	endpoint := fmt.Sprintf("%s/%s?query=%s&hitsPerPage=%d",
		algoliaBase, sortBy, url.QueryEscape(query), limit)
	if typ != "all" {
		endpoint += "&tags=" + typ
	}

	data, err := get(ctx, endpoint)
	if err != nil {
		return tool.NewErrorResult(fmt.Errorf("algolia search: %w", err)), nil
	}

	var resp algoliaResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return tool.NewErrorResult(fmt.Errorf("parse response: %w", err)), nil
	}

	if len(resp.Hits) == 0 {
		return tool.NewTextResult(fmt.Sprintf("No results for %q on Hacker News.", query)), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Hacker News search: %q (%d results)\n\n", query, len(resp.Hits)))
	for i, h := range resp.Hits {
		title := h.displayTitle()
		if title == "" && h.CommentText != "" {
			title = "[comment] " + truncate(stripHTML(h.CommentText), 80)
		}
		sb.WriteString(fmt.Sprintf("%d. %s\n   by %s · %d points · %d comments\n   %s\n\n",
			i+1, title, h.Author, h.Points, h.NumComments, h.hnURL()))
	}

	return tool.NewTextResult(strings.TrimRight(sb.String(), "\n")), nil
}

func (t *SearchTool) Description(_ context.Context) (string, error) { return SearchDescription, nil }
func (t *SearchTool) ValidateInput(_ context.Context, input map[string]any) (map[string]any, error) {
	return input, nil
}
func (t *SearchTool) CheckPermissions(_ context.Context, input map[string]any, _ tool.ToolUseContext) types.PermissionResult {
	return types.Passthrough(input)
}
func (t *SearchTool) IsConcurrencySafe(_ map[string]any) bool { return true }
func (t *SearchTool) IsReadOnly(_ map[string]any) bool        { return true }
func (t *SearchTool) IsEnabled() bool                         { return true }
func (t *SearchTool) FormatResult(data any) string            { return fmt.Sprintf("%v", data) }
func (t *SearchTool) BackfillInput(_ context.Context, input map[string]any) map[string]any {
	return input
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

// stripHTML removes basic HTML tags for display in text output.
func stripHTML(s string) string {
	var out strings.Builder
	inTag := false
	for _, r := range s {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
		case !inTag:
			out.WriteRune(r)
		}
	}
	// Common HTML entities
	result := out.String()
	result = strings.ReplaceAll(result, "&amp;", "&")
	result = strings.ReplaceAll(result, "&lt;", "<")
	result = strings.ReplaceAll(result, "&gt;", ">")
	result = strings.ReplaceAll(result, "&quot;", "\"")
	result = strings.ReplaceAll(result, "&#x27;", "'")
	result = strings.ReplaceAll(result, "<p>", "\n")
	return strings.TrimSpace(result)
}

func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n]) + "…"
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
