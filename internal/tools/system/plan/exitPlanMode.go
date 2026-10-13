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

// Output represents the result of exiting execution mode.
type ExitPlanModeOutput struct {
	Plan     string `json:"plan"`
	FilePath string `json:"file_path,omitempty"`
	IsAgent  bool   `json:"is_agent"`
}

// ExitPlanModeTool represents the ExitPlanMode tool.
type ExitPlanModeTool struct {
	sessionID types.SessionID
	agentID   *types.AgentID
}

// ExitPlanModeConfig represents tool configuration.
type ExitPlanModeConfig struct {
	SessionID types.SessionID
	AgentID   *types.AgentID
}

// DefaultExitPlanModeConfig returns default configuration.
func DefaultExitPlanModeConfig(sessionID types.SessionID) *ExitPlanModeConfig {
	return &ExitPlanModeConfig{
		SessionID: sessionID,
		AgentID:   nil,
	}
}

// NewExitPlanModeTool creates a new ExitPlanMode tool.
// Session ID may be provided later by the runtime tool context.
func NewExitPlanModeTool(config *ExitPlanModeConfig) *ExitPlanModeTool {
	if config == nil {
		return nil
	}

	return &ExitPlanModeTool{
		sessionID: config.SessionID,
		agentID:   config.AgentID,
	}
}

// Definition returns the tool definition.
func (t *ExitPlanModeTool) Definition() tool.Definition {
	return tool.Definition{
		Name:        ToolNameExitPlanMode,
		DisplayName: "ExitPlanMode",
		SearchHint:  SearchHintExitPlanMode,
		Description: ExitPlanModePrompt,
		Category:    "mode",
		InputSchema: schema.FromMap(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"plan": map[string]any{
					"type":        "string",
					"description": "The implementation plan to present for approval. If not provided, the current plan file content will be used.",
				},
			},
		}),
		IsReadOnly:         false, // Can write execution to disk
		IsConcurrencySafe:  false,
		IsDestructive:      false,
		RequiresPermission: true,
		Metadata:           map[string]any{"surface_profiles": []string{"mono_run"}},
	}
}

// Call executes the tool.
// Aligned with OpenClaude's ExitPlanModeV2Tool.call
func (t *ExitPlanModeTool) Call(
	ctx context.Context,
	input tool.CallInput,
	permissionCheck types.CanUseToolFn,
) (tool.CallResult, error) {
	sessionID, err := t.resolveSessionID(input)
	if err != nil {
		return tool.NewErrorResult(err), nil
	}

	toolCtx := input.ToolContextValue()
	currentMode := toolCtx.ExecutionMode
	if currentMode == "" {
		// Fallback to checking PermissionMode for backward compat
		currentMode = string(toolCtx.PermissionMode)
		if currentMode == "" {
			currentMode = string(types.PermissionModeOnRequest)
		}
	}

	// Validate that we're in execution mode
	if !modes.IsPlanModeString(currentMode) {
		return tool.NewErrorResult(fmt.Errorf("not in plan mode. This tool is only for exiting plan mode after writing a plan")), nil
	}

	// Get plan content from input or file. Accept the legacy "execution" field
	// for compatibility with the refactor that renamed plan-mode terminology.
	executionContent, ok := input.Parsed["plan"].(string)
	if !ok || executionContent == "" {
		executionContent, ok = input.Parsed["execution"].(string)
	}
	if !ok || executionContent == "" {
		// Read from execution file
		executionContent, err = execution.GetPlan(sessionID, t.agentID)
		if err != nil {
			return tool.NewErrorResult(fmt.Errorf("failed to read execution file: %w", err)), nil
		}
		if executionContent == "" {
			return tool.NewErrorResult(fmt.Errorf("no execution content provided and execution file is empty")), nil
		}
	} else {
		// Write to execution file if content was provided
		if err := execution.SetPlan(sessionID, t.agentID, executionContent); err != nil {
			return tool.NewErrorResult(fmt.Errorf("failed to write execution file: %w", err)), nil
		}
	}

	// Get execution file path
	executionFilePath := execution.GetPlanFilePath(sessionID, t.agentID)

	// Apply runtime-only plan exit side effects for this session.
	execution.ExitPlanMode(sessionID)
	// The review is settled: drop its state so the next plan cycle starts clean.
	execution.ClearPlanReview(sessionID)

	if emitter, ok := ctx.Value(types.RuntimeEventEmitterKey).(func(types.RuntimeEvent)); ok && emitter != nil {
		emitter(types.RuntimeEvent{
			Type:          types.RuntimeEventTypeExecutionModeChanged,
			SessionID:     sessionID,
			Timestamp:     time.Now().UTC(),
			ExecutionMode: string(modes.ExecutionModeExecute),
		})
	}

	// Restore the approval mode that was active before entering plan mode.
	restoreMode := toolCtx.PrePlanMode
	if restoreMode == "" {
		restoreMode = types.PermissionModeOnRequest
	}

	// Create output
	output := ExitPlanModeOutput{
		Plan:     executionContent,
		FilePath: executionFilePath,
		IsAgent:  t.agentID != nil,
	}

	// Build the result message that the model will receive after user approval.
	var message string
	if output.IsAgent {
		message = fmt.Sprintf("Plan approved.\n\n## Approved Plan:\n%s", executionContent)
	} else {
		message = fmt.Sprintf(`User has approved your plan. You can now start coding.
Start by creating/updating tracked tasks with task_create and task_update if applicable.

Your plan has been saved to: %s

## Approved Plan:
%s`, execution.GetDisplayPath(executionFilePath), executionContent)
	}

	result := tool.NewTextResult(message)
	result.ContextModifier = func(ctx tool.ToolUseContext) tool.ToolUseContext {
		ctx.PermissionMode = restoreMode
		ctx.PrePlanMode = ""
		ctx.ExecutionMode = string(modes.ExecutionModeExecute)
		return ctx
	}
	return result, nil
}

