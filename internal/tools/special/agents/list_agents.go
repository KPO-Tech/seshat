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

type agentEntry struct {
	AgentID         string  `json:"agent_id"`
	Status          string  `json:"status"`
	Nickname        string  `json:"nickname,omitempty"`
	Role            string  `json:"role,omitempty"`
	ElapsedSeconds  float64 `json:"elapsed_seconds"`
	CurrentTurn     int     `json:"current_turn,omitempty"`
	PercentComplete float64 `json:"percent_complete,omitempty"`
}

const listAgentsName = "list_agents"
const listAgentsSearchHint = "list all active background agents with their status and progress"
const listAgentsDescription = `List all currently active sub-agents and their status.

Returns an array of agent entries, each with:
- ` + "`agent_id`" + `: stable identifier
- ` + "`status`" + `: CollabAgentStatus (pendingInit | running | completed | errored | shutdown)
- ` + "`nickname`" + ` / ` + "`role`" + `: if set at spawn time
- ` + "`elapsed_seconds`" + `: time since spawn
- ` + "`current_turn`" + ` / ` + "`percent_complete`" + `: progress if running

Use ` + "`filter_status`" + ` to narrow the list to a specific status.`

// ListAgentsTool lists all active sub-agents.
// Mirrors Codex's CollabAgentTool listing + CollabAgentStatusEntry.
type ListAgentsTool struct {
	manager *coreagent.AsyncAgentManager
}

func NewListAgentsTool() *ListAgentsTool {
	return &ListAgentsTool{manager: coreagent.GetDefaultAsyncManager()}
}

func (t *ListAgentsTool) Definition() tool.Definition {
	return tool.Definition{
		Name:        listAgentsName,
		DisplayName: "ListAgents",
		SearchHint:  listAgentsSearchHint,
		Description: listAgentsDescription,
		Category:    "agents",
		InputSchema: schema.FromMap(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"filter_status": map[string]any{
					"type":        "string",
					"description": "Only return agents in this status. Omit for all agents.",
					"enum":        []string{"pendingInit", "running", "completed", "errored", "shutdown"},
				},
			},
		}),
		IsReadOnly:         true,
		IsConcurrencySafe:  true,
		RequiresPermission: false,
	}
}

func (t *ListAgentsTool) IsEnabled() bool                         { return true }
func (t *ListAgentsTool) IsReadOnly(_ map[string]any) bool        { return true }
func (t *ListAgentsTool) IsConcurrencySafe(_ map[string]any) bool { return true }
func (t *ListAgentsTool) FormatResult(data any) string            { return fmt.Sprintf("%v", data) }
func (t *ListAgentsTool) BackfillInput(_ context.Context, in map[string]any) map[string]any {
	return in
}
func (t *ListAgentsTool) ValidateInput(_ context.Context, in map[string]any) (map[string]any, error) {
	return in, nil
}
func (t *ListAgentsTool) CheckPermissions(_ context.Context, in map[string]any, _ tool.ToolUseContext) types.PermissionResult {
	return types.Passthrough(in)
}
func (t *ListAgentsTool) Description(_ context.Context) (string, error) {
	return listAgentsDescription, nil
}

func (t *ListAgentsTool) Call(
	ctx context.Context,
	input tool.CallInput,
	_ types.CanUseToolFn,
) (tool.CallResult, error) {
	filterStatus, _ := input.Parsed["filter_status"].(string)

	agents := t.manager.ListAgents()

	entries := make([]agentEntry, 0, len(agents))
	for _, ag := range agents {
		collabStatus := ag.CollabStatus()
		if filterStatus != "" && collabStatus != filterStatus {
			continue
		}
		progress := ag.GetProgress()
		e := agentEntry{
			AgentID:        ag.ID,
			Status:         collabStatus,
			Nickname:       ag.Nickname,
			Role:           ag.Role,
			ElapsedSeconds: ag.GetDuration().Seconds(),
		}
		if progress != nil {
			e.CurrentTurn = progress.CurrentTurn
			e.PercentComplete = progress.PercentComplete
		}
		entries = append(entries, e)
	}

	resp := map[string]any{
		"agents": entries,
		"count":  len(entries),
	}
	res := tool.NewJSONResult(resp)
	res.Content = formatAgentList(entries)
	return res, nil
}

func formatAgentList(entries []agentEntry) string {
	if len(entries) == 0 {
		return "No active agents."
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "%d agent(s):\n", len(entries))
	for _, e := range entries {
		label := e.AgentID
		if e.Nickname != "" {
			label = fmt.Sprintf("%s (%s)", e.Nickname, e.AgentID)
		}
		fmt.Fprintf(&sb, "  • %s — %s (%.0fs", label, e.Status, e.ElapsedSeconds)
		if e.CurrentTurn > 0 {
			fmt.Fprintf(&sb, ", turn %d, %.0f%%", e.CurrentTurn, e.PercentComplete)
		}
		sb.WriteString(")\n")
	}
	return strings.TrimRight(sb.String(), "\n")
}
