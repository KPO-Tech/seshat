// Package twitter provides tools for Twitter/X via the v2 API.
//
// Authentication: Bearer token (read-only) or OAuth 1.0a (read+write).
// Required env vars:
//
//	TWITTER_BEARER_TOKEN   — for search and read (required)
//	TWITTER_API_KEY        — for posting (OAuth 1.0a)
//	TWITTER_API_SECRET     — for posting
//	TWITTER_ACCESS_TOKEN   — for posting
//	TWITTER_ACCESS_SECRET  — for posting
//
// NOTE: Twitter API v2 free tier (2025) is severely limited:
//   - Read: 500k tweets/month, only recent search (last 7 days)
//   - Write: 500 posts/month
//   - Basic tier ($100/month) unlocks more
//
// Tools planned (see GitHub issue):
//
//	twitter_search   — search recent tweets (last 7 days)
//	twitter_timeline — fetch a user's recent timeline
//	twitter_tweet    — post a tweet (requires OAuth 1.0a)
//	twitter_thread   — post a thread of connected tweets
package twitter

import (
	"context"
	"errors"
	"fmt"

	tool "github.com/EngineerProjects/seshat/internal/tools/registry"
	"github.com/EngineerProjects/seshat/internal/tools/schema"
	"github.com/EngineerProjects/seshat/internal/types"
)

// ErrNotImplemented is returned by all Twitter tools until the API client
// is complete (see GitHub issue). The API cost model should be considered
// before implementing — basic tier required for serious usage.
var ErrNotImplemented = errors.New("twitter tools: not implemented — see GitHub issue")

// ─── twitter_search stub ─────────────────────────────────────────────────────

const (
	SearchToolName    = "twitter_search"
	SearchDisplayName = "Search Twitter/X"
	SearchDescription = `Search recent tweets on Twitter/X (last 7 days, free tier).

Requires TWITTER_BEARER_TOKEN.

Parameters:
- query:    Twitter search query, supports operators (required)
            Examples: "seshat", "#golang lang:en", "from:user -is:retweet"
- limit:    max tweets to return (1-100, default: 10)
- sort:     "recency" | "relevancy" (default: recency)`
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
				"query": map[string]any{"type": "string"},
				"limit": map[string]any{"type": "integer"},
				"sort":  map[string]any{"type": "string", "enum": []string{"recency", "relevancy"}},
			},
			"required": []string{"query"},
		}),
		IsReadOnly: true, IsConcurrencySafe: true, IsDestructive: false, RequiresPermission: false,
	}
}

func (t *SearchTool) Call(_ context.Context, _ tool.CallInput, _ types.CanUseToolFn) (tool.CallResult, error) {
	return tool.NewErrorResult(fmt.Errorf("%w", ErrNotImplemented)), nil
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
func (t *SearchTool) IsEnabled() bool                         { return false }
func (t *SearchTool) FormatResult(data any) string            { return fmt.Sprintf("%v", data) }
func (t *SearchTool) BackfillInput(_ context.Context, input map[string]any) map[string]any {
	return input
}

// ─── twitter_tweet stub ───────────────────────────────────────────────────────

const (
	TweetToolName    = "twitter_tweet"
	TweetDisplayName = "Post Tweet"
	TweetDescription = `Post a tweet on Twitter/X.

Requires TWITTER_API_KEY, TWITTER_API_SECRET, TWITTER_ACCESS_TOKEN, TWITTER_ACCESS_SECRET.

Parameters:
- text:       tweet content, max 280 characters (required)
- reply_to:   tweet ID to reply to (optional)
- media_urls: list of media URLs to attach (optional, not implemented yet)`
)

type TweetTool struct{}

func NewTweetTool() *TweetTool { return &TweetTool{} }

func (t *TweetTool) Definition() tool.Definition {
	return tool.Definition{
		Name:        TweetToolName,
		DisplayName: TweetDisplayName,
		Description: TweetDescription,
		Category:    "social",
		InputSchema: schema.FromMap(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"text":     map[string]any{"type": "string"},
				"reply_to": map[string]any{"type": "string"},
			},
			"required": []string{"text"},
		}),
		IsReadOnly: false, IsConcurrencySafe: false, IsDestructive: false, RequiresPermission: true,
	}
}

func (t *TweetTool) Call(_ context.Context, _ tool.CallInput, _ types.CanUseToolFn) (tool.CallResult, error) {
	return tool.NewErrorResult(fmt.Errorf("%w", ErrNotImplemented)), nil
}
func (t *TweetTool) Description(_ context.Context) (string, error) { return TweetDescription, nil }
func (t *TweetTool) ValidateInput(_ context.Context, input map[string]any) (map[string]any, error) {
	return input, nil
}
func (t *TweetTool) CheckPermissions(_ context.Context, input map[string]any, _ tool.ToolUseContext) types.PermissionResult {
	return types.Passthrough(input)
}
func (t *TweetTool) IsConcurrencySafe(_ map[string]any) bool { return false }
func (t *TweetTool) IsReadOnly(_ map[string]any) bool        { return false }
func (t *TweetTool) IsEnabled() bool                         { return false }
func (t *TweetTool) FormatResult(data any) string            { return fmt.Sprintf("%v", data) }
func (t *TweetTool) BackfillInput(_ context.Context, input map[string]any) map[string]any {
	return input
}
