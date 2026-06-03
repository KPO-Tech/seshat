package types

import (
	"context"
)

// HookEvent represents a lifecycle event in the engine
type HookEvent string

const (
	// Session hooks
	HookEventSessionStart HookEvent = "session_start"
	HookEventSessionEnd   HookEvent = "session_end"

	// Query/Loop hooks
	HookEventQueryStart        HookEvent = "query_start"
	HookEventQueryComplete     HookEvent = "query_complete"
	HookEventIterationStart    HookEvent = "iteration_start"
	HookEventIterationStop     HookEvent = "iteration_stop"
	HookEventIterationContinue HookEvent = "iteration_continue"
	HookEventIterationComplete HookEvent = "iteration_complete"
	HookEventToolUsesStart     HookEvent = "tool_uses_start"
	HookEventToolUsesComplete  HookEvent = "tool_uses_complete"

	// Turn hooks
	HookEventTurnStart   HookEvent = "turn_start"
	HookEventTurnEnd     HookEvent = "turn_end"
	HookEventTurnStop    HookEvent = "turn_stop"
	HookEventStopFailure HookEvent = "stop_failure"

	// Tool hooks
	HookEventPreToolUse      HookEvent = "pre_tool_use"
	HookEventPostToolUse     HookEvent = "post_tool_use"
	HookEventPostToolUseFail HookEvent = "post_tool_use_fail"

	// Compact hooks
	HookEventPreCompact  HookEvent = "pre_compact"
	HookEventPostCompact HookEvent = "post_compact"

	// API hooks
	HookEventPreAPICall  HookEvent = "pre_api_call"
	HookEventPostAPICall HookEvent = "post_api_call"

	// Error hooks
	HookEventOnError HookEvent = "on_error"

	// Notification hooks
	HookEventNotification HookEvent = "notification"

	// User prompt hooks
	HookEventUserPromptSubmit HookEvent = "user_prompt_submit"

	// Subagent hooks
	HookEventSubagentStart HookEvent = "subagent_start"
	HookEventSubagentStop  HookEvent = "subagent_stop"

	// Permission hooks
	HookEventPermissionRequest HookEvent = "permission_request"
	HookEventPermissionDenied  HookEvent = "permission_denied"

	// System hooks
	HookEventSetup        HookEvent = "setup"
	HookEventConfigChange HookEvent = "config_change"

	// Multi-agent hooks
	HookEventTeammateIdle HookEvent = "teammate_idle"

	// Task hooks
	HookEventTaskCreated   HookEvent = "task_created"
	HookEventTaskCompleted HookEvent = "task_completed"

	// Elicitation hooks
	HookEventElicitation       HookEvent = "elicitation"
	HookEventElicitationResult HookEvent = "elicitation_result"

	// Worktree hooks
	HookEventWorktreeCreate HookEvent = "worktree_create"
	HookEventWorktreeRemove HookEvent = "worktree_remove"

	// Context hooks
	HookEventInstructionsLoaded HookEvent = "instructions_loaded"
	HookEventCwdChanged         HookEvent = "cwd_changed"
	HookEventFileChanged        HookEvent = "file_changed"
)

// HookProgress represents progress information for a hook
type HookProgress struct {
	// Event is the hook event type
	Event HookEvent `json:"event"`

	// Message is a human-readable message
	Message string `json:"message,omitempty"`

	// Data contains event-specific data
	Data map[string]any `json:"data,omitempty"`
}

// HookHandler is a function that handles a hook event.
// It may return a HookResult to signal Deny, Modify, or Retry actions.
// Returning (nil, nil) is equivalent to HookActionContinue.
type HookHandler func(ctx context.Context, progress HookProgress) (*HookResult, error)

// HookStage represents when a hook should be called
type HookStage string

const (
	HookStagePre    HookStage = "pre"
	HookStagePost   HookStage = "post"
	HookStageAround HookStage = "around"

	// Permission-specific hook stages
	HookStagePrePermission  HookStage = "pre_permission"
	HookStagePostPermission HookStage = "post_permission"
)

// Hook represents a permission hook interface.
// Aligned with OpenClaude's PermissionHook (permissions.ts:628-639).
type Hook interface {
	// Execute runs the hook and returns the result.
	Execute(ctx context.Context, stage HookStage, toolName string, toolInput map[string]any, metadata map[string]any) HookResult

	// Priority determines hook execution order (higher = earlier).
	Priority() int

	// Name identifies the hook.
	Name() string
}

// HookResult represents the result of a hook execution.
// Aligned with OpenClaude's HookResult (permissions.ts:629-633).
type HookResult struct {
	// Action indicates what action to take.
	Action HookAction `json:"action"`

	// UpdatedInput contains modified input from the hook.
	UpdatedInput map[string]any `json:"updated_input,omitempty"`

	// Message contains a message to display to the user.
	Message string `json:"message,omitempty"`

	// Retry indicates that the tool should be retried.
	Retry bool `json:"retry,omitempty"`

	// Metadata contains additional hook-specific data.
	Metadata map[string]any `json:"metadata,omitempty"`
}

