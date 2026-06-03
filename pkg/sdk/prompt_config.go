package sdk

import "github.com/EngineerProjects/nexus-engine/internal/prompt"

// ExecutionStage is the re-exported execution stage type.
// Use the Stage* constants to specify the desired behavior overlay.
type ExecutionStage = prompt.ExecutionStage

// Stage constants for use in PromptConfig.
const (
	// StageDefault is the baseline mode — no overlay injected.
	StageDefault ExecutionStage = prompt.StageDefault

	// StageToolCall tells the model it is expected to dispatch tool calls.
	StageToolCall ExecutionStage = prompt.StageToolCall

	// StageToolResult tells the model to integrate the received tool results.
	StageToolResult ExecutionStage = prompt.StageToolResult

	// StageContinuation tells the model to resume from where it stopped.
	StageContinuation ExecutionStage = prompt.StageContinuation

	// StagePlan puts the engine in plan mode: tools are shown but not executed.
	StagePlan ExecutionStage = prompt.StagePlan
)

// PromptConfig provides structured customization of the system prompt.
//
// It is a first-class SDK concept: apps configure it once at the client or
// session level, and the engine applies it every turn without modifying
// the core prompt sections.
//
// Structure of the assembled system prompt:
//
//	[CorePromptSections]        — identity, runtime_contract, working_rules, tool_use, output_discipline
//	[dynamic boundary]
//	[RuntimeContext]            — session_id, turn_number, model, available_tools, …
//	[RuntimeGuidance]
//	[MemoryContext]             — if memory is enabled
//	[StageOverlay]              — injected from Stage / StageOverrides when Stage != StageDefault
//	[AppendSystemPrompt]        — appended after all other sections
//
// Tool definitions sent to the provider are built from the tool registry.
// ToolHints are appended to the provider-facing description of individual tools
// without requiring changes to the tool definition itself.
type PromptConfig struct {
	// SystemPrompt replaces the entire default system prompt.
	// When set, CorePromptSections and StageOverlay are not used.
	SystemPrompt *string `json:"system_prompt,omitempty"`

	// AppendSystemPrompt is appended after all other sections every turn.
	AppendSystemPrompt *string `json:"append_system_prompt,omitempty"`

	// Stage sets the execution stage context for this session.
	// The corresponding overlay is injected into the dynamic section each turn.
	//
	// Use StageDefault (the zero value) for normal operation.
	// Use StagePlan to suspend tool execution and request a plan description.
	// Use StageToolResult / StageContinuation when wiring the loop manually.
	Stage ExecutionStage `json:"stage,omitempty"`

	// StageOverrides replaces the built-in overlay text for specific stages.
	// Key: stage constant (e.g. StagePlan), Value: replacement overlay text.
	StageOverrides map[ExecutionStage]string `json:"stage_overrides,omitempty"`

	// ToolHints provides per-tool guidance appended to provider-facing descriptions.
	// This is an app-level layer: it adds guidance without modifying tool definitions.
	// Key: canonical tool name (e.g. "bash", "read"), Value: hint text.
	ToolHints map[string]string `json:"tool_hints,omitempty"`
}
