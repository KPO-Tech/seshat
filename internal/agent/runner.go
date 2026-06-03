package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/EngineerProjects/nexus-engine/internal/agent/goal"
	"github.com/EngineerProjects/nexus-engine/internal/engine"
	"github.com/EngineerProjects/nexus-engine/internal/hooks"
	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	"github.com/EngineerProjects/nexus-engine/internal/types"
)

// RunResult is the result of running an agent
type RunResult struct {
	// AgentType is the type of agent
	AgentType string `json:"agentType"`

	// Success indicates success
	Success bool `json:"success"`

	// Output is the final output
	Output string `json:"output"`

	// Turns is the number of turns
	Turns int `json:"turns"`

	// ToolUses is the number of tool uses
	ToolUses int `json:"toolUses"`

	// WorktreePath is the worktree directory (for isolation mode)
	WorktreePath string `json:"worktreePath,omitempty"`

	// Error is the error if failed
	Error string `json:"error,omitempty"`
}

// RunConfig is the configuration for running an agent
type RunConfig struct {
	// AgentType is the agent type
	AgentType string

	// Task is the task description
	Task string

	// Tools is the list of available tools (nil = all tools)
	Tools []tool.Tool

	// ToolFilter is the list of tool name patterns to allow (nil = use Tools field directly)
	ToolFilter []string

	// Engine is an optional pre-created engine
	Engine *engine.Engine

	// MaxTurns is the maximum number of session turns the agent may execute
	// autonomously. Each turn is one SubmitMessage call (which internally runs
	// the full tool-use loop). Default 0 means a single turn (backward compat).
	// Set to 2+ for multi-turn autonomous execution.
	MaxTurns int

	// StopCondition, when set, is evaluated after every turn. The agent stops
	// when it returns true. Receives the turn index (1-based) and the turn
	// output. Default: never stops early (runs until MaxTurns).
	StopCondition func(turn int, output string) bool

	// ContinuationMessage, when set, is called after each non-final turn to
	// produce the user message that starts the next turn. If nil the default
	// message is used: "Continue with the task."
	ContinuationMessage func(turn int, output string) string

	// Context is the parent context
	Context context.Context

	// Callback is called on each turn completion
	Callback func(turn int, output string)

	// ForkFromMessages is message context inherited from parent
	ForkFromMessages []types.Message

	// IsolationMode is "worktree" or empty
	IsolationMode string

	// WorktreeDir is the worktree directory (for isolation)
	WorktreeDir string

	// EventFn, when set, receives every RuntimeEvent emitted by the sub-agent session.
	// The caller is responsible for tagging events with AgentToolUseID before forwarding.
	EventFn func(types.RuntimeEvent)

	// Registry is an optional agent registry for resolving dynamic (skill-based) agents.
	// When nil, only built-in agents are resolved.
	Registry *AgentRegistry

	// Nickname is the human-friendly name assigned to this agent (e.g. "Orion").
	// Stored in AsyncAgent.Nickname; does not affect execution.
	Nickname string

	// Role is the role this agent was spawned with (e.g. "reviewer").
	// Stored in AsyncAgent.Role; does not affect execution.
	Role string

	// GoalStore, when non-nil, enables goal-aware continuation prompts.
	// After each turn the runner checks for an active goal keyed by GoalSessionID
	// and injects the appropriate prompt (continuation or budget_limit).
	// Mirrors Codex's server-side goal prompt injection in the thread loop.
	GoalStore *goal.Store

	// GoalSessionID is the key used to look up the goal in GoalStore.
	// Usually set to types.ToolContext.SessionID by the caller.
	GoalSessionID string
}

