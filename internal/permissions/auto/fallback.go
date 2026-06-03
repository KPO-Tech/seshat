package auto

import (
	"fmt"

	"github.com/EngineerProjects/nexus-engine/internal/types"
)

// Fallback handles the transition from auto-approval to prompting when
// denial limits are exceeded. This prevents infinite loops of classifier
// denials and ensures the user gets involved when the classifier is
// repeatedly blocking actions.
// Aligned with OpenClaude's fallback behavior.
type Fallback struct {
	config *DenialLimitConfig
}

// NewFallback creates a new Fallback handler with the given config.
// If config is nil, DefaultDenialLimitConfig is used.
func NewFallback(config *DenialLimitConfig) *Fallback {
	if config == nil {
		defaultConfig := DefaultDenialLimitConfig()
		config = &defaultConfig
	}
	return &Fallback{
		config: config,
	}
}

// Apply applies the fallback behavior to a permission result.
// It checks if denial limits have been exceeded and converts a denial
// to an ask (or deny in headless mode).
// Parameters:
//   - state: The current denial tracking state
//   - result: The original permission result
//   - isHeadless: Whether running in headless mode (no UI available)
//
// Returns the adjusted result with fallback behavior applied.
func (f *Fallback) Apply(state *types.DenialTrackingState, result types.PermissionResult, isHeadless bool) types.PermissionResult {
	if state == nil {
		return result
	}

	state.RecordDenial()
	if !f.config.ShouldFallback(state) {
		return result
	}

	adjusted := cloneResult(result)
	adjusted.Behavior = types.PermissionBehaviorAsk
	adjusted.Reason = fmt.Sprintf("denial limit reached; require explicit confirmation: %s", result.Reason)
	adjusted.DenialTracking = state

	// In headless mode, we can't ask the user, so we must deny
	if isHeadless {
		adjusted.Behavior = types.PermissionBehaviorDeny
		adjusted.Reason = "Agent aborted: too many classifier denials in headless mode"
	}

	return adjusted
}

// RecordSuccess records a successful tool use, resetting consecutive denials.
// Called when a tool use is allowed to prevent false positives in the
// denial tracking.
func (f *Fallback) RecordSuccess(state *types.DenialTrackingState) {
	if state != nil {
		state.RecordSuccess()
	}
}

// cloneResult creates a shallow copy of a PermissionResult to avoid
// mutating the original.
func cloneResult(r types.PermissionResult) types.PermissionResult {
	clone := types.PermissionResult{
		Behavior:       r.Behavior,
		Reason:         r.Reason,
		UpdatedInput:   r.UpdatedInput,
		Confidence:     r.Confidence,
		Metadata:       r.Metadata,
		DecisionReason: r.DecisionReason,
	}
	if r.Metadata != nil {
		clone.Metadata = make(map[string]any)
		for k, v := range r.Metadata {
			clone.Metadata[k] = v
		}
	}
	return clone
}

// HandleDenialLimitExceeded checks if denial limits have been exceeded
// and returns an appropriate permission result.
// Aligned with OpenClaude's handleDenialLimitExceeded.
//
// Parameters:
//   - state: The current denial tracking state
//   - config: The denial limit configuration
//   - isHeadless: Whether running in headless mode
//   - classifierReason: The reason from the classifier denial
//
// Returns a PermissionResult with fallback behavior, or nil if limits not exceeded.
func HandleDenialLimitExceeded(
	state *types.DenialTrackingState,
	config *DenialLimitConfig,
	isHeadless bool,
	classifierReason string,
) *types.PermissionResult {
	if state == nil {
		return nil
	}
	if config == nil {
		defaultConfig := DefaultDenialLimitConfig()
		config = &defaultConfig
	}
	if !config.ShouldFallback(state) {
		return nil
	}

	totalCount := state.GetTotalDenials()

	// Generate appropriate warning message based on which limit was hit
	var warning string
	if config.MaxTotalDenials > 0 && totalCount >= config.MaxTotalDenials {
		warning = "Multiple actions were blocked this session. Please review the transcript before continuing."
	} else {
		warning = "Multiple consecutive actions were blocked. Please review the transcript before continuing."
	}

	// In headless mode, we cannot prompt the user, so we must abort
	if isHeadless {
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

	// In interactive mode, convert to ask to prompt the user
	result := &types.PermissionResult{
		Behavior: types.PermissionBehaviorAsk,
		Reason:   warning,
		DecisionReason: &types.PermissionDecisionReason{
			Type:   types.PermissionDecisionReasonMode,
			Source: "denialTracking",
			Reason: warning,
		},
	}

	// If total limit was hit, reset consecutive denials and re-record this denial
	// This allows the user to approve once and then continue, but tracks the total
	if config.MaxTotalDenials > 0 && totalCount >= config.MaxTotalDenials {
		state.RecordSuccess()
		state.RecordDenial()
		result.DenialTracking = state
	}

	return result
}
