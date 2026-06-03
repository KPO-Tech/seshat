package execution

import (
	"context"
	"fmt"

	tool "github.com/EngineerProjects/nexus-engine/internal/tools/contract"
	"github.com/EngineerProjects/nexus-engine/internal/types"
)

func (o *Orchestrator) prepareToolUses(ctx context.Context, req ExecuteRequest) []preparedToolUse {
	prepared := make([]preparedToolUse, 0, len(req.ToolUses))
	for idx, toolUse := range req.ToolUses {
		prepared = append(prepared, o.prepareToolUse(ctx, toolUse, idx, req))
	}
	return prepared
}

func (o *Orchestrator) prepareToolUse(
	ctx context.Context,
	toolUse types.ToolUseContent,
	index int,
	req ExecuteRequest,
) preparedToolUse {
	prepared := preparedToolUse{
		toolUse: toolUse,
		index:   index,
		trace:   newToolExecutionTrace(toolUse),
	}

	t, ok := req.Tools[toolUse.Name]
	if !ok {
		err := fmt.Errorf("tool not found: %s", toolUse.Name)
		prepared.failure = &preparedToolUseFailure{
			stage:            ErrorStageExecution,
			permissionResult: toolExecutionErrorResult(err),
			err:              err,
		}
		return prepared
	}
	prepared.tool = t

	if !t.IsEnabled() {
		err := fmt.Errorf("tool disabled: %s", toolUse.Name)
		prepared.failure = &preparedToolUseFailure{
			stage:            ErrorStageDisabled,
			permissionResult: toolExecutionErrorResult(err),
			err:              err,
		}
		return prepared
	}

	validatedInput, err := t.ValidateInput(ctx, cloneToolInput(toolUse.Input))
	if err != nil {
		prepared.failure = &preparedToolUseFailure{
			stage:            ErrorStageExecution,
			permissionResult: validationErrorResult(err),
			err:              err,
		}
		return prepared
	}
	prepared.validatedInput = cloneToolInput(validatedInput)
	prepared.trace.ValidatedInput = cloneToolInput(validatedInput)

	backfilledInput := t.BackfillInput(ctx, cloneToolInput(validatedInput))
	prepared.backfilledInput = cloneToolInput(backfilledInput)
	prepared.trace.BackfilledInput = cloneToolInput(backfilledInput)
	prepared.isConcurrencySafe = t.IsConcurrencySafe(cloneToolInput(validatedInput))
	prepared.isReadOnly = t.IsReadOnly(cloneToolInput(validatedInput))

	return prepared
}

func (o *Orchestrator) partitionPreparedToolUses(preparedToolUses []preparedToolUse) []executionBatch {
	batches := make([]executionBatch, 0)
	currentConcurrent := make([]preparedToolUse, 0)

	flushConcurrent := func() {
		if len(currentConcurrent) == 0 {
			return
		}
		batches = append(batches, executionBatch{
			IsConcurrencySafe: true,
			ToolUses:          append([]preparedToolUse(nil), currentConcurrent...),
		})
		currentConcurrent = currentConcurrent[:0]
	}

	for _, prepared := range preparedToolUses {
		if prepared.failure != nil || !prepared.isConcurrencySafe {
			flushConcurrent()
			batches = append(batches, executionBatch{
				IsConcurrencySafe: false,
				ToolUses:          []preparedToolUse{prepared},
			})
			continue
		}

		currentConcurrent = append(currentConcurrent, prepared)
		if len(currentConcurrent) >= o.maxConcurrency {
			flushConcurrent()
		}
	}

	flushConcurrent()
	return batches
}

func (o *Orchestrator) failPreparedToolUse(prepared preparedToolUse, progress []types.ToolProgress, extraMessages []types.Message) toolExecutionOutcome {
	failure := prepared.failure
	if failure == nil {
		err := fmt.Errorf("tool preparation failed")
		failure = &preparedToolUseFailure{
			stage:            ErrorStageExecution,
			permissionResult: toolExecutionErrorResult(err),
			err:              err,
		}
	}
	return o.failedOutcome(
		prepared.toolUse,
		prepared.index,
		progress,
		failure.stage,
		failure.permissionResult,
		prepared.trace,
		failure.err,
		extraMessages,
	)
}

func (o *Orchestrator) newRuntimeStateFromPrepared(prepared preparedToolUse, req ExecuteRequest, toolCtx tool.ToolUseContext) toolRuntimeState {
	resolvedToolCtx := toolCtx
	resolvedToolCtx.ToolUseID = prepared.toolUse.ID
	resolvedToolCtx.CanUseTool = req.PermissionCheck
	if len(req.Transcript) > 0 {
		if resolvedToolCtx.Metadata == nil {
			resolvedToolCtx.Metadata = make(map[string]any)
		}
		resolvedToolCtx.Metadata["transcript_messages"] = append([]types.Message(nil), req.Transcript...)
	}

	callInput := tool.CallInput{
		ToolUseID:   prepared.toolUse.ID,
		SessionID:   req.SessionID,
		TurnID:      req.TurnID,
		ToolContext: &resolvedToolCtx,
	}

	return toolRuntimeState{
		tool:      prepared.tool,
		toolUse:   prepared.toolUse,
		toolCtx:   resolvedToolCtx,
		callInput: callInput,
		trace:     cloneTrace(prepared.trace),
	}
}

func newToolExecutionTrace(toolUse types.ToolUseContent) ToolExecutionTrace {
	return ToolExecutionTrace{
		ToolUseID: toolUse.ID,
		ToolName:  toolUse.Name,
	}
}

func (s toolRuntimeState) withValidatedInput(validatedInput map[string]any) toolRuntimeState {
	s.trace.ValidatedInput = cloneToolInput(validatedInput)
	return s
}

func (s toolRuntimeState) withBackfilledInput(backfilledInput map[string]any) toolRuntimeState {
	s.trace.BackfilledInput = cloneToolInput(backfilledInput)
	return s
}

func (s toolRuntimeState) withPermissionResults(localPermission, globalPermission types.PermissionResult, finalInput map[string]any) toolRuntimeState {
	s.trace.LocalPermission = clonePermissionResult(localPermission)
	s.trace.GlobalPermission = clonePermissionResult(globalPermission)
	s.trace.FinalInput = cloneToolInput(finalInput)
	s.callInput.Raw = fmt.Sprintf("%v", finalInput)
	s.callInput.Parsed = cloneToolInput(finalInput)
	return s
}
