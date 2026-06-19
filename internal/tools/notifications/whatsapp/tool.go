// Package whatsapp provides tools for WhatsApp Business via the Cloud API.
//
// Authentication: WhatsApp Business Cloud API (Meta for Developers).
// Required env vars:
//
//	WHATSAPP_PHONE_NUMBER_ID  — phone number ID from Meta Business dashboard
//	WHATSAPP_TOKEN            — permanent or temporary access token
//	WHATSAPP_VERIFY_TOKEN     — webhook verify token (for receiving messages)
//
// All messaging through WhatsApp API requires:
//   - A Meta Business account
//   - An approved WhatsApp Business app
//   - Message templates approved by Meta for outbound-initiated messages
//
// Tools planned (see GitHub issue):
//
//	whatsapp_send     — send a text or template message
//	whatsapp_send_media — send image/document/audio
//	whatsapp_webhook  — receive and parse incoming messages (via webhook)
package whatsapp

import (
	"context"
	"errors"
	"fmt"

	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	"github.com/EngineerProjects/nexus-engine/internal/tools/schema"
	"github.com/EngineerProjects/nexus-engine/internal/types"
)

// ErrNotImplemented is returned by all WhatsApp tools until the API client
// is complete (see GitHub issue). WhatsApp Business API requires Meta
// developer account approval before testing is possible.
var ErrNotImplemented = errors.New("whatsapp tools: not implemented — see GitHub issue")

// ─── whatsapp_send stub ───────────────────────────────────────────────────────

const (
	SendToolName    = "whatsapp_send"
	SendDisplayName = "Send WhatsApp Message"
	SendDescription = `Send a WhatsApp message via the Business Cloud API.

Requires WHATSAPP_PHONE_NUMBER_ID and WHATSAPP_TOKEN.

WhatsApp Cloud API rules:
- Outbound messages to users who haven't messaged first MUST use approved templates
- Free-form text is only allowed in reply to an inbound message (within 24h)

Parameters:
- to:              recipient phone number in E.164 format (e.g. +15551234567) (required)
- text:            message body (required for text type)
- type:            "text" | "template" (default: text)
- template_name:   template name (required when type=template)
- template_lang:   template language code, e.g. "en_US" (required for templates)
- template_params: list of template body parameter values`
)

type SendTool struct{}

func NewSendTool() *SendTool { return &SendTool{} }

func (t *SendTool) Definition() tool.Definition {
	return tool.Definition{
		Name:        SendToolName,
		DisplayName: SendDisplayName,
		Description: SendDescription,
		Category:    "social",
		InputSchema: schema.FromMap(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"to":              map[string]any{"type": "string"},
				"text":            map[string]any{"type": "string"},
				"type":            map[string]any{"type": "string", "enum": []string{"text", "template"}},
				"template_name":   map[string]any{"type": "string"},
				"template_lang":   map[string]any{"type": "string"},
				"template_params": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			},
			"required": []string{"to"},
		}),
		IsReadOnly: false, IsConcurrencySafe: false, IsDestructive: false, RequiresPermission: true,
	}
}

func (t *SendTool) Call(_ context.Context, _ tool.CallInput, _ types.CanUseToolFn) (tool.CallResult, error) {
	return tool.NewErrorResult(fmt.Errorf("%w", ErrNotImplemented)), nil
}
func (t *SendTool) Description(_ context.Context) (string, error) { return SendDescription, nil }
func (t *SendTool) ValidateInput(_ context.Context, input map[string]any) (map[string]any, error) {
	return input, nil
}
func (t *SendTool) CheckPermissions(_ context.Context, input map[string]any, _ tool.ToolUseContext) types.PermissionResult {
	return types.Passthrough(input)
}
func (t *SendTool) IsConcurrencySafe(_ map[string]any) bool { return false }
func (t *SendTool) IsReadOnly(_ map[string]any) bool        { return false }
func (t *SendTool) IsEnabled() bool                         { return false }
func (t *SendTool) FormatResult(data any) string            { return fmt.Sprintf("%v", data) }
func (t *SendTool) BackfillInput(_ context.Context, input map[string]any) map[string]any {
	return input
}
