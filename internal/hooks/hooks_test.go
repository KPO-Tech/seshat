package hooks

import (
	"context"
	"errors"
	"github.com/EngineerProjects/nexus-engine/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sync"
	"testing"
	"time"
)

// --- from lifecycle_test.go ---

// TestHookState_Constants tests hook state constants
func TestHookState_Constants(t *testing.T) {
	assert.Equal(t, HookState("active"), HookStateActive)
	assert.Equal(t, HookState("paused"), HookStatePaused)
	assert.Equal(t, HookState("inactive"), HookStateInactive)
	assert.Equal(t, HookState("dead"), HookStateDead)
}

// TestRegistry_Lifecycle_Add tests adding hooks with lifecycle fields
func TestRegistry_Lifecycle_Add(t *testing.T) {
	registry := NewRegistry()

	// Add hook with default state
	hook1 := HookRegistration{
		ID:       "hook1",
		Event:    types.HookEventPreToolUse,
		Handler:  func(ctx context.Context, progress types.HookProgress) (*types.HookResult, error) { return nil, nil },
		Priority: 10,
	}

	registry.Add(hook1)

	// Check that default fields were initialized
	retrieved, err := registry.GetHook("hook1")
	require.NoError(t, err)
	assert.Equal(t, HookStateActive, retrieved.State)
	assert.False(t, retrieved.CreatedAt.IsZero())
	assert.Equal(t, 0, retrieved.ExecCount)
	assert.Equal(t, 0, retrieved.ErrorCount)

	// Add hook with explicit state
	hook2 := HookRegistration{
		ID:          "hook2",
		Event:       types.HookEventPreToolUse,
		Handler:     func(ctx context.Context, progress types.HookProgress) (*types.HookResult, error) { return nil, nil },
		Priority:    5,
		State:       HookStatePaused,
		IsPermanent: true,
		MaxLifetime: time.Hour,
	}

	registry.Add(hook2)

	retrieved2, err := registry.GetHook("hook2")
	require.NoError(t, err)
	assert.Equal(t, HookStatePaused, retrieved2.State)
	assert.True(t, retrieved2.IsPermanent)
	assert.Equal(t, time.Hour, retrieved2.MaxLifetime)
}

// TestRegistry_SetHookState tests changing hook states
func TestRegistry_SetHookState(t *testing.T) {
	registry := NewRegistry()

	hook := HookRegistration{
		ID:       "hook1",
		Event:    types.HookEventPreToolUse,
		Handler:  func(ctx context.Context, progress types.HookProgress) (*types.HookResult, error) { return nil, nil },
		Priority: 10,
	}

	registry.Add(hook)

	// Pause hook
	err := registry.PauseHook("hook1")
	require.NoError(t, err)

	retrieved, err := registry.GetHook("hook1")
	require.NoError(t, err)
	assert.Equal(t, HookStatePaused, retrieved.State)

	// Resume hook
	err = registry.ResumeHook("hook1")
	require.NoError(t, err)

	retrieved, err = registry.GetHook("hook1")
	require.NoError(t, err)
	assert.Equal(t, HookStateActive, retrieved.State)

	// Set to inactive
	err = registry.SetHookState("hook1", HookStateInactive)
	require.NoError(t, err)

	retrieved, err = registry.GetHook("hook1")
	require.NoError(t, err)
	assert.Equal(t, HookStateInactive, retrieved.State)
}

// TestRegistry_RecordHookExecution tests recording hook execution
func TestRegistry_RecordHookExecution(t *testing.T) {
	registry := NewRegistry()

	hook := HookRegistration{
		ID:       "hook1",
		Event:    types.HookEventPreToolUse,
		Handler:  func(ctx context.Context, progress types.HookProgress) (*types.HookResult, error) { return nil, nil },
		Priority: 10,
	}

	registry.Add(hook)

	// Record successful execution
	err := registry.RecordHookExecution("hook1", nil)
	require.NoError(t, err)

	retrieved, err := registry.GetHook("hook1")
	require.NoError(t, err)
	assert.Equal(t, 1, retrieved.ExecCount)
	assert.Equal(t, 0, retrieved.ErrorCount)
	assert.Nil(t, retrieved.LastError)
	assert.False(t, retrieved.LastExecuted.IsZero())

	// Record failed execution
	testErr := errors.New("test error")
	err = registry.RecordHookExecution("hook1", testErr)
	require.NoError(t, err)

	retrieved, err = registry.GetHook("hook1")
	require.NoError(t, err)
	assert.Equal(t, 2, retrieved.ExecCount)
	assert.Equal(t, 1, retrieved.ErrorCount)
	assert.Equal(t, testErr, retrieved.LastError)

	// Record another successful execution - error count should reset
	err = registry.RecordHookExecution("hook1", nil)
	require.NoError(t, err)

	retrieved, err = registry.GetHook("hook1")
	require.NoError(t, err)
	assert.Equal(t, 3, retrieved.ExecCount)
	assert.Equal(t, 0, retrieved.ErrorCount) // Error count reset
	assert.Nil(t, retrieved.LastError)
}

