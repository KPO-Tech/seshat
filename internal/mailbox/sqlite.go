package mailbox

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/EngineerProjects/nexus-engine/internal/db"
)

// SQLiteMailbox is a Mailbox backed by SQLite via the shared db.DB handle.
type SQLiteMailbox struct {
	db *db.DB
	// agentLister is an optional function that returns all agent IDs in a team.
	// Required for broadcast expansion. If nil, broadcasts fail with an error.
	agentLister func(ctx context.Context, teamID string) ([]string, error)
}

// New creates a SQLiteMailbox backed by the given DB.
// agentLister is called when sending a KindBroadcast message to expand
// the recipients. Pass nil to disable broadcast support.
func New(database *db.DB, agentLister func(ctx context.Context, teamID string) ([]string, error)) *SQLiteMailbox {
	return &SQLiteMailbox{db: database, agentLister: agentLister}
}

// Send delivers a message to the recipient's inbox.
// For KindBroadcast, msg.ToAgent must be "*" and msg.TeamID must be set.
func (m *SQLiteMailbox) Send(ctx context.Context, msg Message) error {
	if msg.ID == "" {
		msg.ID = uuid.New().String()
	}
	if msg.CreatedAt.IsZero() {
		msg.CreatedAt = time.Now().UTC()
	}

	if msg.Kind == KindBroadcast {
		return m.sendBroadcast(ctx, msg)
	}
	return m.db.InsertMessage(ctx, toGMessage(msg))
}

func (m *SQLiteMailbox) sendBroadcast(ctx context.Context, msg Message) error {
	if msg.TeamID == "" {
		return errors.New("broadcast message requires TeamID")
	}
	if m.agentLister == nil {
		return errors.New("broadcast not supported: no agent lister configured")
	}
	agents, err := m.agentLister(ctx, msg.TeamID)
	if err != nil {
		return fmt.Errorf("resolve broadcast recipients: %w", err)
	}
	if len(agents) == 0 {
		return nil
	}
	// Fan out: one message record per recipient.
	for _, agentID := range agents {
		if agentID == msg.FromAgent {
			continue // don't deliver to self
		}
		copy := msg
		copy.ID = uuid.New().String()
		copy.ToAgent = agentID
		if err := m.db.InsertMessage(ctx, toGMessage(copy)); err != nil {
			return fmt.Errorf("deliver broadcast to %q: %w", agentID, err)
		}
	}
	return nil
}

// Receive returns all unread messages for agentID, oldest first.
func (m *SQLiteMailbox) Receive(ctx context.Context, agentID string) ([]Message, error) {
	rows, err := m.db.GetUnreadMessages(ctx, agentID)
	if err != nil {
		return nil, err
	}
	return rowsToMessages(rows), nil
}

// MarkRead marks a single message as read.
func (m *SQLiteMailbox) MarkRead(ctx context.Context, msgID string) error {
	return m.db.MarkMessageRead(ctx, msgID)
}

// MarkAllRead marks all unread messages for agentID as read.
func (m *SQLiteMailbox) MarkAllRead(ctx context.Context, agentID string) error {
	return m.db.MarkAllMessagesRead(ctx, agentID)
}

// History returns up to limit messages for agentID, newest first.
func (m *SQLiteMailbox) History(ctx context.Context, agentID string, limit int) ([]Message, error) {
	rows, err := m.db.GetMessageHistory(ctx, agentID, limit)
	if err != nil {
		return nil, err
	}
	return rowsToMessages(rows), nil
}

// Thread returns all messages in a conversation thread rooted at rootID,
// oldest first.
func (m *SQLiteMailbox) Thread(ctx context.Context, rootID string) ([]Message, error) {
	rows, err := m.db.GetMessageThread(ctx, rootID)
	if err != nil {
		return nil, err
	}
	return rowsToMessages(rows), nil
}

// Delete removes a message permanently.
func (m *SQLiteMailbox) Delete(ctx context.Context, msgID string) error {
	return m.db.DeleteMessage(ctx, msgID)
}

// ─── conversion helpers ───────────────────────────────────────────────────────

func toGMessage(msg Message) db.GMailboxMessage {
	row := db.GMailboxMessage{
		ID:        msg.ID,
		Kind:      string(msg.Kind),
		FromAgent: msg.FromAgent,
		ToAgent:   msg.ToAgent,
		Subject:   msg.Subject,
		Body:      msg.Body,
		ReplyTo:   msg.ReplyTo,
		TeamID:    msg.TeamID,
		CreatedAt: msg.CreatedAt.Unix(),
	}
	if msg.ReadAt != nil {
		unix := msg.ReadAt.Unix()
		row.ReadAt = &unix
	}
	return row
}

func fromGMessage(row db.GMailboxMessage) Message {
	msg := Message{
		ID:        row.ID,
		Kind:      MessageKind(row.Kind),
		FromAgent: row.FromAgent,
		ToAgent:   row.ToAgent,
		Subject:   row.Subject,
		Body:      row.Body,
		ReplyTo:   row.ReplyTo,
		TeamID:    row.TeamID,
		CreatedAt: time.Unix(row.CreatedAt, 0).UTC(),
	}
	if row.ReadAt != nil {
		t := time.Unix(*row.ReadAt, 0).UTC()
		msg.ReadAt = &t
	}
	return msg
}

func rowsToMessages(rows []db.GMailboxMessage) []Message {
	msgs := make([]Message, len(rows))
	for i, r := range rows {
		msgs[i] = fromGMessage(r)
	}
	return msgs
}

// PendingCounts returns the unread message count per agent for the given IDs.
// Agents with zero unread messages are absent from the map.
func (m *SQLiteMailbox) PendingCounts(ctx context.Context, agentIDs []string) (map[string]int, error) {
	return m.db.CountUnreadByAgents(ctx, agentIDs)
}

// compile-time check: SQLiteMailbox implements Mailbox.
var _ Mailbox = (*SQLiteMailbox)(nil)
