package agents

import (
	"context"
	"fmt"
	"strings"
	"time"

	coreagent "github.com/EngineerProjects/nexus-engine/internal/agent"
	"github.com/EngineerProjects/nexus-engine/internal/engine"
	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	"github.com/EngineerProjects/nexus-engine/internal/tools/schema"
	"github.com/EngineerProjects/nexus-engine/internal/types"
)

const spawnAgentName = "spawn_agent"
const spawnAgentSearchHint = "spawn a background sub-agent with a task and return a stable agent_id"
const spawnAgentDescription = `Launch a new background sub-agent to work on a specific task concurrently.

Returns immediately with a stable ` + "`agent_id`" + ` you can use with ` + "`wait_agent`" + `, ` + "`send_agent_message`" + `, and ` + "`close_agent`" + `.

## When to use
- Parallelise independent sub-tasks (research + implementation, analysis + writing, …)
- Delegate specialised work to a focused agent with a different role
- Run a long-running background task while you continue other work

## Lifecycle
1. Call ` + "`spawn_agent`" + ` → get back ` + "`agent_id`" + ` + initial ` + "`status`" + `
2. Optionally call ` + "`send_agent_message`" + ` to steer the agent between turns
3. Call ` + "`wait_agent`" + ` to block until the agent finishes and get its result
4. Call ` + "`close_agent`" + ` if you want to terminate the agent early

## agent_type
Use one of the built-in agent types: ` + "`general-purpose`" + `, ` + "`explore`" + `, ` + "`plan`" + `. Leave empty for ` + "`general-purpose`" + `.`

// SpawnAgentTool launches a sub-agent asynchronously and returns immediately.
// Mirrors Codex's CollabAgentTool = "spawnAgent" + CollabAgentSpawnBeginEvent / CollabAgentSpawnEndEvent.
type SpawnAgentTool struct {
	manager *coreagent.AsyncAgentManager
	eng     *engine.Engine
	tools   []tool.Tool
	reg     *coreagent.AgentRegistry
}

func NewSpawnAgentTool(eng *engine.Engine, tools []tool.Tool, reg *coreagent.AgentRegistry) *SpawnAgentTool {
	return &SpawnAgentTool{
		manager: coreagent.GetDefaultAsyncManager(),
		eng:     eng,
		tools:   tools,
		reg:     reg,
	}
}

func (t *SpawnAgentTool) Definition() tool.Definition {
	return tool.Definition{
		Name:        spawnAgentName,
		DisplayName: "SpawnAgent",
		SearchHint:  spawnAgentSearchHint,
		Description: spawnAgentDescription,
		Category:    "agents",
		InputSchema: schema.FromMap(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"prompt": map[string]any{
					"type":        "string",
					"description": "The task or instruction to send to the new agent. Be specific and self-contained.",
				},
				"agent_type": map[string]any{
					"type":        "string",
					"description": "Agent type: 'general-purpose' (default), 'explore', or 'plan'.",
					"enum":        []string{"general-purpose", "explore", "plan"},
				},
				"role": map[string]any{
					"type":        "string",
					"description": "Optional role description for this agent (e.g. 'code reviewer', 'data analyst'). Informational only.",
				},
				"nickname": map[string]any{
					"type":        "string",
					"description": "Optional human-friendly name for this agent (e.g. 'Orion'). Informational only.",
				},
				"max_turns": map[string]any{
					"type":        "integer",
					"description": "Maximum autonomous turns the agent may run. Default: 10.",
					"minimum":     1,
					"maximum":     50,
				},
			},
			"required": []string{"prompt"},
		}),
		IsReadOnly:         false,
		IsConcurrencySafe:  true,
		RequiresPermission: false,
	}
}

