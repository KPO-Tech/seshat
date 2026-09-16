package gmail

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	gmailapi "google.golang.org/api/gmail/v1"
)

// Reply delivers a reply in the given Gmail thread (empty threadID sends
// an orphaned message instead). It first fetches the thread's most recent
// message (headers only) to build a properly threaded reply - matching
// Subject plus In-Reply-To/References - rather than sending a message that
// merely shares a ThreadId, which Gmail's own clients don't reliably group
// without those headers. Unlike Microsoft Graph's fire-and-forget reply
// action, Gmail's Send returns a real message ID synchronously.
func (c *Client) Reply(ctx context.Context, threadID, to, body string) (string, error) {
	subject := ""
	inReplyTo := ""
	references := ""
	if threadID != "" {
		thread, err := c.svc.Users.Threads.Get("me", threadID).
			Format("metadata").
			MetadataHeaders("Subject", "Message-Id", "References").
			Context(ctx).Do()
		if err == nil && len(thread.Messages) > 0 {
			last := thread.Messages[len(thread.Messages)-1]
			subject = "Re: " + stripReplyPrefix(headerValue(last.Payload.Headers, "Subject"))
			inReplyTo = headerValue(last.Payload.Headers, "Message-Id")
			references = strings.TrimSpace(headerValue(last.Payload.Headers, "References") + " " + inReplyTo)
		}
	}
	if subject == "" {
		subject = "(no subject)"
	}

	raw := buildRawMessage(composeHeaders{To: to, Subject: subject, Body: body, InReplyTo: inReplyTo, References: references})
	msg := &gmailapi.Message{Raw: raw, ThreadId: threadID}
	sent, err := c.svc.Users.Messages.Send("me", msg).Context(ctx).Do()
	if err != nil {
		return "", fmt.Errorf("send message: %w", err)
	}
	return sent.Id, nil
}

// Send delivers a fresh (non-reply) message to one or more recipients.
func (c *Client) Send(ctx context.Context, to []string, subject, body string) (string, error) {
	if len(to) == 0 {
		return "", fmt.Errorf("at least one recipient is required")
	}
	raw := buildRawMessage(composeHeaders{To: strings.Join(to, ", "), Subject: subject, Body: body})
	msg := &gmailapi.Message{Raw: raw}
	sent, err := c.svc.Users.Messages.Send("me", msg).Context(ctx).Do()
	if err != nil {
		return "", fmt.Errorf("send message: %w", err)
	}
	return sent.Id, nil
}

// ComposeOptions is the full header set a caller-composed (non-reply)
// message can set - CC/BCC/threading, unlike Send's minimal to/subject/body
// (kept as-is since inbox/gmail's connector and Send's own existing callers
// only ever need that minimal shape).
type ComposeOptions struct {
	To, Cc, Bcc []string
	Subject     string
	Body        string
	// ThreadID, when set, sends within an existing thread - Gmail groups by
	// ThreadId server-side, unlike Reply which also rewrites Subject/
	// In-Reply-To/References from the thread's last message.
	ThreadID string
}

// Compose sends a message with the full CC/BCC/threading header set
// ComposeOptions offers - the primitive both the richer "send" MCP tool and
// CreateDraft build on.
func (c *Client) Compose(ctx context.Context, opts ComposeOptions) (string, error) {
	if len(opts.To) == 0 {
		return "", fmt.Errorf("at least one recipient is required")
	}
	raw := buildRawMessage(composeHeaders{
		To: strings.Join(opts.To, ", "), Cc: strings.Join(opts.Cc, ", "), Bcc: strings.Join(opts.Bcc, ", "),
		Subject: opts.Subject, Body: opts.Body,
	})
	msg := &gmailapi.Message{Raw: raw, ThreadId: opts.ThreadID}
	sent, err := c.svc.Users.Messages.Send("me", msg).Context(ctx).Do()
	if err != nil {
		return "", fmt.Errorf("send message: %w", err)
	}
	return sent.Id, nil
}

// CreateDraft saves a message as a draft instead of sending it.
func (c *Client) CreateDraft(ctx context.Context, opts ComposeOptions) (Draft, error) {
	if len(opts.To) == 0 {
		return Draft{}, fmt.Errorf("at least one recipient is required")
	}
	raw := buildRawMessage(composeHeaders{
		To: strings.Join(opts.To, ", "), Cc: strings.Join(opts.Cc, ", "), Bcc: strings.Join(opts.Bcc, ", "),
		Subject: opts.Subject, Body: opts.Body,
	})
	draft := &gmailapi.Draft{Message: &gmailapi.Message{Raw: raw, ThreadId: opts.ThreadID}}
	created, err := c.svc.Users.Drafts.Create("me", draft).Context(ctx).Do()
	if err != nil {
		return Draft{}, fmt.Errorf("create draft: %w", err)
	}
	return translateDraft(created), nil
}

