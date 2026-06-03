package agents

import (
	"context"
	"fmt"
	"strings"

	coreagent "github.com/EngineerProjects/nexus-engine/internal/agent"
	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	"github.com/EngineerProjects/nexus-engine/internal/tools/schema"
	"github.com/EngineerProjects/nexus-engine/internal/types"
)

const closeAgentName = "close_agent"
const closeAgentSearchHint = "terminate a background agent and release its resources"
const closeAgentDescription = `Terminate a sub-agent and remove it from the active registry.

Call this when:
- You no longer need the agent's result
- The agent is running too long and you want to cancel it
- The agent has already finished and you want to free its memory

Unlike ` + "`wait_agent`" + ` (which only reads), ` + "`close_agent`" + ` is irreversible — you will not be able to
retrieve the agent's output after calling this.

Mirrors Codex's CollabAgentTool = "closeAgent" + CollabCloseBeginEvent / CollabCloseEndEvent.`

// CloseAgentTool terminates a sub-agent and removes it from the registry.
type CloseAgentTool struct {
	manager *coreagent.AsyncAgentManager
}

func NewCloseAgentTool() *CloseAgentTool {
	return &CloseAgentTool{manager: coreagent.GetDefaultAsyncManager()}
}

func (t *CloseAgentTool) Definition() tool.Definition {
	return tool.Definition{
		Name:        closeAgentName,
		DisplayName: "CloseAgent",
		SearchHint:  closeAgentSearchHint,
		Description: closeAgentDescription,
		Category:    "agents",
		InputSchema: schema.FromMap(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"agent_id": map[string]any{
					"type":        "string",
					"description": "The agent_id returned by spawn_agent.",
				},
			},
			"required": []string{"agent_id"},
		}),
		IsReadOnly:         false,
		IsConcurrencySafe:  false,
		RequiresPermission: false,
	}
}

func (t *CloseAgentTool) IsEnabled() bool                         { return true }
func (t *CloseAgentTool) IsReadOnly(_ map[string]any) bool        { return false }
func (t *CloseAgentTool) IsConcurrencySafe(_ map[string]any) bool { return false }
func (t *CloseAgentTool) FormatResult(data any) string            { return fmt.Sprintf("%v", data) }
func (t *CloseAgentTool) BackfillInput(_ context.Context, in map[string]any) map[string]any {
	return in
}
func (t *CloseAgentTool) ValidateInput(_ context.Context, in map[string]any) (map[string]any, error) {
	if id, _ := in["agent_id"].(string); strings.TrimSpace(id) == "" {
		return nil, fmt.Errorf("agent_id is required")
	}
	return in, nil
}
func (t *CloseAgentTool) CheckPermissions(_ context.Context, in map[string]any, _ tool.ToolUseContext) types.PermissionResult {
	return types.Passthrough(in)
}
func (t *CloseAgentTool) Description(_ context.Context) (string, error) {
	return closeAgentDescription, nil
}

func (t *CloseAgentTool) Call(
	ctx context.Context,
	input tool.CallInput,
	_ types.CanUseToolFn,
) (tool.CallResult, error) {
	agentID, _ := input.Parsed["agent_id"].(string)
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return tool.NewErrorResult(fmt.Errorf("agent_id is required")), nil
	}

	ag, err := t.manager.GetAgent(agentID)
	if err != nil {
		return tool.NewErrorResult(fmt.Errorf("agent not found: %s", agentID)), nil
	}

	callID := input.ToolContextValue().ToolUseID
	statusBefore := ag.CollabStatus()

	// Emit close.begin — mirrors Codex CollabCloseBeginEvent.
	emitAgentEvent(ctx, types.RuntimeEventTypeAgentCloseBegin, &types.AgentRuntimeEvent{
		CallID:      callID,
		AgentID:     agentID,
		AgentNickname: ag.Nickname,
		AgentRole:   ag.Role,
		Status:      statusBefore,
		StartedAtMs: nowMs(),
	})

	if closeErr := t.manager.CloseAgent(agentID); closeErr != nil {
		return tool.NewErrorResult(closeErr), nil
	}

	// Emit close.end — mirrors Codex CollabCloseEndEvent.
	emitAgentEvent(ctx, types.RuntimeEventTypeAgentCloseEnd, &types.AgentRuntimeEvent{
		CallID:        callID,
		AgentID:       agentID,
		AgentNickname: ag.Nickname,
		AgentRole:     ag.Role,
		Status:        "shutdown",
		CompletedAtMs: nowMs(),
	})

	resp := map[string]any{
		"agent_id":      agentID,
		"status":        "shutdown",
		"status_before": statusBefore,
		"message":       fmt.Sprintf("Agent %s terminated and removed from registry.", agentID),
	}
	res := tool.NewJSONResult(resp)
	res.Content = fmt.Sprintf("Agent %s closed (was: %s).", agentID, statusBefore)
	return res, nil
}