// TestRegistry_CleanupExpiredHooks tests cleanup of expired hooks
func TestRegistry_CleanupExpiredHooks(t *testing.T) {
	registry := NewRegistry()

	// Add hook with short lifetime
	expiredHook := HookRegistration{
		ID:          "expired",
		Event:       types.HookEventPreToolUse,
		Handler:     func(ctx context.Context, progress types.HookProgress) (*types.HookResult, error) { return nil, nil },
		Priority:    10,
		MaxLifetime: 10 * time.Millisecond,          // Very short lifetime
		CreatedAt:   time.Now().Add(-1 * time.Hour), // Created long ago
	}

	registry.Add(expiredHook)

	// Add permanent hook
	permanentHook := HookRegistration{
		ID:          "permanent",
		Event:       types.HookEventPreToolUse,
		Handler:     func(ctx context.Context, progress types.HookProgress) (*types.HookResult, error) { return nil, nil },
		Priority:    5,
		IsPermanent: true,
		MaxLifetime: 10 * time.Millisecond, // Even though it's expired, it's permanent
		CreatedAt:   time.Now().Add(-1 * time.Hour),
	}

	registry.Add(permanentHook)

	// Add hook with no lifetime
	normalHook := HookRegistration{
		ID:       "normal",
		Event:    types.HookEventPreToolUse,
		Handler:  func(ctx context.Context, progress types.HookProgress) (*types.HookResult, error) { return nil, nil },
		Priority: 5,
	}

	registry.Add(normalHook)

	// Perform cleanup
	cleanedCount := registry.CleanupExpiredHooks()
	assert.GreaterOrEqual(t, cleanedCount, 1) // Should clean up at least the expired hook

	// Check that expired hook is gone
	_, err := registry.GetHook("expired")
	assert.Error(t, err)

	// Check that permanent hook still exists
	_, err = registry.GetHook("permanent")
	assert.NoError(t, err)

	// Check that normal hook still exists
	_, err = registry.GetHook("normal")
	assert.NoError(t, err)
}

// TestRegistry_CleanupDeadHooks tests cleanup of dead hooks
func TestRegistry_CleanupDeadHooks(t *testing.T) {
	registry := NewRegistry()

	// Add dead hook
	deadHook := HookRegistration{
		ID:       "dead",
		Event:    types.HookEventPreToolUse,
		Handler:  func(ctx context.Context, progress types.HookProgress) (*types.HookResult, error) { return nil, nil },
		Priority: 10,
		State:    HookStateDead,
	}

	registry.Add(deadHook)

	// Add permanent dead hook
	permanentDeadHook := HookRegistration{
		ID:          "permanent_dead",
		Event:       types.HookEventPreToolUse,
		Handler:     func(ctx context.Context, progress types.HookProgress) (*types.HookResult, error) { return nil, nil },
		Priority:    5,
		State:       HookStateDead,
		IsPermanent: true,
	}

	registry.Add(permanentDeadHook)

	// Add active hook
	activeHook := HookRegistration{
		ID:       "active",
		Event:    types.HookEventPreToolUse,
		Handler:  func(ctx context.Context, progress types.HookProgress) (*types.HookResult, error) { return nil, nil },
		Priority: 5,
		State:    HookStateActive,
	}

	registry.Add(activeHook)

	// Perform cleanup
	cleanedCount := registry.CleanupDeadHooks()
	assert.Equal(t, 1, cleanedCount) // Should clean up only the non-permanent dead hook

	// Check that dead hook is gone
	_, err := registry.GetHook("dead")
	assert.Error(t, err)

	// Check that permanent dead hook still exists
	_, err = registry.GetHook("permanent_dead")
	assert.NoError(t, err)

	// Check that active hook still exists
	_, err = registry.GetHook("active")
	assert.NoError(t, err)
}

