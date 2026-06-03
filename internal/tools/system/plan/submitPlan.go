package plan

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/EngineerProjects/nexus-engine/internal/modes"
	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	"github.com/EngineerProjects/nexus-engine/internal/tools/schema"
	"github.com/EngineerProjects/nexus-engine/internal/types"
)

// PlanPersistFn is the callback the submit_plan tool uses to persist a plan document.
// Returns the plan ID (new or existing), version, and any error.
type PlanPersistFn func(ctx context.Context, planID, sessionID, userID, slug, filename, content string) (id string, version int, err error)

// SubmitPlanOutput is the result returned to the agent.
type SubmitPlanOutput struct {
	PlanID   string `json:"plan_id"`
	Slug     string `json:"slug"`
	Filename string `json:"filename"`
	Version  int    `json:"version"`
	Status   string `json:"status"`
}

// SubmitPlanTool implements submit_plan.
type SubmitPlanTool struct {
	sessionID types.SessionID
	agentID   *types.AgentID
	userID    string
	persistFn PlanPersistFn
}

// SubmitPlanConfig holds the dependencies for SubmitPlanTool.
type SubmitPlanConfig struct {
	SessionID types.SessionID
	AgentID   *types.AgentID
	UserID    string
	PersistFn PlanPersistFn
}

func NewSubmitPlanTool(config *SubmitPlanConfig) *SubmitPlanTool {
	if config == nil || config.PersistFn == nil {
		return nil
	}
	return &SubmitPlanTool{
		sessionID: config.SessionID,
		agentID:   config.AgentID,
		userID:    config.UserID,
		persistFn: config.PersistFn,
	}
}

func (t *SubmitPlanTool) Definition() tool.Definition {
	return tool.Definition{
		Name:        ToolNameSubmitPlan,
		DisplayName: "SubmitPlan",
		SearchHint:  SearchHintSubmitPlan,
		Description: SubmitPlanPrompt,
		Category:    "mode",
		InputSchema: schema.FromMap(map[string]any{
			"type":     "object",
			"required": []string{"slug", "content"},
			"properties": map[string]any{
				"slug": map[string]any{
					"type":        "string",
					"description": "A short kebab-case identifier for this plan (e.g. 'refactor-auth-flow'). Used as the filename base.",
				},
				"content": map[string]any{
					"type":        "string",
					"description": "The full markdown content of the implementation plan.",
				},
				"plan_id": map[string]any{
					"type":        "string",
					"description": "Optional. If set, updates the existing plan document instead of creating a new one.",
				},
			},
		}),
		IsReadOnly:         false,
		IsConcurrencySafe:  false,
		IsDestructive:      false,
		RequiresPermission: false,
		Metadata:           map[string]any{"surface_profiles": []string{"mono_run"}},
	}
}

func (t *SubmitPlanTool) Call(
	ctx context.Context,
	input tool.CallInput,
	_ types.CanUseToolFn,
) (tool.CallResult, error) {
	sessionID, err := t.resolveSessionID(input)
	if err != nil {
		return tool.NewErrorResult(err), nil
	}

	toolCtx := input.ToolContextValue()
	if !modes.IsPlanModeString(toolCtx.ExecutionMode) {
		return tool.NewErrorResult(fmt.Errorf("submit_plan can only be called in plan mode")), nil
	}

	slug, _ := input.Parsed["slug"].(string)
	slug = sanitizeSlug(slug)
	if slug == "" {
		return tool.NewErrorResult(fmt.Errorf("slug is required")), nil
	}

	content, _ := input.Parsed["content"].(string)
	if strings.TrimSpace(content) == "" {
		return tool.NewErrorResult(fmt.Errorf("content is required")), nil
	}

	planID, _ := input.Parsed["plan_id"].(string)

	filename := buildFilename(slug)

	id, version, err := t.persistFn(ctx, planID, string(sessionID), t.userID, slug, filename, content)
	if err != nil {
		return tool.NewErrorResult(fmt.Errorf("failed to persist plan: %w", err)), nil
	}

	// Emit runtime event so the frontend can show the plan artifact.
	if emitter, ok := ctx.Value(types.RuntimeEventEmitterKey).(func(types.RuntimeEvent)); ok {
		emitter(types.RuntimeEvent{
			Type:      types.RuntimeEventTypePlanSubmitted,
			SessionID: sessionID,
			PlanEvent: &types.PlanRuntimeEvent{
				PlanID:   id,
				Slug:     slug,
				Filename: filename,
				Status:   "pending",
				Version:  version,
			},
		})
	}

	out := SubmitPlanOutput{
		PlanID:   id,
		Slug:     slug,
		Filename: filename,
		Version:  version,
		Status:   "pending",
	}

	msg := fmt.Sprintf(`Plan submitted successfully.

Plan ID: %s
Filename: %s
Version: %d

The plan is now pending user review. Do NOT call exit_plan_mode yet.
Wait for the user to review and either:
- Send feedback (they will reply in the conversation — revise and call submit_plan again)
- Approve with "Proceed" (you will receive a confirmation message — then call exit_plan_mode)`, id, filename, version)

	result := tool.NewTextResult(msg)
	result.Metadata = &tool.ResultMetadata{
		Additional: map[string]any{
			"plan_id":  out.PlanID,
			"slug":     out.Slug,
			"filename": out.Filename,
			"version":  out.Version,
			"status":   out.Status,
		},
	}
	return result, nil
}

