package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	coreagent "github.com/EngineerProjects/nexus-engine/internal/agent"
	"github.com/EngineerProjects/nexus-engine/internal/engine"
	"github.com/EngineerProjects/nexus-engine/internal/runtime/tasks"
	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	"github.com/EngineerProjects/nexus-engine/internal/tools/schema"
	worktreeTool "github.com/EngineerProjects/nexus-engine/internal/tools/special/worktree"
	"github.com/EngineerProjects/nexus-engine/internal/types"
)

// subAgentDepthKey is the context key used to propagate spawn depth so that
// each nested agent call can check whether it has exceeded the configured limit.
type subAgentDepthKey struct{}

// effectiveMaxDepth returns the active spawn depth limit for this request.
// Reads the per-request value set by types.WithSubAgentMaxDepth (injected by
// the API handler from the user's preference), clamped to MaxAbsoluteSubAgentDepth.
// Falls back to the compile-time constant MaxSubAgentDepth when no override exists.
func effectiveMaxDepth(ctx context.Context) int {
	if d := types.SubAgentMaxDepthFromContext(ctx); d > 0 {
		if d > coreagent.MaxAbsoluteSubAgentDepth {
			return coreagent.MaxAbsoluteSubAgentDepth
		}
		return d
	}
	return coreagent.MaxSubAgentDepth
}

// subAgentDepth returns the current spawn depth stored in ctx (0 = top-level).
// Returns 0 safely for a nil context.
func subAgentDepth(ctx context.Context) int {
	if ctx == nil {
		return 0
	}
	if d, ok := ctx.Value(subAgentDepthKey{}).(int); ok {
		return d
	}
	return 0
}

// withSubAgentDepth returns a child context carrying depth+1.
// Falls back to context.Background() if ctx is nil.
func withSubAgentDepth(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, subAgentDepthKey{}, subAgentDepth(ctx)+1)
}

// Config for Agent tool
type AgentToolConfig struct {
	Enabled bool
}

// Default config
func DefaultAgentToolConfig() *AgentToolConfig {
	return &AgentToolConfig{Enabled: true}
}

// AgentTool is the agent tool
type AgentTool struct {
	config   *AgentToolConfig
	runner   *coreagent.Runner
	engine   *engine.Engine
	registry *coreagent.AgentRegistry
}

// NewAgentTool creates a new agent tool
func NewAgentTool(config *AgentToolConfig) *AgentTool {
	if config == nil {
		config = DefaultAgentToolConfig()
	}
	return &AgentTool{config: config}
}

// SetRunner sets an advanced runner for the agent tool
func (t *AgentTool) SetRunner(runner *coreagent.Runner) {
	t.runner = runner
}

// GetRunner returns the current runner (if any)
func (t *AgentTool) GetRunner() *coreagent.Runner {
	return t.runner
}

// SetEngine binds this tool instance to a specific engine.
func (t *AgentTool) SetEngine(e *engine.Engine) {
	t.engine = e
}

// SetRegistry sets the agent registry for resolving dynamic (skill-based) agent types.
func (t *AgentTool) SetRegistry(r *coreagent.AgentRegistry) {
	t.registry = r
}

// Definition returns the tool definition
func (t *AgentTool) Definition() tool.Definition {
	return tool.Definition{
		Name:               coreagent.ToolNameAgent,
		DisplayName:        "agent",
		SearchHint:         coreagent.SearchHintAgent,
		Description:        coreagent.DescriptionAgent + "\n\n" + coreagent.AgentPrompt,
		Category:           "agent",
		IsReadOnly:         false,
		IsDestructive:      false,
		IsConcurrencySafe:  false,
		RequiresPermission: true,
		Metadata: map[string]any{
			"is_stateful":   false,
			"subagent_type": "agent",
		},
		InputSchema: schema.FromMap(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"type": map[string]any{
					"type":        "string",
					"description": "Agent type to run. Must be one of the built-in types.",
					"enum":        []string{"general-purpose", "explore", "browse", "plan", "verify"},
				},
				"task": map[string]any{
					"type":        "string",
					"description": "The self-contained task description for the agent. Include exact file paths, goals, constraints, and what to return.",
				},
				"maxTurns": map[string]any{
					"type":        "integer",
					"description": "Maximum autonomous turns. Default: 10 for sub-agents.",
					"minimum":     1,
					"maximum":     200,
				},
				"run_in_background": map[string]any{
					"type":        "boolean",
					"description": "When true, the agent runs asynchronously and returns a task ID immediately. Use TaskGet to poll status.",
					"default":     false,
				},
				"fork": map[string]any{
					"type":        "boolean",
					"description": "When true, the sub-agent inherits the parent session's message history.",
					"default":     false,
				},
				"isolation": map[string]any{
					"type":        "string",
					"description": "Set to 'worktree' to run the agent in an isolated git worktree. Changes are isolated until merged.",
					"enum":        []string{"worktree"},
				},
				"tools": map[string]any{
					"type":        "array",
					"description": "Optional list of tool name patterns to restrict the sub-agent's tool surface.",
					"items":       map[string]any{"type": "string"},
				},
			},
			"required": []string{"type", "task"},
		}),
	}
}

