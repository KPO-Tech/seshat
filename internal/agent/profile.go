package agent

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

// AgentProfile defines a named team-member persona (e.g. Maria the researcher,
// Faouziath the coder). Unlike AgentDefinition (short-lived sub-agent tool),
// an AgentProfile is a persistent identity that lives in the mailbox and
// inter-agent communication system.
type AgentProfile struct {
	// ID is a UUID — globally unique across all teams and roles.
	// Generated once at creation; never changes.
	ID string `json:"id"`

	// Nickname is the personal name displayed in the UI and injected into the
	// system prompt so the agent knows what it is called ("You are Maria…").
	Nickname string `json:"nickname"`

	// Role is a functional tag used by the dispatcher to route tasks
	// (e.g. "researcher", "engineer", "manager").
	// Multiple agents can share the same role across different teams.
	Role string `json:"role"`

	// TeamID groups agents into a named team. Empty means no team assigned.
	TeamID string `json:"team_id,omitempty"`

	// SystemPromptTemplate is the base persona injected at session start.
	// The placeholder {{.Nickname}} is replaced with the agent's Nickname at
	// runtime, and a preamble "You are {{.Nickname}}, …" is prepended automatically
	// if the template does not start with "You are".
	SystemPromptTemplate string `json:"system_prompt_template"`

	// Model is the preferred model identifier in "provider:model" format.
	// Empty string means use the global default.
	Model string `json:"model,omitempty"`

	// Skills lists skill file names to preload for this profile.
	Skills []string `json:"skills,omitempty"`

	// Metadata holds arbitrary extension fields.
	Metadata map[string]string `json:"metadata,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// NewAgentProfile creates a new AgentProfile with a freshly generated UUID.
func NewAgentProfile(nickname, role, systemPromptTemplate string) AgentProfile {
	now := time.Now().UTC()
	return AgentProfile{
		ID:                   uuid.New().String(),
		Nickname:             nickname,
		Role:                 role,
		SystemPromptTemplate: systemPromptTemplate,
		CreatedAt:            now,
		UpdatedAt:            now,
	}
}

// SystemPrompt returns the fully resolved system prompt for this agent,
// prepending the identity preamble and substituting {{.Nickname}}.
func (p AgentProfile) SystemPrompt() string {
	body := replaceNickname(p.SystemPromptTemplate, p.Nickname)
	preamble := fmt.Sprintf("You are %s, a %s.", p.Nickname, p.Role)
	if len(body) > 0 {
		return preamble + "\n\n" + body
	}
	return preamble
}

// replaceNickname substitutes every occurrence of {{.Nickname}} in s.
func replaceNickname(s, nickname string) string {
	const placeholder = "{{.Nickname}}"
	out := []byte{}
	for i := 0; i < len(s); {
		if i+len(placeholder) <= len(s) && s[i:i+len(placeholder)] == placeholder {
			out = append(out, nickname...)
			i += len(placeholder)
		} else {
			out = append(out, s[i])
			i++
		}
	}
	return string(out)
}

// BuiltInProfiles returns the default team profiles shipped with Nexus.
// IDs are fixed UUIDs so they are stable across restarts and installs.
// These are seeded into the registry on first use and can be overridden or
// renamed by the user (change the Nickname without touching the ID).
func BuiltInProfiles() []AgentProfile {
	now := time.Now().UTC()
	return []AgentProfile{
		{
			ID:       "00000000-0000-0000-0000-000000000001",
			Nickname: "Nexus",
			Role:     "manager",
			SystemPromptTemplate: `You coordinate the team. Break the goal into focused sub-tasks,
delegate each to the right team member via the mailbox, track progress,
and synthesise results into a coherent final output.

Principles:
- Delegate work; do not implement details yourself.
- Be explicit about what each agent should deliver and by when.
- Resolve blockers by reassigning or clarifying scope.
- Report progress concisely.`,
			CreatedAt: now,
			UpdatedAt: now,
		},
		{
			ID:       "00000000-0000-0000-0000-000000000002",
			Nickname: "Aria",
			Role:     "researcher",
			SystemPromptTemplate: `You find accurate, up-to-date information using web search,
document analysis, and knowledge synthesis. You deliver structured
research summaries that the team can act on immediately.

Principles:
- Cite sources. Never fabricate facts.
- Prefer primary sources over secondary summaries.
- Flag uncertainty explicitly ("I could not verify…").
- Keep output focused: executive summary first, detail below.`,
			Skills:    []string{"web-research"},
			CreatedAt: now,
			UpdatedAt: now,
		},
		{
			ID:       "00000000-0000-0000-0000-000000000003",
			Nickname: "Kai",
			Role:     "engineer",
			SystemPromptTemplate: `You write, review, refactor, and debug code according to
the task assigned to you. You work in the project directory using
file and shell tools.

Principles:
- Write correct, minimal, well-named code. No unnecessary abstractions.
- Follow existing conventions in the codebase.
- Test before marking a task done.
- Report what you changed and why, not just what you ran.`,
			Skills:    []string{"go-conventions"},
			CreatedAt: now,
			UpdatedAt: now,
		},
	}
}
