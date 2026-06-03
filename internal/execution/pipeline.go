package execution

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	oteltrace "go.opentelemetry.io/otel/trace"

	runtimehooks "github.com/EngineerProjects/nexus-engine/internal/runtime/hooks"
	tool "github.com/EngineerProjects/nexus-engine/internal/tools/contract"
	"github.com/EngineerProjects/nexus-engine/internal/types"
)

func (o *Orchestrator) executePreparedTool(
	ctx context.Context,
	prepared preparedToolUse,
	req ExecuteRequest,
	toolCtx tool.ToolUseContext,
) toolExecutionOutcome {
	progressUpdates := []types.ToolProgress{progressForStage(prepared.toolUse, "resolving tool", 0)}
	extraMessages := []types.Message{}

	if prepared.failure != nil {
		return o.failPreparedToolUse(prepared, progressUpdates, extraMessages)
	}

	state := o.newRuntimeStateFromPrepared(prepared, req, toolCtx)
	state = state.withValidatedInput(prepared.validatedInput)
	state = state.withBackfilledInput(prepared.backfilledInput)
	state.callInput.Raw = fmt.Sprintf("%v", prepared.validatedInput)
	state.callInput.Parsed = cloneToolInput(prepared.validatedInput)
	state.trace.FinalInput = cloneToolInput(prepared.validatedInput)

	return o.executePreparedToolPipeline(ctx, prepared, req, state, progressUpdates, extraMessages, false)
}

func (o *Orchestrator) executePreparedToolPipeline(
	ctx context.Context,
	prepared preparedToolUse,
	req ExecuteRequest,
	state toolRuntimeState,
	progressUpdates []types.ToolProgress,
	extraMessages []types.Message,
	observableInputModified bool,
) toolExecutionOutcome {
	toolUse := prepared.toolUse
	index := prepared.index
	t := prepared.tool

	// OTel span per tool call. Span name follows the convention "tool <name>".
	spanCtx, span := otel.Tracer("nexus-engine").Start(ctx, "tool "+toolUse.Name,
		oteltrace.WithAttributes(
			attribute.String("tool.name", toolUse.Name),
			attribute.String("tool.use_id", toolUse.ID),
		),
	)
	defer span.End()
	ctx = spanCtx

	progressUpdates = append(progressUpdates, progressForStage(toolUse, "running pre-hooks", 20))
	currentInput := cloneToolInput(prepared.backfilledInput)
	preHookResult := runtimehooks.ExecutePre(ctx, o.hooks.Pre(), runtimehooks.ToolHookInput{
		ToolName:  toolUse.Name,
		ToolUseID: toolUse.ID,
		Input:     cloneToolInput(currentInput),
		ToolCtx:   state.toolCtx,
	})
	if ctx.Err() != nil {
		return o.cancelledOutcome(toolUse, index, progressUpdates, state, extraMessages)
	}
	if preHookResult.Stop != nil {
		return o.hookStopOutcome(toolUse, index, progressUpdates, state, preHookResult.Stop, extraMessages)
	}
	if preHookResult.UpdatedInput != nil && !mapsEqual(currentInput, preHookResult.UpdatedInput) {
		currentInput = preHookResult.UpdatedInput
		observableInputModified = true
	}
	extraMessages = append(extraMessages, preHookResult.ExtraMessages...)

	progressUpdates = append(progressUpdates, progressForStage(toolUse, "running safety checks", 25))
	if o.safetyChecker != nil {
		safetyResult := o.safetyChecker.CheckSafety(toolUse.Name, currentInput)
		if safetyResult.IsDangerous {
			err := fmt.Errorf("safety check blocked: %s", safetyResult.Reason)
			return o.failedOutcome(toolUse, index, progressUpdates, ErrorStagePermission, types.PermissionResult{
				Behavior: types.PermissionBehaviorDeny,
				Reason:   fmt.Sprintf("dangerous pattern detected: %s", safetyResult.Reason),
				DecisionReason: &types.PermissionDecisionReason{
					Type:   types.PermissionDecisionReasonSafetyCheck,
					Source: safetyResult.CheckType,
					Reason: safetyResult.Reason,
				},
			}, state.trace, err, extraMessages)
		}
	}

	progressUpdates = append(progressUpdates, progressForStage(toolUse, "checking permissions", 33))
	permResult, failure := o.resolveToolPermissions(ctx, state, currentInput, req)
	if failure != nil {
		state = state.withPermissionResults(permResult.LocalPermission, permResult.GlobalPermission, permResult.FinalInput)
		return o.failedOutcome(toolUse, index, progressUpdates, failure.stage, failure.permissionResult, state.trace, failure.err, extraMessages)
	}
	state = state.withPermissionResults(permResult.LocalPermission, permResult.GlobalPermission, permResult.FinalInput)
	if !observableInputModified && mapsEqual(prepared.backfilledInput, permResult.FinalInput) {
		state.callInput.Raw = fmt.Sprintf("%v", prepared.validatedInput)
		state.callInput.Parsed = cloneToolInput(prepared.validatedInput)
		state.trace.FinalInput = cloneToolInput(prepared.validatedInput)
	}

	if req.DenialTracking != nil && permResult.GlobalPermission.Behavior == types.PermissionBehaviorAllow {
		req.DenialTracking.RecordSuccess()
	}

	progressUpdates = append(progressUpdates, progressForStage(toolUse, "calling tool", 66))
	callResult := o.callToolSafe(ctx, state, req.PermissionCheck)
	if callResult.IsError() {
		span.SetStatus(codes.Error, callResult.GetContent())
	}

	progressUpdates = append(progressUpdates, progressForStage(toolUse, "running post-hooks", 90))
	extraMessages = append(extraMessages, runtimehooks.ExecutePost(ctx, o.hooks.Post(), runtimehooks.ToolHookInput{
		ToolName:  toolUse.Name,
		ToolUseID: toolUse.ID,
		Input:     cloneToolInput(state.trace.FinalInput),
		ToolCtx:   state.toolCtx,
	})...)

	callResult = o.formatAndTruncateResult(t, callResult)
	if browserProgress := browserProgressForResult(toolUse, callResult); browserProgress != nil {
		progressUpdates = append(progressUpdates, *browserProgress)
	}

	progressUpdates = append(progressUpdates, completeProgress(toolUse, callResult))
	return toolExecutionOutcome{
		ToolUse:  toolUse,
		Index:    index,
		Result:   callResult,
		Messages: extraMessages,
		Progress: progressUpdates,
		Trace:    cloneTrace(state.trace),
	}
}

