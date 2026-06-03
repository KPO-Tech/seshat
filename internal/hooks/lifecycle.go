package hooks

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/EngineerProjects/nexus-engine/internal/types"
)

// HookLifecycleConfig configures the hook lifecycle manager behavior
type HookLifecycleConfig struct {
	// CleanupInterval is how often to check for expired hooks
	CleanupInterval time.Duration

	// DefaultHookLifetime is the default lifetime for hooks
	DefaultHookLifetime time.Duration

	// MaxExecutionRetries is the maximum number of retries before marking a hook as dead
	MaxExecutionRetries int

	// ExecutionTimeout is the maximum time a hook can run before being cancelled
	ExecutionTimeout time.Duration

	// EnableAutoCleanup enables automatic cleanup of expired/dead hooks
	EnableAutoCleanup bool

	// EnablePerformanceTracking enables tracking of hook performance metrics
	EnablePerformanceTracking bool

	// HookTimeoutManager is the manager for hook timeouts
	HookTimeoutManager *HookTimeoutManager
}

// DefaultHookLifecycleConfig returns default lifecycle configuration
func DefaultHookLifecycleConfig() *HookLifecycleConfig {
	return &HookLifecycleConfig{
		CleanupInterval:           5 * time.Minute,
		DefaultHookLifetime:       30 * time.Minute,
		MaxExecutionRetries:       3,
		ExecutionTimeout:          30 * time.Second,
		EnableAutoCleanup:         true,
		EnablePerformanceTracking: true,
		HookTimeoutManager:        NewHookTimeoutManager(),
	}
}

// HookLifecycleManager manages the lifecycle of hooks including registration,
// execution tracking, cleanup, and state management.
type HookLifecycleManager struct {
	// Registry is the hook registry this manager works with
	registry *Registry

	// Config is the lifecycle configuration
	config *HookLifecycleConfig

	// Active hooks tracking
	activeHooks   map[string]*ActiveHookInfo
	activeHooksMu sync.RWMutex

	// Performance metrics
	metrics   HookMetrics
	metricsMu sync.RWMutex

	// Lifecycle events channel
	lifecycleEvents       chan HookLifecycleEvent
	lifecycleEventsMu     sync.Mutex // guards concurrent close/send
	lifecycleEventsClosed bool

	// Cleanup control
	stopChan chan struct{}
	doneChan chan struct{}
}

// ActiveHookInfo tracks information about actively running hooks
type ActiveHookInfo struct {
	HookID     string
	Event      types.HookEvent
	StartTime  time.Time
	Context    context.Context
	CancelFunc context.CancelFunc
}

// HookMetrics tracks performance metrics for hooks
type HookMetrics struct {
	TotalExecutions      int64
	SuccessfulExecutions int64
	FailedExecutions     int64
	AverageExecutionTime time.Duration
	TotalExecutionTime   time.Duration
	MaxExecutionTime     time.Duration
	MinExecutionTime     time.Duration
}

// HookLifecycleEvent represents a lifecycle event that occurred
type HookLifecycleEvent struct {
	EventType string
	HookID    string
	Timestamp time.Time
	Error     error
	Metadata  map[string]interface{}
}

// NewHookLifecycleManager creates a new hook lifecycle manager
func NewHookLifecycleManager(registry *Registry) *HookLifecycleManager {
	config := DefaultHookLifecycleConfig()

	manager := &HookLifecycleManager{
		registry:        registry,
		config:          config,
		activeHooks:     make(map[string]*ActiveHookInfo),
		lifecycleEvents: make(chan HookLifecycleEvent, 100),
		stopChan:        make(chan struct{}),
		doneChan:        make(chan struct{}),
	}

	// Start background cleanup if enabled
	if config.EnableAutoCleanup {
		go manager.cleanupLoop()
	}

	return manager
}

