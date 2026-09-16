package gmail

import (
	"encoding/base64"
	"testing"

	gmailapi "google.golang.org/api/gmail/v1"
)

func b64(s string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(s))
}

func header(name, value string) *gmailapi.MessagePartHeader {
	return &gmailapi.MessagePartHeader{Name: name, Value: value}
}

func TestTranslateRawMessage_PlainText(t *testing.T) {
	msg := &gmailapi.Message{
		Id:           "msg-1",
		ThreadId:     "thread-1",
		InternalDate: 1700000000000,
		Snippet:      "fallback snippet",
		Payload: &gmailapi.MessagePart{
			MimeType: "text/plain",
			Headers: []*gmailapi.MessagePartHeader{
				header("From", "Jane Client <jane@example.com>"),
				header("To", "owner@restaurant.com"),
				header("Subject", "Table for 4 tonight?"),
			},
			Body: &gmailapi.MessagePartBody{Data: b64("Is there space for 4 tonight?")},
		},
	}

	m, err := translateRawMessage(msg)
	if err != nil {
		t.Fatalf("translateRawMessage: %v", err)
	}
	if m.From.Address != "jane@example.com" || m.From.Name != "Jane Client" {
		t.Errorf("expected From jane@example.com/Jane Client, got %+v", m.From)
	}
	if m.To.Address != "owner@restaurant.com" {
		t.Errorf("expected To owner@restaurant.com, got %+v", m.To)
	}
	if m.BodyText != "Is there space for 4 tonight?" {
		t.Errorf("unexpected body: %q", m.BodyText)
	}
	if m.Subject != "Table for 4 tonight?" {
		t.Errorf("unexpected subject: %q", m.Subject)
	}
	if m.ThreadID != "thread-1" || m.ID != "msg-1" {
		t.Errorf("unexpected IDs: thread=%q message=%q", m.ThreadID, m.ID)
	}
	if m.SentAt.UnixMilli() != 1700000000000 {
		t.Errorf("unexpected SentAt: %v", m.SentAt)
	}
}

func TestTranslateRawMessage_StripsReplyPrefixFromSubject(t *testing.T) {
	msg := &gmailapi.Message{
		Id:       "msg-2",
		ThreadId: "thread-1",
		Payload: &gmailapi.MessagePart{
			MimeType: "text/plain",
			Headers: []*gmailapi.MessagePartHeader{
				header("From", "owner@restaurant.com"),
				header("To", "jane@example.com"),
				header("Subject", "Re: Table for 4 tonight?"),
			},
			Body: &gmailapi.MessagePartBody{Data: b64("Yes, see you at 8pm!")},
		},
	}
	m, err := translateRawMessage(msg)
	if err != nil {
		t.Fatalf("translateRawMessage: %v", err)
	}
	if m.Subject != "Table for 4 tonight?" {
		t.Errorf("expected Re: prefix stripped, got %q", m.Subject)
	}
}

func TestTranslateRawMessage_MultipartPrefersPlainTextOverHTML(t *testing.T) {
	msg := &gmailapi.Message{
		Id:       "msg-3",
		ThreadId: "thread-2",
		Payload: &gmailapi.MessagePart{
			MimeType: "multipart/alternative",
			Headers: []*gmailapi.MessagePartHeader{
				header("From", "jane@example.com"),
				header("To", "owner@restaurant.com"),
			},
			Parts: []*gmailapi.MessagePart{
				{MimeType: "text/html", Body: &gmailapi.MessagePartBody{Data: b64("<p>Hi <b>there</b></p>")}},
				{MimeType: "text/plain", Body: &gmailapi.MessagePartBody{Data: b64("Hi there")}},
			},
		},
	}
	m, err := translateRawMessage(msg)
	if err != nil {
		t.Fatalf("translateRawMessage: %v", err)
	}
	if m.BodyText != "Hi there" {
		t.Errorf("expected plain text part to win, got %q", m.BodyText)
	}
}