func (o *Orchestrator) callToolSafe(
	ctx context.Context,
	state toolRuntimeState,
	permissionCheck types.CanUseToolFn,
) (callRes tool.CallResult) {
	defer func() {
		if r := recover(); r != nil {
			callRes = tool.NewErrorResult(fmt.Errorf("tool panic: %v", r))
		}
	}()
	result, err := state.tool.Call(ctx, state.callInput, permissionCheck)
	if err != nil {
		return tool.NewErrorResult(err)
	}
	return result
}

func (o *Orchestrator) formatAndTruncateResult(t tool.Tool, result tool.CallResult) tool.CallResult {
	if result.Content == "" && result.Data != nil {
		result.Content = t.FormatResult(result.Data)
	}

	maxSize := t.Definition().MaxResultSize
	if maxSize > 0 && len(result.Content) > maxSize {
		original := result.Content
		result.Content = original[:maxSize] + "\n\n... [truncated: original " +
			fmt.Sprintf("%d", len(original)) + " chars exceeded limit of " +
			fmt.Sprintf("%d", maxSize) + "]"
		result.Metadata = &tool.ResultMetadata{
			ContentReplacement: &types.ContentReplacementState{
				OriginalSize:    int64(len(original)),
				ReplacedSize:    int64(len(result.Content)),
				ReplacementType: types.ContentReplacementTypeTruncated,
				Preview:         original[:min(maxSize, 200)],
			},
		}
	}

	return result
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
