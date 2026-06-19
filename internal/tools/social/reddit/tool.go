// Package reddit provides tools for Reddit via the official REST API.
//
// Authentication: Reddit OAuth2 app (script or web type).
// Required env vars:
//
//	REDDIT_CLIENT_ID      — app client ID
//	REDDIT_CLIENT_SECRET  — app client secret
//	REDDIT_USERNAME       — Reddit username (for script apps)
//	REDDIT_PASSWORD       — Reddit password (for script apps)
//
// Tools planned (see GitHub issue):
//
//	reddit_search     — search posts across Reddit or in a specific subreddit
//	reddit_posts      — list hot/new/top/rising posts from a subreddit
//	reddit_post       — fetch a single post with comments
//	reddit_submit     — submit a post or comment (requires auth)
//	reddit_subreddit  — fetch subreddit info and rules
package reddit

import (
	"context"
	"errors"
	"fmt"

	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	"github.com/EngineerProjects/nexus-engine/internal/tools/schema"
	"github.com/EngineerProjects/nexus-engine/internal/types"
)

// ErrNotImplemented is returned by all Reddit tools until the OAuth client
// and API integration are complete (see GitHub issue).
var ErrNotImplemented = errors.New("reddit tools: not implemented — see GitHub issue")

// ─── reddit_search stub ───────────────────────────────────────────────────────

const (
	SearchToolName    = "reddit_search"
	SearchDisplayName = "Search Reddit"
	SearchDescription = `Search posts on Reddit.

Requires REDDIT_CLIENT_ID and REDDIT_CLIENT_SECRET.

Parameters:
- query:      search terms (required)
- subreddit:  limit search to a specific subreddit (optional)
- sort:        "relevance" | "hot" | "top" | "new" | "comments" (default: relevance)
- time:        "all" | "year" | "month" | "week" | "day" | "hour" (default: all)
- limit:       number of results (1-25, default: 10)`
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
				"query":     map[string]any{"type": "string"},
				"subreddit": map[string]any{"type": "string"},
				"sort":      map[string]any{"type": "string", "enum": []string{"relevance", "hot", "top", "new", "comments"}},
				"time":      map[string]any{"type": "string", "enum": []string{"all", "year", "month", "week", "day", "hour"}},
				"limit":     map[string]any{"type": "integer"},
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
func (t *SearchTool) IsEnabled() bool                         { return false } // disabled until implemented
func (t *SearchTool) FormatResult(data any) string            { return fmt.Sprintf("%v", data) }
func (t *SearchTool) BackfillInput(_ context.Context, input map[string]any) map[string]any {
	return input
}

// ─── reddit_posts stub ────────────────────────────────────────────────────────

const (
	PostsToolName    = "reddit_posts"
	PostsDisplayName = "Reddit Subreddit Posts"
	PostsDescription = `Fetch hot/new/top/rising posts from a subreddit.

Parameters:
- subreddit: subreddit name without r/ prefix (required)
- sort:      "hot" | "new" | "top" | "rising" (default: hot)
- time:      "all" | "year" | "month" | "week" | "day" | "hour" (for top only)
- limit:     number of posts (1-25, default: 10)`
)

type PostsTool struct{}

func NewPostsTool() *PostsTool { return &PostsTool{} }

func (t *PostsTool) Definition() tool.Definition {
	return tool.Definition{
		Name:        PostsToolName,
		DisplayName: PostsDisplayName,
		Description: PostsDescription,
		Category:    "social",
		InputSchema: schema.FromMap(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"subreddit": map[string]any{"type": "string"},
				"sort":      map[string]any{"type": "string", "enum": []string{"hot", "new", "top", "rising"}},
				"time":      map[string]any{"type": "string", "enum": []string{"all", "year", "month", "week", "day", "hour"}},
				"limit":     map[string]any{"type": "integer"},
			},
			"required": []string{"subreddit"},
		}),
		IsReadOnly: true, IsConcurrencySafe: true, IsDestructive: false, RequiresPermission: false,
	}
}

func (t *PostsTool) Call(_ context.Context, _ tool.CallInput, _ types.CanUseToolFn) (tool.CallResult, error) {
	return tool.NewErrorResult(fmt.Errorf("%w", ErrNotImplemented)), nil
}
func (t *PostsTool) Description(_ context.Context) (string, error) { return PostsDescription, nil }
func (t *PostsTool) ValidateInput(_ context.Context, input map[string]any) (map[string]any, error) {
	return input, nil
}
func (t *PostsTool) CheckPermissions(_ context.Context, input map[string]any, _ tool.ToolUseContext) types.PermissionResult {
	return types.Passthrough(input)
}
func (t *PostsTool) IsConcurrencySafe(_ map[string]any) bool { return true }
func (t *PostsTool) IsReadOnly(_ map[string]any) bool        { return true }
func (t *PostsTool) IsEnabled() bool                         { return false }
func (t *PostsTool) FormatResult(data any) string            { return fmt.Sprintf("%v", data) }
func (t *PostsTool) BackfillInput(_ context.Context, input map[string]any) map[string]any {
	return input
}
