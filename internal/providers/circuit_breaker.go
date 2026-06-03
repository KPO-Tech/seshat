package providers

import (
	"context"
	"errors"
	"sync"
	"time"
)

// CircuitState represents the current state of the circuit breaker
type CircuitState string

const (
	CircuitStateClosed   CircuitState = "closed"    // Requests are allowed
	CircuitStateOpen     CircuitState = "open"      // Requests are blocked
	CircuitStateHalfOpen CircuitState = "half-open" // Limited requests are allowed to test recovery
)

// CircuitBreakerConfig represents the configuration for a circuit breaker
type CircuitBreakerConfig struct {
	// MaxFailures is the number of consecutive failures before opening the circuit
	MaxFailures int `json:"max_failures"`

	// CallTimeout is the timeout for individual calls
	CallTimeout time.Duration `json:"call_timeout"`

	// ResetTimeout is the time to wait before transitioning from open to half-open
	ResetTimeout time.Duration `json:"reset_timeout"`

	// HalfOpenMaxCalls is the maximum number of calls allowed in half-open state
	HalfOpenMaxCalls int `json:"half_open_max_calls"`

	// ReadyToTrip is a callback that can veto the transition to open state
	ReadyToTrip func() bool `json:"-"`

	// OnStateChange is a callback for state transitions
	OnStateChange func(from, to CircuitState) `json:"-"`

	// Embedded circuit breaker instance for backward compatibility
	*CircuitBreaker
}

// DefaultCircuitBreakerConfig returns default circuit breaker configuration
func DefaultCircuitBreakerConfig() *CircuitBreakerConfig {
	return &CircuitBreakerConfig{
		MaxFailures:      5,                // 5 consecutive failures trip the circuit
		CallTimeout:      30 * time.Second, // 30 second timeout per call
		ResetTimeout:     60 * time.Second, // 1 minute before attempting recovery
		HalfOpenMaxCalls: 3,                // Allow 3 calls to test recovery
	}
}

// CircuitBreakerStats represents the statistics collected by the circuit breaker
type CircuitBreakerStats struct {
	// TotalRequests is the total number of requests made
	TotalRequests uint64 `json:"total_requests"`

	// TotalSuccesses is the total number of successful requests
	TotalSuccesses uint64 `json:"total_successes"`

	// TotalFailures is the total number of failed requests
	TotalFailures uint64 `json:"total_failures"`

	// ConsecutiveFailures is the current streak of consecutive failures
	ConsecutiveFailures int `json:"consecutive_failures"`

	// HalfOpenCalls is the number of calls made in half-open state
	HalfOpenCalls int `json:"half_open_calls"`

	// LastFailureTime is when the last failure occurred
	LastFailureTime time.Time `json:"last_failure_time"`

	// LastSuccessTime is when the last success occurred
	LastSuccessTime time.Time `json:"last_success_time"`

	// StateTransitions is the total number of state transitions
	StateTransitions uint64 `json:"state_transitions"`
}

// CircuitBreaker implements the circuit breaker pattern for fault tolerance
type CircuitBreaker struct {
	config *CircuitBreakerConfig
	stats  CircuitBreakerStats
	state  CircuitState
	mu     sync.RWMutex

	// Timer for reset timeout
	resetTimer *time.Timer
}

// NewCircuitBreaker creates a new circuit breaker with default configuration
func NewCircuitBreaker() *CircuitBreaker {
	config := &CircuitBreakerConfig{
		MaxFailures:      5,
		CallTimeout:      30 * time.Second,
		ResetTimeout:     60 * time.Second,
		HalfOpenMaxCalls: 3,
	}
	return NewCircuitBreakerWithConfig(config)
}

// NewCircuitBreakerWithConfig creates a new circuit breaker with custom configuration
func NewCircuitBreakerWithConfig(config *CircuitBreakerConfig) *CircuitBreaker {
	if config == nil {
		config = DefaultCircuitBreakerConfig()
	}

	// Validate and set reasonable defaults
	if config.MaxFailures <= 0 {
		config.MaxFailures = 5
	}
	if config.CallTimeout <= 0 {
		config.CallTimeout = 30 * time.Second
	}
	if config.ResetTimeout <= 0 {
		config.ResetTimeout = 60 * time.Second
	}
	if config.HalfOpenMaxCalls <= 0 {
		config.HalfOpenMaxCalls = 3
	}

	return &CircuitBreaker{
		config: config,
		stats: CircuitBreakerStats{
			ConsecutiveFailures: 0,
			HalfOpenCalls:       0,
		},
		state: CircuitStateClosed,
	}
}