func (t *SubmitPlanTool) Description(ctx context.Context) (string, error) {
	return DescriptionSubmitPlan, nil
}

func (t *SubmitPlanTool) ValidateInput(ctx context.Context, input map[string]any) (map[string]any, error) {
	return input, nil
}

func (t *SubmitPlanTool) CheckPermissions(_ context.Context, _ map[string]any, _ tool.ToolUseContext) types.PermissionResult {
	return types.PermissionResult{Behavior: types.PermissionBehaviorAllow}
}

func (t *SubmitPlanTool) IsConcurrencySafe(_ map[string]any) bool { return false }

func (t *SubmitPlanTool) IsReadOnly(_ map[string]any) bool { return false }

func (t *SubmitPlanTool) IsEnabled() bool { return t != nil && t.persistFn != nil }

func (t *SubmitPlanTool) ExecutesInPlanMode(_ map[string]any) bool { return true }

func (t *SubmitPlanTool) Prompt() string { return SubmitPlanPrompt }

func (t *SubmitPlanTool) FormatResult(data any) string {
	if s, ok := data.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", data)
}

func (t *SubmitPlanTool) RequiresUserInteraction() bool { return false }

func (t *SubmitPlanTool) BackfillInput(_ context.Context, input map[string]any) map[string]any {
	return input
}

func (t *SubmitPlanTool) resolveSessionID(input tool.CallInput) (types.SessionID, error) {
	if t != nil && t.sessionID != "" {
		return t.sessionID, nil
	}
	toolCtx := input.ToolContextValue()
	if toolCtx.SessionID != "" {
		return toolCtx.SessionID, nil
	}
	if input.SessionID != "" {
		return input.SessionID, nil
	}
	return "", fmt.Errorf("session ID is required for submit_plan")
}

var slugRe = regexp.MustCompile(`[^a-z0-9-]`)

func sanitizeSlug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, " ", "-")
	s = strings.ReplaceAll(s, "_", "-")
	s = slugRe.ReplaceAllString(s, "")
	s = regexp.MustCompile(`-+`).ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if len(s) > 80 {
		s = s[:80]
	}
	return s
}

func buildFilename(slug string) string {
	now := time.Now()
	return fmt.Sprintf("%s_%s.md", slug, now.Format("2006_01_02_15_04"))
}

// SubmitPlanPrompt is the system prompt for the SubmitPlan tool.
const SubmitPlanPrompt = `Use this tool to submit your implementation plan for user review while in plan mode.

## When to call

- When your plan is complete and ready for the user to review
- You can call it multiple times to revise based on user feedback

## Parameters

- **slug**: A short kebab-case identifier (e.g. "refactor-auth-flow", "add-payment-api"). Used as the filename base. Required.
- **content**: The full markdown plan content. Required.
- **plan_id**: Optional. If provided, updates the existing plan instead of creating a new one. Use when revising after feedback.

## What the plan content must include

The plan is the user's window into your thinking. Write it so a non-expert can understand:

1. **Context and analysis** — what the current state is, what problem you are solving, and why this approach
2. **Trade-offs considered** — alternatives you evaluated and why you chose this path
3. **Implementation steps** — a numbered, ordered list of concrete steps (each step will become a task)
4. **Files touched** — which files will be created, modified, or deleted
5. **Risks and validation** — known risks, edge cases, and how you will verify the result

Avoid vague steps like "update the code". Each step should be specific enough that the user
understands exactly what will change and why.

## After calling submit_plan

- The plan is persisted and shown to the user as an interactive artifact
- You remain in plan mode — do NOT call exit_plan_mode yet
- The user may send feedback (revise and resubmit) or approve via "Proceed"
- On approval you will receive a confirmation — then call exit_plan_mode`
