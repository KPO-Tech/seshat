// Package devto provides tools for the DEV Community (dev.to) API.
//
// Public read endpoints work without authentication.
// Publishing articles requires a DEV_TO_API_KEY environment variable.
//
// Tools exposed:
//
//	devto_search    — search articles on dev.to
//	devto_article   — fetch a single article with full content
//	devto_feed      — list latest/top articles, optionally filtered by tag
//	devto_publish   — create or update an article (requires API key)
package devto

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	"github.com/EngineerProjects/nexus-engine/internal/tools/schema"
	"github.com/EngineerProjects/nexus-engine/internal/types"
)

const (
	apiBase      = "https://dev.to/api"
	defaultLimit = 10
	maxLimit     = 30
)

var httpClient = &http.Client{Timeout: 15 * time.Second}

// ─── Shared API types ─────────────────────────────────────────────────────────

type devtoArticle struct {
	ID              int      `json:"id"`
	Title           string   `json:"title"`
	Description     string   `json:"description"`
	BodyMarkdown    string   `json:"body_markdown"`
	URL             string   `json:"url"`
	Tags            []string `json:"tag_list"`
	PublishedAt     string   `json:"published_at"`
	PositiveRxCount int      `json:"positive_reactions_count"`
	CommentsCount   int      `json:"comments_count"`
	ReadingTimeMin  int      `json:"reading_time_minutes"`
	User            struct {
		Name     string `json:"name"`
		Username string `json:"username"`
	} `json:"user"`
}

func (a devtoArticle) summary() string {
	return fmt.Sprintf("%s\nby %s (@%s) · ❤ %d · 💬 %d · %d min read · %s\n%s",
		a.Title, a.User.Name, a.User.Username,
		a.PositiveRxCount, a.CommentsCount, a.ReadingTimeMin,
		strings.Join(a.Tags, ", "), a.URL)
}

func apiKey() string { return os.Getenv("DEV_TO_API_KEY") }

func get(ctx context.Context, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	if k := apiKey(); k != "" {
		req.Header.Set("api-key", k)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}
	return io.ReadAll(resp.Body)
}

func post(ctx context.Context, rawURL string, payload any) ([]byte, error) {
	k := apiKey()
	if k == "" {
		return nil, fmt.Errorf("DEV_TO_API_KEY not set — publishing requires an API key")
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("api-key", k)
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(data))
	}
	return data, nil
}

// ─── devto_feed tool ──────────────────────────────────────────────────────────

const (
	FeedToolName    = "devto_feed"
	FeedDisplayName = "DEV.to Feed"
	FeedDescription = `List recent articles from dev.to, optionally filtered by tag.

Parameters:
- tag:   filter by tag, e.g. "go", "ai", "webdev" (optional)
- limit: number of articles (1-30, default: 10)
- top:   if true, return top articles by reactions instead of latest`
)

type FeedTool struct{}

func NewFeedTool() *FeedTool { return &FeedTool{} }

func (t *FeedTool) Definition() tool.Definition {
	return tool.Definition{
		Name:        FeedToolName,
		DisplayName: FeedDisplayName,
		Description: FeedDescription,
		Category:    "social",
		InputSchema: schema.FromMap(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"tag":   map[string]any{"type": "string", "description": "Filter by tag (e.g. 'go', 'ai')."},
				"limit": map[string]any{"type": "integer", "description": "Number of articles (1-30, default: 10)."},
				"top":   map[string]any{"type": "boolean", "description": "If true, return top articles by reactions."},
			},
		}),
		IsReadOnly:         true,
		IsConcurrencySafe:  true,
		IsDestructive:      false,
		RequiresPermission: false,
	}
}

func (t *FeedTool) Call(ctx context.Context, input tool.CallInput, _ types.CanUseToolFn) (tool.CallResult, error) {
	limit := defaultLimit
	if v, ok := input.Parsed["limit"].(float64); ok && v > 0 {
		limit = int(v)
	}
	if limit > maxLimit {
		limit = maxLimit
	}

	endpoint := fmt.Sprintf("%s/articles?per_page=%d", apiBase, limit)
	if tag, ok := input.Parsed["tag"].(string); ok && tag != "" {
		endpoint += "&tag=" + url.QueryEscape(tag)
	}
	if top, ok := input.Parsed["top"].(bool); ok && top {
		endpoint += "&top=1"
	}

	data, err := get(ctx, endpoint)
	if err != nil {
		return tool.NewErrorResult(fmt.Errorf("fetch feed: %w", err)), nil
	}

	var articles []devtoArticle
	if err := json.Unmarshal(data, &articles); err != nil {
		return tool.NewErrorResult(fmt.Errorf("parse response: %w", err)), nil
	}

	if len(articles) == 0 {
		return tool.NewTextResult("No articles found."), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("DEV.to feed (%d articles)\n\n", len(articles)))
	for i, a := range articles {
		sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, a.summary()))
		sb.WriteString("\n")
	}

	return tool.NewTextResult(strings.TrimRight(sb.String(), "\n")), nil
}

