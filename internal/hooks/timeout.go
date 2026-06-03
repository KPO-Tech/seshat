package hooks

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// HookTimeoutManager manages timeouts for hook executions
type HookTimeoutManager struct {
	// Active timeouts being tracked
	activeTimeouts   map[string]*HookTimeoutInfo
	activeTimeoutsMu sync.RWMutex

	// Default timeout duration
	defaultTimeout time.Duration

	// Timeout event channel
	timeoutEvents chan HookTimeoutEvent

	// Stop channel
	stopChan chan struct{}
}

// HookTimeoutInfo tracks timeout information for a hook
type HookTimeoutInfo struct {
	HookID      string
	StartTime   time.Time
	Timeout     time.Duration
	Context     context.Context
	CancelFunc  context.CancelFunc
	Expired     bool
	ExpiredTime time.Time
}

// HookTimeoutEvent represents a timeout event that occurred
type HookTimeoutEvent struct {
	HookID    string
	Timestamp time.Time
	Timeout   time.Duration
}

// NewHookTimeoutManager creates a new hook timeout manager
func NewHookTimeoutManager() *HookTimeoutManager {
	return &HookTimeoutManager{
		activeTimeouts: make(map[string]*HookTimeoutInfo),
		defaultTimeout: 30 * time.Second,
		timeoutEvents:  make(chan HookTimeoutEvent, 100),
		stopChan:       make(chan struct{}),
	}
}

// NewHookTimeoutManagerWithTimeout creates a new hook timeout manager with default timeout
func NewHookTimeoutManagerWithTimeout(defaultTimeout time.Duration) *HookTimeoutManager {
	return &HookTimeoutManager{
		activeTimeouts: make(map[string]*HookTimeoutInfo),
		defaultTimeout: defaultTimeout,
		timeoutEvents:  make(chan HookTimeoutEvent, 100),
		stopChan:       make(chan struct{}),
	}
}

// StartTimeout starts tracking a timeout for a hook.
// parentCtx is used as the parent context so the hook is cancelled when
// the calling request is cancelled in addition to its own deadline.
func (m *HookTimeoutManager) StartTimeout(parentCtx context.Context, hookID string, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		timeout = m.defaultTimeout
	}

	ctx, cancel := context.WithTimeout(parentCtx, timeout)

	m.activeTimeoutsMu.Lock()
	defer m.activeTimeoutsMu.Unlock()

	m.activeTimeouts[hookID] = &HookTimeoutInfo{
		HookID:     hookID,
		StartTime:  time.Now(),
		Timeout:    timeout,
		Context:    ctx,
		CancelFunc: cancel,
		Expired:    false,
	}

	return ctx, cancel
}

// StopTimeout stops tracking a timeout for a hook
func (m *HookTimeoutManager) StopTimeout(hookID string) {
	m.activeTimeoutsMu.Lock()
	defer m.activeTimeoutsMu.Unlock()

	if timeoutInfo, exists := m.activeTimeouts[hookID]; exists {
		if timeoutInfo.CancelFunc != nil && !timeoutInfo.Expired {
			timeoutInfo.CancelFunc()
		}
		delete(m.activeTimeouts, hookID)
	}
}

// CheckTimeout checks if a hook has timed out
func (m *HookTimeoutManager) CheckTimeout(hookID string) bool {
	m.activeTimeoutsMu.RLock()
	defer m.activeTimeoutsMu.RUnlock()

	timeoutInfo, exists := m.activeTimeouts[hookID]
	if !exists {
		return false
	}

	select {
	case <-timeoutInfo.Context.Done():
		// Context is done, check if it's a timeout
		if timeoutInfo.Context.Err() == context.DeadlineExceeded && !timeoutInfo.Expired {
			// Mark as expired (only once)
			m.activeTimeoutsMu.RUnlock()
			m.markAsExpired(hookID)
			m.activeTimeoutsMu.RLock()
			return true
		}
		return false
	default:
		// Context is still active
		return false
	}
}

// markAsExpired marks a hook timeout as expired
func (m *HookTimeoutManager) markAsExpired(hookID string) {
	m.activeTimeoutsMu.Lock()
	defer m.activeTimeoutsMu.Unlock()

	if timeoutInfo, exists := m.activeTimeouts[hookID]; exists && !timeoutInfo.Expired {
		timeoutInfo.Expired = true
		timeoutInfo.ExpiredTime = time.Now()

		// Publish timeout event
		select {
		case m.timeoutEvents <- HookTimeoutEvent{
			HookID:    hookID,
			Timestamp: timeoutInfo.ExpiredTime,
			Timeout:   timeoutInfo.Timeout,
		}:
		default:
			slog.Warn("hook timeout events channel full, dropping event", "hook_id", hookID)
		}
	}
}

