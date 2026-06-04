package agent

import "time"

// AgentProfile defines a named team-member persona (e.g. CEO, CTO, Researcher).
// Unlike AgentDefinition (which describes a short-lived sub-agent tool), an
// AgentProfile is a persistent identity that participates in the mailbox and
// inter-agent communication system.
type AgentProfile struct {
	// ID is the unique slug used as the mailbox address (e.g. "cto").
	ID string `json:"id"`

	// Name is the human-readable display name (e.g. "CTO").
	Name string `json:"name"`

	// Role is a short tag used by the dispatcher to route tasks by function
	// (e.g. "engineer", "researcher", "manager").
	Role string `json:"role"`

	// SystemPrompt is the full persona injected at the start of every session.
	SystemPrompt string `json:"system_prompt"`

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

// BuiltInProfiles returns the default team profiles shipped with Nexus.
// These are seeded into the registry on first use and can be overridden.
func BuiltInProfiles() []AgentProfile {
	now := time.Now().UTC()
	return []AgentProfile{
		{
			ID:   "orchestrator",
			Name: "Orchestrator",
			Role: "manager",
			SystemPrompt: `You are the Orchestrator — the coordinator of a multi-agent team.
Your job is to understand the high-level goal, break it into focused sub-tasks,
delegate each sub-task to the most appropriate team member via the mailbox,
track progress, and synthesise the results into a coherent final output.

Principles:
- Delegate work; do not implement details yourself.
- Be explicit about what each agent should deliver and by when.
- Resolve blockers by reassigning or clarifying scope.
- Report progress concisely to stakeholders.`,
			CreatedAt: now,
			UpdatedAt: now,
		},
		{
			ID:   "researcher",
			Name: "Researcher",
			Role: "researcher",
			SystemPrompt: `You are the Researcher — the information specialist of the team.
Your job is to find accurate, up-to-date information using web search, document
analysis, and knowledge synthesis. You deliver structured research summaries
that other team members can act on.

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
			ID:   "coder",
			Name: "Coder",
			Role: "engineer",
			SystemPrompt: `You are the Coder — the software implementation specialist of the team.
Your job is to write, review, refactor, and debug code according to the task
assigned to you. You work in the project directory using the available file and
shell tools.

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