// Description returns a human-readable description.
func (t *ExitPlanModeTool) Description(ctx context.Context) (string, error) {
	return DescriptionExitPlanMode, nil
}

// ValidateInput validates and normalizes input.
func (t *ExitPlanModeTool) ValidateInput(ctx context.Context, input map[string]any) (map[string]any, error) {
	// All fields are optional, no validation needed
	return input, nil
}

// CheckPermissions performs tool-specific permission checks.
func (t *ExitPlanModeTool) CheckPermissions(ctx context.Context, input map[string]any, toolCtx tool.ToolUseContext) types.PermissionResult {
	if !modes.IsPlanModeString(toolCtx.ExecutionMode) {
		return types.Deny("Not in plan mode. This tool is only for exiting plan mode after writing a plan.")
	}

	if t.agentID != nil {
		return types.AllowWithDecisionReason(
			"Agent plan submitted for approval",
			&types.PermissionDecisionReason{
				Type:   types.PermissionDecisionReasonMode,
				Source: "executionMode",
				Reason: "agent plan approval",
			},
		)
	}

	// A plan the user has not decided on is not a plan to implement. This has to
	// be a Deny rather than the Ask below: resolveToolPermissions turns a local
	// Deny into a permission-stage failure, so the tool does not run and the
	// session stays in plan mode, whereas an Ask carrying no Rule or safetyCheck
	// reason is replaced by an Allow before any human sees it.
	//
	// Only a session that actually submitted a plan is held back. submit_plan is
	// optional - it is disabled when the host provides no persistence hook - and
	// without a submission there is no review surface to answer, so plan mode
	// must keep its exit. Sub-agent plans are out of scope here as well: they are
	// auto-allowed above, and gating them would block a path with no review
	// surface of its own.
	sessionID := t.sessionID
	if sessionID == "" {
		sessionID = toolCtx.SessionID
	}
	if sessionID != "" && execution.PlanAwaitingApproval(sessionID) {
		return types.Deny(
			"The plan is still waiting for the user's review. Do not start implementing: " +
				"wait for their decision, and if they asked for changes, revise the plan and " +
				"call submit_plan again.",
		)
	}

	// Include a plan preview in the approval message so the user can see what they're approving.
	plan, _ := input["plan"].(string)
	if plan == "" {
		return types.Ask("Exit plan mode? (no plan content found)")
	}
	preview := plan
	if len(preview) > 800 {
		preview = preview[:800] + "\n\n[... truncated — full plan will be saved to disk ...]"
	}
	return types.Ask(fmt.Sprintf("Approve this plan and start implementation?\n\n---\n%s", preview))
}

