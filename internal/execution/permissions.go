package execution

import (
	"context"
	"fmt"

	tool "github.com/EngineerProjects/nexus-engine/internal/tools/contract"
	"github.com/EngineerProjects/nexus-engine/internal/types"
)

func (o *Orchestrator) resolveToolPermissions(
	ctx context.Context,
	state toolRuntimeState,
	processedInput map[string]any,
	req ExecuteRequest,
) (permissionStageResult, *stageFailure) {
	currentInput := cloneToolInput(processedInput)

	denyProbe := o.runWholeToolPermissionProbe(ctx, state, req, types.ToolPermissionIntentDeny)
	if denyProbe.IsDenied() {
		denyProbe = o.applyDenialTrackingFallback(req, state.toolCtx.PermissionMode, denyProbe)
		return permissionStageResult{
				LocalPermission:  denyProbe,
				GlobalPermission: denyProbe,
				FinalInput:       cloneToolInput(currentInput),
			}, &stageFailure{
				stage:            ErrorStagePermission,
				permissionResult: denyProbe,
				err:              permissionErrorFromResult(denyProbe, "tool denied by rule"),
			}
	}

	askProbe := o.runWholeToolPermissionProbe(ctx, state, req, types.ToolPermissionIntentAsk)
	if askProbe.IsDenied() || askProbe.IsAsk() {
		askProbe = o.applyDenialTrackingFallback(req, state.toolCtx.PermissionMode, askProbe)
		return permissionStageResult{
				LocalPermission:  askProbe,
				GlobalPermission: askProbe,
				FinalInput:       cloneToolInput(currentInput),
			}, &stageFailure{
				stage:            ErrorStagePermission,
				permissionResult: askProbe,
				err:              permissionErrorFromResult(askProbe, "tool requires approval"),
			}
	}

	localPermission := ensurePermissionResult(
		state.tool.CheckPermissions(ctx, cloneToolInput(currentInput), state.toolCtx),
		"local",
		cloneToolInput(currentInput),
	)

	if localPermission.Behavior == types.PermissionBehaviorDeny {
		localPermission = o.applyDenialTrackingFallback(req, state.toolCtx.PermissionMode, localPermission)
		return permissionStageResult{
				LocalPermission:  localPermission,
				GlobalPermission: zeroPermissionResult(),
				FinalInput:       cloneToolInput(currentInput),
			}, &stageFailure{
				stage:            ErrorStagePermission,
				permissionResult: localPermission,
				err:              permissionErrorFromResult(localPermission, "tool requires approval"),
			}
	}

	if tool.RequiresUserInteraction(state.tool) && localPermission.Behavior == types.PermissionBehaviorAsk {
		localPermission = o.applyDenialTrackingFallback(req, state.toolCtx.PermissionMode, localPermission)
		return permissionStageResult{
				LocalPermission:  localPermission,
				GlobalPermission: zeroPermissionResult(),
				FinalInput:       cloneToolInput(currentInput),
			}, &stageFailure{
				stage:            ErrorStagePermission,
				permissionResult: localPermission,
				err:              permissionErrorFromResult(localPermission, "tool requires user interaction"),
			}
	}

	if localPermission.Behavior == types.PermissionBehaviorAsk &&
		localPermission.DecisionReason != nil &&
		localPermission.DecisionReason.Type == types.PermissionDecisionReasonRule &&
		localPermission.DecisionReason.RuleBehavior == types.PermissionBehaviorAsk {
		localPermission = o.applyDenialTrackingFallback(req, state.toolCtx.PermissionMode, localPermission)
		return permissionStageResult{
				LocalPermission:  localPermission,
				GlobalPermission: zeroPermissionResult(),
				FinalInput:       cloneToolInput(currentInput),
			}, &stageFailure{
				stage:            ErrorStagePermission,
				permissionResult: localPermission,
				err:              permissionErrorFromResult(localPermission, "content-specific ask rule"),
			}
	}

	if localPermission.Behavior == types.PermissionBehaviorAsk &&
		localPermission.DecisionReason != nil &&
		localPermission.DecisionReason.Type == types.PermissionDecisionReasonSafetyCheck &&
		!localPermission.DecisionReason.ClassifierApprovable {
		localPermission = o.applyDenialTrackingFallback(req, state.toolCtx.PermissionMode, localPermission)
		return permissionStageResult{
				LocalPermission:  localPermission,
				GlobalPermission: zeroPermissionResult(),
				FinalInput:       cloneToolInput(currentInput),
			}, &stageFailure{
				stage:            ErrorStagePermission,
				permissionResult: localPermission,
				err:              permissionErrorFromResult(localPermission, "safety check requires approval"),
			}
	}

	switch localPermission.Behavior {
	case types.PermissionBehaviorAllow, types.PermissionBehaviorPassthrough:
		currentInput = applyUpdatedInput(currentInput, localPermission)
	default:
		localPermission = ensurePermissionResult(types.AllowWithUpdatedInput(currentInput), "local", currentInput)
	}

	if o.shouldBypassPermissions(state.toolCtx.PermissionMode, state.toolCtx.IsBypassPermissionsModeAvailable) {
		bypassPermission := types.AllowWithInputAndDecisionReason("bypass mode", cloneToolInput(currentInput), &types.PermissionDecisionReason{
			Type:   types.PermissionDecisionReasonMode,
			Source: "mode",
			Reason: string(state.toolCtx.PermissionMode),
		})
		return permissionStageResult{
			LocalPermission:  localPermission,
			GlobalPermission: bypassPermission,
			FinalInput:       cloneToolInput(currentInput),
		}, nil
	}

	allowProbe := o.runWholeToolPermissionProbe(ctx, state, req, types.ToolPermissionIntentAllow)
	if allowProbe.IsAllowed() {
		currentInput = applyUpdatedInput(currentInput, allowProbe)
		return permissionStageResult{
			LocalPermission:  localPermission,
			GlobalPermission: allowProbe,
			FinalInput:       cloneToolInput(currentInput),
		}, nil
	}

	globalPermission := o.runGlobalPermissionCheck(ctx, state, req, currentInput)
	switch globalPermission.Behavior {
	case types.PermissionBehaviorDeny, types.PermissionBehaviorAsk:
		globalPermission = o.applyDenialTrackingFallback(req, state.toolCtx.PermissionMode, globalPermission)
		return permissionStageResult{
				LocalPermission:  localPermission,
				GlobalPermission: globalPermission,
				FinalInput:       cloneToolInput(currentInput),
			}, &stageFailure{
				stage:            ErrorStagePermission,
				permissionResult: globalPermission,
				err:              permissionErrorFromResult(globalPermission, "tool requires approval"),
			}
	case types.PermissionBehaviorAllow:
		currentInput = applyUpdatedInput(currentInput, globalPermission)
	}

	return permissionStageResult{
		LocalPermission:  localPermission,
		GlobalPermission: globalPermission,
		FinalInput:       currentInput,
	}, nil
}

