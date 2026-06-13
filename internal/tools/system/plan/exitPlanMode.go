package plan

import (
	"context"
	"fmt"
	"time"

	"github.com/EngineerProjects/nexus-engine/internal/modes"
	"github.com/EngineerProjects/nexus-engine/internal/modes/execution"
	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	"github.com/EngineerProjects/nexus-engine/internal/tools/schema"
	"github.com/EngineerProjects/nexus-engine/internal/types"
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
Start by updating your todo list with todo_write if applicable.

Your plan has been saved to: %s

## Approved Plan:
%s`, execution.GetDisplayPath(executionFilePath), executionContent)
	}

	result := tool.NewTextResult(message)
	result.ContextModifier = func(ctx tool.ToolUseContext) tool.ToolUseContext {
		ctx.PermissionMode = restoreMode
		ctx.PrePlanMode = ""
		ctx.ExecutionMode = ""
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

// RequiresUserInteraction returns whether this tool requires explicit user interaction.
func (t *ExitPlanModeTool) RequiresUserInteraction() bool {
	// The runtime already routes ask-decisions through the prompt bridge for
	// non-agent sessions. Returning false here keeps ExitPlanMode executable so
	// its ContextModifier can restore the previous session mode.
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
const ExitPlanModePrompt = `Use this tool to exit plan mode after the user has validated your plan.

Call this ONLY after submit_plan has been approved.
When the user clicks "Proceed" in the UI you will receive a confirmation — at that point call exit_plan_mode.

## Do NOT call this tool when

- You are not in plan mode
- You have not yet called submit_plan
- The user has not yet approved the plan

## Immediately after exiting

Convert each implementation step from the approved plan into a task using task_create.
Create ALL tasks upfront before starting any work — this is your execution checklist.
Then work through the list in strict order: mark in_progress before starting each step,
completed when done. Do not skip steps or work out of order.`
