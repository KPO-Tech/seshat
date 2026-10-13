package plan

import (
	"context"
	"fmt"
	"time"

	"github.com/KPO-Tech/seshat-runtime/internal/modes"
	"github.com/KPO-Tech/seshat-runtime/internal/modes/execution"
	tool "github.com/KPO-Tech/seshat-runtime/internal/tools/registry"
	"github.com/KPO-Tech/seshat-runtime/internal/tools/schema"
	"github.com/KPO-Tech/seshat-runtime/internal/types"
)

// EnterPlanModeTool represents the EnterPlanMode tool.
type EnterPlanModeTool struct {
	sessionID types.SessionID
	agentID   *types.AgentID
}

// Config represents tool configuration.
type EnterPlanModeConfig struct {
	SessionID types.SessionID
	AgentID   *types.AgentID
}

// DefaultEnterPlanModeConfig returns default configuration.
func DefaultEnterPlanModeConfig(sessionID types.SessionID) *EnterPlanModeConfig {
	return &EnterPlanModeConfig{
		SessionID: sessionID,
		AgentID:   nil,
	}
}

// NewEnterPlanModeTool creates a new EnterPlanMode tool.
// Session ID may be provided later by the runtime tool context.
func NewEnterPlanModeTool(config *EnterPlanModeConfig) *EnterPlanModeTool {
	if config == nil {
		return nil
	}

	return &EnterPlanModeTool{
		sessionID: config.SessionID,
		agentID:   config.AgentID,
	}
}

// Definition returns the tool definition.
func (t *EnterPlanModeTool) Definition() tool.Definition {
	return tool.Definition{
		Name:        ToolNameEnterPlanMode,
		DisplayName: "EnterPlanMode",
		SearchHint:  SearchHintEnterPlanMode,
		Description: EnterPlanModePrompt,
		Category:    "planning",
		InputSchema: schema.FromMap(map[string]any{
			"type":       "object",
			"properties": map[string]any{},
			"required":   []string{},
		}),
		IsReadOnly:         true,
		IsConcurrencySafe:  false,
		IsDestructive:      false,
		RequiresPermission: true,
		Metadata:           map[string]any{"surface_profiles": []string{"mono_run"}},
	}
}

// Call executes the tool.
// Aligned with OpenClaude's EnterPlanModeTool.call
func (t *EnterPlanModeTool) Call(
	ctx context.Context,
	input tool.CallInput,
	permissionCheck types.CanUseToolFn,
) (tool.CallResult, error) {
	// EnterPlanMode cannot be used in agent contexts
	if t.agentID != nil {
		return tool.NewErrorResult(fmt.Errorf("EnterPlanMode tool cannot be used in agent contexts")), nil
	}

	sessionID, err := t.resolveSessionID(input)
	if err != nil {
		return tool.NewErrorResult(err), nil
	}

	toolCtx := input.ToolContextValue()
	currentMode := toolCtx.PermissionMode
	if currentMode == "" {
		currentMode = types.PermissionModeOnRequest
	}

	// Update runtime-only plan state for this session.
	execution.EnterPlanMode(sessionID, t.agentID)

	if emitter, ok := ctx.Value(types.RuntimeEventEmitterKey).(func(types.RuntimeEvent)); ok && emitter != nil {
		emitter(types.RuntimeEvent{
			Type:          types.RuntimeEventTypeExecutionModeChanged,
			SessionID:     sessionID,
			Timestamp:     time.Now().UTC(),
			ExecutionMode: string(modes.ExecutionModePlan),
		})
	}

	// Compute plan file path so the model knows where its plan will be saved.
	planFilePath := execution.GetPlanFilePath(sessionID, t.agentID)

	instructions := fmt.Sprintf(`Entered plan mode.

In plan mode you should:
1. Thoroughly explore the codebase using read-only tools (read_file, glob, grep, bash for ls/git log/cat/find)
2. Clarify any ambiguous requirements with ask_user_question before designing
3. Design a concrete, ordered implementation approach
4. When ready, call submit_plan with the full plan content and a short slug
5. Wait for the user to review — they may send feedback or approve

Your plan will be saved to: %s

DO NOT edit or create source files during plan mode — this is a read-only exploration and planning phase.
The plan is presented to the user as an interactive artifact for review.
On approval, follow the approval message: start implementing directly if plan mode was already
exited for you, or call exit_plan_mode if you are still in plan mode. Track the work with
task_create/task_update.`, execution.GetDisplayPath(planFilePath))

	result := tool.NewTextResult(instructions)
	result.ContextModifier = func(ctx tool.ToolUseContext) tool.ToolUseContext {
		ctx.PermissionMode = currentMode
		ctx.PrePlanMode = currentMode
		ctx.ExecutionMode = string(modes.ExecutionModePlan)
		if currentMode == types.PermissionModeBypass {
			ctx.IsBypassPermissionsModeAvailable = true
		}
		return ctx
	}
	return result, nil
}

// Description returns a human-readable description.
func (t *EnterPlanModeTool) Description(ctx context.Context) (string, error) {
	return DescriptionEnterPlanMode, nil
}

