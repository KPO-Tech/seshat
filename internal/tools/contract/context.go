package contract

import (
	"context"

	"github.com/EngineerProjects/nexus-engine/internal/types"
	"github.com/EngineerProjects/nexus-engine/internal/workspace"
)

// PermissionRuleMatcherKey is the metadata key used to pass a prepared matcher through runtime metadata.
const PermissionRuleMatcherKey = "permission_rule_matcher"

// ResolvedToolMetadataKey is the metadata key used to pass the resolved tool through runtime metadata.
const ResolvedToolMetadataKey = "resolved_tool"

// ToolFromMetadata extracts a resolved tool from metadata when available.
func ToolFromMetadata(metadata map[string]any) Tool {
	if metadata == nil {
		return nil
	}
	resolvedTool, _ := metadata[ResolvedToolMetadataKey].(Tool)
	return resolvedTool
}

// PermissionMatcherFromMetadata extracts a prepared permission matcher from metadata when available.
func PermissionMatcherFromMetadata(metadata map[string]any) func(string) bool {
	if metadata == nil {
		return nil
	}
	matcher, _ := metadata[PermissionRuleMatcherKey].(func(string) bool)
	return matcher
}

// AttachPermissionMatcherMetadata clones metadata and injects optional tool permission matching context.
func AttachPermissionMatcherMetadata(metadata map[string]any, resolvedTool Tool, matcher func(string) bool) map[string]any {
	cloned := make(map[string]any, len(metadata)+2)
	for k, v := range metadata {
		cloned[k] = v
	}
	if resolvedTool != nil {
		cloned[ResolvedToolMetadataKey] = resolvedTool
	}
	if matcher != nil {
		cloned[PermissionRuleMatcherKey] = matcher
	}
	return cloned
}

// BuildPermissionMatcher prepares a content-specific permission matcher when the tool supports it.
func BuildPermissionMatcher(ctx context.Context, resolvedTool Tool, input map[string]any) (func(string) bool, error) {
	if resolvedTool == nil {
		return nil, nil
	}
	matcherTool, ok := resolvedTool.(PermissionMatcherTool)
	if !ok {
		return nil, nil
	}
	return matcherTool.PreparePermissionMatcher(ctx, input)
}

// RequiresUserInteraction returns true if the tool requires explicit user
// interaction even in bypass mode. Returns false if the tool does not
// implement RequiresUserInteractionTool or if it returns false.
func RequiresUserInteraction(t Tool) bool {
	if uit, ok := t.(RequiresUserInteractionTool); ok {
		return uit.RequiresUserInteraction()
	}
	return false
}

// ExecutesInPlanMode returns true when a tool should execute while the session
// is in plan mode. Read-only tools are always allowed so the agent can inspect
// the codebase, and specific control tools can opt in explicitly.
func ExecutesInPlanMode(t Tool, input map[string]any) bool {
	if t == nil {
		return false
	}
	if t.IsReadOnly(input) {
		return true
	}
	if planTool, ok := t.(PlanModeExecutableTool); ok {
		return planTool.ExecutesInPlanMode(input)
	}
	return false
}

// ContextModifier mutates the tool runtime context after a tool call.
type ContextModifier func(ToolUseContext) ToolUseContext

// ToolUseContext provides context for tool execution.
// Aligned with OpenClaude's ToolUseContext (headless-relevant subset).
type ToolUseContext struct {
	// SessionID identifies the session
	SessionID types.SessionID `json:"session_id"`

	// TurnID identifies the turn
	TurnID types.TurnID `json:"turn_id"`

	// ToolUseID identifies this specific tool use
	ToolUseID string `json:"tool_use_id"`

	// PermissionMode is the current permission mode (who approves)
	PermissionMode types.PermissionMode `json:"permission_mode"`

	// ExecutionMode is the current execution mode (agent behavior: plan, execute, browse)
	// Separated from PermissionMode for clean separation of concerns.
	ExecutionMode string `json:"execution_mode,omitempty"`

	// PrePlanMode stores the mode that was active before entering plan mode.
	PrePlanMode types.PermissionMode `json:"pre_plan_mode,omitempty"`

	// IsAutoModeAvailable indicates whether auto mode is available for the session.
	IsAutoModeAvailable bool `json:"is_auto_mode_available,omitempty"`

	// WorkingDirectory is the current working directory
	WorkingDirectory string `json:"working_directory,omitempty"`

	// Workspace is the enforced filesystem boundary for file-aware tools.
	Workspace *workspace.Context `json:"-"`

	// AdditionalWorkingDirectories holds extra working directories that the
	// tool may access (equivalent to OpenClaude's additionalWorkingDirectories).
	AdditionalWorkingDirectories map[string]string `json:"additional_working_directories,omitempty"`

	// EnableSandbox indicates whether the tool should run in a sandbox.
	// Used for sandbox auto-allow permission logic.
	EnableSandbox bool `json:"enable_sandbox,omitempty"`

	// CanUseTool is the permission check function
	CanUseTool types.CanUseToolFn `json:"-"`

	// RequestPrompt is a function to prompt the user
	RequestPrompt types.PromptFn `json:"-"`

	// Metadata contains additional context
	Metadata map[string]any `json:"metadata,omitempty"`

	// IsBypassPermissionsModeAvailable indicates whether bypass mode was
	// available at the start of the session. Used by plan mode to determine
	// whether it should behave like bypass mode or ask for permissions.
	IsBypassPermissionsModeAvailable bool `json:"is_bypass_permissions_mode_available,omitempty"`
}

// NewToolUseContext creates a new ToolUseContext
func NewToolUseContext(
	sessionID types.SessionID,
	turnID types.TurnID,
	toolUseID string,
	permissionMode types.PermissionMode,
) ToolUseContext {
	return ToolUseContext{
		SessionID:           sessionID,
		TurnID:              turnID,
		ToolUseID:           toolUseID,
		PermissionMode:      permissionMode,
		Metadata:            make(map[string]any),
		IsAutoModeAvailable: false,
	}
}

// ToolContextValue returns the runtime tool context carried by the call input, or a default context.
func (i CallInput) ToolContextValue() ToolUseContext {
	if i.ToolContext != nil {
		return *i.ToolContext
	}
	return NewToolUseContext(i.SessionID, i.TurnID, i.ToolUseID, types.PermissionModeOnRequest)
}
