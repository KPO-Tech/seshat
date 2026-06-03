// Package goal implements create_goal, get_goal, and update_goal tools.
// Mirrors Codex's ThreadGoal system (protocol.rs:3651, ThreadGoalSetParams, ThreadGoalGetParams).
package goal

import (
	"context"
	"fmt"
	"strings"
	"time"

	coregoal "github.com/EngineerProjects/nexus-engine/internal/agent/goal"
	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	"github.com/EngineerProjects/nexus-engine/internal/tools/schema"
	"github.com/EngineerProjects/nexus-engine/internal/types"
)

const createGoalName = "create_goal"
const createGoalSearchHint = "define a persistent goal with an objective and optional token budget"
const createGoalDescription = `Set a persistent goal for the current session.

A goal is a durable objective that survives across turns. Once set:
- Each turn continuation will include the objective as context so you stay on track.
- Use ` + "`get_goal`" + ` to check progress (tokens used, elapsed time, status).
- Use ` + "`update_goal`" + ` to mark it complete, paused, or blocked.

## token_budget
Optional integer. If set, the runner will track token consumption and automatically
transition the goal to ` + "`budgetLimited`" + ` when the budget is reached, then inject
a wrap-up prompt instead of the usual continuation.

## Re-creating a goal
Calling ` + "`create_goal`" + ` again replaces the existing goal for this session and resets
all usage counters. The previous goal is discarded.

## Status lifecycle
` + "`active`" + ` → work in progress
` + "`paused`" + ` → user paused it (call update_goal to resume)
` + "`blocked`" + ` → model hit a genuine impasse (after 3 consecutive blocked turns)
` + "`budgetLimited`" + ` → token budget exhausted (automatic)
` + "`complete`" + ` → call update_goal(status="complete") when done

Mirrors Codex's ThreadGoalSetParams (create path) + ThreadGoalUpdatedNotification.`

// CreateGoalTool creates or replaces the active goal for the session.
type CreateGoalTool struct {
	store *coregoal.Store
}

func NewCreateGoalTool() *CreateGoalTool {
	return &CreateGoalTool{store: coregoal.GetDefaultStore()}
}

func (t *CreateGoalTool) Definition() tool.Definition {
	return tool.Definition{
		Name:        createGoalName,
		DisplayName: "CreateGoal",
		SearchHint:  createGoalSearchHint,
		Description: createGoalDescription,
		Category:    "goal",
		InputSchema: schema.FromMap(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"objective": map[string]any{
					"type":        "string",
					"description": fmt.Sprintf("The goal objective. Plain text, max %d characters.", coregoal.MaxObjectiveChars),
					"maxLength":   coregoal.MaxObjectiveChars,
				},
				"token_budget": map[string]any{
					"type":        "integer",
					"description": "Optional maximum token budget for this goal. When reached the goal transitions to budgetLimited.",
					"minimum":     1,
				},
			},
			"required": []string{"objective"},
		}),
		IsReadOnly:         false,
		IsConcurrencySafe:  false,
		RequiresPermission: false,
	}
}

func (t *CreateGoalTool) IsEnabled() bool                         { return true }
func (t *CreateGoalTool) IsReadOnly(_ map[string]any) bool        { return false }
func (t *CreateGoalTool) IsConcurrencySafe(_ map[string]any) bool { return false }
func (t *CreateGoalTool) FormatResult(data any) string            { return fmt.Sprintf("%v", data) }
func (t *CreateGoalTool) BackfillInput(_ context.Context, in map[string]any) map[string]any {
	return in
}
func (t *CreateGoalTool) ValidateInput(_ context.Context, in map[string]any) (map[string]any, error) {
	obj, _ := in["objective"].(string)
	if err := coregoal.ValidateObjective(obj); err != nil {
		return nil, err
	}
	return in, nil
}
func (t *CreateGoalTool) CheckPermissions(_ context.Context, in map[string]any, _ tool.ToolUseContext) types.PermissionResult {
	return types.Passthrough(in)
}
func (t *CreateGoalTool) Description(_ context.Context) (string, error) {
	return createGoalDescription, nil
}

func (t *CreateGoalTool) Call(
	ctx context.Context,
	input tool.CallInput,
	_ types.CanUseToolFn,
) (tool.CallResult, error) {
	objective, _ := input.Parsed["objective"].(string)
	if err := coregoal.ValidateObjective(objective); err != nil {
		return tool.NewErrorResult(err), nil
	}

	var tokenBudget *int64
	if v, ok := input.Parsed["token_budget"].(float64); ok && v >= 1 {
		b := int64(v)
		tokenBudget = &b
	}

	sessionID := string(input.ToolContextValue().SessionID)
	if sessionID == "" {
		sessionID = "default"
	}

	g := t.store.Set(sessionID, objective, tokenBudget)

	emitGoalEvent(ctx, g)

	resp := goalToMap(g)
	resp["message"] = "Goal created. The objective will be injected as context on each continuation turn. Call update_goal(status='complete') when done."

	res := tool.NewJSONResult(resp)
	res.Content = formatGoalSummary("Goal created", g)
	return res, nil
}

// ─── shared helpers ───────────────────────────────────────────────────────────

func goalToMap(g *coregoal.Goal) map[string]any {
	m := map[string]any{
		"session_id":        g.SessionID,
		"objective":         g.Objective,
		"status":            string(g.Status),
		"tokens_used":       g.TokensUsed,
		"time_used_seconds": g.TimeUsedSeconds,
		"created_at":        g.CreatedAt,
		"updated_at":        g.UpdatedAt,
	}
	if g.TokenBudget != nil {
		m["token_budget"] = *g.TokenBudget
		m["remaining_tokens"] = g.RemainingTokens()
	}
	return m
}

func formatGoalSummary(prefix string, g *coregoal.Goal) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "%s (status: %s)\n", prefix, g.Status)
	fmt.Fprintf(&sb, "Objective: %s\n", truncate(g.Objective, 120))
	if g.TokenBudget != nil {
		fmt.Fprintf(&sb, "Budget: %d/%d tokens used (%d remaining)",
			g.TokensUsed, *g.TokenBudget, g.RemainingTokens())
	} else {
		fmt.Fprintf(&sb, "Tokens used: %d (no budget)", g.TokensUsed)
	}
	return strings.TrimRight(sb.String(), "\n")
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

func emitGoalEvent(ctx context.Context, g *coregoal.Goal) {
	emitter, ok := ctx.Value(types.RuntimeEventEmitterKey).(func(types.RuntimeEvent))
	if !ok || emitter == nil {
		return
	}
	payload := &types.GoalRuntimeEvent{
		SessionID:       g.SessionID,
		Objective:       g.Objective,
		Status:          string(g.Status),
		TokensUsed:      g.TokensUsed,
		TimeUsedSeconds: g.TimeUsedSeconds,
		CreatedAt:       g.CreatedAt,
		UpdatedAt:       g.UpdatedAt,
	}
	if g.TokenBudget != nil {
		payload.TokenBudget = g.TokenBudget
	}
	emitter(types.RuntimeEvent{
		Type:      types.RuntimeEventTypeGoalUpdated,
		Timestamp: time.Now(),
		GoalEvent: payload,
	})
}
