package hooks

import (
	"context"

	"github.com/EngineerProjects/nexus-engine/internal/types"
)

type PreExecutionResult struct {
	Stop          *ToolHookStop
	UpdatedInput  map[string]any
	Metadata      map[string]any
	ExtraMessages []types.Message
}

func ExecutePre(ctx context.Context, hooks []ToolHook, input ToolHookInput) PreExecutionResult {
	currentInput := cloneToolInput(input.Input)
	extraMessages := make([]types.Message, 0)
	combinedMetadata := make(map[string]any)

	for _, hook := range hooks {
		if ctx.Err() != nil {
			break
		}
		result := hook.Execute(ctx, ToolHookInput{
			ToolName:  input.ToolName,
			ToolUseID: input.ToolUseID,
			Input:     cloneToolInput(currentInput),
			ToolCtx:   input.ToolCtx,
		})

		for k, v := range result.Metadata {
			combinedMetadata[k] = v
		}

		if result.Stop != nil {
			return PreExecutionResult{
				Stop:          result.Stop,
				UpdatedInput:  currentInput,
				Metadata:      combinedMetadata,
				ExtraMessages: extraMessages,
			}
		}
		if result.UpdatedInput != nil {
			currentInput = cloneToolInput(result.UpdatedInput)
		}
		extraMessages = append(extraMessages, result.ExtraMessages...)
	}

	return PreExecutionResult{
		UpdatedInput:  currentInput,
		Metadata:      combinedMetadata,
		ExtraMessages: extraMessages,
	}
}

func ExecutePost(ctx context.Context, hooks []ToolHook, input ToolHookInput) []types.Message {
	extraMessages := make([]types.Message, 0)

	for _, hook := range hooks {
		if ctx.Err() != nil {
			break
		}
		result := hook.Execute(ctx, ToolHookInput{
			ToolName:  input.ToolName,
			ToolUseID: input.ToolUseID,
			Input:     cloneToolInput(input.Input),
			ToolCtx:   input.ToolCtx,
		})
		extraMessages = append(extraMessages, result.ExtraMessages...)
	}

	return extraMessages
}

func cloneToolInput(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	cloned := make(map[string]any, len(input))
	for k, v := range input {
		cloned[k] = v
	}
	return cloned
}