func (o *Orchestrator) runWholeToolPermissionProbe(
	ctx context.Context,
	state toolRuntimeState,
	req ExecuteRequest,
	intent types.ToolPermissionIntent,
) types.PermissionResult {
	resolver := req.PermissionResolver
	if resolver == nil {
		if req.PermissionCheck == nil {
			return zeroPermissionResult()
		}
		resolver = req.PermissionCheck
	}

	metadata := cloneMetadata(state.toolCtx.Metadata)
	if metadata == nil {
		metadata = make(map[string]any)
	}
	metadata["working_directory"] = state.toolCtx.WorkingDirectory
	if req.DenialTracking != nil {
		metadata["denialTracking"] = req.DenialTracking
	}
	if len(req.Transcript) > 0 {
		metadata["transcript_messages"] = append([]types.Message(nil), req.Transcript...)
	}

	result := clonePermissionResult(resolver.ResolvePermission(ctx, types.WholeToolPermissionRequest(
		state.toolUse.Name,
		state.toolUse.ID,
		req.SessionID,
		req.TurnID,
		state.toolCtx.PermissionMode,
		state.toolCtx.WorkingDirectory,
		intent,
		metadata,
	)))

	if result.Behavior == "" {
		result.Behavior = types.PermissionBehaviorPassthrough
	}

	return result
}

func (o *Orchestrator) runGlobalPermissionCheck(
	ctx context.Context,
	state toolRuntimeState,
	req ExecuteRequest,
	currentInput map[string]any,
) types.PermissionResult {
	resolver := req.PermissionResolver
	if resolver == nil {
		if req.PermissionCheck == nil {
			return ensurePermissionResult(defaultGlobalPermissionResult(currentInput), "global", currentInput)
		}
		resolver = req.PermissionCheck
	}

	matcher, _ := tool.BuildPermissionMatcher(ctx, state.tool, cloneToolInput(currentInput))
	metadata := tool.AttachPermissionMatcherMetadata(cloneMetadata(state.toolCtx.Metadata), state.tool, matcher)
	if metadata == nil {
		metadata = make(map[string]any)
	}
	metadata["working_directory"] = state.toolCtx.WorkingDirectory
	if req.DenialTracking != nil {
		metadata["denialTracking"] = req.DenialTracking
	}
	if len(req.Transcript) > 0 {
		metadata["transcript_messages"] = append([]types.Message(nil), req.Transcript...)
	}
	if req.ShouldAvoidPermissionPrompts {
		metadata["should_avoid_permission_prompts"] = true
	}

	// Pass sandbox mode info through metadata for canSandboxAutoAllow
	// Check EnableSandbox from the request (set by the caller based on tool config)
	metadata["is_tool_running_in_sandbox"] = req.EnableSandbox

	result := clonePermissionResult(resolver.ResolvePermission(ctx, types.GlobalToolPermissionRequest(
		state.toolUse.Name,
		cloneToolInput(currentInput),
		state.toolUse.ID,
		req.SessionID,
		req.TurnID,
		state.toolCtx.PermissionMode,
		state.toolCtx.WorkingDirectory,
		metadata,
	)))
	if result.Behavior == "" {
		return ensurePermissionResult(defaultGlobalPermissionResult(currentInput), "global", currentInput)
	}
	return ensurePermissionResult(result, "global", currentInput)
}

