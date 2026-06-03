package contract

import (
	"context"

	"github.com/EngineerProjects/nexus-engine/internal/types"
)

// Tool represents a tool that can be called by the AI.
//
// This interface is aligned with OpenClaude's Tool contract. Every method
// beyond the core seven (Definition, Call, Description, ValidateInput,
// CheckPermissions, IsConcurrencySafe, IsReadOnly) has a sensible default
// provided by baseTool so that simple tools only implement what they need.
type Tool interface {
	// Definition returns the tool's definition.
	Definition() Definition

	// Call executes the tool with the given input.
	// The returned error is reserved for truly unrecoverable system failures
	// (e.g. context cancellation). Tool-level errors should be returned inside
	// CallResult.Error so that the runtime always produces a tool_result message.
	Call(ctx context.Context, input CallInput, permissionCheck types.CanUseToolFn) (CallResult, error)

	// Description returns a human-readable description of what this tool does.
	Description(ctx context.Context) (string, error)

	// ValidateInput validates and optionally normalizes tool input before execution.
	ValidateInput(ctx context.Context, input map[string]any) (map[string]any, error)

	// CheckPermissions performs tool-specific permission checks before the global
	// permission pipeline. Return types.Passthrough to delegate to global checks.
	CheckPermissions(ctx context.Context, input map[string]any, toolCtx ToolUseContext) types.PermissionResult

	// IsConcurrencySafe returns whether this specific tool use can run concurrently.
	IsConcurrencySafe(input map[string]any) bool

	// IsReadOnly returns whether this specific tool use is read-only.
	IsReadOnly(input map[string]any) bool

	// IsEnabled returns whether this tool is currently active.
	// Disabled tools are skipped during execution. Default: true.
	IsEnabled() bool

	// FormatResult serialises the tool output into the string that will be sent
	// back to the model inside a tool_result content block. Each tool controls
	// its own serialisation (equivalent to OpenClaude's mapToolResultToToolResultBlockParam).
	// Default: fmt.Sprintf("%v", data) when Content is empty.
	FormatResult(data any) string

	// BackfillInput enriches a shallow clone of the parsed input with derived
	// fields that should be visible to hooks and permissions but are NOT passed
	// to tool.Call() (equivalent to OpenClaude's backfillObservableInput).
	// The returned map is the enriched copy. Default: returns input unchanged.
	BackfillInput(ctx context.Context, input map[string]any) map[string]any
}

// Toolset is a named group of tools resolved dynamically at call time.
//
// Unlike static tool registration (once at session creation), a Toolset is
// evaluated at the beginning of each loop iteration so that the set of
// available tools can change based on runtime context — the current session
// state, user preferences, permissions, or any other dynamic condition.
//
// Usage:
//
//	RunRequest.Toolsets = []contract.Toolset{myDynamicSet}
//
// The loop merges the returned tools with the static Tools map each iteration.
// If a Toolset returns a tool whose name already exists in the static map the
// Toolset version takes precedence (allows overriding defaults at runtime).
type Toolset interface {
	// Name returns a stable identifier for this toolset (used in logs/traces).
	Name() string
	// Tools returns the tools that should be available for this iteration.
	// Implementations must be fast; they are called every loop iteration.
	Tools(ctx context.Context) []Tool
}

// PermissionMatcherTool is an optional capability for tools that support
// content-specific permission-rule matching.
type PermissionMatcherTool interface {
	PreparePermissionMatcher(ctx context.Context, input map[string]any) (func(ruleContent string) bool, error)
}

// RequiresUserInteractionTool is an optional capability for tools that
// require explicit user interaction even in bypass mode. When a tool
// implements this interface and RequiresUserInteraction returns true, the
// permission pipeline will not auto-allow the tool in bypass mode.
// Aligned with OpenClaude's tool.requiresUserInteraction?().
type RequiresUserInteractionTool interface {
	RequiresUserInteraction() bool
}

// PlanModeExecutableTool is an optional capability for tools that must still
// execute while the session is in plan mode, even if they are not read-only.
// This is primarily for plan control-flow tools such as ExitPlanMode.
type PlanModeExecutableTool interface {
	ExecutesInPlanMode(input map[string]any) bool
}
