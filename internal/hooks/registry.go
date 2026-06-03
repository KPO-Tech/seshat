package hooks

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/EngineerProjects/nexus-engine/internal/types"
)

// Registry is a unified hook registry for all lifecycle hooks.
// It supports registration, retrieval, and execution of hooks by event type.
type Registry struct {
	mu    sync.RWMutex
	hooks map[types.HookEvent][]HookRegistration
}

// NewRegistry creates a new hook registry.
func NewRegistry() *Registry {
	return &Registry{
		hooks: make(map[types.HookEvent][]HookRegistration),
	}
}

// HookState represents the current state of a hook
type HookState string

const (
	// HookStateActive means the hook is active and will be executed
	HookStateActive HookState = "active"
	// HookStatePaused means the hook is temporarily paused
	HookStatePaused HookState = "paused"
	// HookStateInactive means the hook is inactive and won't be executed
	HookStateInactive HookState = "inactive"
	// HookStateDead means the hook has encountered an error and is dead
	HookStateDead HookState = "dead"
)

// HookRegistration represents a registered hook with lifecycle support.
type HookRegistration struct {
	// ID uniquely identifies this hook registration
	ID string

	// Event is the event this hook is registered for
	Event types.HookEvent

	// Handler is the function to call when the hook is triggered
	Handler HookHandler

	// Priority determines execution order (higher = earlier)
	Priority int

	// Optional metadata for debugging and tracking
	Metadata map[string]any

	// Lifecycle management fields
	State        HookState     // Current state of the hook
	CreatedAt    time.Time     // When the hook was registered
	LastExecuted time.Time     // When the hook was last executed
	ExecCount    int           // Number of times the hook has been executed
	ErrorCount   int           // Number of times the hook has errored
	LastError    error         // Last error encountered by the hook
	IsPermanent  bool          // If true, hook cannot be auto-cleaned
	MaxLifetime  time.Duration // Maximum lifetime before auto-cleanup (0 = infinite)
}

// HookHandler is an alias for types.HookHandler.
// Returning (nil, nil) is equivalent to HookActionContinue.
type HookHandler = types.HookHandler