// ValidateInput validates and normalizes input.
func (t *EnterPlanModeTool) ValidateInput(ctx context.Context, input map[string]any) (map[string]any, error) {
	// EnterPlanMode takes no input
	if len(input) > 0 {
		return nil, fmt.Errorf("EnterPlanMode takes no input")
	}
	return input, nil
}

// CheckPermissions performs tool-specific permission checks.
func (t *EnterPlanModeTool) CheckPermissions(ctx context.Context, input map[string]any, toolCtx tool.ToolUseContext) types.PermissionResult {
	// Cannot be used in agent contexts
	if t.agentID != nil {
		return types.Deny("EnterPlanMode tool cannot be used in agent contexts")
	}

	// Cannot be used if already in plan mode
	if modes.IsPlanModeString(toolCtx.ExecutionMode) {
		return types.Deny("Already in plan mode")
	}

	// Deliberately not an approval point, and this used to say otherwise: the
	// function returned types.Ask("Enter plan mode to explore and design an
	// implementation approach?") for years without ever asking anybody. A bare
	// Ask with no DecisionReason is replaced by an Allow inside
	// resolveToolPermissions, and engine.isAlwaysSafeTool lists
	// enter_plan_mode/exit_plan_mode in every mode but never - so the declared
	// ask was unreachable twice over (pinned by
	// internal/execution/enter_plan_mode_permission_test.go).
	//
	// That is the right outcome, not an oversight to repair. Entering plan mode
	// is read-only, reversible with one keystroke, and it *reduces* what the
	// agent can do, so a prompt here would guard the safe direction at the
	// moment the agent knows least about the task. The human checkpoint belongs
	// at the other end, on the plan itself: exit_plan_mode asks, and submit_plan
	// offers the plan for review with comments. Claude Code draws the line in
	// exactly the same place - its EnterPlanMode tool is documented as not
	// requiring permission while ExitPlanMode requires it.
	return types.Passthrough(input)
}

// IsConcurrencySafe returns whether this tool use can run concurrently.
func (t *EnterPlanModeTool) IsConcurrencySafe(input map[string]any) bool {
	return false
}

// IsReadOnly returns whether this tool use is read-only.
func (t *EnterPlanModeTool) IsReadOnly(input map[string]any) bool {
	return true
}

// IsEnabled returns whether this tool is currently active.
func (t *EnterPlanModeTool) IsEnabled() bool {
	// Disabled when in agent contexts
	if t.agentID != nil {
		return false
	}
	return true
}

// FormatResult serialises the tool output.
func (t *EnterPlanModeTool) FormatResult(data any) string {
	if s, ok := data.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", data)
}

// BackfillInput enriches the input with derived fields.
func (t *EnterPlanModeTool) BackfillInput(ctx context.Context, input map[string]any) map[string]any {
	return input
}

// Prompt returns the system prompt for this tool.
func (t *EnterPlanModeTool) Prompt() string {
	return EnterPlanModePrompt
}

func (t *EnterPlanModeTool) resolveSessionID(input tool.CallInput) (types.SessionID, error) {
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

	return "", fmt.Errorf("session ID is required for EnterPlanMode")
}

// EnterPlanModePrompt is the system prompt for the EnterPlanMode tool.
const EnterPlanModePrompt = `Use this tool before implementation when the task requires analysis and user alignment before you touch any file.

If the user explicitly asks you to enter plan mode, call ` + "`enter_plan_mode`" + ` before continuing with analysis or implementation.

## Enter plan mode when

This mode exists to obtain the user's alignment, not to organise work. Enter it when a
choice belongs to the user and settling that choice before any file is touched costs less
than settling it after:

- Several defensible designs exist and the user has not indicated which one they want
- The work is hard to undo or expensive to redo: data migrations, auth or permission
  changes, deletions, a published interface or schema
- The acceptance criteria themselves are ambiguous, so you cannot tell what "done" looks
  like from the repository

Investigating a wide, unfamiliar or multi-file area is NOT a reason on its own. Reading
is free in either mode, and a tracked task list carries the roadmap while you read, so a
request that is merely large and unambiguous belongs in execute mode.

## Do NOT enter plan mode when

- The task is a small, obvious, bounded fix
- The user already gave you explicit step-by-step instructions
- The request is pure research or reading with no planned implementation
- You already have enough context to start immediately and the risk is low
- The task is multi-step but unambiguous and the approach is not in question:
  open a tracked task list with ` + "`task_create`" + ` and execute it directly. A
  task list is not a plan, it needs no approval, and entering this mode just to
  be able to track the steps costs a full round of alignment for nothing.

## What to do in plan mode

1. Explore the relevant code using read-only tools (` + "`read_file`" + `, ` + "`glob`" + `, ` + "`grep`" + `, and ` + "`bash`" + ` for ls/cat/git log/find)
2. Clarify ambiguous requirements with ask_user_question if needed
3. Design a concrete, ordered implementation plan
4. Call ` + "`submit_plan`" + ` with the full plan content and a short slug
5. Wait for the user to review — they may send feedback (revise and resubmit) or approve
6. On approval, follow the approval message: implement directly if plan mode was
   already exited for you, or call ` + "`exit_plan_mode`" + ` if you are still in plan
   mode, then track the work with ` + "`task_create`" + ` / ` + "`task_update`" + `

DO NOT edit or create source files while in plan mode.
The plan is presented to the user as an interactive artifact for review and approval.`