// Call executes the tool
func (t *AgentTool) Call(
	ctx context.Context,
	input tool.CallInput,
	permissionCheck types.CanUseToolFn,
) (tool.CallResult, error) {
	var parsedInput map[string]any
	if err := json.Unmarshal([]byte(input.Raw), &parsedInput); err != nil {
		return tool.CallResult{
			Data:    map[string]any{"error": fmt.Sprintf("Failed to parse input: %v", err)},
			Content: fmt.Sprintf("Failed to parse input: %v", err),
		}, nil
	}

	agentType := ""
	task := ""

	// Accept both "type" and "agent_type" — the LLM sometimes uses the spawn_agent
	// convention. Priority: "type" > "agent_type" to keep backwards compat.
	if v, ok := parsedInput["type"].(string); ok {
		agentType = v
	}
	if agentType == "" {
		if v, ok := parsedInput["agent_type"].(string); ok {
			agentType = v
		}
	}
	if v, ok := parsedInput["task"].(string); ok {
		task = v
	}
	if v, ok := parsedInput["prompt"].(string); ok {
		task = v
	}

	if agentType == "" {
		available := t.listAvailableAgents()
		typeList := ""
		for _, a := range available {
			typeList += fmt.Sprintf("  - %s: %s\n", a["type"], a["whenToUse"])
		}
		return tool.CallResult{
			Data: map[string]any{
				"error":     "type is required",
				"available": available,
			},
			Content: fmt.Sprintf("Error: 'type' field is required. Available agent types:\n%s", typeList),
		}, nil
	}
	if task == "" {
		return tool.CallResult{
			Data:    map[string]any{"error": "task is required"},
			Content: "Error: task is required",
		}, nil
	}

	var agentDef *coreagent.AgentDefinition
	if t.registry != nil {
		agentDef, _ = t.registry.Get(agentType)
	}
	if agentDef == nil {
		if builtIn := coreagent.GetBuiltInAgentByType(agentType); builtIn != nil {
			agentDef = coreagent.ToAgentDefinition(*builtIn)
		}
	}
	if agentDef == nil {
		available := t.listAvailableAgents()
		typeList := ""
		for _, a := range available {
			typeList += fmt.Sprintf("  - %s: %s\n", a["type"], a["whenToUse"])
		}
		return tool.CallResult{
			Data: map[string]any{
				"error":     fmt.Sprintf("unknown agent type: %s", agentType),
				"available": available,
			},
			Content: fmt.Sprintf("Error: unknown agent type '%s'. Available types:\n%s", agentType, typeList),
		}, nil
	}

	maxTurns := 0
	if m, ok := parsedInput["maxTurns"].(float64); ok {
		maxTurns = int(m)
	}

	var allowedTools []string
	if toolsVal, ok := parsedInput["tools"].([]any); ok {
		for _, v := range toolsVal {
			if s, ok := v.(string); ok {
				allowedTools = append(allowedTools, s)
			}
		}
	}

	runInBackground := false
	if rb, ok := parsedInput["run_in_background"].(bool); ok {
		runInBackground = rb
	}

	forkMode := false
	if f, ok := parsedInput["fork"].(bool); ok {
		forkMode = f
	}

	isolationMode := ""
	if iso, ok := parsedInput["isolation"].(string); ok {
		isolationMode = iso
	}

	fullPrompt := buildAgentTaskPrompt(agentDef, task)

	// Build a sub-agent event emitter that tags every forwarded event with this tool use ID,
	// so the frontend can route streaming events to the correct sub-agent card.
	var agentEventFn func(types.RuntimeEvent)
	if emitter, ok := ctx.Value(types.RuntimeEventEmitterKey).(func(types.RuntimeEvent)); ok && emitter != nil {
		toolUseID := input.ToolUseID
		agentEventFn = func(event types.RuntimeEvent) {
			event.AgentToolUseID = toolUseID
			emitter(event)
		}
	}

	if isolationMode == "worktree" {
		result, err := t.runAgentInWorktree(ctx, agentType, fullPrompt, maxTurns, allowedTools, parsedInput, agentEventFn)
		if err != nil {
			return tool.CallResult{Data: map[string]any{"error": err.Error()}, Content: err.Error()}, nil
		}
		if !result.Success {
			return tool.CallResult{
				Data:    map[string]any{"error": result.Error, "agentType": agentType},
				Content: fmt.Sprintf("Agent failed (worktree): %s\n\nError: %s", result.Output, result.Error),
			}, nil
		}
		wtData := map[string]any{
			"agentType":    agentType,
			"task":         task,
			"turns":        result.Turns,
			"toolUses":     result.ToolUses,
			"success":      result.Success,
			"isolation":    "worktree",
			"worktreePath": result.WorktreePath,
		}
		if len(result.Sources) > 0 {
			wtData["sources"] = result.Sources
		}
		return tool.CallResult{
			Data:    wtData,
			Content: result.Output + "\n\nWorktree created at: " + result.WorktreePath + "\nUse ExitWorktree to merge or discard changes.",
		}, nil
	}

	if runInBackground {
		result, err := t.runAgentBackground(ctx, agentType, fullPrompt, allowedTools)
		if err != nil {
			return tool.CallResult{Data: map[string]any{"error": err.Error()}, Content: err.Error()}, nil
		}
		return result, nil
	}

	if forkMode {
		forkMessages := extractForkMessagesFromInput(input, parsedInput)
		result := t.runForkAgent(ctx, agentType, task, maxTurns, allowedTools, forkMessages, agentEventFn)
		if !result.Success {
			return tool.CallResult{
				Data:    map[string]any{"error": result.Error, "agentType": agentType},
				Content: fmt.Sprintf("Fork agent failed: %s\n\nError: %s", result.Output, result.Error),
			}, nil
		}
		forkData := map[string]any{
			"agentType": agentType,
			"task":      task,
			"turns":     result.Turns,
			"toolUses":  result.ToolUses,
			"success":   result.Success,
			"fork":      true,
		}
		if len(result.Sources) > 0 {
			forkData["sources"] = result.Sources
		}
		return tool.CallResult{Data: forkData, Content: result.Output}, nil
	}

	result := t.runAgent(ctx, agentType, task, maxTurns, allowedTools, agentEventFn)
	if !result.Success {
		return tool.CallResult{
			Data:    map[string]any{"error": result.Error, "agentType": agentType},
			Content: fmt.Sprintf("Agent failed: %s\n\nError: %s", result.Output, result.Error),
		}, nil
	}
	data := map[string]any{
		"agentType": agentType,
		"task":      task,
		"turns":     result.Turns,
		"toolUses":  result.ToolUses,
		"success":   result.Success,
	}
	if len(result.Sources) > 0 {
		data["sources"] = result.Sources
	}
	return tool.CallResult{Data: data, Content: result.Output}, nil
}

