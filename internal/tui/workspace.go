// Package tui defines the interface and message types used by the interactive
// terminal UI. It has no dependency on pkg/sdk — the workspace implementation
// in cmd/cli bridges between this interface and the engine.
package tui

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"
)

// ─── Tea message types ────────────────────────────────────────────────────────

// ChunkMsg carries a streaming text delta during a turn.
type ChunkMsg struct {
	Text       string
	IsThinking bool
	SessionID  string
}

// ToolProgressMsg reports a tool starting, completing, or failing.
type ToolProgressMsg struct {
	ToolUseID string // unique per-call ID (empty for legacy callers)
	ToolName  string
	Status    string // "pending" | "running" | "completed" | "failed"
	Label     string // human-readable status label
	Metadata  map[string]any
	SessionID string
}

// TurnStartMsg signals the engine began processing a prompt.
type TurnStartMsg struct {
	SessionID string
	TurnID    string
}

// TurnDoneMsg signals a turn has completed (success or error).
type TurnDoneMsg struct {
	SessionID    string
	TurnID       string
	Err          error
	InputTokens  int
	OutputTokens int
	StopReason   string
}

// PromptRequestMsg carries a blocking prompt from the engine.
// The Response channel must be sent a PromptResponse to unblock the agent.
type PromptRequestMsg struct {
	Type     string // "confirm" | "text" | "choice"
	Message  string
	Options  []PromptOption
	Metadata map[string]any // engine metadata: tool_name, tool_input, working_directory, …
	// Response is written to exactly once to unblock the engine goroutine.
	Response chan PromptResponse
}

// PromptOption is a choice item in a "choice"-type prompt.
type PromptOption struct {
	Label string
	Value any
}

// PromptResponse is the user's answer to a PromptRequestMsg.
type PromptResponse struct {
	Value     any
	Cancelled bool
}

// SessionListMsg carries a refreshed session list.
type SessionListMsg struct {
	Sessions []SessionInfo
	Err      error
}

// SessionCreatedMsg signals a new session was created.
type SessionCreatedMsg struct {
	ID  string
	Err error
}

// SessionLoadedMsg signals a session was loaded successfully.
type SessionLoadedMsg struct {
	ID  string
	Err error
}

// ErrMsg wraps an error to display in the UI.
type ErrMsg struct{ Err error }

// ─── Model selection ──────────────────────────────────────────────────────────

// ProviderModel is the TUI's view of a single model entry.
type ProviderModel struct {
	Provider    string
	Identifier  string
	DisplayName string // "Anthropic / claude-sonnet-4"
	Description string
	Context     int
}

// ModelListMsg carries the available models from the workspace.
type ModelListMsg struct {
	Models []ProviderModel
	Err    error
}

// ModelChangedMsg signals the user selected a new model.
type ModelChangedMsg struct {
	Provider string
	Model    string
}

// ─── Provider config types ────────────────────────────────────────────────────

// ProviderFieldStatus describes one credential field for a provider.
type ProviderFieldStatus struct {
	Key      string // credential DB key (e.g. "api_key", "provider_base_url")
	Label    string // human label (e.g. "API Key")
	EnvVar   string // associated env var (e.g. "ANTHROPIC_API_KEY")
	Secret   bool
	Required bool
	IsSet    bool // whether a value is currently stored
}

// ProviderStatus summarises one provider and its current credential state.
type ProviderStatus struct {
	ID          string // "anthropic", "openai", …
	DisplayName string
	Description string
	NeedsKey    bool // false for Ollama and similar local providers
	Fields      []ProviderFieldStatus
}

// ─── Value types ──────────────────────────────────────────────────────────────

// SessionInfo is the TUI's lightweight view of a persisted session.
type SessionInfo struct {
	ID        string
	ShortID   string // first 8 chars of ID for display
	UpdatedAt time.Time
	CreatedAt time.Time
	Turns     int
	Tokens    int
}

// ToolInfo is the TUI's lightweight view of one registered tool.
type ToolInfo struct {
	Name        string
	Description string
	Category    string
}

// MCPServerInfo is the TUI's summary view of one MCP server integration.
type MCPServerInfo struct {
	Name            string
	ToolsRegistered int
	Status          string
	Error           string
}

// SkillInfo is the TUI's summary view of one available slash skill.
type SkillInfo struct {
	Name        string
	Description string
	WhenToUse   string
	Source      string
}

// ─── Workspace interface ──────────────────────────────────────────────────────

// Workspace is the contract between the TUI model and the nexus engine.
// The cmd/cli package provides the implementation that wraps pkg/sdk.
type Workspace interface {
	// Session management
	ListSessions(ctx context.Context)                   // async, sends SessionListMsg
	CreateSession(ctx context.Context)                  // async, sends SessionCreatedMsg
	LoadSession(ctx context.Context, id string)         // async, sends SessionLoadedMsg
	DeleteSession(ctx context.Context, id string) error // synchronous

	// Agent interaction
	// Submit starts a new turn in the active session (non-blocking).
	// Events arrive as ChunkMsg, ToolProgressMsg, TurnStartMsg, TurnDoneMsg,
	// and PromptRequestMsg via the subscribed Program.
	Submit(ctx context.Context, prompt string)
	Cancel() // cancel the running turn

	// Session state
	ActiveSessionID() string
	IsBusy() bool

	// Config (read-only)
	ModelString() string
	WorkingDir() string
	PermissionMode() string

	// Model selection
	// ListModels sends a ModelListMsg with all available models.
	ListModels(ctx context.Context)
	// SetModel switches the active model (provider:model string).
	SetModel(providerID, modelID string)

	// Subscribe registers the tea.Program so the workspace can push events.
	// Must be called before the first Submit.
	Subscribe(p *tea.Program)

	// Close releases all resources.
	Close()

	// ── Provider configuration ────────────────────────────────────────────

	// LoadProviderConfig returns all available providers with their current
	// credential status read from the credentials DB.
	LoadProviderConfig(ctx context.Context) []ProviderStatus

	// LoadToolCatalog returns the current registered tool surface.
	LoadToolCatalog(ctx context.Context) []ToolInfo

	// LoadMCPServers returns the current MCP integration status.
	LoadMCPServers(ctx context.Context) []MCPServerInfo

	// LoadSkills returns user-invocable skills available in the current repo.
	LoadSkills(ctx context.Context) []SkillInfo

	// SaveProviderField persists a credential field for a provider.
	// fieldKey is the DB key (e.g. "api_key", "provider_base_url").
	// Stored under the scoped key "fieldKey:providerID" so each provider
	// keeps its own credentials independently.
	SaveProviderField(ctx context.Context, providerID, fieldKey, value string) error

	// DeleteProviderField removes a credential field for a provider.
	DeleteProviderField(ctx context.Context, providerID, fieldKey string) error
}