func (t *FeedTool) Description(_ context.Context) (string, error) { return FeedDescription, nil }
func (t *FeedTool) ValidateInput(_ context.Context, input map[string]any) (map[string]any, error) {
	return input, nil
}
func (t *FeedTool) CheckPermissions(_ context.Context, input map[string]any, _ tool.ToolUseContext) types.PermissionResult {
	return types.Passthrough(input)
}
func (t *FeedTool) IsConcurrencySafe(_ map[string]any) bool { return true }
func (t *FeedTool) IsReadOnly(_ map[string]any) bool        { return true }
func (t *FeedTool) IsEnabled() bool                         { return true }
func (t *FeedTool) FormatResult(data any) string            { return fmt.Sprintf("%v", data) }
func (t *FeedTool) BackfillInput(_ context.Context, input map[string]any) map[string]any {
	return input
}

// ─── devto_article tool ───────────────────────────────────────────────────────

const (
	ArticleToolName    = "devto_article"
	ArticleDisplayName = "DEV.to Article"
	ArticleDescription = `Fetch a single dev.to article with full content.

Parameters:
- id:              article ID (integer) — use this OR url/slug
- url:             article URL (e.g. https://dev.to/user/title-slug)
- include_content: include the full markdown body (default: true)`
)

type ArticleTool struct{}

func NewArticleTool() *ArticleTool { return &ArticleTool{} }

func (t *ArticleTool) Definition() tool.Definition {
	return tool.Definition{
		Name:        ArticleToolName,
		DisplayName: ArticleDisplayName,
		Description: ArticleDescription,
		Category:    "social",
		InputSchema: schema.FromMap(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":              map[string]any{"type": "integer", "description": "Article ID."},
				"url":             map[string]any{"type": "string", "description": "Article URL."},
				"include_content": map[string]any{"type": "boolean", "description": "Include full markdown body (default: true)."},
			},
		}),
		IsReadOnly:         true,
		IsConcurrencySafe:  true,
		IsDestructive:      false,
		RequiresPermission: false,
	}
}

func (t *ArticleTool) Call(ctx context.Context, input tool.CallInput, _ types.CanUseToolFn) (tool.CallResult, error) {
	var endpoint string
	if v, ok := input.Parsed["id"].(float64); ok && v > 0 {
		endpoint = fmt.Sprintf("%s/articles/%d", apiBase, int(v))
	} else if rawURL, ok := input.Parsed["url"].(string); ok && rawURL != "" {
		// extract /username/slug from URL
		u, err := url.Parse(rawURL)
		if err != nil {
			return tool.NewErrorResult(fmt.Errorf("invalid url: %w", err)), nil
		}
		parts := strings.SplitN(strings.TrimPrefix(u.Path, "/"), "/", 2)
		if len(parts) != 2 {
			return tool.NewErrorResult(fmt.Errorf("url must be https://dev.to/username/slug")), nil
		}
		endpoint = fmt.Sprintf("%s/articles/%s/%s", apiBase, parts[0], parts[1])
	} else {
		return tool.NewErrorResult(fmt.Errorf("either id or url is required")), nil
	}

	withContent := true
	if v, ok := input.Parsed["include_content"].(bool); ok {
		withContent = v
	}

	data, err := get(ctx, endpoint)
	if err != nil {
		return tool.NewErrorResult(fmt.Errorf("fetch article: %w", err)), nil
	}

	var a devtoArticle
	if err := json.Unmarshal(data, &a); err != nil {
		return tool.NewErrorResult(fmt.Errorf("parse article: %w", err)), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# %s\n%s\n\n", a.Title, a.summary()))
	if a.Description != "" {
		sb.WriteString(fmt.Sprintf("**Description:** %s\n\n", a.Description))
	}
	if withContent && a.BodyMarkdown != "" {
		sb.WriteString("---\n\n")
		sb.WriteString(a.BodyMarkdown)
	}

	return tool.NewTextResult(strings.TrimRight(sb.String(), "\n")), nil
}

func (t *ArticleTool) Description(_ context.Context) (string, error) { return ArticleDescription, nil }
func (t *ArticleTool) ValidateInput(_ context.Context, input map[string]any) (map[string]any, error) {
	return input, nil
}
func (t *ArticleTool) CheckPermissions(_ context.Context, input map[string]any, _ tool.ToolUseContext) types.PermissionResult {
	return types.Passthrough(input)
}
func (t *ArticleTool) IsConcurrencySafe(_ map[string]any) bool { return true }
func (t *ArticleTool) IsReadOnly(_ map[string]any) bool        { return true }
func (t *ArticleTool) IsEnabled() bool                         { return true }
func (t *ArticleTool) FormatResult(data any) string            { return fmt.Sprintf("%v", data) }
func (t *ArticleTool) BackfillInput(_ context.Context, input map[string]any) map[string]any {
	return input
}

// ─── devto_publish tool ───────────────────────────────────────────────────────

const (
	PublishToolName    = "devto_publish"
	PublishDisplayName = "Publish to DEV.to"
	PublishDescription = `Create or update an article on dev.to.

Requires DEV_TO_API_KEY environment variable.

Parameters:
- title:        article title (required)
- body:         full article content in Markdown (required)
- tags:         list of tags, max 4 (e.g. ["go", "ai", "tools"])
- canonical_url: original URL if cross-posting
- published:    if true, publish immediately; false = save as draft (default: false)`
)

type PublishTool struct{}

func NewPublishTool() *PublishTool { return &PublishTool{} }

func (t *PublishTool) Definition() tool.Definition {
	return tool.Definition{
		Name:        PublishToolName,
		DisplayName: PublishDisplayName,
		Description: PublishDescription,
		Category:    "social",
		InputSchema: schema.FromMap(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"title":         map[string]any{"type": "string", "description": "Article title."},
				"body":          map[string]any{"type": "string", "description": "Full markdown content."},
				"tags":          map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Tags (max 4)."},
				"canonical_url": map[string]any{"type": "string", "description": "Original URL for cross-posts."},
				"published":     map[string]any{"type": "boolean", "description": "Publish now (true) or save as draft (false, default)."},
			},
			"required": []string{"title", "body"},
		}),
		IsReadOnly:         false,
		IsConcurrencySafe:  false,
		IsDestructive:      false,
		RequiresPermission: true,
	}
}