// runAgent executes the agent synchronously.
//
// Resource limits applied here:
//  1. Spawn depth — rejected if the call stack already reached MaxSubAgentDepth.
//     Prevents infinite delegation chains (A→B→C→…).
//  2. Wall-clock timeout — sub-agents receive a context bounded by
//     DefaultSubAgentTimeout so a stalled LLM cannot block the parent forever.
//  3. MaxTurns reduction — sub-agents default to DefaultSubAgentMaxTurns
//     (lower than the top-level DefaultMaxTurns) to limit token consumption.
func (t *AgentTool) runAgent(ctx context.Context, agentType, task string, maxTurns int, allowedTools []string, eventFn func(types.RuntimeEvent)) *coreagent.RunResult {
	// ── 1. Spawn depth guard ────────────────────────────────────────────────
	depth := subAgentDepth(ctx)
	maxDepth := effectiveMaxDepth(ctx)
	if depth >= maxDepth {
		return &coreagent.RunResult{
			AgentType: agentType,
			Success:   false,
			Error: fmt.Sprintf(
				"sub-agent spawn rejected: maximum depth %d reached (current depth %d). "+
					"Reduce agent nesting or increase the limit in Settings → Agent.",
				maxDepth, depth,
			),
		}
	}

	eng := t.engine
	if eng == nil {
		return &coreagent.RunResult{
			AgentType: agentType,
			Success:   false,
			Error:     "engine not available - call agentToolInstance.SetEngine(engine) first",
		}
	}

	agentDef := t.resolveAgentDef(agentType)
	if agentDef == nil {
		return &coreagent.RunResult{
			AgentType: agentType,
			Success:   false,
			Error:     fmt.Sprintf("unknown agent type: %s", agentType),
		}
	}

	// ── 2. MaxTurns — sub-agents get a tighter default than top-level ───────
	effectiveMaxTurns := maxTurns
	if effectiveMaxTurns == 0 {
		effectiveMaxTurns = agentDef.MaxTurns
	}
	if effectiveMaxTurns == 0 {
		if depth > 0 {
			effectiveMaxTurns = coreagent.DefaultSubAgentMaxTurns
		} else {
			effectiveMaxTurns = coreagent.DefaultMaxTurns
		}
	}

	// ── 3. Wall-clock timeout ───────────────────────────────────────────────
	// Always bound sub-agent execution so a stalled model cannot block the
	// parent session indefinitely. The caller's context deadline (if tighter)
	// still takes precedence via context cancellation chaining.
	timeout := time.Duration(coreagent.DefaultSubAgentTimeout) * time.Second
	subCtx, cancel := context.WithTimeout(withSubAgentDepth(ctx), timeout)
	defer cancel()

	filteredToolPatterns := filterToolPatterns(allowedTools, agentDef.GetToolPatterns())

	config := &coreagent.RunConfig{
		AgentType:  agentType,
		Task:       buildAgentTaskPrompt(agentDef, task),
		Engine:     eng,
		MaxTurns:   effectiveMaxTurns,
		Context:    subCtx,
		Tools:      nil,
		Callback:   nil,
		ToolFilter: filteredToolPatterns,
		EventFn:    eventFn,
		Registry:   t.registry,
	}

	result, err := t.executeRunConfig(subCtx, config)
	if err != nil {
		if result == nil {
			return &coreagent.RunResult{
				AgentType: agentType,
				Success:   false,
				Error:     fmt.Sprintf("agent execution failed: %v", err),
			}
		}
		result.Error = fmt.Sprintf("%s\nExecution error: %v", result.Error, err)
		result.Success = false
	}
	return result
}

