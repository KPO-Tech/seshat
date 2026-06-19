// Package email provides the email_send tool for sending emails via SMTP.
//
// This tool is deliberately transport-agnostic: it speaks SMTP directly,
// which works with any provider (Gmail, SendGrid, Postmark, SES, Mailgun,
// self-hosted) without requiring provider-specific SDKs.
//
// Implementation notes for contributors:
//   - Use net/smtp from the standard library for the SMTP client — no deps needed.
//   - For TLS: use smtp.DialTLS (port 465) or smtp.SendMail with STARTTLS (port 587).
//   - For Gmail: enable "App Passwords" (2FA must be on) and use port 587.
//   - For SendGrid/Mailgun: use their SMTP relay — same code, different credentials.
//   - HTML bodies: set Content-Type: text/html; charset=UTF-8 in the MIME headers.
//   - Multipart: for both HTML and plain-text, use multipart/alternative.
//   - Attachments: multipart/mixed wrapping multipart/alternative + base64 parts.
//   - Start with plain-text only, add HTML + attachments as follow-up.
//
// Env vars:
//
//	SMTP_HOST      — SMTP server hostname (e.g. smtp.gmail.com)
//	SMTP_PORT      — port number (587 for STARTTLS, 465 for TLS)
//	SMTP_USER      — SMTP username / email address
//	SMTP_PASSWORD  — SMTP password or app password
//	SMTP_FROM      — default From address (optional, falls back to SMTP_USER)
//
// See GitHub issue for full implementation spec.
package email

import (
	"context"
	"errors"
	"fmt"

	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	"github.com/EngineerProjects/nexus-engine/internal/tools/schema"
	"github.com/EngineerProjects/nexus-engine/internal/types"
)

var ErrNotImplemented = errors.New("email_send: not implemented — see GitHub issue")

const sendDesc = `Send an email via SMTP.

Requires SMTP_HOST, SMTP_PORT, SMTP_USER, and SMTP_PASSWORD.

Returns:
- "ok":          true on success
- "message_id":  SMTP Message-ID header value
- "to":          list of recipients the message was accepted for

Parameters:
- to:          list of recipient email addresses (required)
- subject:     email subject line (required)
- body:        email body text (required)
- html:        HTML version of the body — sent as multipart/alternative alongside plain text
- from:        sender address (defaults to SMTP_FROM or SMTP_USER)
- cc:          list of CC recipients
- bcc:         list of BCC recipients (not visible to other recipients)
- reply_to:    Reply-To address`

type SendTool struct{}

func NewSendTool() *SendTool { return &SendTool{} }

func (t *SendTool) Definition() tool.Definition {
	return tool.Definition{
		Name: "email_send", DisplayName: "Send Email", Description: sendDesc,
		Category:           "notifications",
		IsReadOnly:         false,
		IsConcurrencySafe:  false,
		RequiresPermission: true,
		InputSchema: schema.FromMap(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"to":       map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Recipient email addresses."},
				"subject":  map[string]any{"type": "string"},
				"body":     map[string]any{"type": "string", "description": "Plain-text body."},
				"html":     map[string]any{"type": "string", "description": "HTML body (sent alongside plain text)."},
				"from":     map[string]any{"type": "string"},
				"cc":       map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				"bcc":      map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				"reply_to": map[string]any{"type": "string"},
			},
			"required": []string{"to", "subject", "body"},
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