// NewHookLifecycleManagerWithConfig creates a new hook lifecycle manager with custom config
func NewHookLifecycleManagerWithConfig(registry *Registry, config *HookLifecycleConfig) *HookLifecycleManager {
	if config.HookTimeoutManager == nil {
		config.HookTimeoutManager = NewHookTimeoutManager()
	}

	manager := &HookLifecycleManager{
		registry:        registry,
		config:          config,
		activeHooks:     make(map[string]*ActiveHookInfo),
		lifecycleEvents: make(chan HookLifecycleEvent, 100),
		stopChan:        make(chan struct{}),
		doneChan:        make(chan struct{}),
	}

	if config.EnableAutoCleanup {
		go manager.cleanupLoop()
	}

	return manager
}

// RegisterHook registers a new hook with lifecycle management
func (m *HookLifecycleManager) RegisterHook(hook HookRegistration) error {
	// Set default lifetime if not specified
	if hook.MaxLifetime == 0 && m.config.DefaultHookLifetime > 0 {
		hook.MaxLifetime = m.config.DefaultHookLifetime
	}

	// Set default state if not specified
	if hook.State == "" {
		hook.State = HookStateActive
	}

	// Add to registry
	m.registry.Add(hook)

	// Publish lifecycle event
	m.publishLifecycleEvent(HookLifecycleEvent{
		EventType: "hook_registered",
		HookID:    hook.ID,
		Timestamp: time.Now(),
		Metadata: map[string]interface{}{
			"event":     string(hook.Event),
			"priority":  hook.Priority,
			"permanent": hook.IsPermanent,
		},
	})

	return nil
}

// ExecuteHook executes a hook with lifecycle management
func (m *HookLifecycleManager) ExecuteHook(
	ctx context.Context,
	hook HookRegistration,
	progress types.HookProgress,
) error {
	startTime := time.Now()

	// Create context with timeout if configured
	executionCtx := ctx
	if m.config.ExecutionTimeout > 0 {
		var cancel context.CancelFunc
		executionCtx, cancel = context.WithTimeout(ctx, m.config.ExecutionTimeout)
		defer cancel()
	}

	// Track active hook
	m.trackActiveHook(hook.ID, hook.Event, executionCtx)

	// Execute the hook — result is intentionally discarded here; lifecycle
	// management only cares about errors. Use hooks.Executor for interactive
	// hooks that need to act on the returned HookResult.
	_, err := hook.Handler(executionCtx, progress)

	// Untrack active hook
	m.untrackActiveHook(hook.ID)

	// Update metrics
	executionTime := time.Since(startTime)
	m.updateMetrics(executionTime, err)

	// Record execution in registry
	recordErr := m.registry.RecordHookExecution(hook.ID, err)
	if recordErr != nil {
		return fmt.Errorf("failed to record execution: %w, original error: %w", recordErr, err)
	}

	// Handle hook errors
	if err != nil {
		m.handleHookError(hook, err)
	}

	// Publish lifecycle event
	eventType := "hook_executed_success"
	if err != nil {
		eventType = "hook_executed_failure"
	}

	m.publishLifecycleEvent(HookLifecycleEvent{
		EventType: eventType,
		HookID:    hook.ID,
		Timestamp: time.Now(),
		Error:     err,
		Metadata: map[string]interface{}{
			"executionTime": executionTime,
			"event":         string(hook.Event),
		},
	})

	return err
}

// PauseHook pauses a hook temporarily
func (m *HookLifecycleManager) PauseHook(hookID string) error {
	err := m.registry.PauseHook(hookID)
	if err != nil {
		return err
	}

	m.publishLifecycleEvent(HookLifecycleEvent{
		EventType: "hook_paused",
		HookID:    hookID,
		Timestamp: time.Now(),
	})

	return nil
}

// ResumeHook resumes a paused hook
func (m *HookLifecycleManager) ResumeHook(hookID string) error {
	err := m.registry.ResumeHook(hookID)
	if err != nil {
		return err
	}

	m.publishLifecycleEvent(HookLifecycleEvent{
		EventType: "hook_resumed",
		HookID:    hookID,
		Timestamp: time.Now(),
	})

	return nil
}

