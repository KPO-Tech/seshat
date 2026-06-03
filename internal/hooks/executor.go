package hooks

import (
	"context"
	"fmt"
	"time"

	"github.com/EngineerProjects/nexus-engine/internal/types"
)

// Executor handles hook execution with timeout and error handling.
type Executor struct {
	registry *Registry

	// Default timeout for hook execution (0 = no timeout)
	defaultTimeout time.Duration

	// Enable/disable hook execution
	enabled bool
}

// NewExecutor creates a new hook executor.
func NewExecutor(registry *Registry) *Executor {
	return &Executor{
		registry:       registry,
		defaultTimeout: 30 * time.Second, // Default 30s timeout
		enabled:        true,
	}
}

// SetTimeout sets the default timeout for hook execution.
func (e *Executor) SetTimeout(timeout time.Duration) {
	e.defaultTimeout = timeout
}

// SetEnabled enables or disables hook execution.
func (e *Executor) SetEnabled(enabled bool) {
	e.enabled = enabled
}

// IsEnabled returns whether hook execution is enabled.
func (e *Executor) IsEnabled() bool {
	return e.enabled
}

// Execute executes all hooks registered for a specific event.
// Hooks are executed in priority order (highest first).
// Returns aggregated results from all hooks.
func (e *Executor) Execute(ctx context.Context, event types.HookEvent, data map[string]any) ([]types.HookResult, error) {
	if !e.enabled {
		return nil, nil
	}

	hooks := e.registry.Get(event)
	if len(hooks) == 0 {
		return nil, nil
	}

	results := make([]types.HookResult, 0, len(hooks))

	for _, hook := range hooks {
		// Create hook progress
		progress := types.HookProgress{
			Event:   event,
			Message: fmt.Sprintf("Executing hook: %s", hook.ID),
			Data:    data,
		}

		// Add hook metadata
		if hook.Metadata != nil {
			progress.Data["hook_id"] = hook.ID
			progress.Data["hook_priority"] = hook.Priority
		}

		// Execute hook with timeout
		result, err := e.executeWithTimeout(ctx, hook.Handler, progress, e.defaultTimeout)
		if err != nil {
			// Hook execution failed, but continue with other hooks
			results = append(results, types.HookResult{
				Action:  types.HookActionContinue, // Continue despite error
				Message: fmt.Sprintf("Hook %s failed: %v", hook.ID, err),
				Metadata: map[string]any{
					"hook_id": hook.ID,
					"error":   err.Error(),
				},
			})
			continue
		}

		if result != nil {
			results = append(results, *result)
		}

		// Stop/Deny terminates the hook chain immediately.
		if result != nil && (result.Action == types.HookActionStop || result.Action == types.HookActionDeny) {
			break
		}
	}

	return results, nil
}

// executeWithTimeout executes a hook with a timeout.
func (e *Executor) executeWithTimeout(
	ctx context.Context,
	handler types.HookHandler,
	progress types.HookProgress,
	timeout time.Duration,
) (*types.HookResult, error) {
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	result, err := handler(ctx, progress)
	if err != nil {
		return nil, fmt.Errorf("hook execution failed: %w", err)
	}
	return result, nil
}

// ExecuteAsync executes hooks asynchronously and returns immediately.
// Useful for non-blocking hooks (e.g., logging, analytics).
func (e *Executor) ExecuteAsync(ctx context.Context, event types.HookEvent, data map[string]any) {
	go func() {
		_, _ = e.Execute(ctx, event, data)
	}()
}

// ExecuteFirst executes hooks until one returns a non-continue action.
// Returns the first hook result that is not "continue".
func (e *Executor) ExecuteFirst(ctx context.Context, event types.HookEvent, data map[string]any) *types.HookResult {
	if !e.enabled {
		return nil
	}

	hooks := e.registry.Get(event)
	if len(hooks) == 0 {
		return nil
	}

	for _, hook := range hooks {
		progress := types.HookProgress{
			Event:   event,
			Message: fmt.Sprintf("Executing hook: %s", hook.ID),
			Data:    data,
		}

		if hook.Metadata != nil {
			progress.Data["hook_id"] = hook.ID
			progress.Data["hook_priority"] = hook.Priority
		}

		result, err := e.executeWithTimeout(ctx, hook.Handler, progress, e.defaultTimeout)
		if err != nil {
			// Hook failed, continue with next hook
			continue
		}

		if result != nil && result.Action != types.HookActionContinue && result.Action != "" {
			// Return first actionable result (Deny, Stop, Modify, Retry)
			return result
		}
	}

	// All hooks returned "continue"
	return &types.HookResult{
		Action: types.HookActionContinue,
	}
}
