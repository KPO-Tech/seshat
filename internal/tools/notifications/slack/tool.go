// Package slack provides the slack_send tool for sending messages to Slack.
//
// Two delivery modes are supported by design:
//
//  1. Incoming Webhooks (simplest — no OAuth required):
//     Create a webhook in your Slack app settings and set SLACK_WEBHOOK_URL.
//     Supports text, blocks, and attachments. One channel per webhook.
//
//  2. Bot API (richer — requires a Slack app with OAuth scopes):
//     Set SLACK_BOT_TOKEN (xoxb-...) and call chat.postMessage.
//     Supports any channel the bot is invited to, threads, reactions, files.
//
// Implementation notes for contributors:
//   - Prefer Incoming Webhooks for the first implementation: simpler auth,
//     no scope approval needed, supports Block Kit payloads.
//   - For Bot API: scope needed is chat:write. Token comes from the app's
//     OAuth & Permissions page.
//   - Block Kit reference: https://api.slack.com/block-kit
//
// Env vars:
//
//	SLACK_WEBHOOK_URL   — Incoming Webhook URL (preferred for simple sends)
//	SLACK_BOT_TOKEN     — Bot token for the Bot API mode
//	SLACK_DEFAULT_CHAN  — Default channel for Bot API mode (e.g. #general)
//
// See GitHub issue for full implementation spec.
package slack

import (
	"context"
	"errors"
	"fmt"

	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	"github.com/EngineerProjects/nexus-engine/internal/tools/schema"
	"github.com/EngineerProjects/nexus-engine/internal/types"
)

// ErrNotImplemented is returned until the HTTP client and Block Kit builder are wired up.
var ErrNotImplemented = errors.New("slack_send: not implemented — see GitHub issue")

const sendDesc = `Send a message to a Slack channel or user.

Delivery modes (auto-detected from env vars):
- SLACK_WEBHOOK_URL set  → Incoming Webhook (simplest, one channel)
- SLACK_BOT_TOKEN set    → Bot API (any channel the bot is in, supports threads)

Returns:
- "ok":        true on success
- "ts":        message timestamp (can be used to reply in a thread)
- "channel":   channel the message was delivered to
- "permalink": URL to the message in Slack (Bot API only)

Parameters:
- text:     message text — supports Slack mrkdwn (bold: *text*, code: ` + "`code`" + `) (required)
- channel:  channel ID or name, e.g. "#alerts" or "C0123ABC" (Bot API only; ignored for webhooks)
- username: override the bot display name (webhook mode only)
- icon:     override the bot icon — emoji string e.g. ":robot_face:" (webhook mode only)
- thread:   reply to this message timestamp to post in a thread (Bot API only)
- blocks:   raw Slack Block Kit payload (JSON string) — overrides text when set`

type SendTool struct{}

func NewSendTool() *SendTool { return &SendTool{} }

func (t *SendTool) Definition() tool.Definition {
	return tool.Definition{
		Name: "slack_send", DisplayName: "Send Slack Message", Description: sendDesc,
		Category:           "notifications",
		IsReadOnly:         false,
		IsConcurrencySafe:  false,
		RequiresPermission: true,
		InputSchema: schema.FromMap(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"text":     map[string]any{"type": "string", "description": "Message text (mrkdwn supported)."},
				"channel":  map[string]any{"type": "string", "description": "Channel ID or name (Bot API mode)."},
				"username": map[string]any{"type": "string"},
				"icon":     map[string]any{"type": "string"},
				"thread":   map[string]any{"type": "string", "description": "Thread ts to reply to."},
				"blocks":   map[string]any{"type": "string", "description": "Block Kit JSON payload."},
			},
			"required": []string{"text"},
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