// TestRegistry_GetHookStats tests getting hook statistics
func TestRegistry_GetHookStats(t *testing.T) {
	registry := NewRegistry()

	// Add hooks in different states
	registry.Add(HookRegistration{
		ID:      "active1",
		Event:   types.HookEventPreToolUse,
		Handler: func(ctx context.Context, progress types.HookProgress) (*types.HookResult, error) { return nil, nil },
		State:   HookStateActive,
	})

	registry.Add(HookRegistration{
		ID:      "active2",
		Event:   types.HookEventSessionStart,
		Handler: func(ctx context.Context, progress types.HookProgress) (*types.HookResult, error) { return nil, nil },
		State:   HookStateActive,
	})

	registry.Add(HookRegistration{
		ID:      "paused",
		Event:   types.HookEventPreToolUse,
		Handler: func(ctx context.Context, progress types.HookProgress) (*types.HookResult, error) { return nil, nil },
		State:   HookStatePaused,
	})

	registry.Add(HookRegistration{
		ID:      "inactive",
		Event:   types.HookEventPreToolUse,
		Handler: func(ctx context.Context, progress types.HookProgress) (*types.HookResult, error) { return nil, nil },
		State:   HookStateInactive,
	})

	registry.Add(HookRegistration{
		ID:      "dead",
		Event:   types.HookEventPreToolUse,
		Handler: func(ctx context.Context, progress types.HookProgress) (*types.HookResult, error) { return nil, nil },
		State:   HookStateDead,
	})

	// Record some executions
	registry.RecordHookExecution("active1", nil)
	registry.RecordHookExecution("active1", nil)
	registry.RecordHookExecution("active2", errors.New("error"))

	// Get stats
	stats := registry.GetHookStats()

	assert.Equal(t, 5, stats["totalHooks"])
	assert.Equal(t, 2, stats["activeHooks"])
	assert.Equal(t, 1, stats["pausedHooks"])
	assert.Equal(t, 1, stats["inactiveHooks"])
	assert.Equal(t, 1, stats["deadHooks"])
	assert.Equal(t, 3, stats["totalExecutions"])
	assert.Equal(t, 1, stats["totalErrors"])
	assert.Equal(t, 2, stats["totalEvents"])
}

// TestRegistry_GetHooksByState tests getting hooks by state
func TestRegistry_GetHooksByState(t *testing.T) {
	registry := NewRegistry()

	// Add hooks in different states
	registry.Add(HookRegistration{
		ID:      "active1",
		Event:   types.HookEventPreToolUse,
		Handler: func(ctx context.Context, progress types.HookProgress) (*types.HookResult, error) { return nil, nil },
		State:   HookStateActive,
	})

	registry.Add(HookRegistration{
		ID:      "active2",
		Event:   types.HookEventSessionStart,
		Handler: func(ctx context.Context, progress types.HookProgress) (*types.HookResult, error) { return nil, nil },
		State:   HookStateActive,
	})

	registry.Add(HookRegistration{
		ID:      "paused",
		Event:   types.HookEventPreToolUse,
		Handler: func(ctx context.Context, progress types.HookProgress) (*types.HookResult, error) { return nil, nil },
		State:   HookStatePaused,
	})

	// Get active hooks
	activeHooks := registry.GetHooksByState(HookStateActive)
	assert.Len(t, activeHooks, 2)

	activeIDs := []string{activeHooks[0].ID, activeHooks[1].ID}
	assert.Contains(t, activeIDs, "active1")
	assert.Contains(t, activeIDs, "active2")

	// Get paused hooks
	pausedHooks := registry.GetHooksByState(HookStatePaused)
	assert.Len(t, pausedHooks, 1)
	assert.Equal(t, "paused", pausedHooks[0].ID)

	// Get hooks with no matches
	deadHooks := registry.GetHooksByState(HookStateDead)
	assert.Len(t, deadHooks, 0)
}

// TestRegistry_GetActiveHooks tests getting only active hooks for an event
func TestRegistry_GetActiveHooks(t *testing.T) {
	registry := NewRegistry()

	// Add hooks for the same event with different states
	registry.Add(HookRegistration{
		ID:       "active1",
		Event:    types.HookEventPreToolUse,
		Handler:  func(ctx context.Context, progress types.HookProgress) (*types.HookResult, error) { return nil, nil },
		State:    HookStateActive,
		Priority: 10,
	})

	registry.Add(HookRegistration{
		ID:       "active2",
		Event:    types.HookEventPreToolUse,
		Handler:  func(ctx context.Context, progress types.HookProgress) (*types.HookResult, error) { return nil, nil },
		State:    HookStateActive,
		Priority: 5,
	})

	registry.Add(HookRegistration{
		ID:       "paused",
		Event:    types.HookEventPreToolUse,
		Handler:  func(ctx context.Context, progress types.HookProgress) (*types.HookResult, error) { return nil, nil },
		State:    HookStatePaused,
		Priority: 15,
	})

	registry.Add(HookRegistration{
		ID:       "inactive",
		Event:    types.HookEventPreToolUse,
		Handler:  func(ctx context.Context, progress types.HookProgress) (*types.HookResult, error) { return nil, nil },
		State:    HookStateInactive,
		Priority: 20,
	})

	// Get active hooks for the event
	activeHooks := registry.GetActiveHooks(types.HookEventPreToolUse)
	assert.Len(t, activeHooks, 2)

	// Check that priority is maintained (higher priority first)
	assert.Equal(t, "active1", activeHooks[0].ID)
	assert.Equal(t, "active2", activeHooks[1].ID)
}

