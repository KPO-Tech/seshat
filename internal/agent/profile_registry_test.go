package agent_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/EngineerProjects/nexus-engine/internal/agent"
	"github.com/EngineerProjects/nexus-engine/internal/db"
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

// ─── NewAgentProfile ─────────────────────────────────────────────────────────

func TestNewAgentProfile_GeneratesUUID(t *testing.T) {
	p1 := agent.NewAgentProfile("Maria", "researcher", "template")
	p2 := agent.NewAgentProfile("Maria", "researcher", "template")
	assert.NotEmpty(t, p1.ID)
	assert.NotEmpty(t, p2.ID)
	assert.NotEqual(t, p1.ID, p2.ID, "each call must produce a unique ID")
	assert.Len(t, p1.ID, 36, "ID should be a standard UUID string")
}

// ─── SystemPrompt ─────────────────────────────────────────────────────────────

func TestSystemPrompt_InjectsNickname(t *testing.T) {
	p := agent.NewAgentProfile("Faouziath", "researcher", "You specialise in academic papers about {{.Nickname}}.")
	prompt := p.SystemPrompt()
	assert.Contains(t, prompt, "You are Faouziath, a researcher.")
	assert.Contains(t, prompt, "You specialise in academic papers about Faouziath.")
}

func TestSystemPrompt_EmptyTemplate(t *testing.T) {
	p := agent.NewAgentProfile("Kai", "engineer", "")
	prompt := p.SystemPrompt()
	assert.Equal(t, "You are Kai, a engineer.", prompt)
}

// ─── Seed ─────────────────────────────────────────────────────────────────────

func TestProfileRegistry_Seed(t *testing.T) {
	ctx := context.Background()
	r := agent.NewProfileRegistry(openTestDB(t))
	require.NoError(t, r.Seed(ctx))

	profiles, err := r.List(ctx)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(profiles), 3)

	nicknames := make(map[string]bool)
	for _, p := range profiles {
		nicknames[p.Nickname] = true
		assert.Len(t, p.ID, 36, "built-in IDs must be UUID-format")
	}
	assert.True(t, nicknames["Nexus"])
	assert.True(t, nicknames["Aria"])
	assert.True(t, nicknames["Kai"])
}

func TestProfileRegistry_Seed_Idempotent(t *testing.T) {
	ctx := context.Background()
	r := agent.NewProfileRegistry(openTestDB(t))
	require.NoError(t, r.Seed(ctx))
	require.NoError(t, r.Seed(ctx))

	profiles, err := r.List(ctx)
	require.NoError(t, err)
	counts := make(map[string]int)
	for _, p := range profiles {
		counts[p.Nickname]++
	}
	assert.Equal(t, 1, counts["Nexus"])
	assert.Equal(t, 1, counts["Aria"])
	assert.Equal(t, 1, counts["Kai"])
}

// ─── Register / Get ───────────────────────────────────────────────────────────

func TestProfileRegistry_Register_And_Get(t *testing.T) {
	ctx := context.Background()
	r := agent.NewProfileRegistry(openTestDB(t))

	p := agent.NewAgentProfile("Sarah", "manager", "You manage the Alpha team.")
	p.TeamID = "alpha"
	p.Model = "anthropic:claude-sonnet-4-20250514"
	p.Skills = []string{"leadership"}
	p.Metadata = map[string]string{"lang": "fr"}

	require.NoError(t, r.Register(ctx, p))

	got, err := r.Get(ctx, p.ID)
	require.NoError(t, err)

	assert.Equal(t, p.ID, got.ID)
	assert.Equal(t, "Sarah", got.Nickname)
	assert.Equal(t, "manager", got.Role)
	assert.Equal(t, "alpha", got.TeamID)
	assert.Equal(t, p.SystemPromptTemplate, got.SystemPromptTemplate)
	assert.Equal(t, p.Model, got.Model)
	assert.Equal(t, p.Skills, got.Skills)
	assert.Equal(t, p.Metadata, got.Metadata)
	assert.False(t, got.CreatedAt.IsZero())
}

func TestProfileRegistry_Register_EmptyID(t *testing.T) {
	ctx := context.Background()
	r := agent.NewProfileRegistry(openTestDB(t))
	err := r.Register(ctx, agent.AgentProfile{Nickname: "no-id"})
	assert.Error(t, err)
}

