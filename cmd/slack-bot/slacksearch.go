package main

import (
	"context"
	"fmt"
	"strings"

	tool "github.com/KPO-Tech/seshat/internal/tools/contract"
	"github.com/KPO-Tech/seshat/internal/tools/schema"
	"github.com/KPO-Tech/seshat/internal/types"
	slackgo "github.com/slack-go/slack"
)

// slackSearchTool implements sdk.Tool and calls Slack's Real-Time Search API
// (assistant.search.context) to search messages, files, and channels in the workspace.
type slackSearchTool struct {
	api *slackgo.Client
}

func (t *slackSearchTool) Definition() tool.Definition {
	return tool.Definition{
		Name:        "slack_search",
		DisplayName: "Slack Search",
		Description: "Search messages, files, and channels in the Slack workspace using Slack's Real-Time Search API (assistant.search.context). Supports Slack search modifiers: from:, in:, before:, after:, has:, etc.",
		Category:    "slack",
		InputSchema: schema.JSONSchema{
			Type: "object",
			Properties: map[string]schema.JSONSchema{
				"query": {
					Type:        "string",
					Description: "Search query. Supports Slack modifiers: from:@user, in:#channel, before:YYYY-MM-DD, after:YYYY-MM-DD, has:link, has:file, etc.",
				},
				"content_types": {
					Type:        "array",
					Description: "Content types to include. Valid values: messages, files, channels. Defaults to [messages].",
					Items:       &schema.JSONSchema{Type: "string", Enum: []string{"messages", "files", "channels"}},
				},
				"limit": {
					Type:        "integer",
					Description: "Maximum number of results per content type (1–20). Defaults to 10.",
				},
				"include_context": {
					Type:        "boolean",
					Description: "When true, returns surrounding messages for each result. Useful for understanding the conversation context.",
				},
			},
			Required: []string{"query"},
		},
		IsReadOnly:         true,
		IsConcurrencySafe:  true,
		RequiresPermission: false,
	}
}

func (t *slackSearchTool) Call(ctx context.Context, input tool.CallInput, _ types.CanUseToolFn) (tool.CallResult, error) {
	query, _ := input.Parsed["query"].(string)
	if query == "" {
		return tool.CallResult{}, fmt.Errorf("query is required")
	}

	limit := 10
	if v, ok := input.Parsed["limit"].(float64); ok && v > 0 {
		limit = int(v)
		if limit > 20 {
			limit = 20
		}
	}

	includeCtx, _ := input.Parsed["include_context"].(bool)

	var contentTypes []string
	if raw, ok := input.Parsed["content_types"].([]any); ok {
		for _, ct := range raw {
			if s, ok := ct.(string); ok {
				contentTypes = append(contentTypes, s)
			}
		}
	}
	if len(contentTypes) == 0 {
		contentTypes = []string{"messages"}
	}

	params := slackgo.AssistantSearchContextParameters{
		Query:                  query,
		ContentTypes:           contentTypes,
		Limit:                  limit,
		IncludeContextMessages: includeCtx,
	}

	// Enrich with per-request Slack context when available.
	if ch, ok := ctx.Value(channelCtxKey{}).(channelCtxVal); ok {
		if ch.Channel != "" {
			params.ContextChannelID = ch.Channel
		}
		if ch.ActionToken != "" {
			params.ActionToken = ch.ActionToken
		}
	}

	resp, err := t.api.SearchAssistantContextContext(ctx, params)
	if err != nil {
		return tool.CallResult{}, fmt.Errorf("slack_search: %w", err)
	}

	return tool.NewTextResult(formatSearchResults(resp, query)), nil
}

func formatSearchResults(resp *slackgo.AssistantSearchContextResponse, query string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("*Slack search:* `%s`\n\n", query))

	total := len(resp.Results.Messages) + len(resp.Results.Files) + len(resp.Results.Channels)
	if total == 0 {
		sb.WriteString("No results found.")
		return sb.String()
	}

	if len(resp.Results.Messages) > 0 {
		sb.WriteString(fmt.Sprintf("*Messages (%d)*\n", len(resp.Results.Messages)))
		for _, m := range resp.Results.Messages {
			ch := m.ChannelName
			if ch == "" {
				ch = m.ChannelID
			}
			author := m.AuthorName
			if author == "" {
				author = m.AuthorUserID
			}
			content := truncate(m.Content, 200)
			sb.WriteString(fmt.Sprintf("• [#%s] *%s*: %s\n", ch, author, content))
			if m.Permalink != "" {
				sb.WriteString(fmt.Sprintf("  <%s|View message>\n", m.Permalink))
			}
			if m.ContextMessages != nil {
				for _, before := range m.ContextMessages.Before {
					sb.WriteString(fmt.Sprintf("  ↑ %s: %s\n", before.AuthorName, truncate(before.Content, 100)))
				}
				for _, after := range m.ContextMessages.After {
					sb.WriteString(fmt.Sprintf("  ↓ %s: %s\n", after.AuthorName, truncate(after.Content, 100)))
				}
			}
		}
		sb.WriteString("\n")
	}

	if len(resp.Results.Files) > 0 {
		sb.WriteString(fmt.Sprintf("*Files (%d)*\n", len(resp.Results.Files)))
		for _, f := range resp.Results.Files {
			sb.WriteString(fmt.Sprintf("• *%s* (%s)", f.Title, f.FileType))
			if f.Permalink != "" {
				sb.WriteString(fmt.Sprintf(" — <%s|Open>", f.Permalink))
			}
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	if len(resp.Results.Channels) > 0 {
		sb.WriteString(fmt.Sprintf("*Channels (%d)*\n", len(resp.Results.Channels)))
		for _, c := range resp.Results.Channels {
			line := fmt.Sprintf("• *#%s*", c.Name)
			if c.Purpose != "" {
				line += fmt.Sprintf(" — %s", truncate(c.Purpose, 120))
			}
			if c.Permalink != "" {
				line += fmt.Sprintf(" <%s|Open>", c.Permalink)
			}
			sb.WriteString(line + "\n")
		}
	}

	return sb.String()
}

func (t *slackSearchTool) Description(_ context.Context) (string, error) {
	return "Search messages, files, and channels in the Slack workspace using the Real-Time Search API.", nil
}

func (t *slackSearchTool) ValidateInput(_ context.Context, input map[string]any) (map[string]any, error) {
	if q, ok := input["query"].(string); !ok || q == "" {
		return nil, fmt.Errorf("query must be a non-empty string")
	}
	return input, nil
}

func (t *slackSearchTool) CheckPermissions(_ context.Context, input map[string]any, _ tool.ToolUseContext) types.PermissionResult {
	return types.Passthrough(input)
}

func (t *slackSearchTool) IsConcurrencySafe(_ map[string]any) bool { return true }
func (t *slackSearchTool) IsReadOnly(_ map[string]any) bool        { return true }
func (t *slackSearchTool) IsEnabled() bool                         { return true }
func (t *slackSearchTool) FormatResult(data any) string            { return fmt.Sprintf("%v", data) }
func (t *slackSearchTool) BackfillInput(_ context.Context, input map[string]any) map[string]any {
	return input
}