// buildAgentTaskPrompt returns the task as-is. The agent's system prompt is
// injected via SetSystemPromptTemplate on the session (see RunAgent in internal/agent).
func buildAgentTaskPrompt(_ *coreagent.AgentDefinition, task string) string {
	return task
}

// ValidateInput validates tool input
func (t *AgentTool) ValidateInput(ctx context.Context, input map[string]any) (map[string]any, error) {
	if input["type"] == nil || input["type"] == "" {
		return map[string]any{
			"result":  false,
			"message": "type is required",
		}, nil
	}
	if input["task"] == nil && input["prompt"] == nil {
		return map[string]any{
			"result":  false,
			"message": "task or prompt is required",
		}, nil
	}
	return input, nil
}

// CheckPermissions checks permissions
func (t *AgentTool) CheckPermissions(ctx context.Context, input map[string]any, toolCtx tool.ToolUseContext) types.PermissionResult {
	return types.PermissionResult{Behavior: types.PermissionBehaviorAsk}
}

// IsConcurrencySafe returns whether tool is concurrency safe
func (t *AgentTool) IsConcurrencySafe(input map[string]any) bool {
	return false
}

// IsReadOnly returns whether tool is read-only
func (t *AgentTool) IsReadOnly(input map[string]any) bool {
	return false
}