func TestTranslateRawMessage_FallsBackToHTMLWhenNoPlainTextPart(t *testing.T) {
	msg := &gmailapi.Message{
		Id:       "msg-4",
		ThreadId: "thread-3",
		Payload: &gmailapi.MessagePart{
			MimeType: "text/html",
			Headers: []*gmailapi.MessagePartHeader{
				header("From", "jane@example.com"),
				header("To", "owner@restaurant.com"),
			},
			Body: &gmailapi.MessagePartBody{Data: b64("<p>Hello <b>World</b></p>")},
		},
	}
	m, err := translateRawMessage(msg)
	if err != nil {
		t.Fatalf("translateRawMessage: %v", err)
	}
	if m.BodyText != "Hello World" {
		t.Errorf("expected stripped HTML fallback, got %q", m.BodyText)
	}
}

func TestTranslateRawMessage_FallsBackToSnippetWhenNoBodyAtAll(t *testing.T) {
	msg := &gmailapi.Message{
		Id:       "msg-5",
		ThreadId: "thread-4",
		Snippet:  "a short preview",
		Payload: &gmailapi.MessagePart{
			MimeType: "application/octet-stream",
			Headers: []*gmailapi.MessagePartHeader{
				header("From", "jane@example.com"),
				header("To", "owner@restaurant.com"),
			},
		},
	}
	m, err := translateRawMessage(msg)
	if err != nil {
		t.Fatalf("translateRawMessage: %v", err)
	}
	if m.BodyText != "a short preview" {
		t.Errorf("expected snippet fallback, got %q", m.BodyText)
	}
}

func TestTranslateRawMessage_CollectsAttachmentMetadata(t *testing.T) {
	msg := &gmailapi.Message{
		Id:       "msg-6",
		ThreadId: "thread-5",
		Payload: &gmailapi.MessagePart{
			MimeType: "multipart/mixed",
			Headers: []*gmailapi.MessagePartHeader{
				header("From", "jane@example.com"),
				header("To", "owner@restaurant.com"),
			},
			Parts: []*gmailapi.MessagePart{
				{MimeType: "text/plain", Body: &gmailapi.MessagePartBody{Data: b64("See attached invoice.")}},
				{
					MimeType: "application/pdf",
					Filename: "invoice.pdf",
					Body:     &gmailapi.MessagePartBody{AttachmentId: "att-1", Size: 12345},
				},
			},
		},
	}
	m, err := translateRawMessage(msg)
	if err != nil {
		t.Fatalf("translateRawMessage: %v", err)
	}
	if len(m.Attachments) != 1 {
		t.Fatalf("expected exactly 1 attachment, got %d", len(m.Attachments))
	}
	att := m.Attachments[0]
	if att.Filename != "invoice.pdf" {
		t.Errorf("expected filename %q, got %q", "invoice.pdf", att.Filename)
	}
	if att.GmailAttachmentID != "att-1" {
		t.Errorf("expected gmail_attachment_id %q, got %q", "att-1", att.GmailAttachmentID)
	}
	if att.Size != 12345 {
		t.Errorf("expected size 12345, got %d", att.Size)
	}
	if att.ContentType != "application/pdf" {
		t.Errorf("expected content type %q, got %q", "application/pdf", att.ContentType)
	}
}

func TestParseFirstAddress(t *testing.T) {
	cases := []struct {
		in       string
		wantAddr string
		wantName string
	}{
		{"jane@example.com", "jane@example.com", ""},
		{"Jane Doe <jane@example.com>", "jane@example.com", "Jane Doe"},
		{"", "", ""},
		{"Jane Doe <jane@example.com>, John <john@example.com>", "jane@example.com", "Jane Doe"},
	}
	for _, c := range cases {
		got := parseFirstAddress(c.in)
		if got.Address != c.wantAddr || got.Name != c.wantName {
			t.Errorf("parseFirstAddress(%q) = %+v, want addr=%q name=%q", c.in, got, c.wantAddr, c.wantName)
		}
	}
}

func TestStripReplyPrefix(t *testing.T) {
	cases := map[string]string{
		"Re: Table for 4":     "Table for 4",
		"RE: Table for 4":     "Table for 4",
		"Fwd: Table for 4":    "Table for 4",
		"Fw: Table for 4":     "Table for 4",
		"Table for 4":         "Table for 4",
		"Re: Re: Table for 4": "Re: Table for 4", // only strips one leading prefix per call
	}
	for in, want := range cases {
		if got := stripReplyPrefix(in); got != want {
			t.Errorf("stripReplyPrefix(%q) = %q, want %q", in, got, want)
		}
	}
}