// TestHookLifecycleManager_RegisterHook tests registering hooks with lifecycle management
func TestHookLifecycleManager_RegisterHook(t *testing.T) {
	registry := NewRegistry()
	manager := NewHookLifecycleManager(registry)

	// Register hook
	hook := HookRegistration{
		ID:       "hook1",
		Event:    types.HookEventPreToolUse,
		Handler:  func(ctx context.Context, progress types.HookProgress) (*types.HookResult, error) { return nil, nil },
		Priority: 10,
	}

	err := manager.RegisterHook(hook)
	require.NoError(t, err)

	// Check hook was registered
	retrieved, err := registry.GetHook("hook1")
	require.NoError(t, err)
	assert.NotNil(t, retrieved)
	assert.Equal(t, HookStateActive, retrieved.State)

	// Check that lifecycle event was published
	select {
	case event := <-manager.GetLifecycleEvents():
		assert.Equal(t, "hook_registered", event.EventType)
		assert.Equal(t, "hook1", event.HookID)
	default:
		assert.Fail(t, "Expected lifecycle event")
	}

	manager.Shutdown()
}

// TestHookLifecycleManager_ExecuteHook tests executing hooks with lifecycle management
func TestHookLifecycleManager_ExecuteHook(t *testing.T) {
	registry := NewRegistry()
	manager := NewHookLifecycleManager(registry)

	executionCount := 0
	var mu sync.Mutex

	// Register hook
	hook := HookRegistration{
		ID:    "hook1",
		Event: types.HookEventPreToolUse,
		Handler: func(ctx context.Context, progress types.HookProgress) (*types.HookResult, error) {
			mu.Lock()
			defer mu.Unlock()
			executionCount++
			return nil, nil
		},
		Priority: 10,
	}

	err := manager.RegisterHook(hook)
	require.NoError(t, err)

	// Execute hook
	progress := types.HookProgress{}
	err = manager.ExecuteHook(context.Background(), hook, progress)
	require.NoError(t, err)

	// Check execution was recorded
	assert.Equal(t, 1, executionCount)

	retrieved, err := registry.GetHook("hook1")
	require.NoError(t, err)
	assert.Equal(t, 1, retrieved.ExecCount)
	assert.False(t, retrieved.LastExecuted.IsZero())

	// Check metrics
	metrics := manager.GetMetrics()
	assert.Equal(t, int64(1), metrics.TotalExecutions)
	assert.Equal(t, int64(1), metrics.SuccessfulExecutions)
	assert.Equal(t, int64(0), metrics.FailedExecutions)

	// Check lifecycle events
	select {
	case event := <-manager.GetLifecycleEvents():
		assert.Equal(t, "hook_registered", event.EventType)
	case <-time.After(time.Second):
		assert.Fail(t, "Expected registration event")
	}

	select {
	case event := <-manager.GetLifecycleEvents():
		assert.Equal(t, "hook_executed_success", event.EventType)
	case <-time.After(time.Second):
		assert.Fail(t, "Expected execution event")
	}

	manager.Shutdown()
}

// TestHookLifecycleManager_ExecuteHookError tests error handling in hook execution
func TestHookLifecycleManager_ExecuteHookError(t *testing.T) {
	registry := NewRegistry()
	config := DefaultHookLifecycleConfig()
	config.MaxExecutionRetries = 2 // Lower threshold for testing
	manager := NewHookLifecycleManagerWithConfig(registry, config)

	executionCount := 0

	// Register hook
	hook := HookRegistration{
		ID:    "hook1",
		Event: types.HookEventPreToolUse,
		Handler: func(ctx context.Context, progress types.HookProgress) (*types.HookResult, error) {
			executionCount++
			return nil, errors.New("test error")
		},
		Priority: 10,
	}

	err := manager.RegisterHook(hook)
	require.NoError(t, err)

	// Execute hook multiple times to exceed error threshold
	progress := types.HookProgress{}

	for i := 0; i < 3; i++ {
		err = manager.ExecuteHook(context.Background(), hook, progress)
		assert.Error(t, err)
	}

	// Check that hook was marked as dead
	retrieved, err := registry.GetHook("hook1")
	require.NoError(t, err)
	assert.Equal(t, HookStateDead, retrieved.State)
	assert.Equal(t, 3, retrieved.ErrorCount)

	// Check metrics
	metrics := manager.GetMetrics()
	assert.Equal(t, int64(3), metrics.TotalExecutions)
	assert.Equal(t, int64(0), metrics.SuccessfulExecutions)
	assert.Equal(t, int64(3), metrics.FailedExecutions)

	manager.Shutdown()
}