// KNOWN DEFECT, documented 2026-10-12 (TODO.md, "the plan-approval gate does
// not reach the user"): neither Ask below is ever shown to anybody. A bare
// types.Ask carries no DecisionReason, and resolveToolPermissions only
// preserves an Ask that has a Rule or safetyCheck reason or that comes from a
// tool implementing RequiresUserInteraction. Everything else reaches its final
// default branch and is replaced by an Allow, so the plan preview this function
// builds is discarded. isAlwaysSafeTool allows it a second time.
//
// The Deny above is the enforced half of that gate: a plan the user has not
// approved cannot be implemented. What remains unenforced is the preview itself
// (nobody is shown the plan to decide on) and the case of a session that never
// submitted a plan at all - see TODO.md for the three separate mechanisms that
// were built to deliver the approval and are still inert.
//
// The consequence is that plan mode's read-only guarantee is advisory: the
// model can leave the mode on its own and start writing, because leaving is
// what grants write access (write_file/edit_file deny while ExecutionMode is
// plan). See exitPlanModeApproval_test.go for the pinned reproduction.
//
// Making the ask survive is not enough on its own: routing it as a real Ask
// stops the tool in the permission stage, and the SDK's plan round-trip then
// never flips the session back to execute even though its host prompt function
// answers "approved" (TestPersistedSessionsCarryPlanModeAcrossRestore,
// pkg/sdk/client_test.go). The gate therefore also needs the approval path to
// re-run the tool so its ContextModifier applies - that is the work of the
// defect, not a one-line change here.

// IsConcurrencySafe returns whether this tool use can run concurrently.
func (t *ExitPlanModeTool) IsConcurrencySafe(input map[string]any) bool {
	return false
}

// IsReadOnly returns whether this tool use is read-only.
func (t *ExitPlanModeTool) IsReadOnly(input map[string]any) bool {
	return false // Can write execution to disk
}

// IsEnabled returns whether this tool is currently active.
func (t *ExitPlanModeTool) IsEnabled() bool {
	// The session tool surface is currently materialized once, so capability
	// gating must stay in CheckPermissions/Call until execution-mode surface rebuilds
	// become runtime-aware.
	return true
}

// FormatResult serialises the tool output.
func (t *ExitPlanModeTool) FormatResult(data any) string {
	if s, ok := data.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", data)
}

// BackfillInput enriches the input with derived fields.
func (t *ExitPlanModeTool) BackfillInput(ctx context.Context, input map[string]any) map[string]any {
	// If plan is not provided, read it from disk so hooks and permissions can
	// inspect the content consistently.
	if _, hasPlan := input["plan"]; !hasPlan {
		if _, hasLegacyExecution := input["execution"]; hasLegacyExecution {
			return input
		}
		sessionID := t.sessionID
		if sessionID != "" {
			if executionContent, err := execution.GetPlan(sessionID, t.agentID); err == nil {
				input["plan"] = executionContent
			}
		}
	}
	return input
}

// RequiresUserInteraction is deliberately false, and it is not a working gate.
//
// The comment here used to assert that "the runtime already routes ask-decisions
// through the prompt bridge for non-agent sessions". That belief is what let the
// approval go missing: a bare Ask is replaced by an Allow in
// resolveToolPermissions before any prompt bridge sees it, so the user is never
// asked and the plan preview is never shown (see the KNOWN DEFECT note and
// exitPlanModeApproval_test.go).
func (t *ExitPlanModeTool) RequiresUserInteraction() bool {
	return false
}

// ExecutesInPlanMode allows ExitPlanMode to run while the session is in plan
// mode so it can restore the previous permission state.
func (t *ExitPlanModeTool) ExecutesInPlanMode(input map[string]any) bool {
	return true
}

// Prompt returns the system prompt for this tool.
func (t *ExitPlanModeTool) Prompt() string {
	return ExitPlanModePrompt
}

func (t *ExitPlanModeTool) resolveSessionID(input tool.CallInput) (types.SessionID, error) {
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

	return "", fmt.Errorf("session ID is required for ExitPlanMode")
}

// ExitPlanModePrompt is the system prompt for the ExitPlanMode tool.
const ExitPlanModePrompt = `Use this tool to leave plan mode once the user has approved the plan you submitted with submit_plan.

Call it only when you are still in plan mode and the user has approved the plan. Approving exits plan
mode in two possible ways, and the approval message tells you which one you are in:
- the host may have already exited plan mode for you — then do NOT call this tool, just start implementing;
- or you receive a plain approval — then call exit_plan_mode to leave the mode and begin.

## Do NOT call this tool when

- You are not in plan mode
- You have not yet called submit_plan
- The user has not yet approved the plan
- The approval already told you plan mode was exited for you

## After exiting

Begin implementing the approved plan. Create or refresh the tracked task list with task_create
so the user can watch each step, and update it as you go: mark a task in_progress before you
start it and completed when it is done. Work through the list in order rather than skipping ahead.`