// RunAgent runs an agent and returns the result
func RunAgent(config *RunConfig) (*RunResult, error) {
	// Resolve agent definition: registry first, then built-in fallback.
	var agentDef *AgentDefinition
	if config.Registry != nil {
		agentDef, _ = config.Registry.Get(config.AgentType)
	}
	if agentDef == nil {
		if builtIn := GetBuiltInAgentByType(config.AgentType); builtIn != nil {
			agentDef = ToAgentDefinition(*builtIn)
		}
	}
	if agentDef == nil {
		return &RunResult{
			AgentType: config.AgentType,
			Success:   false,
			Error:     fmt.Sprintf("unknown agent type: %s", config.AgentType),
		}, nil
	}

	// Get engine from config (callers must supply it via RunConfig.Engine).
	runtimeEngine := config.Engine
	if runtimeEngine == nil {
		return &RunResult{
			AgentType: config.AgentType,
			Success:   false,
			Error:     "engine not available - set RunConfig.Engine before calling RunAgent",
		}, nil
	}

	// Determine tools to allow in the session
	// Use ToolFilter if provided (from main agent's tool list), otherwise use agent's patterns
	var allowedTools []tool.Tool
	if len(config.ToolFilter) > 0 {
		// ToolFilter is a list of tool names, filter from config.Tools
		allowedTools = filterToolsByNames(config.Tools, config.ToolFilter)
	} else {
		// Use agent definition's tool patterns
		allowedTools = filterToolsByPatterns(config.Tools, agentDef.GetToolPatterns())
	}

	// Get context
	ctx := config.Context
	if ctx == nil {
		ctx = context.Background()
	}

	// Create session
	var session *engine.Session
	var err error
	subagentMetadata := &types.SessionMetadata{
		Status:        types.SessionStatusActive,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Additional:    map[string]any{"tool_surface_profile": tool.ToolSurfaceProfileSubagent},
		SchemaVersion: types.SessionMetadataSchemaVersion,
	}
	if len(config.ForkFromMessages) > 0 {
		inheritedMessages := append([]types.Message(nil), config.ForkFromMessages...)
		session, err = runtimeEngine.NewSessionFromState(ctx, "", subagentMetadata, inheritedMessages)
	} else {
		session, err = runtimeEngine.NewSessionFromState(ctx, "", subagentMetadata, nil)
	}
	if err != nil {
		return &RunResult{
			AgentType: config.AgentType,
			Success:   false,
			Error:     fmt.Sprintf("failed to create session: %v", err),
		}, nil
	}

	// Give the agent its own system prompt identity so it does not inherit the
	// Nexus Core identity of the parent session. Without this, the agent's
	// personality was injected only as the first user message, which meant the
	// model still operated under the parent's rules and workflow sections.
	if agentDef.GetSystemPrompt != nil {
		if sp := agentDef.GetSystemPrompt(); sp != "" {
			session.SetSystemPromptTemplate(sp)
		}
	}

	// Note: MCP servers are integrated at the engine level.
	// Agents inherit MCP tools from the parent engine's tool registry.
	// If agent-specific MCP servers are needed, they should be configured
	// when creating the engine, not per-session.

	// Register tools
	for _, t := range allowedTools {
		if err := session.RegisterTool(t); err != nil {
			return &RunResult{
				AgentType: config.AgentType,
				Success:   false,
				Error:     fmt.Sprintf("failed to register tool %s: %v", t.Definition().Name, err),
			}, nil
		}
	}

	// Wire the event emitter so sub-agent events stream back to the parent session.
	if config.EventFn != nil {
		session.SetRuntimeEventCallback(config.EventFn)
	}

	// Autonomous multi-turn loop.
	//
	// Turn 1  : submit config.Task as the initial user message.
	// Turn 2+ : submit a continuation message so the task is not re-sent.
	//
	// Each SubmitMessage call runs the full engine loop (tool-use cycles
	// included). The outer loop here handles autonomous multi-turn execution
	// where the agent keeps working across session turns without user input.
	maxTurns := config.MaxTurns
	if maxTurns <= 0 {
		maxTurns = 1 // backward-compatible default: single turn
	}

	continuationFn := config.ContinuationMessage
	if continuationFn == nil {
		continuationFn = func(_ int, _ string) string {
			return "Continue with the task."
		}
	}

	// goalContinuationFn wraps continuationFn with goal-aware prompt injection.
	// Mirrors Codex's server-side goal prompt injection after each turn.
	goalContinuationFn := func(turn int, output string) string {
		if config.GoalStore != nil && config.GoalSessionID != "" {
			if g, ok := config.GoalStore.Get(config.GoalSessionID); ok && g.Status == goal.StatusActive {
				if g.IsOverBudget() {
					// Transition to budgetLimited and inject the wrap-up prompt.
					budgetLimited := goal.StatusBudgetLimited
					config.GoalStore.Update(config.GoalSessionID, &budgetLimited, nil)
					return goal.BudgetLimitPrompt(g)
				}
				return goal.ContinuationPrompt(g)
			}
		}
		return continuationFn(turn, output)
	}

	var lastOutput strings.Builder
	turns := 0
	totalToolUses := 0

	for turn := 1; turn <= maxTurns; turn++ {
		var msg string
		if turn == 1 {
			msg = config.Task
		} else {
			msg = goalContinuationFn(turn, lastOutput.String())
		}

		resp, err := session.SubmitMessage(ctx, msg)
		if err != nil {
			lastOutput.WriteString(fmt.Sprintf("\nError on turn %d: %v", turn, err))
			return &RunResult{
				AgentType: config.AgentType,
				Success:   false,
				Output:    lastOutput.String(),
				Turns:     turns,
				ToolUses:  totalToolUses,
				Error:     err.Error(),
			}, nil
		}
		turns++
		totalToolUses += len(resp.GetLastToolResults())

		// Collect this turn's output.
		lastOutput.Reset()
		if lastMsg, hasMsg := resp.GetLastAssistantMessage(); hasMsg {
			for _, block := range lastMsg.Content {
				if tc, ok := block.(types.TextContent); ok {
					lastOutput.WriteString(tc.Text)
				}
			}
		}

		if config.Callback != nil {
			config.Callback(turn, lastOutput.String())
		}

		// Check stop condition; if true the agent considers itself done.
		if config.StopCondition != nil && config.StopCondition(turn, lastOutput.String()) {
			break
		}

		// If we are on the last allowed turn, stop regardless.
		if turn >= maxTurns {
			break
		}

		// Select: is there a context cancellation?
		select {
		case <-ctx.Done():
			return &RunResult{
				AgentType: config.AgentType,
				Success:   false,
				Output:    lastOutput.String(),
				Turns:     turns,
				ToolUses:  totalToolUses,
				Error:     ctx.Err().Error(),
			}, nil
		default:
		}
	}

	return &RunResult{
		AgentType: config.AgentType,
		Success:   true,
		Output:    lastOutput.String(),
		Turns:     turns,
		ToolUses:  totalToolUses,
	}, nil
}

