package permissions

import (
	"github.com/EngineerProjects/nexus-engine/internal/types"
)

// CreateDenialTrackingState creates a new denial tracking state.
// Aligned with OpenClaude's createDenialTrackingState (denialTracking.ts:17-22).
func CreateDenialTrackingState() *types.DenialTrackingState {
	return &types.DenialTrackingState{
		// Use the existing DenialTrackingState struct from types
	}
}

// RecordDenial records a classifier denial and returns the updated state.
// Uses the existing RecordDenial method from the DenialTrackingState struct.
func RecordDenial(state *types.DenialTrackingState) *types.DenialTrackingState {
	if state == nil {
		state = CreateDenialTrackingState()
	}
	state.RecordDenial()
	return state
}

// RecordSuccess records a successful classification and resets consecutive denials.
// Uses the existing RecordSuccess method from the DenialTrackingState struct.
func RecordSuccess(state *types.DenialTrackingState) *types.DenialTrackingState {
	if state == nil {
		return CreateDenialTrackingState()
	}
	if state.GetConsecutiveDenials() == 0 {
		return state // No change needed
	}
	state.RecordSuccess()
	return state
}

// ShouldFallbackToPrompting checks if denial limits were exceeded.
// Uses the existing DenialLimitConfig and ShouldFallback method.
func ShouldFallbackToPrompting(state *types.DenialTrackingState, config *types.DenialLimitConfig) bool {
	if state == nil {
		return false
	}
	if config == nil {
		defaultConfig := types.DefaultDenialLimitConfig()
		config = &defaultConfig
	}
	return config.ShouldFallback(state)
}

// HandleDenialLimitExceeded checks if denial limits were exceeded and returns
// an appropriate permission result. Returns nil if no limit was hit.
// Aligned with OpenClaude's handleDenialLimitExceeded (permissions.ts:984-1049).
func HandleDenialLimitExceeded(
	state *types.DenialTrackingState,
	config *types.DenialLimitConfig,
	isHeadless bool,
	classifierReason string,
) *types.PermissionResult {
	if !ShouldFallbackToPrompting(state, config) {
		return nil
	}

	totalCount := state.GetTotalDenials()

	var warning string
	if config.MaxTotalDenials > 0 && totalCount >= config.MaxTotalDenials {
		warning = "Multiple actions were blocked this session. Please review the transcript before continuing."
	} else {
		warning = "Multiple consecutive actions were blocked. Please review the transcript before continuing."
	}

	if isHeadless {
		// In headless mode, we can't prompt the user, so we must abort
		return &types.PermissionResult{
			Behavior: types.PermissionBehaviorDeny,
			Reason:   "Agent aborted: too many classifier denials in headless mode",
			DecisionReason: &types.PermissionDecisionReason{
				Type:   types.PermissionDecisionReasonOther,
				Source: "denialTracking",
				Reason: warning,
			},
		}
	}

	// Return ask result to prompt user
	result := &types.PermissionResult{
		Behavior: types.PermissionBehaviorAsk,
		Reason:   warning,
		DecisionReason: &types.PermissionDecisionReason{
			Type:   types.PermissionDecisionReasonMode,
			Source: "denialTracking",
			Reason: warning,
		},
	}

	// Reset tracking if total limit was hit
	if config.MaxTotalDenials > 0 && totalCount >= config.MaxTotalDenials {
		state.RecordSuccess() // Reset consecutive denials
		state.RecordDenial()  // Re-record this denial
		result.DenialTracking = state
	}

	return result
}
