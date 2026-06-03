package engine

import (
	"github.com/EngineerProjects/nexus-engine/internal/prompt"
	"github.com/EngineerProjects/nexus-engine/internal/tools/system/mcp"
	"github.com/EngineerProjects/nexus-engine/internal/types"
	browsercore "github.com/EngineerProjects/nexus-engine/internal/web/browser"
)

// Config represents the engine configuration.
type Config struct {
	MaxTurns             int                   `json:"max_turns"`
	AutoCompact          bool                  `json:"auto_compact"`
	PermissionMode       types.PermissionMode  `json:"permission_mode"`
	Model                types.ModelIdentifier `json:"model"`
	MaxTokens            int                   `json:"max_tokens"`
	WorkingDirectory     string                `json:"working_directory,omitempty"`
	SystemPromptTemplate string                `json:"system_prompt_template"`
	MCPServers           []mcp.ServerConfig    `json:"mcp_servers"`
	EnableMemory         bool                  `json:"enable_memory"`
	EnableMonitoring     bool                  `json:"enable_monitoring"`

	// MaxIterations caps model→tool→model cycles per turn. 0 = loop default (10).
	MaxIterations int `json:"max_iterations"`

	// TurnTokenBudget enables token-budget continuation when > 0.
	TurnTokenBudget int `json:"turn_token_budget"`

	// BudgetContinuationLimit caps how many budget-continuation cycles may occur.
	// 0 = loop default.
	BudgetContinuationLimit int `json:"budget_continuation_limit"`

	// ContinuationNudgeLimit caps how many text-signal nudges the loop sends.
	// 0 = loop default (3).
	ContinuationNudgeLimit int `json:"continuation_nudge_limit"`

	// StopHooks are post-turn policy hooks that may request one more cycle.
	StopHooks []StopHook `json:"-"`

	// MaxConsecutiveDenials is the number of back-to-back permission denials that
	// trigger orchestrator fallback. 0 = use the default (3).
	MaxConsecutiveDenials int `json:"max_consecutive_denials"`

	// PromptStage sets the execution stage overlay injected into the dynamic
	// section of the system prompt every turn.
	PromptStage prompt.ExecutionStage `json:"prompt_stage,omitempty"`

	// PromptStageOverrides replaces built-in stage overlay text per stage.
	PromptStageOverrides map[prompt.ExecutionStage]string `json:"-"`

	// PromptToolHints provides per-tool extra guidance appended to provider-facing
	// tool descriptions. Key is the canonical tool name.
	PromptToolHints map[string]string `json:"-"`

	// AppendSystemPrompt is appended to the system prompt after all other sections.
	AppendSystemPrompt string `json:"append_system_prompt,omitempty"`

	// BrowserManager manages native browser sessions for browser tools.
	BrowserManager browsercore.Manager `json:"-"`
}

// DefaultConfig returns default engine configuration.
func DefaultConfig() *Config {
	return &Config{
		MaxTurns:       100,
		AutoCompact:    true,
		PermissionMode: types.PermissionModeOnRequest,
		Model: types.ModelIdentifier{
			Provider: types.APIProviderAnthropic,
			Model:    "claude-3-5-sonnet-20241022",
		},
		MaxTokens:            8192,
		SystemPromptTemplate: "",
		EnableMemory:         true,
		EnableMonitoring:     true,
	}
}
