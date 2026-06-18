package mailbox

import (
	"context"
	"fmt"
	"strings"

	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	"github.com/EngineerProjects/nexus-engine/internal/tools/schema"
	"github.com/EngineerProjects/nexus-engine/internal/types"
)

const broadcastName = "mailbox_broadcast"
const broadcastSearchHint = "broadcast a message to all agents in a team"
const broadcastDescription = "Fan-out a message to every agent in a team.\n\n" +
	"Each team member receives a copy in their inbox. The sender is excluded from\n" +
	"the recipients. Use this for:\n" +
	"- Sprint kick-offs, status updates, or policy announcements\n" +
	"- Notifying all engineers when a shared resource changes\n" +
	"- Any communication that every team member needs to act on\n\n" +
	"For targeted tasks, use `mailbox_send` instead."

// BroadcastTool lets an agent fan-out a message to every member of a team.
type BroadcastTool struct {
	dispatcher  Dispatcher
	fromAgentID string
}

// NewBroadcastTool creates a BroadcastTool pre-configured with the sender's agent ID.
func NewBroadcastTool(dispatcher Dispatcher, fromAgentID string) *BroadcastTool {
	return &BroadcastTool{dispatcher: dispatcher, fromAgentID: fromAgentID}
}

func (t *BroadcastTool) Definition() tool.Definition {
	return tool.Definition{
		Name:        broadcastName,
		DisplayName: "MailboxBroadcast",
		SearchHint:  broadcastSearchHint,
		Description: broadcastDescription,
		Category:    "mailbox",
		InputSchema: schema.FromMap(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"team_id": map[string]any{
					"type":        "string",
					"description": "The team to broadcast to. All members except the sender receive the message.",
				},
				"subject": map[string]any{
					"type":        "string",
					"description": "Short summary of the announcement.",
				},
				"body": map[string]any{
					"type":        "string",
					"description": "Full message body.",
				},
			},
			"required": []string{"team_id", "subject", "body"},
		}),
		IsReadOnly:         false,
		IsConcurrencySafe:  true,
		RequiresPermission: false,
	}
}

func (t *BroadcastTool) IsEnabled() bool                         { return true }
func (t *BroadcastTool) IsReadOnly(_ map[string]any) bool        { return false }
func (t *BroadcastTool) IsConcurrencySafe(_ map[string]any) bool { return true }
func (t *BroadcastTool) FormatResult(data any) string            { return fmt.Sprintf("%v", data) }
func (t *BroadcastTool) BackfillInput(_ context.Context, in map[string]any) map[string]any {
	return in
}
func (t *BroadcastTool) Description(_ context.Context) (string, error) {
	return broadcastDescription, nil
}

func (t *BroadcastTool) ValidateInput(_ context.Context, in map[string]any) (map[string]any, error) {
	if tid, _ := in["team_id"].(string); strings.TrimSpace(tid) == "" {
		return nil, fmt.Errorf("team_id is required")
	}
	if subj, _ := in["subject"].(string); strings.TrimSpace(subj) == "" {
		return nil, fmt.Errorf("subject is required")
	}
	if body, _ := in["body"].(string); strings.TrimSpace(body) == "" {
		return nil, fmt.Errorf("body is required")
	}
	return in, nil
}

func (t *BroadcastTool) CheckPermissions(_ context.Context, in map[string]any, _ tool.ToolUseContext) types.PermissionResult {
	return types.Passthrough(in)
}

func (t *BroadcastTool) Call(
	ctx context.Context,
	input tool.CallInput,
	_ types.CanUseToolFn,
) (tool.CallResult, error) {
	teamID := strings.TrimSpace(input.Parsed["team_id"].(string))
	subject := strings.TrimSpace(input.Parsed["subject"].(string))
	body := strings.TrimSpace(input.Parsed["body"].(string))

	if err := t.dispatcher.Broadcast(ctx, t.fromAgentID, teamID, subject, body); err != nil {
		return tool.NewErrorResult(fmt.Errorf("mailbox_broadcast: %w", err)), nil
	}

	resp := map[string]any{
		"sent":    true,
		"team_id": teamID,
		"subject": subject,
	}
	res := tool.NewJSONResult(resp)
	res.Content = fmt.Sprintf("Broadcast sent to team %q: %s", teamID, subject)
	return res, nil
}
