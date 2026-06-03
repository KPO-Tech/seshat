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

const sendAgentMessageName = "send_agent_message"
const sendAgentMessageSearchHint = "send a message to a running background agent between turns"
const sendAgentMessageDescription = `Send a message to a running sub-agent. The message is delivered as the continuation
prompt on the agent's next turn boundary, replacing the default "Continue with the task." message.

Use this to:
- Provide additional context discovered after spawning
- Redirect the agent when intermediate output suggests a different approach
- Pass a clarification or constraint mid-task

The tool returns immediately (` + "`queued: true`" + `). The message will be picked up the next time the
agent starts a new turn. If the agent has already finished, this returns an error.

Mirrors Codex's CollabAgentTool = "sendInput" + CollabAgentInteractionBeginEvent / CollabAgentInteractionEndEvent.`

// SendAgentMessageTool sends a message to a running agent's continuation queue.
type SendAgentMessageTool struct {
	manager *coreagent.AsyncAgentManager
}

func NewSendAgentMessageTool() *SendAgentMessageTool {
	return &SendAgentMessageTool{manager: coreagent.GetDefaultAsyncManager()}
}

func (t *SendAgentMessageTool) Definition() tool.Definition {
	return tool.Definition{
		Name:        sendAgentMessageName,
		DisplayName: "SendAgentMessage",
		SearchHint:  sendAgentMessageSearchHint,
		Description: sendAgentMessageDescription,
		Category:    "agents",
		InputSchema: schema.FromMap(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"agent_id": map[string]any{
					"type":        "string",
					"description": "The agent_id returned by spawn_agent.",
				},
				"message": map[string]any{
					"type":        "string",
					"description": "The message to send. Will be used as the agent's next continuation prompt.",
				},
			},
			"required": []string{"agent_id", "message"},
		}),
		IsReadOnly:         false,
		IsConcurrencySafe:  true,
		RequiresPermission: false,
	}
}

func (t *SendAgentMessageTool) IsEnabled() bool                         { return true }
func (t *SendAgentMessageTool) IsReadOnly(_ map[string]any) bool        { return false }
func (t *SendAgentMessageTool) IsConcurrencySafe(_ map[string]any) bool { return true }
func (t *SendAgentMessageTool) FormatResult(data any) string            { return fmt.Sprintf("%v", data) }
func (t *SendAgentMessageTool) BackfillInput(_ context.Context, in map[string]any) map[string]any {
	return in
}
func (t *SendAgentMessageTool) ValidateInput(_ context.Context, in map[string]any) (map[string]any, error) {
	if id, _ := in["agent_id"].(string); strings.TrimSpace(id) == "" {
		return nil, fmt.Errorf("agent_id is required")
	}
	if msg, _ := in["message"].(string); strings.TrimSpace(msg) == "" {
		return nil, fmt.Errorf("message is required")
	}
	return in, nil
}
func (t *SendAgentMessageTool) CheckPermissions(_ context.Context, in map[string]any, _ tool.ToolUseContext) types.PermissionResult {
	return types.Passthrough(in)
}
func (t *SendAgentMessageTool) Description(_ context.Context) (string, error) {
	return sendAgentMessageDescription, nil
}

func (t *SendAgentMessageTool) Call(
	ctx context.Context,
	input tool.CallInput,
	_ types.CanUseToolFn,
) (tool.CallResult, error) {
	agentID, _ := input.Parsed["agent_id"].(string)
	agentID = strings.TrimSpace(agentID)
	message, _ := input.Parsed["message"].(string)
	message = strings.TrimSpace(message)

	if agentID == "" {
		return tool.NewErrorResult(fmt.Errorf("agent_id is required")), nil
	}
	if message == "" {
		return tool.NewErrorResult(fmt.Errorf("message is required")), nil
	}

	ag, err := t.manager.GetAgent(agentID)
	if err != nil {
		return tool.NewErrorResult(fmt.Errorf("agent not found: %s", agentID)), nil
	}

	callID := input.ToolContextValue().ToolUseID

	// Emit interaction.begin — mirrors Codex CollabAgentInteractionBeginEvent.
	emitAgentEvent(ctx, types.RuntimeEventTypeAgentInteractionBegin, &types.AgentRuntimeEvent{
		CallID:      callID,
		AgentID:     agentID,
		AgentNickname: ag.Nickname,
		AgentRole:   ag.Role,
		Message:     message,
		Status:      ag.CollabStatus(),
		StartedAtMs: nowMs(),
	})

	if sendErr := t.manager.SendMessage(agentID, message); sendErr != nil {
		return tool.NewErrorResult(sendErr), nil
	}

	// Emit interaction.end — mirrors Codex CollabAgentInteractionEndEvent.
	emitAgentEvent(ctx, types.RuntimeEventTypeAgentInteractionEnd, &types.AgentRuntimeEvent{
		CallID:        callID,
		AgentID:       agentID,
		AgentNickname: ag.Nickname,
		AgentRole:     ag.Role,
		Message:       message,
		Status:        ag.CollabStatus(),
		CompletedAtMs: nowMs(),
	})

	resp := map[string]any{
		"agent_id": agentID,
		"queued":   true,
		"message":  message,
		"note":     "Message queued. It will be delivered as the agent's continuation prompt on its next turn.",
	}
	res := tool.NewJSONResult(resp)
	res.Content = fmt.Sprintf("Message queued for agent %s. It will be picked up on the next turn.", agentID)
	return res, nil
}