// listDraftsLimit caps ListDrafts the same way searchMessageLimit caps
// Search - an agent-facing list with no cap could otherwise return years of
// accumulated drafts.
const listDraftsLimit = 25

// ListDrafts returns saved drafts. Gmail's Drafts.List response carries only
// each draft's ID/ThreadID/MessageID, not its subject or snippet - a
// per-draft Get call would be needed for that, which this deliberately
// doesn't do (N+1 for a list operation). maxResults <= 0 uses
// listDraftsLimit.
func (c *Client) ListDrafts(ctx context.Context, maxResults int64) ([]Draft, error) {
	if maxResults <= 0 {
		maxResults = listDraftsLimit
	}
	resp, err := c.svc.Users.Drafts.List("me").MaxResults(maxResults).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("list drafts: %w", err)
	}
	drafts := make([]Draft, 0, len(resp.Drafts))
	for _, d := range resp.Drafts {
		drafts = append(drafts, translateDraft(d))
	}
	return drafts, nil
}

// SendDraft sends a previously saved draft and returns the sent message ID.
func (c *Client) SendDraft(ctx context.Context, draftID string) (string, error) {
	sent, err := c.svc.Users.Drafts.Send("me", &gmailapi.Draft{Id: draftID}).Context(ctx).Do()
	if err != nil {
		return "", fmt.Errorf("send draft %s: %w", draftID, err)
	}
	return sent.Id, nil
}

// DeleteDraft discards a draft without sending it.
func (c *Client) DeleteDraft(ctx context.Context, draftID string) error {
	if err := c.svc.Users.Drafts.Delete("me", draftID).Context(ctx).Do(); err != nil {
		return fmt.Errorf("delete draft %s: %w", draftID, err)
	}
	return nil
}

func translateDraft(d *gmailapi.Draft) Draft {
	out := Draft{ID: d.Id}
	if d.Message != nil {
		out.MessageID = d.Message.Id
		out.ThreadID = d.Message.ThreadId
	}
	return out
}

// FetchAttachment downloads one attachment's bytes by the GmailAttachmentID
// captured on its Attachment metadata at sync time.
func (c *Client) FetchAttachment(ctx context.Context, messageID, attachmentID string) ([]byte, error) {
	body, err := c.svc.Users.Messages.Attachments.Get("me", messageID, attachmentID).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("get attachment: %w", err)
	}
	// Same unpadded base64url convention as message.go's decodeBody.
	data, err := base64.RawURLEncoding.DecodeString(body.Data)
	if err != nil {
		return nil, fmt.Errorf("decode attachment data: %w", err)
	}
	return data, nil
}

// composeHeaders is the internal header set buildRawMessage assembles into
// an RFC 2822 message - a struct rather than positional params now that
// Compose/CreateDraft added Cc/Bcc on top of Reply/Send's original
// to/subject/body/threading fields.
type composeHeaders struct {
	To, Cc, Bcc           string
	Subject, Body         string
	InReplyTo, References string
}

// buildRawMessage builds a minimal RFC 2822 message and returns it as the
// unpadded base64url string the Gmail API's Message.Raw field expects.
func buildRawMessage(h composeHeaders) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "To: %s\r\n", h.To)
	if h.Cc != "" {
		fmt.Fprintf(&sb, "Cc: %s\r\n", h.Cc)
	}
	if h.Bcc != "" {
		fmt.Fprintf(&sb, "Bcc: %s\r\n", h.Bcc)
	}
	fmt.Fprintf(&sb, "Subject: %s\r\n", h.Subject)
	if h.InReplyTo != "" {
		fmt.Fprintf(&sb, "In-Reply-To: %s\r\n", h.InReplyTo)
	}
	if h.References != "" {
		fmt.Fprintf(&sb, "References: %s\r\n", h.References)
	}
	sb.WriteString("MIME-Version: 1.0\r\n")
	sb.WriteString("Content-Type: text/plain; charset=\"UTF-8\"\r\n")
	sb.WriteString("\r\n")
	sb.WriteString(h.Body)
	return base64.RawURLEncoding.EncodeToString([]byte(sb.String()))
}
