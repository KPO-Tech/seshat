package team

import (
	"context"
	"strings"
	"testing"

	"github.com/EngineerProjects/nexus-engine/internal/agent"
	"github.com/EngineerProjects/nexus-engine/internal/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func openPromptDB(t *testing.T) *db.DB {
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

// TestBuildAgentSystemPrompt_IdentityBlock verifies the identity section is
// always injected: the agent's name, role, and mailbox ID must appear.
func TestBuildAgentSystemPrompt_IdentityBlock(t *testing.T) {
	ctx := context.Background()
	database := openPromptDB(t)
	profiles := agent.NewProfileRegistry(database)

	ada := agent.NewAgentProfile("Ada", "engineer", "You write great code.")
	require.NoError(t, profiles.Register(ctx, ada))

	prompt := buildAgentSystemPrompt(ctx, ada, profiles, nil)

	assert.Contains(t, prompt, ada.ID, "Agent ID must appear in system prompt")
	assert.Contains(t, prompt, "Ada", "Nickname must appear")
	assert.Contains(t, prompt, "engineer", "Role must appear")
	assert.Contains(t, prompt, "## Your identity", "Identity section header must be present")
}

// TestBuildAgentSystemPrompt_NoTeam_NoRosterSection verifies that the team
// section is absent when the agent has no TeamID set.
func TestBuildAgentSystemPrompt_NoTeam_NoRosterSection(t *testing.T) {
	ctx := context.Background()
	database := openPromptDB(t)
	profiles := agent.NewProfileRegistry(database)
	teams := NewTeamRegistry(database, profiles)

	solo := agent.NewAgentProfile("Solo", "analyst", "You work alone.")
	require.NoError(t, profiles.Register(ctx, solo))

	// solo.TeamID is empty — no team section should appear.
	prompt := buildAgentSystemPrompt(ctx, solo, profiles, teams)

	assert.NotContains(t, prompt, "## Your team", "No team section expected for solo agent")
	assert.NotContains(t, prompt, "mailbox_send", "No mailbox instructions expected")
}

// TestBuildAgentSystemPrompt_WithTeam_ContainsTeammateIDs verifies the roster
// section contains each teammate's Agent ID and excludes the agent itself.
func TestBuildAgentSystemPrompt_WithTeam_ContainsTeammateIDs(t *testing.T) {
	ctx := context.Background()
	database := openPromptDB(t)
	profiles := agent.NewProfileRegistry(database)
	teams := NewTeamRegistry(database, profiles)

	ada := agent.NewAgentProfile("Ada", "engineer", "")
	rex := agent.NewAgentProfile("Rex", "manager", "")
	zoe := agent.NewAgentProfile("Zoe", "qa", "")
	for _, p := range []agent.AgentProfile{ada, rex, zoe} {
		require.NoError(t, profiles.Register(ctx, p))
	}

	grp := NewTeam("Alpha", "cross-functional")
	require.NoError(t, teams.Create(ctx, grp))
	require.NoError(t, teams.AddMember(ctx, grp.ID, ada.ID))
	require.NoError(t, teams.AddMember(ctx, grp.ID, rex.ID))
	require.NoError(t, teams.AddMember(ctx, grp.ID, zoe.ID))

	// Re-fetch Ada so TeamID is populated.
	ada, err := profiles.Get(ctx, ada.ID)
	require.NoError(t, err)
	require.Equal(t, grp.ID, ada.TeamID)

	prompt := buildAgentSystemPrompt(ctx, ada, profiles, teams)

	// Team header.
	assert.Contains(t, prompt, "## Your team", "Team section header required")
	assert.Contains(t, prompt, "Alpha", "Team name must appear")
	assert.Contains(t, prompt, grp.ID, "Team ID must appear")

	// Teammate IDs present.
	assert.Contains(t, prompt, rex.ID, "Rex's Agent ID must appear in roster")
	assert.Contains(t, prompt, zoe.ID, "Zoe's Agent ID must appear in roster")

	// Self must NOT be listed as a teammate.
	// Ada appears in the identity block already; check the roster section doesn't
	// duplicate her row by verifying Ada.ID appears only once (identity block).
	count := strings.Count(prompt, ada.ID)
	assert.Equal(t, 1, count, "Agent's own ID should appear exactly once (identity block), not in roster")

	// mailbox_send hint present.
	assert.Contains(t, prompt, "mailbox_send", "Hint about mailbox_send tool must appear")
}

// TestBuildAgentSystemPrompt_NilRegistries_GracefulDegradation verifies that
// passing nil profileReg or nil teams does not panic and omits the team section.
func TestBuildAgentSystemPrompt_NilRegistries_GracefulDegradation(t *testing.T) {
	ctx := context.Background()
	database := openPromptDB(t)
	profiles := agent.NewProfileRegistry(database)
	teams := NewTeamRegistry(database, profiles)

	// Create a team and assign the agent so TeamID is set.
	grp := NewTeam("Gamma", "")
	require.NoError(t, teams.Create(ctx, grp))

	p := agent.NewAgentProfile("Orphan", "writer", "")
	require.NoError(t, profiles.Register(ctx, p))
	require.NoError(t, teams.AddMember(ctx, grp.ID, p.ID))

	p, err := profiles.Get(ctx, p.ID)
	require.NoError(t, err)
	require.NotEmpty(t, p.TeamID)

	// nil teams → no panic, no team section.
	prompt := buildAgentSystemPrompt(ctx, p, profiles, nil)
	assert.NotContains(t, prompt, "## Your team")

	// nil profileReg → no panic, no team section.
	prompt = buildAgentSystemPrompt(ctx, p, nil, teams)
	assert.NotContains(t, prompt, "## Your team")
}
