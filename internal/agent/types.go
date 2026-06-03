package agent

import (
	"context"

	"github.com/EngineerProjects/nexus-engine/internal/engine"
	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	"github.com/EngineerProjects/nexus-engine/internal/types"
)

// AgentSource indicates where the agent definition comes from
type AgentSource string

const (
	AgentSourceBuiltIn AgentSource = "built-in"
	AgentSourceUser    AgentSource = "user"
	AgentSourceProject AgentSource = "project"
)

// AgentDefinition defines an agent
type AgentDefinition struct {
	// AgentType is the unique agent type identifier
	AgentType string `json:"agentType"`

	// WhenToUse is the description for when to use this agent
	WhenToUse string `json:"whenToUse"`

	// Source indicates where this agent comes from
	Source AgentSource `json:"source"`

	// BaseDir is the base directory (for user/project agents)
	BaseDir string `json:"baseDir,omitempty"`

	// Filename is the original filename (for user/project agents)
	Filename string `json:"filename,omitempty"`

	// Tools is the list of allowed tool patterns (glob patterns)
	// nil or ["*"] means all tools
	Tools []string `json:"tools,omitempty"`

	// DisallowedTools is the list of disallowed tool patterns
	DisallowedTools []string `json:"disallowedTools,omitempty"`

	// Skills is the list of skill names to preload
	Skills []string `json:"skills,omitempty"`

	// MCPservers is the list of MCP server names to connect
	McpServers []string `json:"mcpServers,omitempty"`

	// Model is the model to use (empty = default)
	Model string `json:"model,omitempty"`

	// Effort is the effort level (1-5)
	Effort int `json:"effort,omitempty"`

	// PermissionMode is the permission mode
	PermissionMode types.PermissionMode `json:"permissionMode,omitempty"`

	// MaxTurns is the maximum number of turns
	MaxTurns int `json:"maxTurns,omitempty"`

	// Memory is the memory scope
	Memory string `json:"memory,omitempty"`

	// Background indicates if the agent should run in background
	Background bool `json:"background,omitempty"`

	// Isolation is the isolation mode
	Isolation string `json:"isolation,omitempty"`

	// SystemPromptGetter is the function to get the system prompt
	GetSystemPrompt func() string `json:"-"`
}

// GetToolPatterns returns the tool patterns (nil = all tools)
func (a *AgentDefinition) GetToolPatterns() []string {
	if a.Tools == nil {
		return []string{"*"}
	}
	return a.Tools
}

// BuiltInAgentDefinition is an agent defined in code (not loaded from disk)
type BuiltInAgentDefinition struct {
	AgentType       string
	WhenToUse       string
	Tools           []string
	Source          AgentSource
	BaseDir         string
	Model           string
	GetSystemPrompt func() string
	MaxTurns        int
}

// AgentResult represents the result of running an agent
type AgentResult struct {
	// AgentType is the type of agent that ran
	AgentType string `json:"agentType"`

	// Success indicates if the agent completed successfully
	Success bool `json:"success"`

	// Result is the agent's final result
	Result string `json:"result"`

	// Turns is the number of turns taken
	Turns int `json:"turns"`

	// ToolUses is the number of tool uses
	ToolUses int `json:"toolUses"`

	// Error is the error message if failed
	Error string `json:"error,omitempty"`
}

// AgentContext is the context for running an agent
type AgentContext struct {
	// Definition is the agent definition
	Definition *AgentDefinition

	// Engine is the query engine
	Engine *engine.Engine

	// Session is the query session
	Session *engine.Session

	// Tools is the available tools
	Tools []tool.Tool

	// Input is the user input
	Input string

	// MaxTurns is the maximum turns (overrides definition)
	MaxTurns int

	// OnProgress is called on progress updates
	OnProgress func(result *AgentResult)

	// Context is the parent context
	Context context.Context
}