// TestHookLifecycleManager_PauseResumeHook tests pausing and resuming hooks
func TestHookLifecycleManager_PauseResumeHook(t *testing.T) {
	registry := NewRegistry()
	manager := NewHookLifecycleManager(registry)

	// Register hook
	hook := HookRegistration{
		ID:       "hook1",
		Event:    types.HookEventPreToolUse,
		Handler:  func(ctx context.Context, progress types.HookProgress) (*types.HookResult, error) { return nil, nil },
		Priority: 10,
	}

	err := manager.RegisterHook(hook)
	require.NoError(t, err)

	// Pause hook
	err = manager.PauseHook("hook1")
	require.NoError(t, err)

	retrieved, err := registry.GetHook("hook1")
	require.NoError(t, err)
	assert.Equal(t, HookStatePaused, retrieved.State)

	// Resume hook
	err = manager.ResumeHook("hook1")
	require.NoError(t, err)

	retrieved, err = registry.GetHook("hook1")
	require.NoError(t, err)
	assert.Equal(t, HookStateActive, retrieved.State)

	manager.Shutdown()
}

// TestHookLifecycleManager_AutoCleanup tests automatic cleanup of hooks
func TestHookLifecycleManager_AutoCleanup(t *testing.T) {
	registry := NewRegistry()
	config := DefaultHookLifecycleConfig()
	config.CleanupInterval = 100 * time.Millisecond // Fast cleanup for testing
	config.EnableAutoCleanup = true

	manager := NewHookLifecycleManagerWithConfig(registry, config)

	// Register hook with short lifetime
	hook := HookRegistration{
		ID:          "temporary",
		Event:       types.HookEventPreToolUse,
		Handler:     func(ctx context.Context, progress types.HookProgress) (*types.HookResult, error) { return nil, nil },
		Priority:    10,
		MaxLifetime: 50 * time.Millisecond,
	}

	err := manager.RegisterHook(hook)
	require.NoError(t, err)

	// Hook should exist initially
	_, err = registry.GetHook("temporary")
	require.NoError(t, err)

	// Wait for cleanup
	time.Sleep(200 * time.Millisecond)

	// Hook should be cleaned up
	_, err = registry.GetHook("temporary")
	assert.Error(t, err)

	manager.Shutdown()
}

// TestHookLifecycleManager_GetActiveHooks tests getting active hooks
func TestHookLifecycleManager_GetActiveHooks(t *testing.T) {
	registry := NewRegistry()
	manager := NewHookLifecycleManager(registry)

	// Register and execute hooks
	hook1 := HookRegistration{
		ID:    "hook1",
		Event: types.HookEventPreToolUse,
		Handler: func(ctx context.Context, progress types.HookProgress) (*types.HookResult, error) {
			time.Sleep(100 * time.Millisecond)
			return nil, nil
		},
		Priority: 10,
	}

	err := manager.RegisterHook(hook1)
	require.NoError(t, err)

	// Start hook execution
	ctx := context.Background()
	progress := types.HookProgress{}
	go func() {
		_ = manager.ExecuteHook(ctx, hook1, progress)
	}()

	// Wait a bit for execution to start
	time.Sleep(10 * time.Millisecond)

	// Get active hooks
	activeHooks := manager.GetActiveHooks()
	assert.Len(t, activeHooks, 1)
	assert.Equal(t, "hook1", activeHooks["hook1"].HookID)

	// Wait for execution to complete
	time.Sleep(150 * time.Millisecond)

	// No active hooks now
	activeHooks = manager.GetActiveHooks()
	assert.Len(t, activeHooks, 0)

	manager.Shutdown()
}

// TestHookTimeoutManager_StartStopTimeout tests starting and stopping timeouts
func TestHookTimeoutManager_StartStopTimeout(t *testing.T) {
	manager := NewHookTimeoutManager()

	// Start timeout
	ctx, _ := manager.StartTimeout(context.Background(), "hook1", time.Second)

	// Check timeout info
	info, err := manager.GetTimeoutInfo("hook1")
	require.NoError(t, err)
	assert.Equal(t, "hook1", info.HookID)
	assert.Equal(t, time.Second, info.Timeout)
	assert.False(t, info.Expired)

	// Stop timeout
	manager.StopTimeout("hook1")

	// Timeout should be gone
	_, err = manager.GetTimeoutInfo("hook1")
	assert.Error(t, err)

	// Cancel function should have been called
	select {
	case <-ctx.Done():
		// Context was cancelled
	default:
		assert.Fail(t, "Expected context to be cancelled")
	}
}

