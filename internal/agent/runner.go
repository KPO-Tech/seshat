package agent

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/KPO-Tech/seshat-runtime/internal/agent/goal"
	"github.com/KPO-Tech/seshat-runtime/internal/engine"
	"github.com/KPO-Tech/seshat-runtime/internal/hooks"
	tool "github.com/KPO-Tech/seshat-runtime/internal/tools/registry"
	skilltool "github.com/KPO-Tech/seshat-runtime/internal/tools/system/skills"
	"github.com/KPO-Tech/seshat-runtime/internal/types"
)

// RunResult is the result of running an agent
// SourceRef records a resource consulted by the sub-agent during execution.
type SourceRef struct {
	Type  string `json:"type"`  // "file", "url", "search", "glob", "grep"
	Value string `json:"value"` // path, URL, or query string
}

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

	// Sources lists every file, URL, and search query the agent consulted.
	// Collected automatically from tool calls — the parent receives this
	// without relying on the sub-agent to format it in its output.
	Sources []SourceRef `json:"sources,omitempty"`

	// WorktreePath is the worktree directory (for isolation mode)
	WorktreePath string `json:"worktreePath,omitempty"`

	// SessionID is the engine session ID for this run. Callers can pass it to
	// RunConfig.ResumeFromSessionID to continue from where this run left off.
	SessionID types.SessionID `json:"session_id,omitempty"`

	// Error is the error if failed
	Error string `json:"error,omitempty"`
}

