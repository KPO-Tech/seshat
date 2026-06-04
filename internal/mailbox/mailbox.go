// Package mailbox provides a persistent, async inbox for each agent.
// Agents communicate by sending typed messages to each other's mailbox;
// the recipient processes them independently without needing to be online
// at the same time as the sender.
package mailbox

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// MessageKind classifies the intent of a message.
type MessageKind string

const (
	// KindTask delegates a unit of work to another agent.
	KindTask MessageKind = "task"
	// KindReply responds to a previous message (linked via ReplyTo).
	KindReply MessageKind = "reply"
	// KindBroadcast is a fan-out message sent to all agents in a team.
	// The Mailbox implementation expands it to individual per-recipient records.
	KindBroadcast MessageKind = "broadcast"
	// KindEvent carries a system notification (agent started, finished, errored).
	KindEvent MessageKind = "event"
)

// Message is a single item in an agent's inbox.
type Message struct {
	// ID is a UUID generated at send time.
	ID string `json:"id"`

	// Kind classifies the message intent.
	Kind MessageKind `json:"kind"`

	// FromAgent is the sender's agent UUID (or "system" for engine events).
	FromAgent string `json:"from_agent"`

	// ToAgent is the recipient's agent UUID.
	// For broadcast messages this is the expanded individual recipient.
	ToAgent string `json:"to_agent"`

	// Subject is a short one-line summary of the message.
	Subject string `json:"subject"`

	// Body is the full message content (plain text or markdown).
	Body string `json:"body"`

	// ReplyTo is the ID of the parent message when Kind == KindReply.
	ReplyTo string `json:"reply_to,omitempty"`

	// TeamID scopes the message to a team (optional).
	TeamID string `json:"team_id,omitempty"`

	CreatedAt time.Time  `json:"created_at"`
	ReadAt    *time.Time `json:"read_at,omitempty"`
}

// NewMessage creates a Message with a fresh UUID and the current timestamp.
func NewMessage(kind MessageKind, from, to, subject, body string) Message {
	return Message{
		ID:        uuid.New().String(),
		Kind:      kind,
		FromAgent: from,
		ToAgent:   to,
		Subject:   subject,
		Body:      body,
		CreatedAt: time.Now().UTC(),
	}
}

// Mailbox is the interface for sending and receiving agent messages.
type Mailbox interface {
	// Send delivers a message to the recipient's inbox.
	// For KindBroadcast, ToAgent must be "*" and TeamID must be set;
	// the implementation expands it to all agents in that team.
	Send(ctx context.Context, msg Message) error

	// Receive returns all unread messages for agentID, oldest first.
	Receive(ctx context.Context, agentID string) ([]Message, error)

	// MarkRead marks a single message as read.
	MarkRead(ctx context.Context, msgID string) error

	// MarkAllRead marks all unread messages for agentID as read.
	MarkAllRead(ctx context.Context, agentID string) error

	// History returns up to limit messages for agentID (read and unread),
	// newest first.
	History(ctx context.Context, agentID string, limit int) ([]Message, error)

	// Thread returns all messages in a conversation thread rooted at rootID.
	Thread(ctx context.Context, rootID string) ([]Message, error)

	// Delete removes a message permanently.
	Delete(ctx context.Context, msgID string) error
}
