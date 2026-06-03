package execution

import (
	"fmt"

	"github.com/EngineerProjects/nexus-engine/internal/types"
)

func clonePermissionResult(result types.PermissionResult) types.PermissionResult {
	cloned := result
	if result.UpdatedInput != nil {
		cloned.UpdatedInput = cloneToolInput(result.UpdatedInput)
	}
	if result.Metadata != nil {
		cloned.Metadata = cloneMetadata(result.Metadata)
	}
	if result.DecisionReason != nil {
		decisionReason := *result.DecisionReason
		cloned.DecisionReason = &decisionReason
	}
	return cloned
}

func cloneTrace(trace ToolExecutionTrace) ToolExecutionTrace {
	copied := trace
	copied.ValidatedInput = cloneToolInput(trace.ValidatedInput)
	copied.BackfilledInput = cloneToolInput(trace.BackfilledInput)
	copied.FinalInput = cloneToolInput(trace.FinalInput)
	copied.LocalPermission = clonePermissionResult(trace.LocalPermission)
	copied.GlobalPermission = clonePermissionResult(trace.GlobalPermission)
	return copied
}

func cloneMetadata(metadata map[string]any) map[string]any {
	if metadata == nil {
		return nil
	}
	cloned := make(map[string]any, len(metadata))
	for k, v := range metadata {
		cloned[k] = v
	}
	return cloned
}

func cloneToolInput(input map[string]any) map[string]any {
	if input == nil {
		return map[string]any{}
	}
	cloned := make(map[string]any, len(input))
	for k, v := range input {
		cloned[k] = v
	}
	return cloned
}

func applyUpdatedInput(base map[string]any, permissionResult types.PermissionResult) map[string]any {
	if permissionResult.UpdatedInput != nil {
		return cloneToolInput(permissionResult.UpdatedInput)
	}
	return cloneToolInput(base)
}

func mapsEqual(a, b map[string]any) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		other, ok := b[k]
		if !ok || fmt.Sprintf("%v", other) != fmt.Sprintf("%v", v) {
			return false
		}
	}
	return true
}