func TestProfileRegistry_Register_Upsert(t *testing.T) {
	ctx := context.Background()
	r := agent.NewProfileRegistry(openTestDB(t))

	p := agent.NewAgentProfile("Elena", "engineer", "v1 template")
	require.NoError(t, r.Register(ctx, p))

	p.Nickname = "Elena V2"
	p.SystemPromptTemplate = "v2 template"
	require.NoError(t, r.Register(ctx, p))

	got, err := r.Get(ctx, p.ID)
	require.NoError(t, err)
	assert.Equal(t, "Elena V2", got.Nickname)
	assert.Equal(t, "v2 template", got.SystemPromptTemplate)
}

func TestProfileRegistry_Get_NotFound(t *testing.T) {
	ctx := context.Background()
	r := agent.NewProfileRegistry(openTestDB(t))
	_, err := r.Get(ctx, "00000000-0000-0000-0000-999999999999")
	assert.True(t, errors.Is(err, db.ErrProfileNotFound))
}

// ─── Multiple agents, same role ───────────────────────────────────────────────

func TestProfileRegistry_MultipleAgentsSameRole(t *testing.T) {
	ctx := context.Background()
	r := agent.NewProfileRegistry(openTestDB(t))

	maria := agent.NewAgentProfile("Maria", "researcher", "Alpha team researcher.")
	maria.TeamID = "alpha"

	faouziath := agent.NewAgentProfile("Faouziath", "researcher", "Beta team researcher.")
	faouziath.TeamID = "beta"

	require.NoError(t, r.Register(ctx, maria))
	require.NoError(t, r.Register(ctx, faouziath))

	// Both have different UUIDs.
	assert.NotEqual(t, maria.ID, faouziath.ID)

	// FindByRole returns both.
	researchers, err := r.FindByRole(ctx, "researcher")
	require.NoError(t, err)
	assert.Len(t, researchers, 2)

	// FindByTeam isolates each one.
	alphaTeam, err := r.FindByTeam(ctx, "alpha")
	require.NoError(t, err)
	require.Len(t, alphaTeam, 1)
	assert.Equal(t, "Maria", alphaTeam[0].Nickname)

	betaTeam, err := r.FindByTeam(ctx, "beta")
	require.NoError(t, err)
	require.Len(t, betaTeam, 1)
	assert.Equal(t, "Faouziath", betaTeam[0].Nickname)
}

// ─── FindByRole case-insensitive ─────────────────────────────────────────────

func TestProfileRegistry_FindByRole_CaseInsensitive(t *testing.T) {
	ctx := context.Background()
	r := agent.NewProfileRegistry(openTestDB(t))
	require.NoError(t, r.Seed(ctx))

	upper, err := r.FindByRole(ctx, "ENGINEER")
	require.NoError(t, err)
	lower, err := r.FindByRole(ctx, "engineer")
	require.NoError(t, err)
	assert.Equal(t, len(upper), len(lower))
}

// ─── SystemPrompt via registry ────────────────────────────────────────────────

func TestProfileRegistry_SystemPrompt_ContainsNickname(t *testing.T) {
	ctx := context.Background()
	r := agent.NewProfileRegistry(openTestDB(t))
	require.NoError(t, r.Seed(ctx))

	aria, err := r.FindByRole(ctx, "researcher")
	require.NoError(t, err)
	require.Len(t, aria, 1)

	prompt := aria[0].SystemPrompt()
	assert.True(t, strings.HasPrefix(prompt, "You are Aria, a researcher."))
}

// ─── Delete ───────────────────────────────────────────────────────────────────

func TestProfileRegistry_Delete(t *testing.T) {
	ctx := context.Background()
	r := agent.NewProfileRegistry(openTestDB(t))

	p := agent.NewAgentProfile("Temp", "misc", "x")
	require.NoError(t, r.Register(ctx, p))
	require.NoError(t, r.Delete(ctx, p.ID))

	_, err := r.Get(ctx, p.ID)
	assert.True(t, errors.Is(err, db.ErrProfileNotFound))
}

func TestProfileRegistry_Delete_NoOp(t *testing.T) {
	ctx := context.Background()
	r := agent.NewProfileRegistry(openTestDB(t))
	assert.NoError(t, r.Delete(ctx, "00000000-0000-0000-0000-000000000000"))
}