// RemoveHook removes a hook from the registry
func (m *HookLifecycleManager) RemoveHook(hookID string) error {
	m.registry.Remove(hookID)

	m.publishLifecycleEvent(HookLifecycleEvent{
		EventType: "hook_removed",
		HookID:    hookID,
		Timestamp: time.Now(),
	})

	return nil
}

// GetMetrics returns current hook metrics
func (m *HookLifecycleManager) GetMetrics() HookMetrics {
	m.metricsMu.RLock()
	defer m.metricsMu.RUnlock()

	return HookMetrics{
		TotalExecutions:      m.metrics.TotalExecutions,
		SuccessfulExecutions: m.metrics.SuccessfulExecutions,
		FailedExecutions:     m.metrics.FailedExecutions,
		AverageExecutionTime: m.metrics.AverageExecutionTime,
		TotalExecutionTime:   m.metrics.TotalExecutionTime,
		MaxExecutionTime:     m.metrics.MaxExecutionTime,
		MinExecutionTime:     m.metrics.MinExecutionTime,
	}
}

// GetLifecycleEvents returns the channel for lifecycle events
func (m *HookLifecycleManager) GetLifecycleEvents() <-chan HookLifecycleEvent {
	return m.lifecycleEvents
}

// trackActiveHook tracks an actively running hook
func (m *HookLifecycleManager) trackActiveHook(hookID string, event types.HookEvent, ctx context.Context) {
	m.activeHooksMu.Lock()
	defer m.activeHooksMu.Unlock()

	// Cancel any existing context for this hook
	if existing, exists := m.activeHooks[hookID]; exists {
		if existing.CancelFunc != nil {
			existing.CancelFunc()
		}
	}

	// Create cancelable context
	hookCtx, cancel := context.WithCancel(ctx)

	m.activeHooks[hookID] = &ActiveHookInfo{
		HookID:     hookID,
		Event:      event,
		StartTime:  time.Now(),
		Context:    hookCtx,
		CancelFunc: cancel,
	}
}

// untrackActiveHook removes a hook from active tracking
func (m *HookLifecycleManager) untrackActiveHook(hookID string) {
	m.activeHooksMu.Lock()
	defer m.activeHooksMu.Unlock()

	delete(m.activeHooks, hookID)
}

// updateMetrics updates performance metrics after hook execution
func (m *HookLifecycleManager) updateMetrics(executionTime time.Duration, err error) {
	if !m.config.EnablePerformanceTracking {
		return
	}

	m.metricsMu.Lock()
	defer m.metricsMu.Unlock()

	m.metrics.TotalExecutions++
	m.metrics.TotalExecutionTime += executionTime

	// Update min/max execution times
	if m.metrics.MinExecutionTime == 0 || executionTime < m.metrics.MinExecutionTime {
		m.metrics.MinExecutionTime = executionTime
	}
	if executionTime > m.metrics.MaxExecutionTime {
		m.metrics.MaxExecutionTime = executionTime
	}

	// Calculate average
	m.metrics.AverageExecutionTime = m.metrics.TotalExecutionTime / time.Duration(m.metrics.TotalExecutions)

	// Update success/failure counts
	if err == nil {
		m.metrics.SuccessfulExecutions++
	} else {
		m.metrics.FailedExecutions++
	}
}

// handleHookError handles errors from hook execution
func (m *HookLifecycleManager) handleHookError(hook HookRegistration, err error) {
	// Get current hook state to check error count
	currentHook, getErr := m.registry.GetHook(hook.ID)
	if getErr != nil {
		return
	}

	// Check if error threshold exceeded
	if currentHook.ErrorCount >= m.config.MaxExecutionRetries {
		// Mark hook as dead
		_ = m.registry.SetHookState(hook.ID, HookStateDead)

		m.publishLifecycleEvent(HookLifecycleEvent{
			EventType: "hook_marked_dead",
			HookID:    hook.ID,
			Timestamp: time.Now(),
			Error:     err,
			Metadata: map[string]interface{}{
				"errorCount": currentHook.ErrorCount,
				"maxRetries": m.config.MaxExecutionRetries,
			},
		})
	}
}

