package execution

import (
	"time"

	tool "github.com/EngineerProjects/nexus-engine/internal/tools/contract"
	"github.com/EngineerProjects/nexus-engine/internal/types"
)

// ExecuteRequest represents a request to execute tools.
type ExecuteRequest struct {
	ToolUses []types.ToolUseContent `json:"tool_uses"`
	Tools    map[string]tool.Tool   `json:"-"`

	PermissionCheck    types.CanUseToolFn         `json:"-"`
	PermissionResolver types.PermissionResolver   `json:"-"`
	PermissionContext  *types.PermissionContext   `json:"-"`
	SessionID          types.SessionID            `json:"session_id"`
	TurnID             types.TurnID               `json:"turn_id"`
	WorkingDirectory   string                     `json:"working_directory,omitempty"`
	PermissionMode     types.PermissionMode       `json:"permission_mode"`
	ProgressCallback   func(types.ToolProgress)   `json:"-"`
	DenialTracking     *types.DenialTrackingState `json:"-"`
	Transcript         []types.Message            `json:"-"`

	// EnableSandbox indicates whether tools should run in sandbox mode.
	// Used for sandbox auto-allow permission logic.
	EnableSandbox bool `json:"enable_sandbox,omitempty"`

	// ShouldAvoidPermissionPrompts is true for headless/background agents
	// that cannot display UI prompts.
	ShouldAvoidPermissionPrompts bool `json:"should_avoid_permission_prompts,omitempty"`
}

// ExecuteResult represents the result of executing tools.
type ExecuteResult struct {
	Results          []tool.CallResult    `json:"results"`
	Messages         []types.Message      `json:"messages"`
	Traces           []ToolExecutionTrace `json:"traces"`
	Errors           []ExecutionError     `json:"errors"`
	TotalDuration    time.Duration        `json:"total_duration_ms"`
	ProgressUpdates  []types.ToolProgress `json:"progress_updates"`
	FinalToolContext tool.ToolUseContext  `json:"-"`
}

// ToolExecutionTrace captures the main execution stages for a single tool use.
type ToolExecutionTrace struct {
	ToolUseID string `json:"tool_use_id"`
	ToolName  string `json:"tool_name"`

	ValidatedInput  map[string]any `json:"validated_input,omitempty"`
	BackfilledInput map[string]any `json:"backfilled_input,omitempty"`
	FinalInput      map[string]any `json:"final_input,omitempty"`

	LocalPermission  types.PermissionResult `json:"local_permission"`
	GlobalPermission types.PermissionResult `json:"global_permission"`

	Metadata map[string]any `json:"metadata,omitempty"`
}

// ExecutionError represents an error during tool execution.
type ExecutionError struct {
	ToolUseID string     `json:"tool_use_id"`
	ToolName  string     `json:"tool_name"`
	Error     error      `json:"error"`
	Stage     ErrorStage `json:"stage"`
}

// ErrorStage represents when an error occurred.
type ErrorStage string

const (
	ErrorStagePermission ErrorStage = "permission"
	ErrorStageExecution  ErrorStage = "execution"
	ErrorStageTimeout    ErrorStage = "timeout"
	ErrorStageDisabled   ErrorStage = "disabled"
	ErrorStageHook       ErrorStage = "hook"
)

type permissionStageResult struct {
	LocalPermission  types.PermissionResult
	GlobalPermission types.PermissionResult
	FinalInput       map[string]any
}

type preparedToolUse struct {
	toolUse           types.ToolUseContent
	index             int
	tool              tool.Tool
	trace             ToolExecutionTrace
	validatedInput    map[string]any
	backfilledInput   map[string]any
	isConcurrencySafe bool
	isReadOnly        bool
	failure           *preparedToolUseFailure
}

type preparedToolUseFailure struct {
	stage            ErrorStage
	permissionResult types.PermissionResult
	err              error
}

type executionBatch struct {
	IsConcurrencySafe bool
	ToolUses          []preparedToolUse
}

type toolExecutionOutcome struct {
	ToolUse    types.ToolUseContent
	Index      int
	Result     tool.CallResult
	Messages   []types.Message
	Error      error
	ErrorStage ErrorStage
	Progress   []types.ToolProgress
	Trace      ToolExecutionTrace
}

// StreamingExecutionResult is the public snapshot returned by StreamingExecutor.
// Messages contains only the extra runtime messages emitted around the tool call;
// the canonical tool_result message is rebuilt by the engine when merging paths.
type StreamingExecutionResult struct {
	ToolUse    types.ToolUseContent
	Index      int
	Result     tool.CallResult
	Messages   []types.Message
	Error      error
	ErrorStage ErrorStage
	Progress   []types.ToolProgress
	Trace      ToolExecutionTrace
}

type toolRuntimeState struct {
	tool      tool.Tool
	toolUse   types.ToolUseContent
	toolCtx   tool.ToolUseContext
	callInput tool.CallInput
	trace     ToolExecutionTrace
}

type stageFailure struct {
	stage            ErrorStage
	permissionResult types.PermissionResult
	err              error
}

func (f *stageFailure) Error() string {
	if f == nil || f.err == nil {
		return ""
	}
	return f.err.Error()
}
