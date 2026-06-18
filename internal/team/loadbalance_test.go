package team_test

import (
	"context"
	"testing"

	"github.com/EngineerProjects/nexus-engine/internal/agent"
	"github.com/EngineerProjects/nexus-engine/internal/db"
	"github.com/EngineerProjects/nexus-engine/internal/mailbox"
	"github.com/EngineerProjects/nexus-engine/internal/team"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// lbSetup creates an in-memory DB, seeds built-in profiles, and returns a
// Dispatcher + SQLiteMailbox wired together.
func lbSetup(t *testing.T) (*team.Dispatcher, *agent.ProfileRegistry, *mailbox.SQLiteMailbox) {
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

// TestAssign_RoutesToLeastLoaded verifies that Assign sends to the agent with
// the fewest unread messages when multiple agents share the same role.
func TestAssign_RoutesToLeastLoaded(t *testing.T) {
	ctx := context.Background()
	d, reg, mb := lbSetup(t)

	// Use a custom role absent from built-in profiles so there are exactly
	// two candidates and no interference from seeded agents.
	r1 := agent.NewAgentProfile("Busy", "analyst", "")
	r2 := agent.NewAgentProfile("Free", "analyst", "")
	require.NoError(t, reg.Register(ctx, r1))
	require.NoError(t, reg.Register(ctx, r2))

	// Pre-load r1's inbox with 20 pending tasks — enough to stay ahead
	// even after several new tasks accumulate in r2.
	for i := 0; i < 20; i++ {
		msg := mailbox.NewMessage(mailbox.KindTask, "system", r1.ID, "pre-load", "")
		require.NoError(t, mb.Send(ctx, msg))
	}

	// Assign 5 tasks — all should go to r2 since r2.pending (0…4) < r1.pending (20).
	for i := 0; i < 5; i++ {
		require.NoError(t, d.Assign(ctx, "manager", "analyst", "", "task", "body"))
	}

	r1Msgs, err := mb.Receive(ctx, r1.ID)
	require.NoError(t, err)
	r2Msgs, err := mb.Receive(ctx, r2.ID)
	require.NoError(t, err)

	assert.Len(t, r1Msgs, 20, "busy agent must not receive new tasks while another is free")
	assert.Len(t, r2Msgs, 5, "free agent must receive all new tasks")
}

// TestAssign_SpreadsTiedAgents verifies that when all candidates have equal
// load, tasks are distributed (not all to the first agent).
func TestAssign_SpreadsTiedAgents(t *testing.T) {
	ctx := context.Background()
	d, reg, mb := lbSetup(t)

	// Three agents with a custom role (not used by built-in profiles) so
	// exactly three candidates exist and Kai doesn't absorb tasks.
	e1 := agent.NewAgentProfile("E1", "qa", "")
	e2 := agent.NewAgentProfile("E2", "qa", "")
	e3 := agent.NewAgentProfile("E3", "qa", "")
	require.NoError(t, reg.Register(ctx, e1))
	require.NoError(t, reg.Register(ctx, e2))
	require.NoError(t, reg.Register(ctx, e3))

	const total = 300
	for i := 0; i < total; i++ {
		require.NoError(t, d.Assign(ctx, "manager", "qa", "", "t", "b"))
		// Mark all messages read after each assign so counts stay equal
		// and the tiebreak logic is exercised every iteration.
		_ = mb.MarkAllRead(ctx, e1.ID)
		_ = mb.MarkAllRead(ctx, e2.ID)
		_ = mb.MarkAllRead(ctx, e3.ID)
	}

	// Each agent should have received roughly total/3 tasks.
	// Allow ±25% tolerance to avoid flakiness from randomness.
	e1h, _ := mb.History(ctx, e1.ID, 0)
	e2h, _ := mb.History(ctx, e2.ID, 0)
	e3h, _ := mb.History(ctx, e3.ID, 0)

	assert.Equal(t, total, len(e1h)+len(e2h)+len(e3h), "total task count must be correct")

	threshold := total / 3 / 4 // 25 % of equal share
	assert.InDelta(t, total/3, len(e1h), float64(threshold), "E1 should get roughly equal share")
	assert.InDelta(t, total/3, len(e2h), float64(threshold), "E2 should get roughly equal share")
	assert.InDelta(t, total/3, len(e3h), float64(threshold), "E3 should get roughly equal share")
}

// TestPendingCounts verifies the mailbox counts unread messages correctly.
func TestPendingCounts(t *testing.T) {
	ctx := context.Background()
	_, reg, mb := lbSetup(t)

	a := agent.NewAgentProfile("A", "researcher", "")
	b := agent.NewAgentProfile("B", "researcher", "")
	require.NoError(t, reg.Register(ctx, a))
	require.NoError(t, reg.Register(ctx, b))

	for i := 0; i < 4; i++ {
		require.NoError(t, mb.Send(ctx, mailbox.NewMessage(mailbox.KindTask, "sys", a.ID, "t", "")))
	}
	require.NoError(t, mb.Send(ctx, mailbox.NewMessage(mailbox.KindTask, "sys", b.ID, "t", "")))

	counts, err := mb.PendingCounts(ctx, []string{a.ID, b.ID})
	require.NoError(t, err)
	assert.Equal(t, 4, counts[a.ID])
	assert.Equal(t, 1, counts[b.ID])

	// After marking a's messages read, a should drop to 0 (absent from map).
	require.NoError(t, mb.MarkAllRead(ctx, a.ID))
	counts, err = mb.PendingCounts(ctx, []string{a.ID, b.ID})
	require.NoError(t, err)
	assert.Equal(t, 0, counts[a.ID], "absent key must default to 0")
	assert.Equal(t, 1, counts[b.ID])
}