func (t *SpawnAgentTool) IsEnabled() bool                         { return true }
func (t *SpawnAgentTool) IsReadOnly(_ map[string]any) bool        { return false }
func (t *SpawnAgentTool) IsConcurrencySafe(_ map[string]any) bool { return true }
func (t *SpawnAgentTool) FormatResult(data any) string            { return fmt.Sprintf("%v", data) }
func (t *SpawnAgentTool) BackfillInput(_ context.Context, in map[string]any) map[string]any {
	return in
}
func (t *SpawnAgentTool) ValidateInput(_ context.Context, in map[string]any) (map[string]any, error) {
	if p, _ := in["prompt"].(string); strings.TrimSpace(p) == "" {
		return nil, fmt.Errorf("prompt is required")
	}
	return in, nil
}
func (t *SpawnAgentTool) CheckPermissions(_ context.Context, in map[string]any, _ tool.ToolUseContext) types.PermissionResult {
	return types.Passthrough(in)
}
func (t *SpawnAgentTool) Description(_ context.Context) (string, error) {
	return spawnAgentDescription, nil
}

func (t *SpawnAgentTool) Call(
	ctx context.Context,
	input tool.CallInput,
	_ types.CanUseToolFn,
) (tool.CallResult, error) {
	prompt, _ := input.Parsed["prompt"].(string)
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return tool.NewErrorResult(fmt.Errorf("prompt is required")), nil
	}

	agentType, _ := input.Parsed["agent_type"].(string)
	if agentType == "" {
		agentType = "general-purpose"
	}
	role, _ := input.Parsed["role"].(string)
	nickname, _ := input.Parsed["nickname"].(string)
	maxTurns := 10
	if v, ok := input.Parsed["max_turns"].(float64); ok && v >= 1 {
		maxTurns = int(v)
	}

	toolCtx := input.ToolContextValue()
	callID := toolCtx.ToolUseID

	// Emit spawn.begin — mirrors Codex CollabAgentSpawnBeginEvent.
	emitAgentEvent(ctx, types.RuntimeEventTypeAgentSpawnBegin, &types.AgentRuntimeEvent{
		CallID:        callID,
		AgentID:       "",
		AgentNickname: nickname,
		AgentRole:     role,
		Prompt:        prompt,
		Status:        "pendingInit",
		StartedAtMs:   nowMs(),
	})

	config := &coreagent.RunConfig{
		AgentType: agentType,
		Task:      prompt,
		Tools:     t.tools,
		Engine:    t.eng,
		MaxTurns:  maxTurns,
		Context:   ctx,
		Registry:  t.reg,
		Nickname:  nickname,
		Role:      role,
	}

	ag, err := t.manager.StartAgent(config)
	if err != nil {
		return tool.NewErrorResult(fmt.Errorf("spawn_agent failed: %w", err)), nil
	}

	// Emit spawn.end — mirrors Codex CollabAgentSpawnEndEvent.
	emitAgentEvent(ctx, types.RuntimeEventTypeAgentSpawnEnd, &types.AgentRuntimeEvent{
		CallID:        callID,
		AgentID:       ag.ID,
		AgentNickname: ag.Nickname,
		AgentRole:     ag.Role,
		Prompt:        prompt,
		Status:        ag.CollabStatus(),
		CompletedAtMs: nowMs(),
	})

	resp := map[string]any{
		"agent_id": ag.ID,
		"status":   ag.CollabStatus(),
	}
	if ag.Nickname != "" {
		resp["nickname"] = ag.Nickname
	}
	if ag.Role != "" {
		resp["role"] = ag.Role
	}
	resp["message"] = fmt.Sprintf("Agent '%s' spawned successfully. Use wait_agent('%s') to get the result.", ag.ID, ag.ID)

	res := tool.NewJSONResult(resp)
	res.Content = fmt.Sprintf("Agent spawned: %s (status: %s)", ag.ID, ag.CollabStatus())
	return res, nil
}

// ─── shared helpers ───────────────────────────────────────────────────────────

func nowMs() int64 {
	return time.Now().UnixMilli()
}

func emitAgentEvent(ctx context.Context, eventType types.RuntimeEventType, payload *types.AgentRuntimeEvent) {
	emitter, ok := ctx.Value(types.RuntimeEventEmitterKey).(func(types.RuntimeEvent))
	if !ok || emitter == nil {
		return
	}
	emitter(types.RuntimeEvent{
		Type:       eventType,
		Timestamp:  time.Now(),
		AgentEvent: payload,
	})
}
