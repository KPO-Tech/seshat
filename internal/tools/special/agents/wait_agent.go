package agents

import (
	"context"
	"fmt"
	"strings"
	"time"

	coreagent "github.com/EngineerProjects/nexus-engine/internal/agent"
	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	"github.com/EngineerProjects/nexus-engine/internal/tools/schema"
	"github.com/EngineerProjects/nexus-engine/internal/types"
)

const waitAgentName = "wait_agent"
const waitAgentSearchHint = "wait for a background agent to complete and retrieve its result"
const waitAgentDescription = `Block until a previously spawned agent finishes, then return its result.

Use the ` + "`agent_id`" + ` returned by ` + "`spawn_agent`" + `. Specify ` + "`timeout_seconds`" + ` (default 300) to cap the wait.
If the agent is already done when you call this, it returns immediately.

The result includes:
- ` + "`status`" + `: final CollabAgentStatus (completed | errored | shutdown | …)
- ` + "`output`" + `: the agent's final output text
- ` + "`turns`" + `: number of turns taken
- ` + "`elapsed_seconds`" + `: wall-clock time the agent ran

Mirrors Codex's CollabAgentTool = "wait" + CollabWaitingBeginEvent / CollabWaitingEndEvent.`

// WaitAgentTool blocks until an agent finishes and returns its result.
type WaitAgentTool struct {
	manager *coreagent.AsyncAgentManager
}

func NewWaitAgentTool() *WaitAgentTool {
	return &WaitAgentTool{manager: coreagent.GetDefaultAsyncManager()}
}

func (t *WaitAgentTool) Definition() tool.Definition {
	return tool.Definition{
		Name:        waitAgentName,
		DisplayName: "WaitAgent",
		SearchHint:  waitAgentSearchHint,
		Description: waitAgentDescription,
		Category:    "agents",
		InputSchema: schema.FromMap(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"agent_id": map[string]any{
					"type":        "string",
					"description": "The agent_id returned by spawn_agent.",
				},
				"timeout_seconds": map[string]any{
					"type":        "integer",
					"description": "Maximum seconds to wait before returning with status 'running'. Default: 300.",
					"minimum":     1,
					"maximum":     3600,
				},
			},
			"required": []string{"agent_id"},
		}),
		IsReadOnly:         true,
		IsConcurrencySafe:  true,
		RequiresPermission: false,
	}
}

func (t *WaitAgentTool) IsEnabled() bool                         { return true }
func (t *WaitAgentTool) IsReadOnly(_ map[string]any) bool        { return true }
func (t *WaitAgentTool) IsConcurrencySafe(_ map[string]any) bool { return true }
func (t *WaitAgentTool) FormatResult(data any) string            { return fmt.Sprintf("%v", data) }
func (t *WaitAgentTool) BackfillInput(_ context.Context, in map[string]any) map[string]any {
	return in
}
func (t *WaitAgentTool) ValidateInput(_ context.Context, in map[string]any) (map[string]any, error) {
	if id, _ := in["agent_id"].(string); strings.TrimSpace(id) == "" {
		return nil, fmt.Errorf("agent_id is required")
	}
	return in, nil
}
func (t *WaitAgentTool) CheckPermissions(_ context.Context, in map[string]any, _ tool.ToolUseContext) types.PermissionResult {
	return types.Passthrough(in)
}
func (t *WaitAgentTool) Description(_ context.Context) (string, error) {
	return waitAgentDescription, nil
}

func (t *WaitAgentTool) Call(
	ctx context.Context,
	input tool.CallInput,
	_ types.CanUseToolFn,
) (tool.CallResult, error) {
	agentID, _ := input.Parsed["agent_id"].(string)
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return tool.NewErrorResult(fmt.Errorf("agent_id is required")), nil
	}

	timeout := 300 * time.Second
	if v, ok := input.Parsed["timeout_seconds"].(float64); ok && v >= 1 {
		timeout = time.Duration(v) * time.Second
	}

	ag, err := t.manager.GetAgent(agentID)
	if err != nil {
		return tool.NewErrorResult(fmt.Errorf("agent not found: %s", agentID)), nil
	}

	callID := input.ToolContextValue().ToolUseID

	// Emit wait.begin — mirrors Codex CollabWaitingBeginEvent.
	emitAgentEvent(ctx, types.RuntimeEventTypeAgentWaitBegin, &types.AgentRuntimeEvent{
		CallID:      callID,
		AgentID:     agentID,
		AgentNickname: ag.Nickname,
		AgentRole:   ag.Role,
		Status:      ag.CollabStatus(),
		StartedAtMs: nowMs(),
	})

	// Wait with timeout (non-blocking if already done).
	if !ag.IsComplete() {
		if waitErr := ag.WaitWithTimeout(timeout); waitErr != nil {
			// Timed out — return current status so caller can decide.
			emitAgentEvent(ctx, types.RuntimeEventTypeAgentWaitEnd, &types.AgentRuntimeEvent{
				CallID:        callID,
				AgentID:       agentID,
				AgentNickname: ag.Nickname,
				AgentRole:     ag.Role,
				Status:        ag.CollabStatus(),
				CompletedAtMs: nowMs(),
			})
			resp := map[string]any{
				"agent_id": agentID,
				"status":   ag.CollabStatus(),
				"timed_out": true,
				"message":  fmt.Sprintf("Timeout after %.0fs — agent still running. Call wait_agent again or close_agent to cancel.", timeout.Seconds()),
			}
			res := tool.NewJSONResult(resp)
			res.Content = fmt.Sprintf("Agent %s still running after %.0fs timeout.", agentID, timeout.Seconds())
			return res, nil
		}
	}

	// Emit wait.end — mirrors Codex CollabWaitingEndEvent.
	emitAgentEvent(ctx, types.RuntimeEventTypeAgentWaitEnd, &types.AgentRuntimeEvent{
		CallID:        callID,
		AgentID:       agentID,
		AgentNickname: ag.Nickname,
		AgentRole:     ag.Role,
		Status:        ag.CollabStatus(),
		CompletedAtMs: nowMs(),
	})

	resp := buildWaitResult(ag)
	res := tool.NewJSONResult(resp)
	res.Content = formatWaitSummary(ag)
	return res, nil
}

func buildWaitResult(ag *coreagent.AsyncAgent) map[string]any {
	resp := map[string]any{
		"agent_id":        ag.ID,
		"status":          ag.CollabStatus(),
		"elapsed_seconds": ag.GetDuration().Seconds(),
	}
	if ag.Nickname != "" {
		resp["nickname"] = ag.Nickname
	}
	if ag.Role != "" {
		resp["role"] = ag.Role
	}

	ag.GetProgress() // ensure progress snapshot is consistent
	if ag.Result != nil {
		resp["output"] = ag.Result.Output
		resp["turns"] = ag.Result.Turns
		resp["tool_uses"] = ag.Result.ToolUses
	}
	if ag.Error != nil {
		resp["error"] = ag.Error.Error()
	}
	return resp
}

func formatWaitSummary(ag *coreagent.AsyncAgent) string {
	status := ag.CollabStatus()
	switch status {
	case "completed":
		output := ""
		if ag.Result != nil {
			output = ag.Result.Output
		}
		return fmt.Sprintf("Agent %s completed in %.1fs:\n%s", ag.ID, ag.GetDuration().Seconds(), output)
	case "errored":
		errMsg := ""
		if ag.Error != nil {
			errMsg = ag.Error.Error()
		}
		return fmt.Sprintf("Agent %s failed: %s", ag.ID, errMsg)
	default:
		return fmt.Sprintf("Agent %s finished with status: %s", ag.ID, status)
	}
}
