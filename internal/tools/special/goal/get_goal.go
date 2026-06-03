package goal

import (
	"context"
	"fmt"

	coregoal "github.com/EngineerProjects/nexus-engine/internal/agent/goal"
	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	"github.com/EngineerProjects/nexus-engine/internal/tools/schema"
	"github.com/EngineerProjects/nexus-engine/internal/types"
)

const getGoalName = "get_goal"
const getGoalSearchHint = "get the current goal objective, status, and usage metrics"
const getGoalDescription = `Return the current goal for this session, including live usage metrics.

Returns:
- ` + "`objective`" + `: the goal text
- ` + "`status`" + `: active | paused | blocked | budgetLimited | complete
- ` + "`tokens_used`" + ` / ` + "`token_budget`" + ` / ` + "`remaining_tokens`" + `
- ` + "`time_used_seconds`" + `: elapsed time since goal creation
- ` + "`created_at`" + ` / ` + "`updated_at`" + `: Unix millisecond timestamps

Returns ` + "`{\"goal\": null}`" + ` when no goal has been set for this session.

Mirrors Codex's ThreadGoalGetParams.`

// GetGoalTool returns the current goal for the session.
type GetGoalTool struct {
	store *coregoal.Store
}

func NewGetGoalTool() *GetGoalTool {
	return &GetGoalTool{store: coregoal.GetDefaultStore()}
}

func (t *GetGoalTool) Definition() tool.Definition {
	return tool.Definition{
		Name:        getGoalName,
		DisplayName: "GetGoal",
		SearchHint:  getGoalSearchHint,
		Description: getGoalDescription,
		Category:    "goal",
		InputSchema: schema.FromMap(map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		}),
		IsReadOnly:         true,
		IsConcurrencySafe:  true,
		RequiresPermission: false,
	}
}

func (t *GetGoalTool) IsEnabled() bool                         { return true }
func (t *GetGoalTool) IsReadOnly(_ map[string]any) bool        { return true }
func (t *GetGoalTool) IsConcurrencySafe(_ map[string]any) bool { return true }
func (t *GetGoalTool) FormatResult(data any) string            { return fmt.Sprintf("%v", data) }
func (t *GetGoalTool) BackfillInput(_ context.Context, in map[string]any) map[string]any {
	return in
}
func (t *GetGoalTool) ValidateInput(_ context.Context, in map[string]any) (map[string]any, error) {
	return in, nil
}
func (t *GetGoalTool) CheckPermissions(_ context.Context, in map[string]any, _ tool.ToolUseContext) types.PermissionResult {
	return types.Passthrough(in)
}
func (t *GetGoalTool) Description(_ context.Context) (string, error) {
	return getGoalDescription, nil
}

func (t *GetGoalTool) Call(
	ctx context.Context,
	input tool.CallInput,
	_ types.CanUseToolFn,
) (tool.CallResult, error) {
	sessionID := string(input.ToolContextValue().SessionID)
	if sessionID == "" {
		sessionID = "default"
	}

	g, ok := t.store.Get(sessionID)
	if !ok {
		res := tool.NewJSONResult(map[string]any{"goal": nil})
		res.Content = "No active goal for this session. Use create_goal to set one."
		return res, nil
	}

	resp := map[string]any{"goal": goalToMap(g)}
	res := tool.NewJSONResult(resp)
	res.Content = formatGoalSummary("Current goal", g)
	return res, nil
}
