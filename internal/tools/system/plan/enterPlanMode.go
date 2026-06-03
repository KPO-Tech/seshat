package plan

import (
	"context"
	"fmt"

	"github.com/EngineerProjects/nexus-engine/internal/modes"
	"github.com/EngineerProjects/nexus-engine/internal/modes/execution"
	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	"github.com/EngineerProjects/nexus-engine/internal/tools/schema"
	"github.com/EngineerProjects/nexus-engine/internal/types"
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

	// Compute plan file path so the model knows where its plan will be saved.
	planFilePath := execution.GetPlanFilePath(sessionID, t.agentID)

	instructions := fmt.Sprintf(`Entered plan mode.

In plan mode you should:
1. Thoroughly explore the codebase using read-only tools (read_file, glob, grep, bash for ls/git log/cat/find)
2. Clarify any ambiguous requirements with ask_user_question before designing
3. Design a concrete, ordered implementation approach
4. When ready, call exit_plan_mode and pass your full plan as the "plan" parameter

Your plan will be saved to: %s

DO NOT edit or create source files during plan mode — this is a read-only exploration and planning phase.
Once you call exit_plan_mode the plan will be presented to the user for approval.
After approval you will receive the plan back and can start implementation with todo_write.`, execution.GetDisplayPath(planFilePath))

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

	// Requires user approval
	return types.Ask("Enter plan mode to explore and design an implementation approach?")
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

## Enter plan mode when

- The task is a feature, refactor, migration, or broad bug fix
- Multiple valid approaches exist and the right one is unclear
- The change spans several files or subsystems and you need to explore first
- The request is ambiguous enough that starting without alignment risks wasted work
- The task involves risky or irreversible operations (DB migrations, auth changes, major deletions)

## Do NOT enter plan mode when

- The task is a small, obvious, bounded fix
- The user already gave you explicit step-by-step instructions
- The request is pure research or reading with no planned implementation
- You already have enough context to start immediately and the risk is low

## What to do in plan mode

1. Explore the relevant code using read-only tools (Read, Bash with ls/cat/git log, grep)
2. Clarify ambiguous requirements with ask_user_question if needed
3. Design a concrete, ordered implementation plan
4. Call ` + "`submit_plan`" + ` with the full plan content and a short slug
5. Wait for the user to review — they may send feedback (revise and resubmit) or approve
6. When you receive approval, call ` + "`exit_plan_mode`" + ` and immediately create your task list

DO NOT edit or create source files while in plan mode.
The plan is presented to the user as an interactive artifact for review and approval.`