// RunConfig is the configuration for running an agent
type RunConfig struct {
	// AgentType is the agent type
	AgentType string

	// Task is the task description
	Task string

	// ToolFilter is a list of tool name patterns to restrict the sub-agent's
	// session to (wildcard-aware - see matchesPattern). Empty/nil falls back
	// to the resolved agent type's own GetToolPatterns(); a bare "*" (or an
	// empty pattern list on both) means no restriction at all.
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

	// Callback is called on each turn completion with cumulative tool use count.
	Callback func(turn int, output string, toolUses int)

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

	// ParentSessionID is the session that spawned this agent (set by spawn_agent
	// from types.ToolContext.SessionID). Stored in AsyncAgent.ParentSessionID;
	// does not affect execution. Host applications use it to authorize
	// management operations on the resulting AsyncAgent (e.g. seshat-backend
	// only lets a cancel request through when the caller can prove ownership of
	// this session) — generated agent IDs are a predictable counter, not a
	// capability token, so this check matters once more than one user can reach
	// the same AsyncAgentManager.
	ParentSessionID types.SessionID

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

	// PermissionMode, when non-empty, overrides the session's permission mode.
	// Headless sub-agents cannot display interactive prompts, so callers
	// derive this from the parent session's mode via subAgentPermissionMode:
	// normally types.PermissionModeBypass, or types.PermissionModeNever when
	// the parent is itself in never mode (see that function's doc comment).
	PermissionMode types.PermissionMode

	// ForkRunner, when set, is wired into the sub-agent session's skill tool
	// if restrictSessionSkillsIfDeclared swaps it for a restricted one
	// (AgentDefinition.Skills non-empty). Without this, a sub-agent that
	// preloads a Skills list loses the ability to run any context: fork
	// skill entirely, even though its parent session had one - the
	// restricted tool RestrictSessionSkills builds is otherwise a bare
	// NewSkillTool(nil) with no fork runner of its own. Callers in
	// internal/tools/agents construct this from the same engine/package
	// that already builds SkillForkRunner for the top-level skill tool.
	ForkRunner skilltool.ForkRunner

	// ResumeFromSessionID, when set, opens an existing persisted session instead
	// of creating a new one. The first message sent to the resumed session is
	// config.Task, continuing where the previous run left off.
	ResumeFromSessionID types.SessionID
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

	// Tool restriction patterns: ToolFilter is the caller's already-combined
	// list (the model's own tools:[...] intersected with the agent type's
	// own patterns - see agent_tool.go's filterToolPatterns) when given,
	// otherwise fall back to the agent type's own declared patterns alone.
	restrictPatterns := config.ToolFilter
	if len(restrictPatterns) == 0 {
		restrictPatterns = agentDef.GetToolPatterns()
	}

	// Get context
	ctx := config.Context
	if ctx == nil {
		ctx = context.Background()
	}

	// Create or restore session.
	var session *engine.Session
	var err error
	if config.ResumeFromSessionID != "" {
		session, err = runtimeEngine.OpenSession(ctx, config.ResumeFromSessionID)
		if err != nil {
			return &RunResult{
				AgentType: config.AgentType,
				Success:   false,
				Error:     fmt.Sprintf("failed to restore session %s: %v", config.ResumeFromSessionID, err),
			}, nil
		}
	} else {
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
	}

	RestrictSessionTools(session, restrictPatterns)
	restrictSessionSkillsIfDeclared(session, agentDef, ctx, config.ForkRunner)

	// Apply permission mode override (e.g. bypass for headless background agents).
	if config.PermissionMode != "" {
		session.SetPermissionMode(config.PermissionMode)
	}

	// Give the agent its own system prompt identity so it does not inherit the
	// Seshat identity of the parent session. Without this, the agent's
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
		continuationFn = func(_ int, output string) string {
			if strings.TrimSpace(output) == "" {
				return "Return a concise final answer now. Summarize what you checked, what you found, and any blocker or error. Do not call more tools unless a final answer would be misleading without one."
			}
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
	var sources []SourceRef
	seenSources := make(map[string]bool)

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
				Sources:   sources,
				SessionID: session.GetSessionID(),
				Error:     err.Error(),
			}, nil
		}
		turns++
		totalToolUses += len(resp.GetLastToolResults())

		// Collect sources from every tool call in this turn.
		for _, tu := range resp.ToolUses {
			for _, ref := range extractSources(tu) {
				key := ref.Type + ":" + ref.Value
				if !seenSources[key] {
					seenSources[key] = true
					sources = append(sources, ref)
				}
			}
		}

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
			config.Callback(turn, lastOutput.String(), totalToolUses)
		}

		if strings.TrimSpace(lastOutput.String()) == "" && len(resp.GetLastToolResults()) > 0 && turn < maxTurns {
			continue
		}

		// Check stop condition; if true the agent considers itself done.
		if config.StopCondition != nil && config.StopCondition(turn, lastOutput.String()) {
			break
		}

		// If a goal is active, stop if the goal is completed or budget is exhausted.
		if config.GoalStore != nil && config.GoalSessionID != "" {
			if g, ok := config.GoalStore.Get(config.GoalSessionID); ok && goal.IsFinal(g.Status) {
				break
			}
		}

		// Stop if the turn completed naturally (end_turn) and no active goal remains.
		hasActiveGoal := false
		if config.GoalStore != nil && config.GoalSessionID != "" {
			if g, ok := config.GoalStore.Get(config.GoalSessionID); ok && g.Status == goal.StatusActive {
				hasActiveGoal = !g.IsOverBudget()
			}
		}
		if resp.StopReason == types.StopReasonEndTurn && !hasActiveGoal {
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
				Sources:   sources,
				SessionID: session.GetSessionID(),
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
		Sources:   sources,
		SessionID: session.GetSessionID(),
	}, nil
}

