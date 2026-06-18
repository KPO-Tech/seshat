// Package mailbox provides tools for inter-agent communication via the
// persistent mailbox system. Agents call these tools to send tasks, replies,
// and broadcasts to other team members without direct coupling.
package mailbox

import (
	"context"
	"fmt"
	"strings"

	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	"github.com/EngineerProjects/nexus-engine/internal/tools/schema"
	"github.com/EngineerProjects/nexus-engine/internal/types"
)

// Dispatcher is the subset of team.Dispatcher consumed by mailbox tools.
// The concrete implementation (team.Dispatcher) satisfies this interface.
type Dispatcher interface {
	Send(ctx context.Context, fromID, toID, subject, body string) error
	Assign(ctx context.Context, fromID, role, teamID, subject, body string) error
	Broadcast(ctx context.Context, fromID, teamID, subject, body string) error
	Reply(ctx context.Context, fromID, toID, replyToID, subject, body string) error
}

const sendName = "mailbox_send"
const sendSearchHint = "send a task or reply to another team agent via the mailbox"
const sendDescription = "Send a message to another team agent via the persistent mailbox.\n\n" +
	"## Addressing\n" +
	"Specify exactly one of:\n" +
	"- **`to_agent_id`** — send directly to a known agent UUID\n" +
	"- **`to_role`** — route to the first agent with that role (e.g. \"engineer\"); optionally scoped to **`to_team`**\n\n" +
	"## Reply threading\n" +
	"Set `reply_to_id` to the original message ID when replying to link messages into a thread.\n\n" +
	"## Notes\n" +
	"- Messages are persisted in SQLite — they survive restarts.\n" +
	"- The receiving agent's TeamBus will pick up the message on its next poll cycle.\n" +
	"- Use `mailbox_broadcast` to fan-out to every member of a team."

// SendTool lets an agent send a direct task or reply via the mailbox.
type SendTool struct {
	dispatcher  Dispatcher
	fromAgentID string
}

// NewSendTool creates a SendTool pre-configured with the sender's agent ID.
func NewSendTool(dispatcher Dispatcher, fromAgentID string) *SendTool {
	return &SendTool{dispatcher: dispatcher, fromAgentID: fromAgentID}
}

func (t *SendTool) Definition() tool.Definition {
	return tool.Definition{
		Name:        sendName,
		DisplayName: "MailboxSend",
		SearchHint:  sendSearchHint,
		Description: sendDescription,
		Category:    "mailbox",
		InputSchema: schema.FromMap(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"to_agent_id": map[string]any{
					"type":        "string",
					"description": "UUID of the target agent. Exclusive with to_role.",
				},
				"to_role": map[string]any{
					"type":        "string",
					"description": "Route to the first agent with this role. Exclusive with to_agent_id.",
				},
				"to_team": map[string]any{
					"type":        "string",
					"description": "Scope to_role lookup to this team ID. Ignored when to_agent_id is set.",
				},
				"subject": map[string]any{
					"type":        "string",
					"description": "Short summary of the task or reply.",
				},
				"body": map[string]any{
					"type":        "string",
					"description": "Full message body with instructions, context, or findings.",
				},
				"reply_to_id": map[string]any{
					"type":        "string",
					"description": "ID of the message this is a reply to. Omit for a new task.",
				},
			},
			"required": []string{"subject", "body"},
		}),
		IsReadOnly:         false,
		IsConcurrencySafe:  true,
		RequiresPermission: false,
	}
}

func (t *SendTool) IsEnabled() bool                         { return true }
func (t *SendTool) IsReadOnly(_ map[string]any) bool        { return false }
func (t *SendTool) IsConcurrencySafe(_ map[string]any) bool { return true }
func (t *SendTool) FormatResult(data any) string            { return fmt.Sprintf("%v", data) }
func (t *SendTool) BackfillInput(_ context.Context, in map[string]any) map[string]any {
	return in
}
func (t *SendTool) Description(_ context.Context) (string, error) {
	return sendDescription, nil
}

func (t *SendTool) ValidateInput(_ context.Context, in map[string]any) (map[string]any, error) {
	toID, _ := in["to_agent_id"].(string)
	toRole, _ := in["to_role"].(string)
	if strings.TrimSpace(toID) == "" && strings.TrimSpace(toRole) == "" {
		return nil, fmt.Errorf("one of to_agent_id or to_role is required")
	}
	if strings.TrimSpace(toID) != "" && strings.TrimSpace(toRole) != "" {
		return nil, fmt.Errorf("to_agent_id and to_role are mutually exclusive")
	}
	if subj, _ := in["subject"].(string); strings.TrimSpace(subj) == "" {
		return nil, fmt.Errorf("subject is required")
	}
	if body, _ := in["body"].(string); strings.TrimSpace(body) == "" {
		return nil, fmt.Errorf("body is required")
	}
	return in, nil
}

func (t *SendTool) CheckPermissions(_ context.Context, in map[string]any, _ tool.ToolUseContext) types.PermissionResult {
	return types.Passthrough(in)
}

func (t *SendTool) Call(
	ctx context.Context,
	input tool.CallInput,
	_ types.CanUseToolFn,
) (tool.CallResult, error) {
	toAgentID := strings.TrimSpace(input.Parsed["to_agent_id"].(string))
	toRole, _ := input.Parsed["to_role"].(string)
	toRole = strings.TrimSpace(toRole)
	toTeam, _ := input.Parsed["to_team"].(string)
	toTeam = strings.TrimSpace(toTeam)
	subject := strings.TrimSpace(input.Parsed["subject"].(string))
	body := strings.TrimSpace(input.Parsed["body"].(string))
	replyTo, _ := input.Parsed["reply_to_id"].(string)
	replyTo = strings.TrimSpace(replyTo)

	var err error
	switch {
	case toAgentID != "" && replyTo != "":
		err = t.dispatcher.Reply(ctx, t.fromAgentID, toAgentID, replyTo, subject, body)
	case toAgentID != "":
		err = t.dispatcher.Send(ctx, t.fromAgentID, toAgentID, subject, body)
	default:
		err = t.dispatcher.Assign(ctx, t.fromAgentID, toRole, toTeam, subject, body)
	}
	if err != nil {
		return tool.NewErrorResult(fmt.Errorf("mailbox_send: %w", err)), nil
	}

	target := toAgentID
	if target == "" {
		target = fmt.Sprintf("role:%s", toRole)
		if toTeam != "" {
			target = fmt.Sprintf("role:%s@%s", toRole, toTeam)
		}
	}
	resp := map[string]any{
		"sent":    true,
		"to":      target,
		"subject": subject,
	}
	res := tool.NewJSONResult(resp)
	res.Content = fmt.Sprintf("Message sent to %s: %s", target, subject)
	return res, nil
}