func (o *Orchestrator) applyDenialTrackingFallback(req ExecuteRequest, currentMode types.PermissionMode, result types.PermissionResult) types.PermissionResult {
	adjusted := clonePermissionResult(result)
	if req.DenialTracking == nil {
		return adjusted
	}

	req.DenialTracking.RecordDenial()
	if currentMode != types.PermissionModeAuto || !o.denialLimitConfig.ShouldFallback(req.DenialTracking) {
		return adjusted
	}

	if adjusted.Metadata == nil {
		adjusted.Metadata = make(map[string]any)
	}
	adjusted.Metadata["fallback_to_prompt"] = true

	reason := firstNonEmpty(adjusted.Reason, "denial limit reached")
	adjusted.Reason = fmt.Sprintf("denial limit reached; require explicit confirmation: %s", reason)
	if adjusted.Behavior == types.PermissionBehaviorDeny {
		adjusted.Behavior = types.PermissionBehaviorAsk
	}
	if adjusted.DecisionReason == nil {
		adjusted.DecisionReason = &types.PermissionDecisionReason{
			Type:   types.PermissionDecisionReasonOther,
			Source: "denial_limit",
			Reason: adjusted.Reason,
		}
	}

	return adjusted
}

func (o *Orchestrator) shouldBypassPermissions(mode types.PermissionMode, isBypassAvailable bool) bool {
	// Only PermissionModeBypass causes permission bypass
	// Plan mode is handled at execution level via ExecutionMode, not here
	return mode == types.PermissionModeBypass
}

func ensurePermissionResult(result types.PermissionResult, source string, input map[string]any) types.PermissionResult {
	if result.Behavior == "" {
		result.Behavior = types.PermissionBehaviorAllow
	}
	if (result.Behavior == types.PermissionBehaviorAllow || result.Behavior == types.PermissionBehaviorPassthrough) && result.UpdatedInput == nil {
		result.UpdatedInput = cloneToolInput(input)
	}
	if result.DecisionReason == nil {
		reasonType := types.PermissionDecisionReasonOther
		if source == "local" {
			reasonType = types.PermissionDecisionReasonTool
		}
		result.DecisionReason = &types.PermissionDecisionReason{
			Type:   reasonType,
			Source: source,
			Reason: result.Reason,
		}
	}
	return clonePermissionResult(result)
}

func validationErrorResult(err error) types.PermissionResult {
	return types.PermissionResult{
		Behavior: types.PermissionBehaviorDeny,
		Reason:   err.Error(),
		DecisionReason: &types.PermissionDecisionReason{
			Type:   types.PermissionDecisionReasonOther,
			Source: "validation",
			Reason: err.Error(),
		},
	}
}

func toolExecutionErrorResult(err error) types.PermissionResult {
	return types.PermissionResult{
		Behavior: types.PermissionBehaviorDeny,
		Reason:   err.Error(),
		DecisionReason: &types.PermissionDecisionReason{
			Type:   types.PermissionDecisionReasonTool,
			Source: "call",
			Reason: err.Error(),
		},
	}
}

func defaultGlobalPermissionResult(input map[string]any) types.PermissionResult {
	return types.PermissionResult{
		Behavior:     types.PermissionBehaviorAllow,
		UpdatedInput: cloneToolInput(input),
		DecisionReason: &types.PermissionDecisionReason{
			Type:   types.PermissionDecisionReasonOther,
			Source: "global",
			Reason: "no global permission check",
		},
	}
}

func zeroPermissionResult() types.PermissionResult {
	return types.PermissionResult{}
}

func permissionErrorFromResult(result types.PermissionResult, fallback string) error {
	reason := result.Reason
	if reason == "" {
		reason = fallback
	}
	if result.Behavior == types.PermissionBehaviorAsk {
		return fmt.Errorf("user confirmation required: %s", reason)
	}
	return fmt.Errorf("permission denied: %s", reason)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
