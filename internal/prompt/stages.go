package prompt

import "strings"

// ExecutionStage is the current execution stage of the engine.
// An overlay section is injected into the dynamic part of the system prompt
// when the stage is not StageDefault.
type ExecutionStage string

const (
	// StageDefault is the baseline mode — no overlay injected.
	StageDefault ExecutionStage = ""

	// StageToolCall is set when the model is expected to dispatch tool calls.
	StageToolCall ExecutionStage = "tool_call"

	// StageToolResult is set when tool results are being sent back to the model.
	StageToolResult ExecutionStage = "tool_result"

	// StageContinuation is set when the loop injects a continuation nudge.
	StageContinuation ExecutionStage = "continuation"

	// StagePlan is set when the engine is in plan mode (tools shown, not executed).
	StagePlan ExecutionStage = "plan"
)

const stageToolCallOverlay = `# Stage: tool dispatch

Proceed with the identified tool calls.
- Use only the tools strictly necessary for this step.
- Do not repeat tool calls already completed this turn.
- Prefer the simplest valid path.`

const stageToolResultOverlay = `# Stage: integrating tool results

Tool execution is complete. Integrate the results above into a direct, useful response.
- If results fully answer the request, provide a complete answer now.
- If another tool is needed, proceed without restating what was already done.
- Do not apologize, recap, or describe the execution process.`

const stageContinuationOverlay = `# Stage: continuation

The previous response did not reach a terminal state. Continue directly.
- Resume from where the last response stopped.
- Do not apologize, recap, or restate prior progress.
- Use tools if needed; otherwise provide a complete answer now.`

const stagePlanOverlay = `# Stage: plan mode

Tool execution is suspended. Describe the plan of action.
- List the tools you would use and the order of operations.
- Present the plan as a numbered list of concrete, verifiable steps.
- Do not execute tools or make changes.`

var stageOverlayDefaults = map[ExecutionStage]string{
	StageToolCall:     stageToolCallOverlay,
	StageToolResult:   stageToolResultOverlay,
	StageContinuation: stageContinuationOverlay,
	StagePlan:         stagePlanOverlay,
}

// stageSection returns a Section for the given execution stage.
// Returns nil for StageDefault (no overlay needed).
// stageOverrides takes precedence over the built-in templates.
func stageSection(stage ExecutionStage, stageOverrides map[ExecutionStage]string) *Section {
	if stage == StageDefault {
		return nil
	}
	content := ""
	if stageOverrides != nil {
		if override, ok := stageOverrides[stage]; ok {
			content = strings.TrimSpace(override)
		}
	}
	if content == "" {
		if template, ok := stageOverlayDefaults[stage]; ok {
			content = strings.TrimSpace(template)
		}
	}
	if content == "" {
		return nil
	}
	return &Section{
		Type:      SectionTypeDynamic,
		Name:      "stage_overlay",
		Content:   content,
		Priority:  770, // after runtime_memory (760)
		Cacheable: false,
		Enabled:   true,
	}
}