// IsEnabled returns whether tool is enabled
func (t *AgentTool) IsEnabled() bool {
	return t.config.Enabled
}

// FormatResult formats result
func (t *AgentTool) FormatResult(data any) string {
	bytes, _ := json.Marshal(data)
	return string(bytes)
}

// BackfillInput enriches input
func (t *AgentTool) BackfillInput(ctx context.Context, input map[string]any) map[string]any {
	if input["maxTurns"] == nil {
		input["maxTurns"] = coreagent.DefaultMaxTurns
	}
	return input
}

// Description returns description
func (t *AgentTool) Description(ctx context.Context) (string, error) {
	return coreagent.DescriptionAgent, nil
}

// resolveAgentDef looks up an agent definition using the registry first, then built-ins.
func (t *AgentTool) resolveAgentDef(agentType string) *coreagent.AgentDefinition {
	if t.registry != nil {
		if def, ok := t.registry.Get(agentType); ok {
			return def
		}
	}
	if builtIn := coreagent.GetBuiltInAgentByType(agentType); builtIn != nil {
		return coreagent.ToAgentDefinition(*builtIn)
	}
	return nil
}

// listAvailableAgents returns all agents visible to this tool.
func (t *AgentTool) listAvailableAgents() []map[string]any {
	if t.registry != nil {
		defs := t.registry.All()
		result := make([]map[string]any, 0, len(defs))
		for _, def := range defs {
			result = append(result, map[string]any{
				"type":      def.AgentType,
				"whenToUse": def.WhenToUse,
				"maxTurns":  def.MaxTurns,
			})
		}
		return result
	}
	return coreagent.ListAvailableAgents()
}

// filterToolPatterns computes the intersection of main agent's tools and agent definition's allowed patterns.
func filterToolPatterns(mainAgentTools []string, agentPatterns []string) []string {
	if len(mainAgentTools) == 0 {
		return agentPatterns
	}
	if len(agentPatterns) == 0 || (len(agentPatterns) == 1 && agentPatterns[0] == "*") {
		return mainAgentTools
	}
	result := make([]string, 0)
	for _, t := range mainAgentTools {
		if matchesPattern(t, agentPatterns) {
			result = append(result, t)
		}
	}
	return result
}

// matchesPattern checks if a tool name matches any of the patterns.
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

// runAgentBackground runs an agent in the background using the task manager
func (t *AgentTool) runAgentBackground(ctx context.Context, agentType, prompt string, allowedTools []string) (tool.CallResult, error) {
	manager := tasks.GlobalManager()
	if manager == nil {
		return tool.CallResult{
			Data:    map[string]any{"error": "task manager not available"},
			Content: "Error: background task manager not available",
		}, nil
	}

	registerTaskCompletionCallbackOnce.Do(func() {
		manager.SetCompletionCallback(func(task *tasks.Task) {
			if task.Type == tasks.TaskTypeAgent {
				NotifyAgentCompletion(string(task.ID), "agent", task.Description, task.Output, task.Status == tasks.TaskStatusCompleted)
			}
		})
	})

	eng := t.engine
	if eng == nil {
		return tool.CallResult{
			Data:    map[string]any{"error": "engine not available"},
			Content: "Error: engine not available for background agent",
		}, nil
	}

	task, err := manager.CreateAgentTask(ctx, prompt, nil)
	if err != nil {
		return tool.CallResult{
			Data:    map[string]any{"error": fmt.Sprintf("failed to create background agent: %v", err)},
			Content: fmt.Sprintf("Error: failed to create background agent: %v", err),
		}, nil
	}

	return tool.CallResult{
		Data: map[string]any{
			"agentType":   agentType,
			"taskId":      string(task.ID),
			"status":      string(task.Status),
			"background":  true,
			"description": task.Description,
			"onComplete":  "You will be notified when the agent completes",
		},
		Content: fmt.Sprintf("Agent '%s' started in background (task ID: %s)\n\nYou will be notified when it completes. Use TaskGet to check status or TaskList to see all running tasks.", agentType, task.ID),
	}, nil
}

// AgentCompletionHandler is a function type for handling agent completion
type AgentCompletionHandler func(taskID, agentType, description, output string, success bool)

var (
	agentCompletionHandlers            = make([]AgentCompletionHandler, 0)
	completionHandlerMu                sync.Mutex
	registerTaskCompletionCallbackOnce sync.Once
)

