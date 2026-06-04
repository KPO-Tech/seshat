package team_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/EngineerProjects/nexus-engine/internal/agent"
	"github.com/EngineerProjects/nexus-engine/internal/db"
	"github.com/EngineerProjects/nexus-engine/internal/mailbox"
	"github.com/EngineerProjects/nexus-engine/internal/team"
)

// ─── helpers ─────────────────────────────────────────────────────────────────

func setup(t *testing.T) (*team.Dispatcher, *agent.ProfileRegistry, *mailbox.SQLiteMailbox) {
	t.Helper()
	database, err := db.Open(context.Background(), db.Config{
		Driver:      db.DriverSQLite,
		DSN:         ":memory:",
		AutoMigrate: true,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = database.Close() })

	reg := agent.NewProfileRegistry(database)
	require.NoError(t, reg.Seed(context.Background()))

	lister := func(ctx context.Context, teamID string) ([]string, error) {
		profiles, err := reg.FindByTeam(ctx, teamID)
		if err != nil {
			return nil, err
		}
		ids := make([]string, len(profiles))
		for i, p := range profiles {
			ids[i] = p.ID
		}
		return ids, nil
	}

	mb := mailbox.New(database, lister)
	d := team.NewDispatcher(reg, mb)
	return d, reg, mb
}

// ─── Dispatcher.Send ─────────────────────────────────────────────────────────

func TestDispatcher_Send(t *testing.T) {
	ctx := context.Background()
	d, reg, mb := setup(t)

	profiles, err := reg.List(ctx)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(profiles), 2)

	from, to := profiles[0].ID, profiles[1].ID
	require.NoError(t, d.Send(ctx, from, to, "Do the thing", "Details."))

	msgs, err := mb.Receive(ctx, to)
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	assert.Equal(t, mailbox.KindTask, msgs[0].Kind)
	assert.Equal(t, from, msgs[0].FromAgent)
	assert.Equal(t, "Do the thing", msgs[0].Subject)
}

func TestDispatcher_Send_EmptyIDs(t *testing.T) {
	ctx := context.Background()
	d, _, _ := setup(t)
	assert.Error(t, d.Send(ctx, "", "to", "s", "b"))
	assert.Error(t, d.Send(ctx, "from", "", "s", "b"))
}

// ─── Dispatcher.Reply ─────────────────────────────────────────────────────────

func TestDispatcher_Reply(t *testing.T) {
	ctx := context.Background()
	d, reg, mb := setup(t)

	profiles, _ := reg.List(ctx)
	from, to := profiles[0].ID, profiles[1].ID

	require.NoError(t, d.Send(ctx, from, to, "Original task", ""))
	msgs, _ := mb.Receive(ctx, to)
	require.Len(t, msgs, 1)
	original := msgs[0]

	require.NoError(t, d.Reply(ctx, to, from, original.ID, "Re: Original task", "Done."))

	replies, err := mb.Receive(ctx, from)
	require.NoError(t, err)
	require.Len(t, replies, 1)
	assert.Equal(t, mailbox.KindReply, replies[0].Kind)
	assert.Equal(t, original.ID, replies[0].ReplyTo)
}

func TestDispatcher_Reply_EmptyReplyTo(t *testing.T) {
	ctx := context.Background()
	d, _, _ := setup(t)
	assert.Error(t, d.Reply(ctx, "a", "b", "", "s", "b"))
}

// ─── Dispatcher.Broadcast ────────────────────────────────────────────────────

func TestDispatcher_Broadcast(t *testing.T) {
	ctx := context.Background()
	d, reg, mb := setup(t)

	aria := agent.NewAgentProfile("Aria", "researcher", "")
	aria.TeamID = "alpha"
	kai := agent.NewAgentProfile("Kai", "engineer", "")
	kai.TeamID = "alpha"
	nexus := agent.NewAgentProfile("Nexus", "manager", "")
	nexus.TeamID = "alpha"
	require.NoError(t, reg.Register(ctx, aria))
	require.NoError(t, reg.Register(ctx, kai))
	require.NoError(t, reg.Register(ctx, nexus))

	require.NoError(t, d.Broadcast(ctx, nexus.ID, "alpha", "Stand-up", "Daily sync at 09:00."))

	ariaInbox, err := mb.Receive(ctx, aria.ID)
	require.NoError(t, err)
	assert.Len(t, ariaInbox, 1)

	kaiInbox, err := mb.Receive(ctx, kai.ID)
	require.NoError(t, err)
	assert.Len(t, kaiInbox, 1)

	// Sender must not receive its own broadcast.
	nexusInbox, err := mb.Receive(ctx, nexus.ID)
	require.NoError(t, err)
	assert.Empty(t, nexusInbox)
}