// RunForkedAgent runs an agent in fork mode (like OpenClaude's forkSubagent)
func RunForkedAgent(config *RunConfig) (*RunResult, error) {
	return RunAgent(config)
}

// matchesPattern checks if a tool name matches any of the patterns
func matchesPattern(name string, patterns []string) bool {
	for _, pattern := range patterns {
		if pattern == "*" || pattern == name {
			return true
		}
		if len(pattern) > 1 && pattern[len(pattern)-1] == '*' {
			prefix := pattern[:len(pattern)-1]
			if len(name) >= len(prefix) && name[:len(prefix)] == prefix {
				return true
			}
		}
		if len(pattern) > 1 && pattern[0] == '*' {
			suffix := pattern[1:]
			if len(name) >= len(suffix) && name[len(name)-len(suffix):] == suffix {
				return true
			}
		}
	}
	return false
}

// filterToolsByPatterns filters tools by allowed patterns
func filterToolsByPatterns(tools []tool.Tool, patterns []string) []tool.Tool {
	if len(patterns) == 0 || (len(patterns) == 1 && patterns[0] == "*") {
		return tools
	}

	result := make([]tool.Tool, 0)
	for _, t := range tools {
		if matchesPattern(t.Definition().Name, patterns) {
			result = append(result, t)
		}
	}
	return result
}

