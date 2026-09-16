// Package gmail is a thin wrapper around the official Gmail API client
// (google.golang.org/api/gmail/v1) - unlike core/msgraph, there's a real
// SDK to build on here, so this package's job is the higher-level
// orchestration around it (history-based sync with bootstrap fallback,
// MIME parsing/translation, reply threading), not raw HTTP.
//
// Shared, tenant-agnostic logic used two ways: seshat-backend wraps it in
// a real inbox.Connector (internal/inbox/gmail) for the local Inbox
// workflow, and seshat-server wraps it in a small self-hosted MCP server
// (internal/server/gmailmcp) so the workspace chat agent can call it as a
// tool, bridged through a per-employee OAuth-connected account the same
// way every other agent_action connector kind works. Neither side stores
// or manages OAuth tokens here - callers pass an already-token-bound
// oauth2.TokenSource, mirroring core/msgraph's Client.
package gmail

import "time"

// EmailAddress is a parsed "Name <address>" (or bare address) header value.
type EmailAddress struct {
	Address string
	Name    string
}

// Attachment is one attachment's metadata, not its bytes - see
// Client.FetchAttachment for the lazy download step.
type Attachment struct {
	Filename          string
	ContentType       string
	Size              int64
	GmailAttachmentID string
}

// Message is one email, already MIME-parsed. Deliberately has no
// "direction" field - inbound vs. outbound depends on which address is
// the connected account's own, an inbox-domain concept the caller (e.g.
// inbox/gmail.translateMessage) computes itself from From/To.
type Message struct {
	ID          string
	ThreadID    string
	Subject     string
	From        EmailAddress
	To          EmailAddress
	BodyText    string
	Attachments []Attachment
	SentAt      time.Time
	// LabelIds are Gmail's own label IDs (both system, e.g. "UNREAD"/"INBOX"/
	// "TRASH", and user-created ones) - the raw source Unread and label-
	// management operations (Client.ModifyLabels et al.) read/write.
	LabelIds []string
	// Unread mirrors whether LabelIds contains "UNREAD", computed once at
	// translation time so callers don't need to know Gmail's label-based
	// read/unread convention themselves.
	Unread bool
}

// Label is a Gmail label (system or user-created) - see
// Client.ListLabels/CreateLabel/DeleteLabel.
type Label struct {
	ID   string
	Name string
}

// Draft is a saved-but-unsent message. Gmail's own Drafts.List only ever
// returns the ID/ThreadID/MessageID triple (no subject/snippet) without a
// separate per-draft Get call, so that's all this carries - see
// Client.ListDrafts's doc comment.
type Draft struct {
	ID        string
	MessageID string
	ThreadID  string
}

// Profile is the connected account's own mailbox summary.
type Profile struct {
	EmailAddress  string
	MessagesTotal int64
	ThreadsTotal  int64
}