// TestHookTimeoutManager_CheckTimeout tests checking if a timeout has occurred
func TestHookTimeoutManager_CheckTimeout(t *testing.T) {
	manager := NewHookTimeoutManagerWithDefaultTimeout(100 * time.Millisecond)

	// Start timeout
	manager.StartTimeout(context.Background(), "hook1", 50*time.Millisecond)

	// Immediately check - should not be expired
	expired := manager.CheckTimeout("hook1")
	assert.False(t, expired)

	// Wait for timeout to expire
	time.Sleep(60 * time.Millisecond)

	// Check again - should be expired
	expired = manager.CheckTimeout("hook1")
	assert.True(t, expired)

	// Get timeout info
	info, err := manager.GetTimeoutInfo("hook1")
	require.NoError(t, err)
	assert.True(t, info.Expired)
	assert.False(t, info.ExpiredTime.IsZero())
}

// TestHookTimeoutManager_TimeoutEvents tests timeout event publishing
func TestHookTimeoutManager_TimeoutEvents(t *testing.T) {
	manager := NewHookTimeoutManagerWithDefaultTimeout(50 * time.Millisecond)

	// Start timeout
	manager.StartTimeout(context.Background(), "hook1", 30*time.Millisecond)

	// Wait for timeout to expire and event to be published
	time.Sleep(60 * time.Millisecond)

	// Check timeout
	manager.CheckTimeout("hook1")

	// Check for timeout event
	select {
	case event := <-manager.GetTimeoutEvents():
		assert.Equal(t, "hook1", event.HookID)
		assert.False(t, event.Timestamp.IsZero())
	default:
		assert.Fail(t, "Expected timeout event")
	}
}

// TestHookTimeoutManager_GetExpiredTimeouts tests getting expired timeouts
func TestHookTimeoutManager_GetExpiredTimeouts(t *testing.T) {
	manager := NewHookTimeoutManager()

	// Start multiple timeouts
	manager.StartTimeout(context.Background(), "hook1", 30*time.Millisecond)
	manager.StartTimeout(context.Background(), "hook2", 50*time.Millisecond)
	manager.StartTimeout(context.Background(), "hook3", 100*time.Millisecond)

	// Wait for some to expire
	time.Sleep(60 * time.Millisecond)

	// Check timeouts
	manager.CheckTimeout("hook1")
	manager.CheckTimeout("hook2")
	manager.CheckTimeout("hook3")

	// Get expired timeouts
	expired := manager.GetExpiredTimeouts()
	assert.Len(t, expired, 2) // hook1 and hook2 should be expired

	_, hook1Expired := expired["hook1"]
	assert.True(t, hook1Expired)

	_, hook2Expired := expired["hook2"]
	assert.True(t, hook2Expired)

	_, hook3Expired := expired["hook3"]
	assert.False(t, hook3Expired)
}

// TestHookTimeoutManager_ClearExpiredTimeouts tests clearing expired timeouts
func TestHookTimeoutManager_ClearExpiredTimeouts(t *testing.T) {
	manager := NewHookTimeoutManager()

	// Start multiple timeouts
	manager.StartTimeout(context.Background(), "hook1", 30*time.Millisecond)
	manager.StartTimeout(context.Background(), "hook2", 30*time.Millisecond)
	manager.StartTimeout(context.Background(), "hook3", 100*time.Millisecond)

	// Wait for some to expire
	time.Sleep(60 * time.Millisecond)

	// Check timeouts
	manager.CheckTimeout("hook1")
	manager.CheckTimeout("hook2")

	// Clear expired timeouts
	clearedCount := manager.ClearExpiredTimeouts()
	assert.Equal(t, 2, clearedCount)

	// Check remaining active count
	assert.Equal(t, 1, manager.GetActiveCount())
	assert.Equal(t, 0, manager.GetExpiredCount())
}

// TestDefaultHookLifecycleConfig tests default lifecycle configuration
func TestDefaultHookLifecycleConfig(t *testing.T) {
	config := DefaultHookLifecycleConfig()

	assert.Equal(t, 5*time.Minute, config.CleanupInterval)
	assert.Equal(t, 30*time.Minute, config.DefaultHookLifetime)
	assert.Equal(t, 3, config.MaxExecutionRetries)
	assert.Equal(t, 30*time.Second, config.ExecutionTimeout)
	assert.True(t, config.EnableAutoCleanup)
	assert.True(t, config.EnablePerformanceTracking)
	assert.NotNil(t, config.HookTimeoutManager)
}

