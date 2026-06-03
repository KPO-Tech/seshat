package goal

import (
	"context"
	"fmt"
	"strings"

	coregoal "github.com/EngineerProjects/nexus-engine/internal/agent/goal"
	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	"github.com/EngineerProjects/nexus-engine/internal/tools/schema"
	"github.com/EngineerProjects/nexus-engine/internal/types"
)

const updateGoalName = "update_goal"
const updateGoalSearchHint = "update the goal status or objective — mark complete, blocked, or paused"
const updateGoalDescription = `Update the active goal's status and/or objective.

## When to call

| status | When |
|---|---|
| ` + "`complete`" + ` | Every requirement is proven and no work remains |
| ` + "`blocked`" + ` | Same blocker has appeared for ≥3 consecutive turns — not just difficult |
| ` + "`paused`" + ` | User requested a pause; call update_goal(status="active") to resume |

## Rules (from Codex continuation.md)
- Do **not** call update_goal merely because the budget is nearly exhausted or because
  you are stopping work.
- Do **not** mark complete unless every requirement is proven against current state.
- Do **not** use "blocked" the first time a blocker appears; require 3+ consecutive turns.
- You may update only the objective (leave status unchanged) to reflect scope clarification.

## Emitted event
Every call emits a ` + "`goal.updated`" + ` RuntimeEvent so the UI can reflect the change.

Mirrors Codex's ThreadGoalSetParams (update path) + ThreadGoalUpdatedNotification.`

// UpdateGoalTool updates the status and/or objective of the active goal.
type UpdateGoalTool struct {
	store *coregoal.Store
}

func NewUpdateGoalTool() *UpdateGoalTool {
	return &UpdateGoalTool{store: coregoal.GetDefaultStore()}
}

func (t *UpdateGoalTool) Definition() tool.Definition {
	return tool.Definition{
		Name:        updateGoalName,
		DisplayName: "UpdateGoal",
		SearchHint:  updateGoalSearchHint,
		Description: updateGoalDescription,
		Category:    "goal",
		InputSchema: schema.FromMap(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"status": map[string]any{
					"type":        "string",
					"description": "New status for the goal.",
					"enum":        []string{"active", "paused", "blocked", "complete"},
				},
				"objective": map[string]any{
					"type":        "string",
					"description": fmt.Sprintf("Updated objective text (max %d chars). Leave blank to keep current.", coregoal.MaxObjectiveChars),
					"maxLength":   coregoal.MaxObjectiveChars,
				},
			},
		}),
		IsReadOnly:         false,
		IsConcurrencySafe:  false,
		RequiresPermission: false,
	}
}

func (t *UpdateGoalTool) IsEnabled() bool                         { return true }
func (t *UpdateGoalTool) IsReadOnly(_ map[string]any) bool        { return false }
func (t *UpdateGoalTool) IsConcurrencySafe(_ map[string]any) bool { return false }
func (t *UpdateGoalTool) FormatResult(data any) string            { return fmt.Sprintf("%v", data) }
func (t *UpdateGoalTool) BackfillInput(_ context.Context, in map[string]any) map[string]any {
	return in
}
func (t *UpdateGoalTool) ValidateInput(_ context.Context, in map[string]any) (map[string]any, error) {
	statusRaw, hasStatus := in["status"].(string)
	objectiveRaw, hasObjective := in["objective"].(string)
	if !hasStatus && !hasObjective {
		return nil, fmt.Errorf("at least one of 'status' or 'objective' must be provided")
	}
	if hasStatus {
		switch coregoal.Status(statusRaw) {
		case coregoal.StatusActive, coregoal.StatusPaused, coregoal.StatusBlocked, coregoal.StatusComplete:
			// valid
		default:
			return nil, fmt.Errorf("invalid status %q: must be one of active, paused, blocked, complete", statusRaw)
		}
	}
	if hasObjective && strings.TrimSpace(objectiveRaw) != "" {
		if err := coregoal.ValidateObjective(objectiveRaw); err != nil {
			return nil, err
		}
	}
	return in, nil
}
func (t *UpdateGoalTool) CheckPermissions(_ context.Context, in map[string]any, _ tool.ToolUseContext) types.PermissionResult {
	return types.Passthrough(in)
}
func (t *UpdateGoalTool) Description(_ context.Context) (string, error) {
	return updateGoalDescription, nil
}

func (t *UpdateGoalTool) Call(
	ctx context.Context,
	input tool.CallInput,
	_ types.CanUseToolFn,
) (tool.CallResult, error) {
	sessionID := string(input.ToolContextValue().SessionID)
	if sessionID == "" {
		sessionID = "default"
	}

	// Check goal exists.
	if _, ok := t.store.Get(sessionID); !ok {
		return tool.NewErrorResult(fmt.Errorf("no active goal for this session — call create_goal first")), nil
	}

	var newStatus *coregoal.Status
	if rawStatus, ok := input.Parsed["status"].(string); ok && rawStatus != "" {
		s := coregoal.Status(rawStatus)
		newStatus = &s
	}

	var newObjective *string
	if rawObj, ok := input.Parsed["objective"].(string); ok && strings.TrimSpace(rawObj) != "" {
		newObjective = &rawObj
	}

	if newStatus == nil && newObjective == nil {
		return tool.NewErrorResult(fmt.Errorf("at least one of 'status' or 'objective' must be provided")), nil
	}

	g, ok := t.store.Update(sessionID, newStatus, newObjective)
	if !ok {
		return tool.NewErrorResult(fmt.Errorf("goal update failed: goal not found")), nil
	}

	emitGoalEvent(ctx, g)

	// If objective was updated, note that the objective_updated prompt will be
	// injected on the next turn automatically via the runner.
	summaryPrefix := "Goal updated"
	if newObjective != nil && newStatus == nil {
		summaryPrefix = "Goal objective updated"
	} else if newStatus != nil && *newStatus == coregoal.StatusComplete {
		summaryPrefix = "Goal marked complete"
	}

	resp := goalToMap(g)
	res := tool.NewJSONResult(resp)
	res.Content = formatGoalSummary(summaryPrefix, g)
	return res, nil
}
