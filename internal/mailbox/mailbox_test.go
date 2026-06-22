package mailbox_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/EngineerProjects/seshat/internal/db"
	"github.com/EngineerProjects/seshat/internal/mailbox"
)

func openTestDB(t *testing.T) *db.DB {
	t.Helper()
	database, err := db.Open(context.Background(), db.Config{
		Driver:      db.DriverSQLite,
		DSN:         ":memory:",
		AutoMigrate: true,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = database.Close() })
	return database
}

func newMailbox(t *testing.T) *mailbox.SQLiteMailbox {
	t.Helper()
	return mailbox.New(openTestDB(t), nil)
}

// ─── NewMessage ───────────────────────────────────────────────────────────────

func TestNewMessage_HasUUID(t *testing.T) {
	m1 := mailbox.NewMessage(mailbox.KindTask, "alice", "bob", "sub", "body")
	m2 := mailbox.NewMessage(mailbox.KindTask, "alice", "bob", "sub", "body")
	assert.Len(t, m1.ID, 36)
	assert.NotEqual(t, m1.ID, m2.ID)
	assert.False(t, m1.CreatedAt.IsZero())
}

// ─── Send / Receive ───────────────────────────────────────────────────────────

func TestSend_And_Receive(t *testing.T) {
	ctx := context.Background()
	mb := newMailbox(t)

	msg := mailbox.NewMessage(mailbox.KindTask, "aria", "kai", "Fix the bug", "Details here.")
	require.NoError(t, mb.Send(ctx, msg))

	msgs, err := mb.Receive(ctx, "kai")
	require.NoError(t, err)
	require.Len(t, msgs, 1)

	got := msgs[0]
	assert.Equal(t, msg.ID, got.ID)
	assert.Equal(t, mailbox.KindTask, got.Kind)
	assert.Equal(t, "aria", got.FromAgent)
	assert.Equal(t, "kai", got.ToAgent)
	assert.Equal(t, "Fix the bug", got.Subject)
	assert.Equal(t, "Details here.", got.Body)
	assert.Nil(t, got.ReadAt)
}

func TestReceive_OnlyUnread(t *testing.T) {
	ctx := context.Background()
	mb := newMailbox(t)

	m1 := mailbox.NewMessage(mailbox.KindTask, "aria", "kai", "Task 1", "")
	m2 := mailbox.NewMessage(mailbox.KindTask, "aria", "kai", "Task 2", "")
	require.NoError(t, mb.Send(ctx, m1))
	require.NoError(t, mb.Send(ctx, m2))

	require.NoError(t, mb.MarkRead(ctx, m1.ID))

	unread, err := mb.Receive(ctx, "kai")
	require.NoError(t, err)
	require.Len(t, unread, 1)
	assert.Equal(t, m2.ID, unread[0].ID)
}

func TestReceive_OldestFirst(t *testing.T) {
	ctx := context.Background()
	mb := newMailbox(t)

	for i := 0; i < 3; i++ {
		m := mailbox.NewMessage(mailbox.KindTask, "aria", "kai", "msg", "")
		m.CreatedAt = time.Now().UTC().Add(time.Duration(i) * time.Second)
		require.NoError(t, mb.Send(ctx, m))
	}

	msgs, err := mb.Receive(ctx, "kai")
	require.NoError(t, err)
	require.Len(t, msgs, 3)
	for i := 1; i < len(msgs); i++ {
		assert.True(t, !msgs[i].CreatedAt.Before(msgs[i-1].CreatedAt))
	}
}

// ─── MarkRead / MarkAllRead ───────────────────────────────────────────────────

func TestMarkRead(t *testing.T) {
	ctx := context.Background()
	mb := newMailbox(t)

	msg := mailbox.NewMessage(mailbox.KindTask, "aria", "kai", "s", "b")
	require.NoError(t, mb.Send(ctx, msg))
	require.NoError(t, mb.MarkRead(ctx, msg.ID))

	unread, err := mb.Receive(ctx, "kai")
	require.NoError(t, err)
	assert.Empty(t, unread)
}

func TestMarkAllRead(t *testing.T) {
	ctx := context.Background()
	mb := newMailbox(t)

	for i := 0; i < 4; i++ {
		require.NoError(t, mb.Send(ctx, mailbox.NewMessage(mailbox.KindTask, "aria", "kai", "s", "b")))
	}

	require.NoError(t, mb.MarkAllRead(ctx, "kai"))

	unread, err := mb.Receive(ctx, "kai")
	require.NoError(t, err)
	assert.Empty(t, unread)
}

// ─── History ──────────────────────────────────────────────────────────────────

func TestHistory_NewestFirst(t *testing.T) {
	ctx := context.Background()
	mb := newMailbox(t)

	for i := 0; i < 5; i++ {
		m := mailbox.NewMessage(mailbox.KindTask, "aria", "kai", "s", "b")
		m.CreatedAt = time.Now().UTC().Add(time.Duration(i) * time.Second)
		require.NoError(t, mb.Send(ctx, m))
	}

	hist, err := mb.History(ctx, "kai", 3)
	require.NoError(t, err)
	require.Len(t, hist, 3)
	for i := 1; i < len(hist); i++ {
		assert.True(t, !hist[i].CreatedAt.After(hist[i-1].CreatedAt))
	}
}