// publishLifecycleEvent publishes a lifecycle event to the events channel.
// Safe to call concurrently with Shutdown.
func (m *HookLifecycleManager) publishLifecycleEvent(event HookLifecycleEvent) {
	m.lifecycleEventsMu.Lock()
	defer m.lifecycleEventsMu.Unlock()
	if m.lifecycleEventsClosed {
		return
	}
	select {
	case m.lifecycleEvents <- event:
	default:
		fmt.Printf("[hook-lifecycle] Warning: lifecycle events channel full, dropping event %s for hook %s\n",
			event.EventType, event.HookID)
	}
}

// cleanupLoop periodically cleans up expired and dead hooks
func (m *HookLifecycleManager) cleanupLoop() {
	ticker := time.NewTicker(m.config.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.performCleanup()
		case <-m.stopChan:
			return
		}
	}
}

// performCleanup performs cleanup of expired and dead hooks
func (m *HookLifecycleManager) performCleanup() {
	// Clean up expired hooks
	expiredCount := m.registry.CleanupExpiredHooks()
	if expiredCount > 0 {
		m.publishLifecycleEvent(HookLifecycleEvent{
			EventType: "cleanup_expired_hooks",
			HookID:    "system",
			Timestamp: time.Now(),
			Metadata: map[string]interface{}{
				"cleanedCount": expiredCount,
			},
		})
	}

	// Clean up dead hooks
	deadCount := m.registry.CleanupDeadHooks()
	if deadCount > 0 {
		m.publishLifecycleEvent(HookLifecycleEvent{
			EventType: "cleanup_dead_hooks",
			HookID:    "system",
			Timestamp: time.Now(),
			Metadata: map[string]interface{}{
				"cleanedCount": deadCount,
			},
		})
	}
}

// GetActiveHooks returns all currently active hooks
func (m *HookLifecycleManager) GetActiveHooks() map[string]*ActiveHookInfo {
	m.activeHooksMu.RLock()
	defer m.activeHooksMu.RUnlock()

	// Return a copy to prevent external modification
	result := make(map[string]*ActiveHookInfo, len(m.activeHooks))
	for id, hook := range m.activeHooks {
		result[id] = &ActiveHookInfo{
			HookID:     hook.HookID,
			Event:      hook.Event,
			StartTime:  hook.StartTime,
			Context:    hook.Context,
			CancelFunc: hook.CancelFunc,
		}
	}

	return result
}

// GetHookState returns the current state of a hook
func (m *HookLifecycleManager) GetHookState(hookID string) (HookState, error) {
	hook, err := m.registry.GetHook(hookID)
	if err != nil {
		return "", err
	}
	return hook.State, nil
}

// Shutdown gracefully shuts down the lifecycle manager
func (m *HookLifecycleManager) Shutdown() {
	// Signal cleanup loop to stop
	close(m.stopChan)

	// Cancel all active hooks
	m.activeHooksMu.Lock()
	for _, hook := range m.activeHooks {
		if hook.CancelFunc != nil {
			hook.CancelFunc()
		}
	}
	m.activeHooks = make(map[string]*ActiveHookInfo)
	m.activeHooksMu.Unlock()

	// Close lifecycle events channel — protected against concurrent sends.
	m.lifecycleEventsMu.Lock()
	m.lifecycleEventsClosed = true
	close(m.lifecycleEvents)
	m.lifecycleEventsMu.Unlock()

	// Signal that shutdown is complete
	close(m.doneChan)
}

// WaitShutdown waits for the shutdown to complete
func (m *HookLifecycleManager) WaitShutdown() {
	<-m.doneChan
}

// GetConfig returns the current configuration
func (m *HookLifecycleManager) GetConfig() *HookLifecycleConfig {
	return m.config
}

// UpdateConfig updates the lifecycle configuration
func (m *HookLifecycleManager) UpdateConfig(config *HookLifecycleConfig) {
	m.config = config
}
