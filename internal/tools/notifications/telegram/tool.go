// Package telegram provides the telegram_send tool for sending messages via the Telegram Bot API.
//
// Authentication:
//   - Create a bot via @BotFather on Telegram → receive a bot token.
//   - The bot must have been started by (or added to) the target chat.
//   - For private messages: the user must have sent /start to the bot first.
//   - For groups: add the bot to the group.
//
// Implementation notes for contributors:
//   - API endpoint: https://api.telegram.org/bot<TOKEN>/sendMessage
//   - Telegram supports HTML and MarkdownV2 parse modes.
//   - For MarkdownV2: special characters must be escaped with \
//     (e.g. \. \- \! \( \) \[ \]). Use HTML mode to avoid this.
//   - sendPhoto, sendDocument, sendAudio share the same base structure —
//     implement sendMessage first and the rest follow the same pattern.
//   - chat_id can be a numeric ID or a @username for public groups.
//   - Telegram Bot API docs: https://core.telegram.org/bots/api
//
// Env vars:
//
//	TELEGRAM_BOT_TOKEN  — bot token from @BotFather (required)
//	TELEGRAM_CHAT_ID    — default chat ID to send to (optional override)
//
// See GitHub issue for full implementation spec.
package telegram

import (
	"context"
	"errors"
	"fmt"

	tool "github.com/KPO-Tech/seshat/internal/tools/registry"
	"github.com/KPO-Tech/seshat/internal/tools/schema"
	"github.com/KPO-Tech/seshat/internal/types"
)

var ErrNotImplemented = errors.New("telegram_send: not implemented — see GitHub issue")

const sendDesc = `Send a message to a Telegram chat, group, or channel via Bot API.

Requires TELEGRAM_BOT_TOKEN (from @BotFather).

Returns:
- "ok":         true on success
- "message_id": Telegram message ID
- "chat_id":    the chat the message was delivered to
- "date":       Unix timestamp of the sent message

Parameters:
- chat_id:    target chat ID or @username of a public group/channel (required unless TELEGRAM_CHAT_ID is set)
- text:       message text — supports HTML or MarkdownV2 (required)
- parse_mode: "HTML" | "MarkdownV2" | "plain" (default: "HTML")
- silent:     send without notification sound (default: false)
- reply_to:   message ID to reply to (optional)
- link_preview: enable link previews (default: true)`

type SendTool struct{}

func NewSendTool() *SendTool { return &SendTool{} }

func (t *SendTool) Definition() tool.Definition {
	return tool.Definition{
		Name: "telegram_send", DisplayName: "Send Telegram Message", Description: sendDesc,
		Category:           "notifications",
		IsReadOnly:         false,
		IsConcurrencySafe:  false,
		RequiresPermission: true,
		InputSchema: schema.FromMap(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"chat_id":      map[string]any{"type": "string", "description": "Target chat ID or @username."},
				"text":         map[string]any{"type": "string", "description": "Message text."},
				"parse_mode":   map[string]any{"type": "string", "enum": []string{"HTML", "MarkdownV2", "plain"}, "default": "HTML"},
				"silent":       map[string]any{"type": "boolean", "default": false},
				"reply_to":     map[string]any{"type": "integer"},
				"link_preview": map[string]any{"type": "boolean", "default": true},
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