func TestHistory_IncludesReadMessages(t *testing.T) {
	ctx := context.Background()
	mb := newMailbox(t)

	msg := mailbox.NewMessage(mailbox.KindTask, "aria", "kai", "s", "b")
	require.NoError(t, mb.Send(ctx, msg))
	require.NoError(t, mb.MarkRead(ctx, msg.ID))

	hist, err := mb.History(ctx, "kai", 10)
	require.NoError(t, err)
	assert.Len(t, hist, 1)
}

// ─── Thread ───────────────────────────────────────────────────────────────────

func TestThread(t *testing.T) {
	ctx := context.Background()
	mb := newMailbox(t)

	root := mailbox.NewMessage(mailbox.KindTask, "seshat", "aria", "Research Go generics", "")
	require.NoError(t, mb.Send(ctx, root))

	reply := mailbox.NewMessage(mailbox.KindReply, "aria", "seshat", "Re: Research Go generics", "Done.")
	reply.ReplyTo = root.ID
	require.NoError(t, mb.Send(ctx, reply))

	unrelated := mailbox.NewMessage(mailbox.KindTask, "seshat", "kai", "Other task", "")
	require.NoError(t, mb.Send(ctx, unrelated))

	thread, err := mb.Thread(ctx, root.ID)
	require.NoError(t, err)
	require.Len(t, thread, 2)
	assert.Equal(t, root.ID, thread[0].ID)
	assert.Equal(t, reply.ID, thread[1].ID)
}

// ─── Delete ───────────────────────────────────────────────────────────────────

func TestDelete(t *testing.T) {
	ctx := context.Background()
	mb := newMailbox(t)

	msg := mailbox.NewMessage(mailbox.KindTask, "aria", "kai", "s", "b")
	require.NoError(t, mb.Send(ctx, msg))
	require.NoError(t, mb.Delete(ctx, msg.ID))

	msgs, err := mb.Receive(ctx, "kai")
	require.NoError(t, err)
	assert.Empty(t, msgs)
}

func TestDelete_NotFound(t *testing.T) {
	ctx := context.Background()
	mb := newMailbox(t)
	err := mb.Delete(ctx, "00000000-0000-0000-0000-000000000000")
	assert.ErrorIs(t, err, db.ErrMessageNotFound)
}

// ─── Broadcast ────────────────────────────────────────────────────────────────

func TestBroadcast(t *testing.T) {
	ctx := context.Background()

	team := map[string]bool{"aria": true, "kai": true, "seshat": true}
	lister := func(_ context.Context, teamID string) ([]string, error) {
		if teamID == "alpha" {
			return []string{"aria", "kai", "seshat"}, nil
		}
		return nil, nil
	}
	mb := mailbox.New(openTestDB(t), lister)

	msg := mailbox.NewMessage(mailbox.KindBroadcast, "seshat", "*", "Stand-up", "Daily stand-up at 09:00.")
	msg.TeamID = "alpha"
	require.NoError(t, mb.Send(ctx, msg))

	// seshat is the sender — should NOT receive its own broadcast.
	delete(team, "seshat")

	for agentID := range team {
		msgs, err := mb.Receive(ctx, agentID)
		require.NoError(t, err)
		require.Len(t, msgs, 1, "agent %s should have 1 unread message", agentID)
		assert.Equal(t, mailbox.KindBroadcast, msgs[0].Kind)
		assert.Equal(t, "seshat", msgs[0].FromAgent)
		assert.Equal(t, agentID, msgs[0].ToAgent)
	}
}

func TestBroadcast_NoLister(t *testing.T) {
	ctx := context.Background()
	mb := mailbox.New(openTestDB(t), nil)

	msg := mailbox.NewMessage(mailbox.KindBroadcast, "seshat", "*", "s", "b")
	msg.TeamID = "alpha"
	err := mb.Send(ctx, msg)
	assert.Error(t, err)
}

func TestBroadcast_RequiresTeamID(t *testing.T) {
	ctx := context.Background()
	lister := func(_ context.Context, _ string) ([]string, error) { return nil, nil }
	mb := mailbox.New(openTestDB(t), lister)

	msg := mailbox.NewMessage(mailbox.KindBroadcast, "seshat", "*", "s", "b")
	// TeamID intentionally empty
	err := mb.Send(ctx, msg)
	assert.Error(t, err)
}

// ─── Isolation between agents ─────────────────────────────────────────────────

func TestInbox_Isolation(t *testing.T) {
	ctx := context.Background()
	mb := newMailbox(t)

	require.NoError(t, mb.Send(ctx, mailbox.NewMessage(mailbox.KindTask, "seshat", "aria", "for aria", "")))
	require.NoError(t, mb.Send(ctx, mailbox.NewMessage(mailbox.KindTask, "seshat", "kai", "for kai", "")))

	ariaMsgs, err := mb.Receive(ctx, "aria")
	require.NoError(t, err)
	require.Len(t, ariaMsgs, 1)
	assert.Equal(t, "for aria", ariaMsgs[0].Subject)

	kaiMsgs, err := mb.Receive(ctx, "kai")
	require.NoError(t, err)
	require.Len(t, kaiMsgs, 1)
	assert.Equal(t, "for kai", kaiMsgs[0].Subject)
}
