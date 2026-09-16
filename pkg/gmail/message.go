package gmail

import (
	"encoding/base64"
	"fmt"
	"net/mail"
	"regexp"
	"strings"
	"time"

	gmailapi "google.golang.org/api/gmail/v1"
)

// translateRawMessage converts a fully-populated (format=full) Gmail
// message into this package's own Message shape - MIME parsing only, no
// direction/contact resolution (see Message's doc comment).
func translateRawMessage(msg *gmailapi.Message) (Message, error) {
	if msg == nil || msg.Payload == nil {
		return Message{}, fmt.Errorf("message has no payload")
	}
	headers := msg.Payload.Headers

	from := parseFirstAddress(headerValue(headers, "From"))
	to := parseFirstAddress(headerValue(headers, "To"))
	subject := stripReplyPrefix(headerValue(headers, "Subject"))

	bodyText, attachments := extractBodyAndAttachments(msg.Payload)
	if bodyText == "" {
		bodyText = msg.Snippet
	}

	return Message{
		ID:          msg.Id,
		ThreadID:    msg.ThreadId,
		Subject:     subject,
		From:        EmailAddress{Address: from.Address, Name: from.Name},
		To:          EmailAddress{Address: to.Address, Name: to.Name},
		BodyText:    bodyText,
		Attachments: attachments,
		SentAt:      time.UnixMilli(msg.InternalDate),
		LabelIds:    msg.LabelIds,
		Unread:      hasLabel(msg.LabelIds, "UNREAD"),
	}, nil
}

func hasLabel(labelIds []string, target string) bool {
	for _, id := range labelIds {
		if id == target {
			return true
		}
	}
	return false
}

func headerValue(headers []*gmailapi.MessagePartHeader, name string) string {
	for _, h := range headers {
		if strings.EqualFold(h.Name, name) {
			return h.Value
		}
	}
	return ""
}

// parseFirstAddress extracts the first address out of a header value that
// may contain a display name ("Jane Doe <jane@example.com>") or a bare
// address - returns a zero mail.Address if the header is empty or
// unparseable, so callers can check .Address == "" rather than handling an
// error for what's usually just a missing header.
func parseFirstAddress(headerVal string) mail.Address {
	if strings.TrimSpace(headerVal) == "" {
		return mail.Address{}
	}
	addrs, err := mail.ParseAddressList(headerVal)
	if err != nil || len(addrs) == 0 {
		// Some real-world senders send slightly malformed headers that
		// net/mail rejects outright (e.g. an unquoted display name with a
		// comma) - falling back to ParseAddress on just the first
		// comma-delimited segment recovers the common case instead of
		// discarding the whole message's contact identity.
		first := strings.TrimSpace(strings.Split(headerVal, ",")[0])
		if a, err := mail.ParseAddress(first); err == nil {
			return *a
		}
		return mail.Address{}
	}
	return *addrs[0]
}

var replyPrefixRe = regexp.MustCompile(`(?i)^(re|fwd?)\s*:\s*`)

// stripReplyPrefix removes one leading "Re:"/"Fwd:" so a thread's stored
// subject stays stable across a whole back-and-forth instead of
// accumulating "Re: Re: Re: ..." as each reply's header gets stored.
func stripReplyPrefix(subject string) string {
	return replyPrefixRe.ReplaceAllString(strings.TrimSpace(subject), "")
}

// extractBodyAndAttachments walks a (possibly multipart) message payload
// depth-first, preferring the first text/plain part it finds for the body
// and falling back to a crude tag-strip of text/html if no plain part
// exists - Gmail always sends at least one of the two for a real email.
// Any part with a Filename is collected as an attachment reference - not
// downloaded here (see Client.FetchAttachment, called on demand via
// GmailAttachmentID the first time a user actually downloads it).
func extractBodyAndAttachments(part *gmailapi.MessagePart) (string, []Attachment) {
	var plainText, htmlText string
	var attachments []Attachment

	var walk func(p *gmailapi.MessagePart)
	walk = func(p *gmailapi.MessagePart) {
		if p == nil {
			return
		}
		if p.Filename != "" {
			attachments = append(attachments, Attachment{
				Filename:    p.Filename,
				ContentType: p.MimeType,
				Size:        bodySize(p.Body),
				GmailAttachmentID: func() string {
					if p.Body != nil {
						return p.Body.AttachmentId
					}
					return ""
				}(),
			})
			return
		}
		switch p.MimeType {
		case "text/plain":
			if plainText == "" {
				plainText = decodeBody(p.Body)
			}
			return
		case "text/html":
			if htmlText == "" {
				htmlText = decodeBody(p.Body)
			}
			return
		}
		for _, child := range p.Parts {
			walk(child)
		}
	}
	walk(part)

	if plainText != "" {
		return strings.TrimSpace(plainText), attachments
	}
	if htmlText != "" {
		return strings.TrimSpace(stripHTML(htmlText)), attachments
	}
	return "", attachments
}

func bodySize(body *gmailapi.MessagePartBody) int64 {
	if body == nil {
		return 0
	}
	return body.Size
}

func decodeBody(body *gmailapi.MessagePartBody) string {
	if body == nil || body.Data == "" {
		return ""
	}
	// Gmail encodes part bodies as unpadded base64url (RFC 4648 §5).
	decoded, err := base64.RawURLEncoding.DecodeString(body.Data)
	if err != nil {
		return ""
	}
	return string(decoded)
}

var (
	htmlTagRe   = regexp.MustCompile(`(?s)<[^>]*>`)
	htmlSpaceRe = regexp.MustCompile(`[ \t]+`)
)

// stripHTML is a deliberately crude fallback, not a real HTML renderer -
// only used when a message has no text/plain part at all.
func stripHTML(htmlText string) string {
	text := htmlTagRe.ReplaceAllString(htmlText, " ")
	text = htmlSpaceRe.ReplaceAllString(text, " ")
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimSpace(line)
	}
	return strings.Join(lines, "\n")
}