func TestDispatcher_Broadcast_EmptyTeamID(t *testing.T) {
	ctx := context.Background()
	d, _, _ := setup(t)
	assert.Error(t, d.Broadcast(ctx, "from", "", "s", "b"))
}

// ─── Dispatcher.Assign ───────────────────────────────────────────────────────

func TestDispatcher_Assign_ByRole(t *testing.T) {
	ctx := context.Background()
	d, reg, mb := setup(t)

	researchers, err := reg.FindByRole(ctx, "researcher")
	require.NoError(t, err)
	require.Len(t, researchers, 1)
	aria := researchers[0]

	managers, err := reg.FindByRole(ctx, "manager")
	require.NoError(t, err)
	from := managers[0].ID

	require.NoError(t, d.Assign(ctx, from, "researcher", "", "Research Go generics", ""))

	inbox, err := mb.Receive(ctx, aria.ID)
	require.NoError(t, err)
	require.Len(t, inbox, 1)
	assert.Equal(t, "Research Go generics", inbox[0].Subject)
}

func TestDispatcher_Assign_ByRoleAndTeam(t *testing.T) {
	ctx := context.Background()
	d, reg, mb := setup(t)

	r1 := agent.NewAgentProfile("Maria", "researcher", "")
	r1.TeamID = "alpha"
	r2 := agent.NewAgentProfile("Faouziath", "researcher", "")
	r2.TeamID = "beta"
	require.NoError(t, reg.Register(ctx, r1))
	require.NoError(t, reg.Register(ctx, r2))

	require.NoError(t, d.Assign(ctx, "system", "researcher", "beta", "Beta task", ""))

	r1Inbox, err := mb.Receive(ctx, r1.ID)
	require.NoError(t, err)
	assert.Empty(t, r1Inbox)

	r2Inbox, err := mb.Receive(ctx, r2.ID)
	require.NoError(t, err)
	assert.Len(t, r2Inbox, 1)
}

func TestDispatcher_Assign_NoMatch(t *testing.T) {
	ctx := context.Background()
	d, _, _ := setup(t)
	err := d.Assign(ctx, "from", "unicorn", "", "s", "b")
	require.Error(t, err)
	var noAgent *team.ErrNoAgentForRole
	assert.True(t, errors.As(err, &noAgent))
	assert.Equal(t, "unicorn", noAgent.Role)
}

// ─── TeamBus ─────────────────────────────────────────────────────────────────

func TestTeamBus_DeliversMessages(t *testing.T) {
	ctx := context.Background()
	database, err := db.Open(context.Background(), db.Config{
		Driver:      db.DriverSQLite,
		DSN:         ":memory:",
		AutoMigrate: true,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = database.Close() })

	reg := agent.NewProfileRegistry(database)
	require.NoError(t, reg.Seed(ctx))
	mb := mailbox.New(database, nil)

	var (
		mu       sync.Mutex
		received []mailbox.Message
	)
	handler := func(_ context.Context, _ agent.AgentProfile, msg mailbox.Message) {
		mu.Lock()
		received = append(received, msg)
		mu.Unlock()
	}

	bus := team.NewTeamBus(reg, mb, handler, 50*time.Millisecond)
	require.NoError(t, bus.Start(ctx))
	t.Cleanup(bus.Stop)

	researchers, err := reg.FindByRole(ctx, "researcher")
	require.NoError(t, err)
	require.Len(t, researchers, 1)

	msg := mailbox.NewMessage(mailbox.KindTask, "system", researchers[0].ID, "Research task", "Go generics.")
	require.NoError(t, mb.Send(ctx, msg))

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := len(received)
		mu.Unlock()
		if n > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, received, 1)
	assert.Equal(t, msg.ID, received[0].ID)
}

func TestTeamBus_Start_Idempotent(t *testing.T) {
	ctx := context.Background()
	database, err := db.Open(context.Background(), db.Config{
		Driver:      db.DriverSQLite,
		DSN:         ":memory:",
		AutoMigrate: true,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = database.Close() })

	reg := agent.NewProfileRegistry(database)
	require.NoError(t, reg.Seed(ctx))
	mb := mailbox.New(database, nil)

	bus := team.NewTeamBus(reg, mb, nil, 50*time.Millisecond)
	require.NoError(t, bus.Start(ctx))
	require.NoError(t, bus.Start(ctx), "second Start must be a no-op")
	bus.Stop()
}