// TestHookMetrics tests hook metrics collection
func TestHookMetrics(t *testing.T) {
	registry := NewRegistry()
	config := DefaultHookLifecycleConfig()
	config.EnablePerformanceTracking = true

	manager := NewHookLifecycleManagerWithConfig(registry, config)

	// Register multiple hooks
	hooks := []HookRegistration{
		{
			ID:    "fast",
			Event: types.HookEventPreToolUse,
			Handler: func(ctx context.Context, progress types.HookProgress) (*types.HookResult, error) {
				time.Sleep(10 * time.Millisecond)
				return nil, nil
			},
			Priority: 10,
		},
		{
			ID:    "slow",
			Event: types.HookEventPreToolUse,
			Handler: func(ctx context.Context, progress types.HookProgress) (*types.HookResult, error) {
				time.Sleep(50 * time.Millisecond)
				return nil, nil
			},
			Priority: 5,
		},
		{
			ID:    "error",
			Event: types.HookEventPreToolUse,
			Handler: func(ctx context.Context, progress types.HookProgress) (*types.HookResult, error) {
				time.Sleep(20 * time.Millisecond)
				return nil, errors.New("test error")
			},
			Priority: 1,
		},
	}

	for _, hook := range hooks {
		err := manager.RegisterHook(hook)
		require.NoError(t, err)
	}

	// Execute hooks
	progress := types.HookProgress{}
	for _, hook := range hooks {
		_ = manager.ExecuteHook(context.Background(), hook, progress)
	}

	// Check metrics
	metrics := manager.GetMetrics()
	assert.Equal(t, int64(3), metrics.TotalExecutions)
	assert.Equal(t, int64(2), metrics.SuccessfulExecutions)
	assert.Equal(t, int64(1), metrics.FailedExecutions)
	assert.Greater(t, metrics.AverageExecutionTime, time.Duration(0))
	assert.Greater(t, metrics.TotalExecutionTime, time.Duration(0))
	assert.Greater(t, metrics.MaxExecutionTime, time.Duration(0))
	assert.Greater(t, metrics.MinExecutionTime, time.Duration(0))

	manager.Shutdown()
}

// TestHookLifecycleManager_ConcurrentExecution tests concurrent hook execution
func TestHookLifecycleManager_ConcurrentExecution(t *testing.T) {
	registry := NewRegistry()
	manager := NewHookLifecycleManager(registry)

	var wg sync.WaitGroup
	var mu sync.Mutex
	executionCount := 0

	// Register hook
	hook := HookRegistration{
		ID:    "concurrent",
		Event: types.HookEventPreToolUse,
		Handler: func(ctx context.Context, progress types.HookProgress) (*types.HookResult, error) {
			time.Sleep(10 * time.Millisecond)
			mu.Lock()
			defer mu.Unlock()
			executionCount++
			return nil, nil
		},
		Priority: 10,
	}

	err := manager.RegisterHook(hook)
	require.NoError(t, err)

	// Execute hook concurrently
	progress := types.HookProgress{}
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = manager.ExecuteHook(context.Background(), hook, progress)
		}()
	}

	wg.Wait()

	// Check all executions completed
	assert.Equal(t, 10, executionCount)

	// Check metrics
	metrics := manager.GetMetrics()
	assert.Equal(t, int64(10), metrics.TotalExecutions)
	assert.Equal(t, int64(10), metrics.SuccessfulExecutions)

	manager.Shutdown()
}

// TestHookLifecycleManager_ErrorRecovery tests error recovery behavior
func TestHookLifecycleManager_ErrorRecovery(t *testing.T) {
	registry := NewRegistry()
	config := DefaultHookLifecycleConfig()
	config.MaxExecutionRetries = 5
	manager := NewHookLifecycleManagerWithConfig(registry, config)

	shouldFail := true

	// Register hook
	hook := HookRegistration{
		ID:    "flaky",
		Event: types.HookEventPreToolUse,
		Handler: func(ctx context.Context, progress types.HookProgress) (*types.HookResult, error) {
			if shouldFail {
				return nil, errors.New("flaky error")
			}
			return nil, nil
		},
		Priority: 10,
	}

	err := manager.RegisterHook(hook)
	require.NoError(t, err)

	// Fail a few times (but not enough to mark as dead)
	progress := types.HookProgress{}
	for i := 0; i < 3; i++ {
		err = manager.ExecuteHook(context.Background(), hook, progress)
		assert.Error(t, err)
	}

	// Hook should still be active
	state, err := manager.GetHookState("flaky")
	require.NoError(t, err)
	assert.Equal(t, HookStateActive, state)

	// Now succeed
	shouldFail = false
	err = manager.ExecuteHook(context.Background(), hook, progress)
	assert.NoError(t, err)

	// Hook should still be active and error count should be reset
	state, err = manager.GetHookState("flaky")
	require.NoError(t, err)
	assert.Equal(t, HookStateActive, state)

	retrieved, err := registry.GetHook("flaky")
	require.NoError(t, err)
	assert.Equal(t, 0, retrieved.ErrorCount)

	manager.Shutdown()
}

