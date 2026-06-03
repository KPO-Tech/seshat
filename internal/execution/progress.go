package execution

import (
	"fmt"

	tool "github.com/EngineerProjects/nexus-engine/internal/tools/contract"
	"github.com/EngineerProjects/nexus-engine/internal/types"
)

func progressForStage(toolUse types.ToolUseContent, stage string, percent float64) types.ToolProgress {
	return types.ToolProgress{
		ToolName:        toolUse.Name,
		ToolUseID:       toolUse.ID,
		Stage:           types.ToolProgressStageRunning,
		Message:         fmt.Sprintf("%s: %s", toolUse.Name, stage),
		PercentComplete: percent,
		Metadata:        progressMetadata(toolUse),
	}
}

func completeProgress(toolUse types.ToolUseContent, result tool.CallResult) types.ToolProgress {
	progress := types.ToolProgress{
		ToolName:        toolUse.Name,
		ToolUseID:       toolUse.ID,
		Stage:           types.ToolProgressStageCompleted,
		Message:         fmt.Sprintf("Tool %s completed", toolUse.Name),
		PercentComplete: 100,
		Metadata:        progressMetadata(toolUse),
	}
	if metadata := toolResultMetadata(toolUse, result); len(metadata) > 0 {
		for key, value := range metadata {
			progress.Metadata[key] = value
		}
	}
	return progress
}

func failedProgress(toolUse types.ToolUseContent, err error) types.ToolProgress {
	return types.ToolProgress{
		ToolName:        toolUse.Name,
		ToolUseID:       toolUse.ID,
		Stage:           types.ToolProgressStageFailed,
		Message:         fmt.Sprintf("Tool %s failed: %v", toolUse.Name, err),
		PercentComplete: 100,
		Metadata:        progressMetadata(toolUse),
	}
}

func progressMetadata(toolUse types.ToolUseContent) map[string]any {
	metadata := map[string]any{
		"tool_name": toolUse.Name,
	}
	if len(toolUse.Input) > 0 {
		metadata["tool_input"] = cloneToolInput(toolUse.Input)
	}
	return metadata
}

func toolResultMetadata(toolUse types.ToolUseContent, result tool.CallResult) map[string]any {
	metadata := progressMetadata(toolUse)

	if content := result.GetContent(); content != "" {
		metadata["content"] = content
	}

	if result.Metadata != nil {
		if result.Metadata.ExecutionDuration > 0 {
			metadata["execution_duration_ms"] = result.Metadata.ExecutionDuration
		}
		if result.Metadata.ContentReplacement != nil {
			metadata["content_replacement"] = result.Metadata.ContentReplacement
		}
		for key, value := range result.Metadata.Additional {
			metadata[key] = value
		}
	}

	return metadata
}

func (o *Orchestrator) failedOutcome(
	toolUse types.ToolUseContent,
	index int,
	progress []types.ToolProgress,
	stage ErrorStage,
	permissionResult types.PermissionResult,
	trace ToolExecutionTrace,
	err error,
	extraMessages []types.Message,
) toolExecutionOutcome {
	progress = append(progress, failedProgress(toolUse, err))
	return toolExecutionOutcome{
		ToolUse:    toolUse,
		Index:      index,
		Result:     tool.NewErrorResult(err),
		Messages:   extraMessages,
		Error:      err,
		ErrorStage: stage,
		Progress:   progress,
		Trace:      cloneTrace(trace),
	}
}

func (o *Orchestrator) cancelledOutcome(
	toolUse types.ToolUseContent,
	index int,
	progress []types.ToolProgress,
	state toolRuntimeState,
	extraMessages []types.Message,
) toolExecutionOutcome {
	err := fmt.Errorf("cancelled")
	progress = append(progress, failedProgress(toolUse, err))
	return toolExecutionOutcome{
		ToolUse:    toolUse,
		Index:      index,
		Result:     tool.NewErrorResult(err),
		Messages:   extraMessages,
		Error:      err,
		ErrorStage: ErrorStageExecution,
		Progress:   progress,
		Trace:      cloneTrace(state.trace),
	}
}

func (o *Orchestrator) hookStopOutcome(
	toolUse types.ToolUseContent,
	index int,
	progress []types.ToolProgress,
	state toolRuntimeState,
	stop *ToolHookStop,
	extraMessages []types.Message,
) toolExecutionOutcome {
	err := fmt.Errorf("stopped by hook")
	progress = append(progress, failedProgress(toolUse, err))
	result := tool.NewErrorResult(err)
	if stop.Content != "" {
		result.Content = stop.Content
	}
	return toolExecutionOutcome{
		ToolUse:    toolUse,
		Index:      index,
		Result:     result,
		Messages:   extraMessages,
		Error:      err,
		ErrorStage: ErrorStageHook,
		Progress:   progress,
		Trace:      cloneTrace(state.trace),
	}
}
