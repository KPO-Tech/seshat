// Package discord provides the discord_send tool for sending messages to Discord.
//
// Two delivery modes:
//
//  1. Webhook (simplest — no bot account required):
//     Create a webhook in your Discord server settings → Integrations.
//     Set DISCORD_WEBHOOK_URL. One webhook per channel.
//
//  2. Bot API (richer — requires a Discord application and bot token):
//     Set DISCORD_BOT_TOKEN. The bot must be invited to the server with
//     the "Send Messages" permission. Supports embeds, files, threads.
//
// Implementation notes for contributors:
//   - Discord webhooks accept the same payload shape as Slack webhooks
//     (content + embeds). Start with webhooks — no OAuth flow needed.
//   - Embeds are the Discord equivalent of Block Kit: title, description,
//     color (hex int), fields (name/value pairs), footer, thumbnail.
//   - Rate limits: 5 requests per 2 seconds per webhook. Add exponential
//     backoff with Retry-After header handling.
//   - Discord API docs: https://discord.com/developers/docs/resources/webhook
//
// Env vars:
//
//	DISCORD_WEBHOOK_URL — Webhook URL (preferred for simple sends)
//	DISCORD_BOT_TOKEN   — Bot token for the Bot API mode
//
// See GitHub issue for full implementation spec.
package discord

import (
	"context"
	"errors"
	"fmt"

	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	"github.com/EngineerProjects/nexus-engine/internal/tools/schema"
	"github.com/EngineerProjects/nexus-engine/internal/types"
)

var ErrNotImplemented = errors.New("discord_send: not implemented — see GitHub issue")

const sendDesc = `Send a message to a Discord channel via webhook or Bot API.

Delivery mode is auto-detected:
- DISCORD_WEBHOOK_URL set → Webhook mode (simplest, no OAuth)
- DISCORD_BOT_TOKEN set   → Bot API mode (any channel the bot can access)

Returns:
- "ok":         true on success
- "message_id": Discord snowflake ID of the sent message
- "channel_id": channel the message was sent to

Parameters:
- content:    message text (supports Discord markdown: **bold**, ` + "`code`" + `, > quote) (required)
- username:   override the webhook display name (webhook mode only)
- avatar_url: override the webhook avatar URL (webhook mode only)
- channel:    channel ID to post in (Bot API mode only)
- embed:       JSON string representing a Discord embed object (title, description, color, fields)
- thread:      thread channel ID to post into (Bot API mode only)`

type SendTool struct{}

func NewSendTool() *SendTool { return &SendTool{} }

func (t *SendTool) Definition() tool.Definition {
	return tool.Definition{
		Name: "discord_send", DisplayName: "Send Discord Message", Description: sendDesc,
		Category:           "notifications",
		IsReadOnly:         false,
		IsConcurrencySafe:  false,
		RequiresPermission: true,
		InputSchema: schema.FromMap(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"content":    map[string]any{"type": "string", "description": "Message text (Discord markdown)."},
				"username":   map[string]any{"type": "string"},
				"avatar_url": map[string]any{"type": "string"},
				"channel":    map[string]any{"type": "string"},
				"embed":      map[string]any{"type": "string", "description": "Discord embed object as JSON."},
				"thread":     map[string]any{"type": "string"},
			},
			"required": []string{"content"},
		}),
	}
}
func (t *SendTool) Call(_ context.Context, _ tool.CallInput, _ types.CanUseToolFn) (tool.CallResult, error) {
	return tool.NewErrorResult(fmt.Errorf("%w", ErrNotImplemented)), nil
}
func (t *SendTool) Description(_ context.Context) (string, error) { return sendDesc, nil }
func (t *SendTool) ValidateInput(_ context.Context, in map[string]any) (map[string]any, error) {
	return in, nil
}
func (t *SendTool) CheckPermissions(_ context.Context, in map[string]any, _ tool.ToolUseContext) types.PermissionResult {
	return types.Passthrough(in)
}
func (t *SendTool) IsConcurrencySafe(_ map[string]any) bool                           { return false }
func (t *SendTool) IsReadOnly(_ map[string]any) bool                                  { return false }
func (t *SendTool) IsEnabled() bool                                                   { return false }
func (t *SendTool) FormatResult(data any) string                                      { return fmt.Sprintf("%v", data) }
func (t *SendTool) BackfillInput(_ context.Context, in map[string]any) map[string]any { return in }