// Helper function for timeout manager
func NewHookTimeoutManagerWithDefaultTimeout(timeout time.Duration) *HookTimeoutManager {
	return &HookTimeoutManager{
		activeTimeouts: make(map[string]*HookTimeoutInfo),
		defaultTimeout: timeout,
		timeoutEvents:  make(chan HookTimeoutEvent, 100),
		stopChan:       make(chan struct{}),
	}
}

// --- from registry_test.go ---

// TestRegistryOperations tests registry CRUD operations.
func TestRegistryOperations(t *testing.T) {
	t.Run("Add and Get hooks", func(t *testing.T) {
		registry := NewRegistry()
		hook := HookRegistration{
			ID:       "test-hook",
			Event:    types.HookEventSessionStart,
			Priority: 100,
			Handler: func(ctx context.Context, progress types.HookProgress) (*types.HookResult, error) {
				return nil, nil
			},
		}

		registry.Add(hook)

		// Get hooks for event
		hooks := registry.Get(types.HookEventSessionStart)
		if len(hooks) != 1 {
			t.Fatalf("Expected 1 hook, got %d", len(hooks))
		}

		if hooks[0].ID != "test-hook" {
			t.Errorf("Expected hook ID 'test-hook', got '%s'", hooks[0].ID)
		}
	})

	t.Run("Remove hook", func(t *testing.T) {
		registry := NewRegistry()
		hook := HookRegistration{
			ID:       "removable-hook",
			Event:    types.HookEventSessionStart,
			Priority: 100,
			Handler: func(ctx context.Context, progress types.HookProgress) (*types.HookResult, error) {
				return nil, nil
			},
		}

		registry.Add(hook)

		// Verify hook is registered
		hooks := registry.Get(types.HookEventSessionStart)
		if len(hooks) == 0 {
			t.Fatal("Hook was not registered")
		}

		// Remove hook
		registry.Remove("removable-hook")

		// Verify hook is removed
		hooks = registry.Get(types.HookEventSessionStart)
		if len(hooks) != 0 {
			t.Fatal("Hook was not removed")
		}
	})

	t.Run("HasHooks", func(t *testing.T) {
		registry := NewRegistry()
		event := types.HookEventSessionStart

		// Initially should have no hooks
		if registry.HasHooks(event) {
			t.Fatal("Should have no hooks initially")
		}

		// Add a hook
		registry.Add(HookRegistration{
			ID:       "test-hook",
			Event:    event,
			Priority: 100,
			Handler: func(ctx context.Context, progress types.HookProgress) (*types.HookResult, error) {
				return nil, nil
			},
		})

		// Now should have hooks
		if !registry.HasHooks(event) {
			t.Fatal("Should have hooks after adding")
		}
	})

	t.Run("Count hooks", func(t *testing.T) {
		registry := NewRegistry()
		event := types.HookEventSessionStart

		// Initially should have 0 hooks
		if count := registry.Count(event); count != 0 {
			t.Fatalf("Expected 0 hooks, got %d", count)
		}

		// Add 3 hooks
		for i := 0; i < 3; i++ {
			registry.Add(HookRegistration{
				ID:       string(rune('a' + i)),
				Event:    event,
				Priority: 100,
				Handler: func(ctx context.Context, progress types.HookProgress) (*types.HookResult, error) {
					return nil, nil
				},
			})
		}

		// Should have 3 hooks
		if count := registry.Count(event); count != 3 {
			t.Fatalf("Expected 3 hooks, got %d", count)
		}
	})

	t.Run("Clear hooks", func(t *testing.T) {
		registry := NewRegistry()
		// Add hooks for two events
		registry.Add(HookRegistration{
			ID:       "hook-1",
			Event:    types.HookEventSessionStart,
			Priority: 100,
			Handler: func(ctx context.Context, progress types.HookProgress) (*types.HookResult, error) {
				return nil, nil
			},
		})

		registry.Add(HookRegistration{
			ID:       "hook-2",
			Event:    types.HookEventSessionEnd,
			Priority: 100,
			Handler: func(ctx context.Context, progress types.HookProgress) (*types.HookResult, error) {
				return nil, nil
			},
		})

		// Clear all hooks
		registry.Clear("")

		// Should have no hooks for any event
		if registry.HasHooks(types.HookEventSessionStart) {
			t.Fatal("SessionStart hooks should be cleared")
		}

		if registry.HasHooks(types.HookEventSessionEnd) {
			t.Fatal("SessionEnd hooks should be cleared")
		}
	})
}
