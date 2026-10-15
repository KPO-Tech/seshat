package agents

import (
	"context"
	"fmt"
	"strings"
	"time"

	coreagent "github.com/KPO-Tech/seshat-runtime/internal/agent"
	"github.com/KPO-Tech/seshat-runtime/internal/runtime/tasks"
	tool "github.com/KPO-Tech/seshat-runtime/internal/tools/registry"
	"github.com/KPO-Tech/seshat-runtime/internal/tools/schema"
	"github.com/KPO-Tech/seshat-runtime/internal/types"
)

const waitAgentName = "wait_agent"
const waitAgentSearchHint = "wait for a background agent to complete and retrieve its result"
const waitAgentDescription = `Block until a previously spawned agent finishes, then return its result.

Use the ` + "`agent_id`" + ` returned by ` + "`spawn_agent`" + `, or the task ID returned by ` + "`agent`" + ` when it ran in the background. Specify ` + "`timeout_seconds`" + ` (default 300) to cap the wait.
If the agent is already done when you call this, it returns immediately.

The result includes:
- ` + "`status`" + `: final CollabAgentStatus (completed | errored | shutdown | …)
- ` + "`output`" + `: the agent's final output text
- ` + "`turns`" + `: number of turns taken
- ` + "`elapsed_seconds`" + `: wall-clock time the agent ran
- ` + "`session_id`" + `: persisted session ID — pass to ` + "`resume_agent`" + ` to continue from where the agent left off

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
		// An agent started with agent in the background is a task of the task
		// manager, not an agent of spawn_agent, and the model naturally passes its id
		// here: wait for it instead of answering "agent not found".
		if res, ok := waitBackgroundTask(ctx, agentID, timeout); ok {
			return res, nil
		}
		// Help the LLM distinguish agent_id (from spawn_agent) from tool_use_id.
		hint := ""
		if len(agentID) > 10 && (agentID[8] == '-' || agentID[4] == '-') {
			hint = " (this looks like a tool_use_id — use the agent_id returned by spawn_agent instead)"
		}
		return tool.NewErrorResult(fmt.Errorf("agent not found: %s%s", agentID, hint)), nil
	}

	callID := input.ToolContextValue().ToolUseID

	// Emit wait.begin — mirrors Codex CollabWaitingBeginEvent.
	emitAgentEvent(ctx, types.RuntimeEventTypeAgentWaitBegin, &types.AgentRuntimeEvent{
		CallID:        callID,
		AgentID:       agentID,
		AgentNickname: ag.Nickname,
		AgentRole:     ag.Role,
		Status:        ag.CollabStatus(),
		StartedAtMs:   nowMs(),
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
				"agent_id":  agentID,
				"status":    ag.CollabStatus(),
				"timed_out": true,
				"message":   fmt.Sprintf("Timeout after %.0fs — agent still running. Call wait_agent again or close_agent to cancel.", timeout.Seconds()),
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
		if len(ag.Result.Sources) > 0 {
			resp["sources"] = ag.Result.Sources
		}
	}
	if ag.SessionID != "" {
		resp["session_id"] = ag.SessionID
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
		summary := fmt.Sprintf("Agent %s completed in %.1fs:\n%s", ag.ID, ag.GetDuration().Seconds(), output)
		if ag.Result != nil && len(ag.Result.Sources) > 0 {
			summary += fmt.Sprintf("\n\nSources consulted (%d):", len(ag.Result.Sources))
			for _, s := range ag.Result.Sources {
				summary += fmt.Sprintf("\n  [%s] %s", s.Type, s.Value)
			}
		}
		return summary
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

// maxBackgroundOutputRunes caps the answer of a background agent returned by wait_agent.
const maxBackgroundOutputRunes = 20000

// waitBackgroundTask waits for a task of the task manager and returns its answer. ok is
// false when the id is not a task, so the caller can report the agent as unknown.
func waitBackgroundTask(ctx context.Context, id string, timeout time.Duration) (tool.CallResult, bool) {
	manager := tasks.GlobalManager()
	if manager == nil {
		return tool.CallResult{}, false
	}
	task, err := manager.GetTask(tasks.TaskID(id))
	if err != nil {
		return tool.CallResult{}, false
	}
	terminal := func(t *tasks.Task) bool {
		return t.Status == tasks.TaskStatusCompleted || t.Status == tasks.TaskStatusFailed || t.Status == tasks.TaskStatusKilled
	}
	if !terminal(task) {
		waited, waitErr := manager.WaitForTask(ctx, tasks.TaskID(id), timeout)
		if waited != nil {
			task = waited
		}
		if waitErr != nil && !terminal(task) {
			resp := map[string]any{
				"agent_id":  id,
				"status":    "running",
				"timed_out": true,
				"message":   fmt.Sprintf("Timeout after %.0fs - the agent is still running. Call wait_agent again.", timeout.Seconds()),
			}
			res := tool.NewJSONResult(resp)
			res.Content = fmt.Sprintf("Agent %s still running after %.0fs timeout.", id, timeout.Seconds())
			return res, true
		}
	}

	status := map[tasks.TaskStatus]string{
		tasks.TaskStatusCompleted: "completed",
		tasks.TaskStatusFailed:    "errored",
		tasks.TaskStatusKilled:    "shutdown",
	}[task.Status]
	resp := map[string]any{"agent_id": id, "status": status}
	if task.StartedAt != nil && task.CompletedAt != nil {
		resp["elapsed_seconds"] = task.CompletedAt.Sub(*task.StartedAt).Seconds()
	}
	output, _ := manager.ReadTaskOutput(tasks.TaskID(id))
	if runes := []rune(output); len(runes) > maxBackgroundOutputRunes {
		output = string(runes[:maxBackgroundOutputRunes])
		resp["output_truncated"] = true
	}
	if output != "" {
		resp["output"] = output
	}
	if task.Error != "" {
		resp["error"] = task.Error
	}
	res := tool.NewJSONResult(resp)
	switch status {
	case "completed":
		res.Content = fmt.Sprintf("Agent %s completed:\n%s", id, output)
	case "errored":
		res.Content = fmt.Sprintf("Agent %s failed: %s", id, task.Error)
	default:
		res.Content = fmt.Sprintf("Agent %s finished with status: %s", id, status)
	}
	return res, true
}
