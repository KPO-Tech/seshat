package permissions

import (
	"context"
	"fmt"

	"github.com/EngineerProjects/nexus-engine/internal/types"
)

// HookAdapter adapts the internal Hook type to implement the types.Hook interface.
// This allows both internal and external hooks to coexist.
type HookAdapter struct {
	hook Hook
}

// NewHookAdapter creates a new hook adapter.
func NewHookAdapter(hook Hook) *HookAdapter {
	return &HookAdapter{hook: hook}
}

// Execute implements types.Hook.
func (a *HookAdapter) Execute(ctx context.Context, stage types.HookStage, toolName string, toolInput map[string]any, metadata map[string]any) types.HookResult {
	// Convert to PermissionContext for internal hook handler
	pctx := &PermissionContext{
		ToolName:   toolName,
		ToolInput:  toolInput,
		Additional: metadata,
		Stage:      types.ToolPermissionStageWholeTool, // Default stage for hooks
	}

	// Call internal hook handler
	result, err := a.hook.Handler(ctx, pctx)
	if err != nil {
		// Convert error to HookResult with stop action
		return types.HookResult{
			Action:  types.HookActionStop,
			Message: fmt.Sprintf("Hook error: %v", err),
		}
	}

	// Convert PermissionResult to HookResult
	var action types.HookAction
	switch result.Behavior {
	case types.PermissionBehaviorAllow, types.PermissionBehaviorPassthrough:
		action = types.HookActionContinue
	case types.PermissionBehaviorDeny:
		action = types.HookActionStop
	case types.PermissionBehaviorAsk:
		action = types.HookActionModify // Ask means modify/prompt
	default:
		action = types.HookActionContinue
	}

	return types.HookResult{
		Action:       action,
		UpdatedInput: result.UpdatedInput,
		Message:      result.Reason,
		Metadata:     result.Metadata,
	}
}

// Priority implements types.Hook.
func (a *HookAdapter) Priority() int {
	return a.hook.Priority
}

// Name implements types.Hook.
func (a *HookAdapter) Name() string {
	return a.hook.ID
}

// AddInterfaceHook adds a types.Hook interface to the engine.
// This allows external hooks to be registered with the engine.
func (e *Engine) AddInterfaceHook(hook types.Hook) {
	// Wrap interface hook in internal Hook structure
	internalHook := Hook{
		Stage:    types.HookStagePrePermission, // Default to pre-permission
		Priority: hook.Priority(),
		ID:       hook.Name(),
		Handler: func(ctx context.Context, pctx *PermissionContext) (types.PermissionResult, error) {
			// Extract metadata from pctx
			metadata := pctx.Additional
			if metadata == nil {
				metadata = make(map[string]any)
			}

			// Convert ToolPermissionStage to HookStage
			var hookStage types.HookStage
			switch pctx.Stage {
			case types.ToolPermissionStageWholeTool, types.ToolPermissionStageGlobal:
				hookStage = types.HookStagePrePermission
			default:
				hookStage = types.HookStagePre
			}

			// Call interface hook
			result := hook.Execute(ctx, hookStage, pctx.ToolName, pctx.ToolInput, metadata)

			// Convert HookResult back to PermissionResult
			var behavior types.PermissionBehavior
			switch result.Action {
			case types.HookActionContinue:
				behavior = types.PermissionBehaviorAllow
			case types.HookActionStop:
				behavior = types.PermissionBehaviorDeny
			case types.HookActionModify:
				behavior = types.PermissionBehaviorAsk
			case types.HookActionRetry:
				// Retry is not directly supported in PermissionResult
				// We treat it as ask with a retry flag
				behavior = types.PermissionBehaviorAsk
			default:
				behavior = types.PermissionBehaviorPassthrough
			}

			permResult := types.PermissionResult{
				Behavior:     behavior,
				Reason:       result.Message,
				UpdatedInput: result.UpdatedInput,
				Metadata:     result.Metadata,
				DecisionReason: &types.PermissionDecisionReason{
					Type:   types.PermissionDecisionReasonOther,
					Source: hook.Name(),
					Reason: result.Message,
				},
			}

			// Handle retry action by setting metadata
			if result.Retry {
				if permResult.Metadata == nil {
					permResult.Metadata = make(map[string]any)
				}
				permResult.Metadata["retry"] = true
			}

			return permResult, nil
		},
	}

	e.AddHook(internalHook)
}

// RunPostPermissionHooks runs hooks after a permission decision has been made.
// This is useful for logging, analytics, or modifying the result based on policy.
func (e *Engine) RunPostPermissionHooks(ctx context.Context, pctx *PermissionContext, result *types.PermissionResult) error {
	for _, hook := range e.hooks {
		if hook.Stage != types.HookStagePostPermission {
			continue
		}

		// For post-permission hooks, we pass the current result in metadata
		if pctx.Additional == nil {
			pctx.Additional = make(map[string]any)
		}
		pctx.Additional["permission_result"] = *result

		hookResult, err := hook.Handler(ctx, pctx)
		if err != nil {
			// Log but don't fail the permission check due to hook error
			// Aligned with OpenClaude's approach of graceful failure
			continue
		}

		// Post-permission hooks can only modify metadata, not the decision
		if hookResult.Metadata != nil {
			if result.Metadata == nil {
				result.Metadata = make(map[string]any)
			}
			for k, v := range hookResult.Metadata {
				result.Metadata[k] = v
			}
		}
	}
	return nil
}

// RemoveHook removes a hook by name.
func (e *Engine) RemoveHook(name string) {
	var newHooks []Hook
	for _, hook := range e.hooks {
		if hook.ID != name {
			newHooks = append(newHooks, hook)
		}
	}
	e.hooks = newHooks
	e.sortHooks()
}

// GetHooks returns all hooks for debugging/inspection.
func (e *Engine) GetHooks() []Hook {
	return e.hooks
}