// RegisterAgentCompletionHandler registers a callback to be called when background agents complete
func RegisterAgentCompletionHandler(handler AgentCompletionHandler) {
	completionHandlerMu.Lock()
	defer completionHandlerMu.Unlock()
	agentCompletionHandlers = append(agentCompletionHandlers, handler)
}

// NotifyAgentCompletion is called when a background agent task completes
func NotifyAgentCompletion(taskID, agentType, description, output string, success bool) {
	completionHandlerMu.Lock()
	handlers := make([]AgentCompletionHandler, len(agentCompletionHandlers))
	copy(handlers, agentCompletionHandlers)
	completionHandlerMu.Unlock()
	for _, handler := range handlers {
		handler(taskID, agentType, description, output, success)
	}
}

// extractForkMessagesFromInput extracts inherited transcript for fork mode.
func extractForkMessagesFromInput(callInput tool.CallInput, input map[string]any) []types.Message {
	if messages := transcriptMessagesFromToolContext(callInput); len(messages) > 0 {
		return messages
	}
	if messages := transcriptMessagesFromRawInput(input); len(messages) > 0 {
		return messages
	}
	return nil
}

// runForkAgent runs an agent in fork mode (inherits parent's messages).
// Applies the same depth and timeout guards as runAgent.
func (t *AgentTool) runForkAgent(ctx context.Context, agentType, task string, maxTurns int, allowedTools []string, forkMessages []types.Message, eventFn func(types.RuntimeEvent)) *coreagent.RunResult {
	depth := subAgentDepth(ctx)
	maxDepth := effectiveMaxDepth(ctx)
	if depth >= maxDepth {
		return &coreagent.RunResult{
			AgentType: agentType,
			Success:   false,
			Error: fmt.Sprintf(
				"fork agent spawn rejected: maximum depth %d reached (current depth %d)",
				maxDepth, depth,
			),
		}
	}

	eng := t.engine
	if eng == nil {
		return &coreagent.RunResult{
			AgentType: agentType,
			Success:   false,
			Error:     "engine not available - call agentToolInstance.SetEngine(engine) first",
		}
	}

	agentDef := t.resolveAgentDef(agentType)
	if agentDef == nil {
		return &coreagent.RunResult{
			AgentType: agentType,
			Success:   false,
			Error:     fmt.Sprintf("unknown agent type: %s", agentType),
		}
	}

	effectiveMaxTurns := maxTurns
	if effectiveMaxTurns == 0 {
		effectiveMaxTurns = agentDef.MaxTurns
	}
	if effectiveMaxTurns == 0 {
		if depth > 0 {
			effectiveMaxTurns = coreagent.DefaultSubAgentMaxTurns
		} else {
			effectiveMaxTurns = coreagent.DefaultMaxTurns
		}
	}

	timeout := time.Duration(coreagent.DefaultSubAgentTimeout) * time.Second
	subCtx, cancel := context.WithTimeout(withSubAgentDepth(ctx), timeout)
	defer cancel()

	config := &coreagent.RunConfig{
		AgentType:        agentType,
		Task:             buildAgentTaskPrompt(agentDef, task),
		Engine:           eng,
		MaxTurns:         effectiveMaxTurns,
		Context:          subCtx,
		Tools:            nil,
		ToolFilter:       filterToolPatterns(allowedTools, agentDef.GetToolPatterns()),
		ForkFromMessages: forkMessages,
		Callback:         nil,
		EventFn:          eventFn,
		Registry:         t.registry,
	}

	result, err := t.executeRunConfig(subCtx, config)
	if err != nil {
		if result == nil {
			return &coreagent.RunResult{
				AgentType: agentType,
				Success:   false,
				Error:     fmt.Sprintf("fork agent execution failed: %v", err),
			}
		}
		result.Error = fmt.Sprintf("%s\nExecution error: %v", result.Error, err)
		result.Success = false
	}
	return result
}

func transcriptMessagesFromToolContext(callInput tool.CallInput) []types.Message {
	toolCtx := callInput.ToolContextValue()
	if len(toolCtx.Metadata) == 0 {
		return nil
	}
	rawMessages, ok := toolCtx.Metadata["transcript_messages"]
	if !ok {
		return nil
	}
	messages, ok := rawMessages.([]types.Message)
	if !ok {
		return nil
	}
	cloned := make([]types.Message, len(messages))
	copy(cloned, messages)
	return cloned
}