// State returns the current state of the circuit breaker
func (cb *CircuitBreaker) State() CircuitState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// Stats returns a snapshot of the current statistics
func (cb *CircuitBreaker) Stats() CircuitBreakerStats {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.stats
}

// Execute runs the given function through the circuit breaker
func (cb *CircuitBreaker) Execute(fn func() error) error {
	if !cb.beginRequest() {
		return &CircuitBreakerOpenError{
			State: cb.State(),
		}
	}

	startTime := time.Now()
	err := fn()
	duration := time.Since(startTime)

	cb.recordResult(err, duration)

	return err
}

func (cb *CircuitBreaker) beginRequest() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case CircuitStateClosed:
		return true
	case CircuitStateOpen:
		return false
	case CircuitStateHalfOpen:
		if cb.stats.HalfOpenCalls >= cb.config.HalfOpenMaxCalls {
			return false
		}
		cb.stats.HalfOpenCalls++
		return true
	default:
		return false
	}
}

// ExecuteWithTimeout runs the given function with timeout and circuit breaker protection.
// The context passed to fn carries the configured CallTimeout deadline; fn must respect
// ctx.Done() to guarantee no goroutine leaks when the deadline fires.
func (cb *CircuitBreaker) ExecuteWithTimeout(ctx context.Context, fn func(context.Context) error) error {
	if !cb.beginRequest() {
		return &CircuitBreakerOpenError{State: cb.State()}
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, cb.config.CallTimeout)
	defer cancel()

	startTime := time.Now()
	err := fn(timeoutCtx)
	duration := time.Since(startTime)

	// If our deadline fired and the function returned a deadline exceeded error,
	// treat it as a circuit-breaker timeout rather than a generic failure.
	if errors.Is(err, context.DeadlineExceeded) && timeoutCtx.Err() == context.DeadlineExceeded {
		cb.recordResult(errors.New("timeout"), duration)
		return &CircuitBreakerTimeoutError{
			Timeout:  cb.config.CallTimeout,
			Duration: duration,
		}
	}

	cb.recordResult(err, duration)
	return err
}

// CanExecute checks if a request would be allowed right now
func (cb *CircuitBreaker) CanExecute() bool {
	return cb.allowRequest()
}

// Reset resets the circuit breaker to closed state
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	oldState := cb.state

	// Stop any pending reset timer
	if cb.resetTimer != nil {
		cb.resetTimer.Stop()
		cb.resetTimer = nil
	}

	// Reset state and stats
	cb.state = CircuitStateClosed
	cb.stats.ConsecutiveFailures = 0
	cb.stats.HalfOpenCalls = 0
	cb.stats.StateTransitions++

	// Call state change callback
	if cb.config.OnStateChange != nil && oldState != cb.state {
		cb.config.OnStateChange(oldState, cb.state)
	}
}

// allowRequest determines if a request should be allowed
func (cb *CircuitBreaker) allowRequest() bool {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	switch cb.state {
	case CircuitStateClosed:
		return true
	case CircuitStateOpen:
		return false
	case CircuitStateHalfOpen:
		// Allow limited requests in half-open state
		return cb.stats.HalfOpenCalls < cb.config.HalfOpenMaxCalls
	default:
		return false
	}
}

// recordResult records the result of an execution
func (cb *CircuitBreaker) recordResult(err error, duration time.Duration) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.stats.TotalRequests++

	if err == nil {
		cb.recordSuccessLocked(duration)
	} else {
		cb.recordFailureLocked()
	}

	// Check for state transitions
	cb.maybeTransitionToOpenLocked()
	cb.maybeTransitionToClosedLocked()
	cb.maybeTransitionToHalfOpenLocked()
}

// recordSuccessLocked records a successful result
func (cb *CircuitBreaker) recordSuccessLocked(duration time.Duration) {
	cb.stats.TotalSuccesses++
	cb.stats.ConsecutiveFailures = 0
	cb.stats.LastSuccessTime = time.Now()
}

// recordFailureLocked records a failed result
func (cb *CircuitBreaker) recordFailureLocked() {
	cb.stats.TotalFailures++
	cb.stats.ConsecutiveFailures++
	cb.stats.LastFailureTime = time.Now()
}