// filterToolsByNames filters tools by exact name matching (no wildcards)
// Returns tools whose name is in the allowedNames list
func filterToolsByNames(tools []tool.Tool, allowedNames []string) []tool.Tool {
	if len(allowedNames) == 0 {
		return tools
	}

	// Build a set for O(1) lookup
	allowedSet := make(map[string]bool, len(allowedNames))
	for _, name := range allowedNames {
		allowedSet[name] = true
	}

	result := make([]tool.Tool, 0)
	for _, t := range tools {
		if allowedSet[t.Definition().Name] {
			result = append(result, t)
		}
	}
	return result
}

// Runner is an advanced agent runner with hook and memory support
type Runner struct {
	engine          *engine.Engine
	hookExecutor    *hooks.Executor
	enableMemory    bool
	enableHooks     bool
	defaultMaxTurns int
}

// NewRunner creates a new advanced agent runner
func NewRunner(engine *engine.Engine) *Runner {
	return &Runner{
		engine:          engine,
		hookExecutor:    nil, // Will use engine's hook executor
		enableMemory:    true,
		enableHooks:     true,
		defaultMaxTurns: 50,
	}
}

// SetHookExecutor sets a custom hook executor
func (r *Runner) SetHookExecutor(executor *hooks.Executor) {
	r.hookExecutor = executor
}

// SetEnableMemory enables or disables memory for agents
func (r *Runner) SetEnableMemory(enabled bool) {
	r.enableMemory = enabled
}

// SetEnableHooks enables or disables hooks for agents
func (r *Runner) SetEnableHooks(enabled bool) {
	r.enableHooks = enabled
}

// SetDefaultMaxTurns sets the default maximum turns for agents
func (r *Runner) SetDefaultMaxTurns(maxTurns int) {
	r.defaultMaxTurns = maxTurns
}

// RunAgentAdvanced runs an agent with advanced features (hooks, memory)
func (r *Runner) RunAgentAdvanced(ctx context.Context, config *RunConfig) (*RunResult, error) {
	// Execute agent start hook if enabled
	if r.enableHooks && r.hookExecutor != nil {
		r.executeHook(ctx, types.HookEventSubagentStart, map[string]any{
			"agent_type": config.AgentType,
			"task":       config.Task,
		})
	}

	// Set max turns if not specified
	if config.MaxTurns == 0 {
		config.MaxTurns = r.defaultMaxTurns
	}

	// Use the engine from the runner if not provided
	if config.Engine == nil {
		config.Engine = r.engine
	}

	// Run the agent
	result, err := RunAgent(config)
	if err != nil {
		// Execute agent error hook if enabled
		if r.enableHooks && r.hookExecutor != nil {
			r.executeHook(ctx, types.HookEventOnError, map[string]any{
				"agent_type": config.AgentType,
				"error":      err.Error(),
			})
		}
		return result, err
	}

	// Execute agent complete hook if enabled
	if r.enableHooks && r.hookExecutor != nil {
		r.executeHook(ctx, types.HookEventSubagentStop, map[string]any{
			"agent_type":    config.AgentType,
			"success":       result.Success,
			"turns":         result.Turns,
			"tool_uses":     result.ToolUses,
			"worktree_path": result.WorktreePath,
		})
	}

	return result, nil
}

// executeHook executes a hook event
func (r *Runner) executeHook(ctx context.Context, event types.HookEvent, data map[string]any) {
	if r.hookExecutor == nil {
		return
	}

	results, err := r.hookExecutor.Execute(ctx, event, data)
	if err != nil {
		fmt.Printf("[agent-runner] Hook error for %s: %v\n", event, err)
		return
	}

	// Process hook results
	for _, result := range results {
		if result.Action == types.HookActionStop {
			fmt.Printf("[agent-runner] Hook %s requested stop: %s\n", event, result.Message)
		}
	}
}