func transcriptMessagesFromRawInput(input map[string]any) []types.Message {
	rawMessages, ok := input["messages"]
	if !ok {
		return nil
	}
	messageMaps, ok := rawMessages.([]any)
	if !ok {
		return nil
	}

	messages := make([]types.Message, 0, len(messageMaps))
	for _, rawMessage := range messageMaps {
		messageMap, ok := rawMessage.(map[string]any)
		if !ok {
			continue
		}
		role, _ := messageMap["role"].(string)
		text, _ := messageMap["text"].(string)
		if role == "" || text == "" {
			continue
		}
		message := types.Message{
			ID:      types.MessageID(fmt.Sprintf("fork-%d", len(messages)+1)),
			Role:    types.Role(role),
			Content: []types.ContentBlock{types.TextContent{Text: text}},
		}
		messages = append(messages, message)
	}
	return messages
}

func (t *AgentTool) executeRunConfig(ctx context.Context, config *coreagent.RunConfig) (*coreagent.RunResult, error) {
	if t.runner != nil {
		return t.runner.RunAgentAdvanced(ctx, config)
	}
	return coreagent.RunAgent(config)
}

// runAgentInWorktree runs an agent in an isolated git worktree
func (t *AgentTool) runAgentInWorktree(ctx context.Context, agentType, task string, maxTurns int, allowedTools []string, input map[string]any, eventFn func(types.RuntimeEvent)) (*coreagent.RunResult, error) {
	worktreeManager := worktreeTool.NewWorktreeManager(worktreeTool.WorktreeConfig{
		UseGitWorktree:  true,
		WorktreeBaseDir: ".worktrees",
	})

	worktreeSlug := fmt.Sprintf("agent-%s-%d", agentType, time.Now().Unix())

	branch := ""
	if b, ok := input["branch"].(string); ok {
		branch = b
	}

	session, err := worktreeManager.CreateWorktree(ctx, worktreeSlug, branch, ".")
	if err != nil {
		return &coreagent.RunResult{
			AgentType: agentType,
			Success:   false,
			Error:     fmt.Sprintf("failed to create worktree: %v", err),
		}, nil
	}

	eng := t.engine
	if eng == nil {
		_ = worktreeManager.RemoveWorktree(session, false)
		return &coreagent.RunResult{
			AgentType: agentType,
			Success:   false,
			Error:     "engine not available",
		}, nil
	}

	agentDef := t.resolveAgentDef(agentType)
	if agentDef == nil {
		_ = worktreeManager.RemoveWorktree(session, false)
		return &coreagent.RunResult{
			AgentType: agentType,
			Success:   false,
			Error:     fmt.Sprintf("unknown agent type: %s", agentType),
		}, nil
	}

	effectiveMaxTurns := maxTurns
	if effectiveMaxTurns == 0 {
		effectiveMaxTurns = agentDef.MaxTurns
	}
	if effectiveMaxTurns == 0 {
		effectiveMaxTurns = coreagent.DefaultMaxTurns
	}

	config := &coreagent.RunConfig{
		AgentType:   agentType,
		Task:        buildAgentTaskPrompt(agentDef, task),
		Engine:      eng,
		MaxTurns:    effectiveMaxTurns,
		Context:     ctx,
		Tools:       nil,
		ToolFilter:  filterToolPatterns(allowedTools, agentDef.GetToolPatterns()),
		WorktreeDir: session.WorktreePath,
		Callback:    nil,
		EventFn:     eventFn,
		Registry:    t.registry,
	}

	result, err := coreagent.RunAgent(config)
	if err != nil {
		if result == nil {
			_ = worktreeManager.RemoveWorktree(session, false)
			return &coreagent.RunResult{
				AgentType: agentType,
				Success:   false,
				Error:     fmt.Sprintf("worktree agent execution failed: %v", err),
			}, nil
		}
		result.Error = fmt.Sprintf("%s\nExecution error: %v", result.Error, err)
		result.Success = false
	}

	result.WorktreePath = session.WorktreePath
	return result, nil
}