// extractSources inspects a single tool call and returns any source references
// (files read, URLs fetched, search queries). Deduplication happens in the caller.
func extractSources(tu types.ToolUseContent) []SourceRef {
	str := func(key string) string {
		if v, ok := tu.Input[key].(string); ok {
			return strings.TrimSpace(v)
		}
		return ""
	}

	switch tu.Name {
	case "read_file", "write_file", "edit_file":
		if p := str("path"); p != "" {
			return []SourceRef{{Type: "file", Value: p}}
		}
	case "glob":
		if p := str("pattern"); p != "" {
			return []SourceRef{{Type: "glob", Value: p}}
		}
	case "grep":
		refs := []SourceRef{}
		if p := str("pattern"); p != "" {
			refs = append(refs, SourceRef{Type: "grep", Value: p})
		}
		if p := str("path"); p != "" {
			refs = append(refs, SourceRef{Type: "file", Value: p})
		}
		return refs
	case "web_search", "wikipedia", "scholarly_search", "langsearch":
		if q := str("query"); q != "" {
			return []SourceRef{{Type: "search", Value: q}}
		}
	case "web_fetch", "web_crawl", "web_map":
		if u := str("url"); u != "" {
			return []SourceRef{{Type: "url", Value: u}}
		}
	case "browser_navigate", "browser_open":
		if u := str("url"); u != "" {
			return []SourceRef{{Type: "url", Value: u}}
		}
	}
	return nil
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

// RestrictSessionTools unregisters every tool on session whose name does not
// match one of patterns (wildcard-aware, via matchesPattern) - the
// restriction a sub-agent's Tools/ToolFilter was always meant to apply, but
// never did (see TODO.md's M5c entry, 2026-10-11): the session already has
// its full tool surface populated by NewSessionFromState's own surface
// builder before this runs, so trying to additively RegisterTool a filtered
// subset on top (the old approach) either did nothing (the filtered list
// was always empty - config.Tools was never populated by any caller) or
// would have failed outright on every tool that overlapped with what the
// surface builder already registered (RegisterTool errors on a duplicate
// name). Pruning what's already there in place avoids both problems.
//
// A nil/empty patterns list, or a bare "*", means no restriction - every
// tool the session started with stays.
func RestrictSessionTools(session *engine.Session, patterns []string) {
	if len(patterns) == 0 || (len(patterns) == 1 && patterns[0] == "*") {
		return
	}
	for _, name := range session.GetToolNames() {
		if !matchesPattern(name, patterns) {
			_ = session.UnregisterTool(name)
		}
	}
}

// restrictSessionSkillsIfDeclared calls RestrictSessionSkills when agentDef
// names specific skills to preload, split out of RunAgent to keep that
// function's own branch count down rather than inlining the check there.
func restrictSessionSkillsIfDeclared(session *engine.Session, agentDef *AgentDefinition, ctx context.Context, forkRunner skilltool.ForkRunner) {
	if len(agentDef.Skills) > 0 {
		RestrictSessionSkills(session, agentDef.Skills, ctx, forkRunner)
	}
}

// RestrictSessionSkills replaces the session's skill tool with one scoped to
// exactly allowed, so the sub-agent's own skill list and lookups only ever
// see those skills - every other skill behaves as if it did not exist. A
// session that does not expose the skill tool under its surface profile has
// nothing to swap; UnregisterTool's error is treated as "nothing to do" and
// not a failure.
//
// forkRunner, when non-nil, is wired into the replacement tool so a
// restricted sub-agent can still run a context: fork skill from its allowed
// list - the fresh NewSkillTool(nil) this builds otherwise has none, even if
// the tool it just unregistered did.
func RestrictSessionSkills(session *engine.Session, allowed []string, ctx context.Context, forkRunner skilltool.ForkRunner) {
	if err := session.UnregisterTool(skilltool.SkillToolName); err != nil {
		return
	}
	restricted := skilltool.NewSkillTool(nil)
	restricted.SetCwd(session.GetMetadata().RootPath)
	restricted.SetUserID(types.AgentUserIDFromContext(ctx))
	restricted.SetAllowedSkills(allowed)
	if forkRunner != nil {
		restricted.SetForkRunner(forkRunner)
	}
	if err := session.RegisterTool(restricted); err != nil {
		slog.Warn("failed to register restricted skill tool for sub-agent", "error", err)
	}
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
		slog.Warn("agent runner: hook failed", "event", event, "error", err)
		return
	}

	// Process hook results
	for _, result := range results {
		if result.Action == types.HookActionStop {
			slog.Info("agent runner: hook requested stop", "event", event, "message", result.Message)
		}
	}
}