// Add registers a new hook for the specified event.
func (r *Registry) Add(hook HookRegistration) {
	if r == nil {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.hooks == nil {
		r.hooks = make(map[types.HookEvent][]HookRegistration)
	}

	// Initialize lifecycle fields if not set
	if hook.State == "" {
		hook.State = HookStateActive
	}
	if hook.CreatedAt.IsZero() {
		hook.CreatedAt = time.Now()
	}

	r.hooks[hook.Event] = append(r.hooks[hook.Event], hook)

	// Sort by priority (higher = earlier)
	sort.SliceStable(r.hooks[hook.Event], func(i, j int) bool {
		return r.hooks[hook.Event][i].Priority > r.hooks[hook.Event][j].Priority
	})
}

// Remove removes a hook registration by ID.
func (r *Registry) Remove(id string) {
	if r == nil {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	for event, hooks := range r.hooks {
		filtered := make([]HookRegistration, 0, len(hooks))
		for _, hook := range hooks {
			if hook.ID != id {
				filtered = append(filtered, hook)
			}
		}
		r.hooks[event] = filtered
	}
}

// Get retrieves all hooks registered for a specific event.
func (r *Registry) Get(event types.HookEvent) []HookRegistration {
	if r == nil {
		return nil
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	if hooks, ok := r.hooks[event]; ok {
		// Return a copy to prevent external modification
		result := make([]HookRegistration, len(hooks))
		copy(result, hooks)
		return result
	}

	return nil
}

// HasHooks checks if there are any hooks registered for a specific event.
func (r *Registry) HasHooks(event types.HookEvent) bool {
	if r == nil {
		return false
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	hooks, ok := r.hooks[event]
	return ok && len(hooks) > 0
}

// Clear removes all hooks for a specific event, or all events if event is empty.
func (r *Registry) Clear(event types.HookEvent) {
	if r == nil {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if event == "" {
		// Clear all hooks
		r.hooks = make(map[types.HookEvent][]HookRegistration)
	} else {
		// Clear hooks for specific event
		r.hooks[event] = nil
	}
}

// Count returns the number of hooks registered for a specific event.
func (r *Registry) Count(event types.HookEvent) int {
	if r == nil {
		return 0
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	if hooks, ok := r.hooks[event]; ok {
		return len(hooks)
	}

	return 0
}

// List returns all registered hooks across all events.
func (r *Registry) List() map[types.HookEvent][]HookRegistration {
	if r == nil {
		return nil
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(map[types.HookEvent][]HookRegistration, len(r.hooks))
	for event, hooks := range r.hooks {
		result[event] = make([]HookRegistration, len(hooks))
		copy(result[event], hooks)
	}

	return result
}

// ---------------------------------------------------------------------------
// Lifecycle Management Methods
// ---------------------------------------------------------------------------

// SetHookState changes the state of a hook by ID
func (r *Registry) SetHookState(id string, state HookState) error {
	if r == nil {
		return fmt.Errorf("registry is nil")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	for event, hooks := range r.hooks {
		for i, hook := range hooks {
			if hook.ID == id {
				r.hooks[event][i].State = state
				return nil
			}
		}
	}

	return fmt.Errorf("hook not found: %s", id)
}

// PauseHook temporarily pauses a hook by ID
func (r *Registry) PauseHook(id string) error {
	return r.SetHookState(id, HookStatePaused)
}

// ResumeHook resumes a paused hook by ID
func (r *Registry) ResumeHook(id string) error {
	return r.SetHookState(id, HookStateActive)
}

// GetHook retrieves a specific hook by ID
func (r *Registry) GetHook(id string) (*HookRegistration, error) {
	if r == nil {
		return nil, fmt.Errorf("registry is nil")
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, hooks := range r.hooks {
		for _, hook := range hooks {
			if hook.ID == id {
				// Return a copy to prevent external modification
				hookCopy := hook
				return &hookCopy, nil
			}
		}
	}

	return nil, fmt.Errorf("hook not found: %s", id)
}

// GetActiveHooks returns only active hooks for a specific event
func (r *Registry) GetActiveHooks(event types.HookEvent) []HookRegistration {
	hooks := r.Get(event)
	if hooks == nil {
		return nil
	}

	activeHooks := make([]HookRegistration, 0, len(hooks))
	for _, hook := range hooks {
		if hook.State == HookStateActive {
			activeHooks = append(activeHooks, hook)
		}
	}

	return activeHooks
}

// RecordHookExecution records that a hook was executed
func (r *Registry) RecordHookExecution(id string, err error) error {
	if r == nil {
		return fmt.Errorf("registry is nil")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	for event, hooks := range r.hooks {
		for i, hook := range hooks {
			if hook.ID == id {
				r.hooks[event][i].LastExecuted = time.Now()
				r.hooks[event][i].ExecCount++

				if err != nil {
					r.hooks[event][i].ErrorCount++
					r.hooks[event][i].LastError = err
				} else {
					// Reset error count on successful execution
					r.hooks[event][i].ErrorCount = 0
					r.hooks[event][i].LastError = nil
				}
				return nil
			}
		}
	}

	return fmt.Errorf("hook not found: %s", id)
}

// CleanupExpiredHooks removes hooks that have exceeded their lifetime
func (r *Registry) CleanupExpiredHooks() int {
	if r == nil {
		return 0
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	cleanedCount := 0

	for event, hooks := range r.hooks {
		filtered := make([]HookRegistration, 0, len(hooks))
		for _, hook := range hooks {
			// Skip permanent hooks
			if hook.IsPermanent {
				filtered = append(filtered, hook)
				continue
			}

			// Check if hook has exceeded its lifetime
			if hook.MaxLifetime > 0 && now.Sub(hook.CreatedAt) > hook.MaxLifetime {
				// Clean up this hook
				cleanedCount++
				continue
			}

			// Keep hooks that haven't exceeded lifetime or are dead/inactive but not too old
			filtered = append(filtered, hook)
		}
		r.hooks[event] = filtered
	}

	return cleanedCount
}

// CleanupDeadHooks removes hooks that are in dead state
func (r *Registry) CleanupDeadHooks() int {
	if r == nil {
		return 0
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	cleanedCount := 0

	for event, hooks := range r.hooks {
		filtered := make([]HookRegistration, 0, len(hooks))
		for _, hook := range hooks {
			if hook.State == HookStateDead && !hook.IsPermanent {
				cleanedCount++
				continue
			}
			filtered = append(filtered, hook)
		}
		r.hooks[event] = filtered
	}

	return cleanedCount
}

// GetHookStats returns statistics for all hooks
func (r *Registry) GetHookStats() map[string]interface{} {
	if r == nil {
		return nil
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	stats := make(map[string]interface{})
	totalHooks := 0
	activeHooks := 0
	pausedHooks := 0
	inactiveHooks := 0
	deadHooks := 0
	totalExecutions := 0
	totalErrors := 0

	for _, hooks := range r.hooks {
		totalHooks += len(hooks)
		for _, hook := range hooks {
			switch hook.State {
			case HookStateActive:
				activeHooks++
			case HookStatePaused:
				pausedHooks++
			case HookStateInactive:
				inactiveHooks++
			case HookStateDead:
				deadHooks++
			}
			totalExecutions += hook.ExecCount
			totalErrors += hook.ErrorCount
		}
	}

	stats["totalHooks"] = totalHooks
	stats["activeHooks"] = activeHooks
	stats["pausedHooks"] = pausedHooks
	stats["inactiveHooks"] = inactiveHooks
	stats["deadHooks"] = deadHooks
	stats["totalExecutions"] = totalExecutions
	stats["totalErrors"] = totalErrors
	stats["totalEvents"] = len(r.hooks)

	return stats
}

// GetHooksByState returns all hooks in a specific state
func (r *Registry) GetHooksByState(state HookState) []HookRegistration {
	if r == nil {
		return nil
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []HookRegistration
	for _, hooks := range r.hooks {
		for _, hook := range hooks {
			if hook.State == state {
				result = append(result, hook)
			}
		}
	}

	return result
}