// maybeTransitionToOpenLocked transitions to open state if conditions are met
func (cb *CircuitBreaker) maybeTransitionToOpenLocked() {
	if cb.state == CircuitStateOpen {
		return
	}

	// Check if we should trip
	shouldTrip := cb.stats.ConsecutiveFailures >= cb.config.MaxFailures

	// Allow veto via callback
	if shouldTrip && cb.config.ReadyToTrip != nil {
		shouldTrip = cb.config.ReadyToTrip()
	}

	if shouldTrip {
		oldState := cb.state
		cb.state = CircuitStateOpen
		cb.stats.StateTransitions++

		// Schedule reset
		cb.scheduleReset()

		// Call state change callback
		if cb.config.OnStateChange != nil && oldState != cb.state {
			cb.config.OnStateChange(oldState, cb.state)
		}
	}
}

// maybeTransitionToClosedLocked transitions to closed state if conditions are met
func (cb *CircuitBreaker) maybeTransitionToClosedLocked() {
	if cb.state != CircuitStateHalfOpen {
		return
	}

	// If we had successful calls in half-open, transition to closed
	if cb.stats.ConsecutiveFailures == 0 && cb.stats.HalfOpenCalls > 0 {
		oldState := cb.state
		cb.state = CircuitStateClosed
		cb.stats.HalfOpenCalls = 0
		cb.stats.StateTransitions++

		// Stop reset timer if running
		if cb.resetTimer != nil {
			cb.resetTimer.Stop()
			cb.resetTimer = nil
		}

		// Call state change callback
		if cb.config.OnStateChange != nil && oldState != cb.state {
			cb.config.OnStateChange(oldState, cb.state)
		}
	}
}

// maybeTransitionToHalfOpenLocked transitions to half-open state from open
func (cb *CircuitBreaker) maybeTransitionToHalfOpenLocked() {
	if cb.state != CircuitStateOpen {
		return
	}

	// Check if enough time has passed to attempt recovery
	if time.Since(cb.stats.LastFailureTime) >= cb.config.ResetTimeout {
		oldState := cb.state
		cb.state = CircuitStateHalfOpen
		cb.stats.HalfOpenCalls = 0
		cb.stats.StateTransitions++

		// Stop reset timer
		if cb.resetTimer != nil {
			cb.resetTimer.Stop()
			cb.resetTimer = nil
		}

		// Call state change callback
		if cb.config.OnStateChange != nil && oldState != cb.state {
			cb.config.OnStateChange(oldState, cb.state)
		}
	}
}

// scheduleReset schedules a transition from open to half-open
func (cb *CircuitBreaker) scheduleReset() {
	if cb.resetTimer != nil {
		cb.resetTimer.Stop()
	}

	cb.resetTimer = time.AfterFunc(cb.config.ResetTimeout, func() {
		cb.mu.Lock()
		defer cb.mu.Unlock()

		// Only transition if we're still in open state
		if cb.state == CircuitStateOpen {
			oldState := cb.state
			cb.state = CircuitStateHalfOpen
			cb.stats.HalfOpenCalls = 0
			cb.stats.StateTransitions++

			// Call state change callback
			if cb.config.OnStateChange != nil && oldState != cb.state {
				cb.config.OnStateChange(oldState, cb.state)
			}
		}

		cb.resetTimer = nil
	})
}

// CircuitBreakerOpenError is returned when the circuit is open
type CircuitBreakerOpenError struct {
	State CircuitState
}

func (e *CircuitBreakerOpenError) Error() string {
	return "circuit breaker is open - requests are blocked"
}

// CircuitBreakerTimeoutError is returned when a call times out
type CircuitBreakerTimeoutError struct {
	Timeout  time.Duration
	Duration time.Duration
}

func (e *CircuitBreakerTimeoutError) Error() string {
	return "circuit breaker call timeout"
}

// IsCircuitBreakerOpenError checks if an error is a circuit breaker open error
func IsCircuitBreakerOpenError(err error) bool {
	_, ok := err.(*CircuitBreakerOpenError)
	return ok
}

// IsCircuitBreakerTimeoutError checks if an error is a circuit breaker timeout error
func IsCircuitBreakerTimeoutError(err error) bool {
	_, ok := err.(*CircuitBreakerTimeoutError)
	return ok
}

// RecordSuccess records a successful call (compatibility method for engine integration)
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.recordSuccessLocked(0) // No duration available in compatibility mode
}

// RecordFailure records a failed call (compatibility method for engine integration)
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.recordFailureLocked()
}

// IsAvailable checks if the circuit breaker allows calls (compatibility method for engine integration)
func (cb *CircuitBreaker) IsAvailable() bool {
	return cb.allowRequest()
}