// HookAction represents the action to take after a hook.
// Aligned with OpenClaude's HookAction (permissions.ts:630-633).
type HookAction string

const (
	HookActionContinue HookAction = "continue"
	HookActionStop     HookAction = "stop"
	// HookActionDeny explicitly blocks an operation (tool call, model call).
	// Semantically equivalent to Stop but carries an explicit denial message.
	HookActionDeny   HookAction = "deny"
	HookActionModify HookAction = "modify"
	HookActionRetry  HookAction = "retry"
)

// HookRegistration represents a registered hook
type HookRegistration struct {
	// Event is the event this hook is registered for
	Event HookEvent `json:"event"`

	// Stage is when this hook should be called
	Stage HookStage `json:"stage"`

	// Handler is the function to call
	Handler HookHandler `json:"-"`

	// Priority determines order (higher = earlier)
	Priority int `json:"priority"`

	// ID uniquely identifies this hook registration
	ID string `json:"id"`
}

// PromptRequest represents a request to prompt the user
type PromptRequest struct {
	// Type is the type of prompt
	Type PromptType `json:"type"`

	// Message is the message to show the user
	Message string `json:"message"`

	// Options are the available options (for choice prompts)
	Options []PromptOption `json:"options,omitempty"`

	// Default is the default value
	Default any `json:"default,omitempty"`

	// Metadata contains additional information
	Metadata map[string]any `json:"metadata,omitempty"`
}

// PromptType represents the type of prompt
type PromptType string

const (
	PromptTypeText    PromptType = "text"
	PromptTypeChoice  PromptType = "choice"
	PromptTypeConfirm PromptType = "confirm"
)

// PromptOption represents an option in a choice prompt
type PromptOption struct {
	// Label is the human-readable label
	Label string `json:"label"`

	// Value is the value to return if selected
	Value any `json:"value"`

	// Description is an optional description
	Description string `json:"description,omitempty"`
}

// PromptResponse represents a response from the user
type PromptResponse struct {
	// Value is the user's response
	Value any `json:"value"`

	// Cancelled indicates if the user cancelled
	Cancelled bool `json:"cancelled"`

	// Metadata contains additional information
	Metadata map[string]any `json:"metadata,omitempty"`
}

// PromptFn is a function that can prompt the user
type PromptFn func(ctx context.Context, request PromptRequest) (PromptResponse, error)

// ==============================================================================
// Additional Hook Data Types
// ==============================================================================

// NotificationData contains data for notification events
type NotificationData struct {
	Level   string `json:"level"` // info, warning, error
	Message string `json:"message"`
	Source  string `json:"source,omitempty"`
}

// UserPromptSubmitData contains data for user prompt submission
type UserPromptSubmitData struct {
	Prompt         string `json:"prompt"`
	SessionID      string `json:"session_id"`
	IsFirstMessage bool   `json:"is_first_message"`
}

// StopFailureData contains data for stop failure events
type StopFailureData struct {
	Reason string `json:"reason"`
	Error  error  `json:"error,omitempty"`
}

// SubagentData contains data for subagent events
type SubagentData struct {
	AgentID   string `json:"agent_id"`
	AgentName string `json:"agent_name"`
	ParentID  string `json:"parent_id,omitempty"`
	TaskType  string `json:"task_type,omitempty"`
	IsFork    bool   `json:"is_fork"`
}

// PermissionRequestData contains data for permission requests
type PermissionRequestData struct {
	ToolName       string         `json:"tool_name"`
	Input          map[string]any `json:"input"`
	PermissionMode string         `json:"permission_mode"`
	Context        map[string]any `json:"context,omitempty"`
}

// PermissionDeniedData contains data for permission denial
type PermissionDeniedData struct {
	ToolName     string         `json:"tool_name"`
	Input        map[string]any `json:"input"`
	Reason       string         `json:"reason"`
	RetryAllowed bool           `json:"retry_allowed"`
}

// SetupData contains data for setup events
type SetupData struct {
	SessionID      string `json:"session_id"`
	InitialMessage string `json:"initial_message,omitempty"`
}

// TaskData contains data for task events
type TaskData struct {
	TaskID   string `json:"task_id"`
	TaskType string `json:"task_type"`
	TaskName string `json:"task_name"`
	Status   string `json:"status"`
	Result   any    `json:"result,omitempty"`
	Error    error  `json:"error,omitempty"`
}

// ElicitationData contains data for elicitation events
type ElicitationData struct {
	RequestID string           `json:"request_id"`
	Prompt    string           `json:"prompt"`
	Options   []map[string]any `json:"options,omitempty"`
}

// WorktreeData contains data for worktree events
type WorktreeData struct {
	WorktreePath string `json:"worktree_path"`
	BaseBranch   string `json:"base_branch,omitempty"`
}

// ContextChangeData contains data for context change events
type ContextChangeData struct {
	OldValue string `json:"old_value,omitempty"`
	NewValue string `json:"new_value"`
	Reason   string `json:"reason,omitempty"`
}