// GetTimeoutInfo returns timeout information for a hook
func (m *HookTimeoutManager) GetTimeoutInfo(hookID string) (*HookTimeoutInfo, error) {
	m.activeTimeoutsMu.RLock()
	defer m.activeTimeoutsMu.RUnlock()

	timeoutInfo, exists := m.activeTimeouts[hookID]
	if !exists {
		return nil, fmt.Errorf("timeout not found for hook: %s", hookID)
	}

	// Return a copy to prevent external modification
	copyInfo := &HookTimeoutInfo{
		HookID:      timeoutInfo.HookID,
		StartTime:   timeoutInfo.StartTime,
		Timeout:     timeoutInfo.Timeout,
		Expired:     timeoutInfo.Expired,
		ExpiredTime: timeoutInfo.ExpiredTime,
	}

	return copyInfo, nil
}

// GetActiveTimeouts returns all active timeout information
func (m *HookTimeoutManager) GetActiveTimeouts() map[string]*HookTimeoutInfo {
	m.activeTimeoutsMu.RLock()
	defer m.activeTimeoutsMu.RUnlock()

	// Return a copy to prevent external modification
	result := make(map[string]*HookTimeoutInfo, len(m.activeTimeouts))
	for _, timeoutInfo := range m.activeTimeouts {
		result[timeoutInfo.HookID] = &HookTimeoutInfo{
			HookID:      timeoutInfo.HookID,
			StartTime:   timeoutInfo.StartTime,
			Timeout:     timeoutInfo.Timeout,
			Expired:     timeoutInfo.Expired,
			ExpiredTime: timeoutInfo.ExpiredTime,
		}
	}

	return result
}

// GetTimeoutEvents returns the channel for timeout events
func (m *HookTimeoutManager) GetTimeoutEvents() <-chan HookTimeoutEvent {
	return m.timeoutEvents
}

// GetExpiredTimeouts returns all expired timeout information
func (m *HookTimeoutManager) GetExpiredTimeouts() map[string]*HookTimeoutInfo {
	m.activeTimeoutsMu.RLock()
	defer m.activeTimeoutsMu.RUnlock()

	result := make(map[string]*HookTimeoutInfo)
	for id, timeoutInfo := range m.activeTimeouts {
		if timeoutInfo.Expired {
			result[id] = &HookTimeoutInfo{
				HookID:      timeoutInfo.HookID,
				StartTime:   timeoutInfo.StartTime,
				Timeout:     timeoutInfo.Timeout,
				Expired:     timeoutInfo.Expired,
				ExpiredTime: timeoutInfo.ExpiredTime,
			}
		}
	}

	return result
}

// ClearExpiredTimeouts removes all expired timeout information
func (m *HookTimeoutManager) ClearExpiredTimeouts() int {
	m.activeTimeoutsMu.Lock()
	defer m.activeTimeoutsMu.Unlock()

	clearedCount := 0
	for id, timeoutInfo := range m.activeTimeouts {
		if timeoutInfo.Expired {
			delete(m.activeTimeouts, id)
			clearedCount++
		}
	}

	return clearedCount
}

// ClearAllTimeouts removes all timeout information
func (m *HookTimeoutManager) ClearAllTimeouts() {
	m.activeTimeoutsMu.Lock()
	defer m.activeTimeoutsMu.Unlock()

	for _, timeoutInfo := range m.activeTimeouts {
		if timeoutInfo.CancelFunc != nil && !timeoutInfo.Expired {
			timeoutInfo.CancelFunc()
		}
	}

	m.activeTimeouts = make(map[string]*HookTimeoutInfo)
}

// GetActiveCount returns the number of active timeout trackers
func (m *HookTimeoutManager) GetActiveCount() int {
	m.activeTimeoutsMu.RLock()
	defer m.activeTimeoutsMu.RUnlock()

	return len(m.activeTimeouts)
}

// GetExpiredCount returns the number of expired timeout trackers
func (m *HookTimeoutManager) GetExpiredCount() int {
	m.activeTimeoutsMu.RLock()
	defer m.activeTimeoutsMu.RUnlock()

	expiredCount := 0
	for _, timeoutInfo := range m.activeTimeouts {
		if timeoutInfo.Expired {
			expiredCount++
		}
	}

	return expiredCount
}

// SetDefaultTimeout sets the default timeout duration
func (m *HookTimeoutManager) SetDefaultTimeout(timeout time.Duration) {
	m.activeTimeoutsMu.Lock()
	defer m.activeTimeoutsMu.Unlock()

	m.defaultTimeout = timeout
}

// GetDefaultTimeout returns the default timeout duration
func (m *HookTimeoutManager) GetDefaultTimeout() time.Duration {
	m.activeTimeoutsMu.RLock()
	defer m.activeTimeoutsMu.RUnlock()

	return m.defaultTimeout
}

// Shutdown gracefully shuts down the timeout manager
func (m *HookTimeoutManager) Shutdown() {
	// Stop all active timeouts
	m.ClearAllTimeouts()

	// Close timeout events channel
	close(m.timeoutEvents)

	// Signal shutdown
	close(m.stopChan)
}