func (t *PublishTool) Call(ctx context.Context, input tool.CallInput, permissionCheck types.CanUseToolFn) (tool.CallResult, error) {
	title, _ := input.Parsed["title"].(string)
	body, _ := input.Parsed["body"].(string)
	if strings.TrimSpace(title) == "" || strings.TrimSpace(body) == "" {
		return tool.NewErrorResult(fmt.Errorf("title and body are required")), nil
	}

	if permissionCheck != nil {
		res := permissionCheck(ctx, types.ToolPermissionRequest{ToolName: PublishToolName, ToolInput: input.Parsed})
		if res.Behavior != types.PermissionBehaviorAllow {
			return tool.NewErrorResult(fmt.Errorf("permission denied: devto_publish requires approval")), nil
		}
	}

	article := map[string]any{
		"title":         title,
		"body_markdown": body,
		"published":     false,
	}
	if v, ok := input.Parsed["published"].(bool); ok {
		article["published"] = v
	}
	if rawTags, ok := input.Parsed["tags"].([]any); ok {
		tags := make([]string, 0, len(rawTags))
		for _, rt := range rawTags {
			if s, ok := rt.(string); ok {
				tags = append(tags, s)
			}
		}
		if len(tags) > 4 {
			tags = tags[:4]
		}
		article["tags"] = tags
	}
	if v, ok := input.Parsed["canonical_url"].(string); ok && v != "" {
		article["canonical_url"] = v
	}

	data, err := post(ctx, fmt.Sprintf("%s/articles", apiBase), map[string]any{"article": article})
	if err != nil {
		return tool.NewErrorResult(fmt.Errorf("publish article: %w", err)), nil
	}

	var result devtoArticle
	if err := json.Unmarshal(data, &result); err != nil {
		return tool.NewTextResult("Article created (could not parse response)."), nil
	}

	status := "draft"
	if pub, _ := article["published"].(bool); pub {
		status = "published"
	}

	return tool.NewTextResult(fmt.Sprintf("Article %s on DEV.to!\nID: %d\nURL: %s", status, result.ID, result.URL)), nil
}

func (t *PublishTool) Description(_ context.Context) (string, error) { return PublishDescription, nil }
func (t *PublishTool) ValidateInput(_ context.Context, input map[string]any) (map[string]any, error) {
	return input, nil
}
func (t *PublishTool) CheckPermissions(_ context.Context, input map[string]any, _ tool.ToolUseContext) types.PermissionResult {
	return types.Passthrough(input)
}
func (t *PublishTool) IsConcurrencySafe(_ map[string]any) bool { return false }
func (t *PublishTool) IsReadOnly(_ map[string]any) bool        { return false }
func (t *PublishTool) IsEnabled() bool                         { return true }
func (t *PublishTool) FormatResult(data any) string            { return fmt.Sprintf("%v", data) }
func (t *PublishTool) BackfillInput(_ context.Context, input map[string]any) map[string]any {
	return input
}
