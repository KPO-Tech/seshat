package agent_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/EngineerProjects/nexus-engine/internal/agent"
	"github.com/EngineerProjects/nexus-engine/internal/db"
)

// openTestDB opens an in-memory SQLite database and runs all core migrations.
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

func TestProfileRegistry_Seed(t *testing.T) {
	ctx := context.Background()
	r := agent.NewProfileRegistry(openTestDB(t))

	require.NoError(t, r.Seed(ctx))

	profiles, err := r.List(ctx)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(profiles), 3, "expected at least 3 built-in profiles")

	ids := make(map[string]bool)
	for _, p := range profiles {
		ids[p.ID] = true
	}
	assert.True(t, ids["orchestrator"])
	assert.True(t, ids["researcher"])
	assert.True(t, ids["coder"])
}

func TestProfileRegistry_Seed_Idempotent(t *testing.T) {
	ctx := context.Background()
	r := agent.NewProfileRegistry(openTestDB(t))

	require.NoError(t, r.Seed(ctx))
	require.NoError(t, r.Seed(ctx), "second Seed must not fail")

	profiles, err := r.List(ctx)
	require.NoError(t, err)

	// Count occurrences of built-in IDs — must be exactly 1 each.
	counts := make(map[string]int)
	for _, p := range profiles {
		counts[p.ID]++
	}
	assert.Equal(t, 1, counts["orchestrator"])
	assert.Equal(t, 1, counts["researcher"])
	assert.Equal(t, 1, counts["coder"])
}

func TestProfileRegistry_Register_And_Get(t *testing.T) {
	ctx := context.Background()
	r := agent.NewProfileRegistry(openTestDB(t))

	p := agent.AgentProfile{
		ID:           "test-agent",
		Name:         "Test Agent",
		Role:         "tester",
		SystemPrompt: "You are a test agent.",
		Model:        "anthropic:claude-sonnet-4-20250514",
		Skills:       []string{"go-conventions"},
		Metadata:     map[string]string{"env": "ci"},
	}

	require.NoError(t, r.Register(ctx, p))

	got, err := r.Get(ctx, "test-agent")
	require.NoError(t, err)

	assert.Equal(t, p.ID, got.ID)
	assert.Equal(t, p.Name, got.Name)
	assert.Equal(t, "tester", got.Role) // stored lowercase
	assert.Equal(t, p.SystemPrompt, got.SystemPrompt)
	assert.Equal(t, p.Model, got.Model)
	assert.Equal(t, p.Skills, got.Skills)
	assert.Equal(t, p.Metadata, got.Metadata)
	assert.False(t, got.CreatedAt.IsZero())
	assert.False(t, got.UpdatedAt.IsZero())
}

func TestProfileRegistry_Register_EmptyID(t *testing.T) {
	ctx := context.Background()
	r := agent.NewProfileRegistry(openTestDB(t))

	err := r.Register(ctx, agent.AgentProfile{Name: "no id"})
	assert.Error(t, err)
}

func TestProfileRegistry_Register_Upsert(t *testing.T) {
	ctx := context.Background()
	r := agent.NewProfileRegistry(openTestDB(t))

	original := agent.AgentProfile{
		ID:           "updatable",
		Name:         "Original",
		Role:         "worker",
		SystemPrompt: "v1",
	}
	require.NoError(t, r.Register(ctx, original))

	updated := original
	updated.Name = "Updated"
	updated.SystemPrompt = "v2"
	require.NoError(t, r.Register(ctx, updated))

	got, err := r.Get(ctx, "updatable")
	require.NoError(t, err)
	assert.Equal(t, "Updated", got.Name)
	assert.Equal(t, "v2", got.SystemPrompt)
}

func TestProfileRegistry_Get_NotFound(t *testing.T) {
	ctx := context.Background()
	r := agent.NewProfileRegistry(openTestDB(t))

	_, err := r.Get(ctx, "does-not-exist")
	assert.True(t, errors.Is(err, db.ErrProfileNotFound))
}

func TestProfileRegistry_FindByRole(t *testing.T) {
	ctx := context.Background()
	r := agent.NewProfileRegistry(openTestDB(t))
	require.NoError(t, r.Seed(ctx))

	engineers, err := r.FindByRole(ctx, "engineer")
	require.NoError(t, err)
	require.Len(t, engineers, 1)
	assert.Equal(t, "coder", engineers[0].ID)

	// Case-insensitive.
	managers, err := r.FindByRole(ctx, "MANAGER")
	require.NoError(t, err)
	require.Len(t, managers, 1)
	assert.Equal(t, "orchestrator", managers[0].ID)
}

func TestProfileRegistry_Delete(t *testing.T) {
	ctx := context.Background()
	r := agent.NewProfileRegistry(openTestDB(t))

	p := agent.AgentProfile{ID: "temp", Name: "Temp", Role: "x", SystemPrompt: "x"}
	require.NoError(t, r.Register(ctx, p))

	require.NoError(t, r.Delete(ctx, "temp"))

	_, err := r.Get(ctx, "temp")
	assert.True(t, errors.Is(err, db.ErrProfileNotFound))
}

func TestProfileRegistry_Delete_NoOp(t *testing.T) {
	ctx := context.Background()
	r := agent.NewProfileRegistry(openTestDB(t))

	// Deleting a non-existent profile must not error.
	assert.NoError(t, r.Delete(ctx, "ghost"))
}

func TestProfileRegistry_List(t *testing.T) {
	ctx := context.Background()
	r := agent.NewProfileRegistry(openTestDB(t))
	require.NoError(t, r.Seed(ctx))

	extra := agent.AgentProfile{ID: "zzz-extra", Name: "Extra", Role: "misc", SystemPrompt: "x"}
	require.NoError(t, r.Register(ctx, extra))

	profiles, err := r.List(ctx)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(profiles), 4)

	// Verify ordering by ID.
	for i := 1; i < len(profiles); i++ {
		assert.LessOrEqual(t, profiles[i-1].ID, profiles[i].ID)
	}
}
