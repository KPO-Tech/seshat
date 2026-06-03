package monitoring

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// System provides centralized monitoring for Nexus Engine
type System struct {
	// Metrics for different subsystems
	// API Requests
	apiRequestsTotal       *Counter
	apiRequestsSuccess     *Counter
	apiRequestsFailure     *Counter
	apiLatency             *Timer
	apiRateLimitHits       *Counter
	apiCircuitBreakerTrips *Counter

	// Tool Execution
	toolCallsTotal        *Counter
	toolCallsSuccess      *Counter
	toolCallsFailure      *Counter
	toolLatency           *Timer
	toolPermissionDenials *Counter

	// Query Loop
	queryTurnsTotal      *Counter
	queryIterationsTotal *Counter
	queryLatency         *Timer
	queryErrors          *Counter

	// Circuit Breaker
	circuitBreakerStateTransitions *Counter
	circuitBreakerCurrentState     *Gauge

	// Prompt Cache Metrics (Anthropic)
	cacheReadTokens     *Counter
	cacheCreationTokens *Counter
	cacheHits           *Counter
	cacheWrites         *Counter

	// System Metrics
	activeSessions    *Gauge
	activeConnections *Gauge
	memoryUsage       *Gauge
	totalErrors       *Counter

	// Logger
	logger *Logger

	// Registry of all metrics
	allMetrics map[string]interface{}
	mu         sync.RWMutex
}

// NewSystem creates a new monitoring system
func NewSystem(logger *Logger) *System {
	if logger == nil {
		logger = NewLogger()
	}

	system := &System{
		logger:     logger,
		allMetrics: make(map[string]interface{}),
	}

	// Initialize API metrics
	system.apiRequestsTotal = NewCounter("api_requests_total", map[string]string{
		"subsystem": "api",
		"type":      "requests",
	})
	system.apiRequestsSuccess = NewCounter("api_requests_success", map[string]string{
		"subsystem": "api",
		"type":      "success",
	})
	system.apiRequestsFailure = NewCounter("api_requests_failure", map[string]string{
		"subsystem": "api",
		"type":      "failure",
	})
	system.apiLatency = NewTimer("api_latency_ms", map[string]string{
		"subsystem": "api",
		"type":      "latency",
	})
	system.apiRateLimitHits = NewCounter("api_rate_limit_hits", map[string]string{
		"subsystem": "api",
		"type":      "rate_limit",
	})
	system.apiCircuitBreakerTrips = NewCounter("api_circuit_breaker_trips", map[string]string{
		"subsystem": "api",
		"type":      "circuit_breaker",
	})

	// Initialize tool metrics
	system.toolCallsTotal = NewCounter("tool_calls_total", map[string]string{
		"subsystem": "tools",
		"type":      "calls",
	})
	system.toolCallsSuccess = NewCounter("tool_calls_success", map[string]string{
		"subsystem": "tools",
		"type":      "success",
	})
	system.toolCallsFailure = NewCounter("tool_calls_failure", map[string]string{
		"subsystem": "tools",
		"type":      "failure",
	})
	system.toolLatency = NewTimer("tool_latency_ms", map[string]string{
		"subsystem": "tools",
		"type":      "latency",
	})
	system.toolPermissionDenials = NewCounter("tool_permission_denials", map[string]string{
		"subsystem": "tools",
		"type":      "permissions",
	})

	// Initialize query loop metrics
	system.queryTurnsTotal = NewCounter("query_turns_total", map[string]string{
		"subsystem": "query",
		"type":      "turns",
	})
	system.queryIterationsTotal = NewCounter("query_iterations_total", map[string]string{
		"subsystem": "query",
		"type":      "iterations",
	})
	system.queryLatency = NewTimer("query_latency_ms", map[string]string{
		"subsystem": "query",
		"type":      "latency",
	})
	system.queryErrors = NewCounter("query_errors", map[string]string{
		"subsystem": "query",
		"type":      "errors",
	})

	// Initialize circuit breaker metrics
	system.circuitBreakerStateTransitions = NewCounter("circuit_breaker_state_transitions", map[string]string{
		"subsystem": "circuit_breaker",
		"type":      "transitions",
	})
	system.circuitBreakerCurrentState = NewGauge("circuit_breaker_current_state", map[string]string{
		"subsystem": "circuit_breaker",
		"type":      "state",
	})

	// Initialize prompt cache metrics
	system.cacheReadTokens = NewCounter("prompt_cache_read_tokens_total", map[string]string{
		"subsystem": "cache",
		"type":      "read",
	})
	system.cacheCreationTokens = NewCounter("prompt_cache_creation_tokens_total", map[string]string{
		"subsystem": "cache",
		"type":      "creation",
	})
	system.cacheHits = NewCounter("prompt_cache_hits_total", map[string]string{
		"subsystem": "cache",
		"type":      "hits",
	})
	system.cacheWrites = NewCounter("prompt_cache_writes_total", map[string]string{
		"subsystem": "cache",
		"type":      "writes",
	})

	// Initialize system metrics
	system.activeSessions = NewGauge("active_sessions", map[string]string{
		"subsystem": "system",
		"type":      "sessions",
	})
	system.activeConnections = NewGauge("active_connections", map[string]string{
		"subsystem": "system",
		"type":      "connections",
	})
	system.memoryUsage = NewGauge("memory_usage_mb", map[string]string{
		"subsystem": "system",
		"type":      "memory",
	})
	system.totalErrors = NewCounter("total_errors", map[string]string{
		"subsystem": "system",
		"type":      "errors",
	})

	// Register all metrics
	system.registerMetrics()

	return system
}

// registerMetrics registers all metrics in system
func (m *System) registerMetrics() {
	m.mu.Lock()
	defer m.mu.Unlock()

	metrics := []interface{}{
		m.apiRequestsTotal, m.apiRequestsSuccess, m.apiRequestsFailure,
		m.apiLatency, m.apiRateLimitHits, m.apiCircuitBreakerTrips,
		m.toolCallsTotal, m.toolCallsSuccess, m.toolCallsFailure,
		m.toolLatency, m.toolPermissionDenials,
		m.queryTurnsTotal, m.queryIterationsTotal, m.queryLatency, m.queryErrors,
		m.circuitBreakerStateTransitions, m.circuitBreakerCurrentState,
		m.cacheReadTokens, m.cacheCreationTokens, m.cacheHits, m.cacheWrites,
		m.activeSessions, m.activeConnections, m.memoryUsage, m.totalErrors,
	}

	for _, metric := range metrics {
		if counter, ok := metric.(*Counter); ok {
			m.allMetrics[counter.name] = counter
		} else if gauge, ok := metric.(*Gauge); ok {
			m.allMetrics[gauge.name] = gauge
		} else if timer, ok := metric.(*Timer); ok {
			m.allMetrics[timer.name] = timer
		}
	}
}

// GetLogger returns logger
func (m *System) GetLogger() *Logger {
	return m.logger
}

// SetLogger sets logger
func (m *System) SetLogger(logger *Logger) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.logger = logger
}

// ============================================================================
// API Metrics
// ============================================================================

// RecordAPIRequest records an API request
func (m *System) RecordAPIRequest() {
	m.apiRequestsTotal.Inc()

	// Add tool name as label
	if m.logger != nil {
		m.logger.Info("API request", map[string]interface{}{
			"request_count": m.apiRequestsTotal.Value(),
		})
	}
}

// RecordAPISuccess records a successful API request
func (m *System) RecordAPISuccess(duration time.Duration) {
	m.apiRequestsSuccess.Inc()
	m.apiLatency.Record(duration)

	if m.logger != nil {
		m.logger.Info("API request succeeded", map[string]interface{}{
			"success_count": m.apiRequestsSuccess.Value(),
			"duration_ms":   duration.Milliseconds(),
		})
	}
}

// RecordAPIFailure records a failed API request
func (m *System) RecordAPIFailure(err error, duration time.Duration) {
	m.apiRequestsFailure.Inc()
	m.apiLatency.Record(duration)
	m.totalErrors.Inc()

	if m.logger != nil {
		m.logger.Error("API request failed", map[string]interface{}{
			"failure_count": m.apiRequestsFailure.Value(),
			"error":         err.Error(),
			"duration_ms":   duration.Milliseconds(),
		})
	}
}

// RecordCacheStats records Anthropic prompt cache statistics from a response.
// readTokens is cache_read_input_tokens, creationTokens is cache_creation_input_tokens.
func (m *System) RecordCacheStats(readTokens, creationTokens int) {
	if readTokens > 0 {
		m.cacheReadTokens.Add(uint64(readTokens))
		m.cacheHits.Inc()
	}
	if creationTokens > 0 {
		m.cacheCreationTokens.Add(uint64(creationTokens))
		m.cacheWrites.Inc()
	}
	if m.logger != nil && (readTokens > 0 || creationTokens > 0) {
		m.logger.Info("Prompt cache stats", map[string]interface{}{
			"cache_read_tokens":     readTokens,
			"cache_creation_tokens": creationTokens,
			"total_cache_hits":      m.cacheHits.Value(),
			"total_cache_writes":    m.cacheWrites.Value(),
		})
	}
}

// RecordAPIRateLimit records a rate limit hit
func (m *System) RecordAPIRateLimit() {
	m.apiRateLimitHits.Inc()

	if m.logger != nil {
		m.logger.Warn("API rate limit hit", nil)
	}
}

// RecordCircuitBreakerTrip records a circuit breaker trip
func (m *System) RecordCircuitBreakerTrip(provider string) {
	m.apiCircuitBreakerTrips.Inc()

	if m.logger != nil {
		m.logger.Info("Circuit breaker tripped", map[string]interface{}{
			"provider":    provider,
			"trips_count": m.apiCircuitBreakerTrips.Value(),
		})
	}
}

// ============================================================================
// Tool Metrics
// ============================================================================

// RecordToolCall records a tool call
func (m *System) RecordToolCall(toolName string) {
	m.toolCallsTotal.Inc()

	// Add tool name as label
	if m.logger != nil {
		m.logger.Info("Tool called", map[string]interface{}{
			"tool_name":  toolName,
			"call_count": m.toolCallsTotal.Value(),
		})
	}
}

// RecordToolSuccess records a successful tool call
func (m *System) RecordToolSuccess(toolName string, duration time.Duration) {
	m.toolCallsSuccess.Inc()
	m.toolLatency.Record(duration)

	if m.logger != nil {
		m.logger.Info("Tool call succeeded", map[string]interface{}{
			"tool_name":     toolName,
			"success_count": m.toolCallsSuccess.Value(),
			"duration_ms":   duration.Milliseconds(),
		})
	}
}

// RecordToolFailure records a failed tool call
func (m *System) RecordToolFailure(toolName string, err error, duration time.Duration) {
	m.toolCallsFailure.Inc()
	m.toolLatency.Record(duration)
	m.totalErrors.Inc()

	if m.logger != nil {
		m.logger.Error("Tool execution failed", map[string]interface{}{
			"tool_name":     toolName,
			"failure_count": m.toolCallsFailure.Value(),
			"error":         err.Error(),
			"duration_ms":   duration.Milliseconds(),
		})
	}
}

// RecordToolPermissionDenial records a permission denial
func (m *System) RecordToolPermissionDenial(toolName string) {
	m.toolPermissionDenials.Inc()

	if m.logger != nil {
		m.logger.Warn("Tool permission denied", map[string]interface{}{
			"tool_name":    toolName,
			"denial_count": m.toolPermissionDenials.Value(),
		})
	}
}

// ============================================================================
// Query Loop Metrics
// ============================================================================

// RecordQueryTurn records a query turn
func (m *System) RecordQueryTurn() {
	m.queryTurnsTotal.Inc()

	if m.logger != nil {
		m.logger.Info("Query turn started", map[string]interface{}{
			"turn_count": m.queryTurnsTotal.Value(),
		})
	}
}

// RecordQueryIteration records a query loop iteration
func (m *System) RecordQueryIteration() {
	m.queryIterationsTotal.Inc()

	if m.logger != nil {
		m.logger.Debug("Query iteration", map[string]interface{}{
			"iteration_count": m.queryIterationsTotal.Value(),
		})
	}
}

// RecordQuerySuccess records a successful query turn
func (m *System) RecordQuerySuccess(duration time.Duration) {
	m.queryLatency.Record(duration)

	if m.logger != nil {
		m.logger.Info("Query turn completed successfully", map[string]interface{}{
			"duration_ms": duration.Milliseconds(),
		})
	}
}

// RecordQueryFailure records a failed query turn
func (m *System) RecordQueryFailure(err error, duration time.Duration) {
	m.queryErrors.Inc()
	m.queryLatency.Record(duration)
	m.totalErrors.Inc()

	if m.logger != nil {
		m.logger.Error("Query turn failed", map[string]interface{}{
			"error":       err.Error(),
			"duration_ms": duration.Milliseconds(),
		})
	}
}

// ============================================================================
// Circuit Breaker Metrics
// ============================================================================

// RecordCircuitBreakerStateTransition records a state transition
func (m *System) RecordCircuitBreakerStateTransition(from, to string, reason string) {
	m.circuitBreakerStateTransitions.Inc()

	if m.logger != nil {
		m.logger.Info("Circuit breaker state changed", map[string]interface{}{
			"from":   from,
			"to":     to,
			"reason": reason,
		})
	}
}

// SetCircuitBreakerState updates current circuit breaker state
func (m *System) SetCircuitBreakerState(state string) {
	var stateValue float64
	switch state {
	case "closed":
		stateValue = 0
	case "open":
		stateValue = 1
	case "half-open":
		stateValue = 2
	}
	m.circuitBreakerCurrentState.Set(stateValue)
}

// ============================================================================
// System Metrics
// ============================================================================

// UpdateActiveSessions updates count of active sessions
func (m *System) UpdateActiveSessions(count float64) {
	m.activeSessions.Set(count)

	if m.logger != nil {
		m.logger.Debug("Active sessions updated", map[string]interface{}{
			"count": count,
		})
	}
}

// UpdateActiveConnections updates count of active connections
func (m *System) UpdateActiveConnections(count float64) {
	m.activeConnections.Set(count)

	if m.logger != nil {
		m.logger.Debug("Active connections updated", map[string]interface{}{
			"count": count,
		})
	}
}

// UpdateMemoryUsage updates memory usage
func (m *System) UpdateMemoryUsage(mb float64) {
	m.memoryUsage.Set(mb)

	if m.logger != nil {
		m.logger.Debug("Memory usage updated", map[string]interface{}{
			"memory_mb": mb,
		})
	}
}

// RecordError records a general error
func (m *System) RecordError(err error, context string) {
	m.totalErrors.Inc()

	if m.logger != nil {
		m.logger.Error("Error recorded", map[string]interface{}{
			"context": context,
			"error":   err.Error(),
		})
	}
}

// ============================================================================
// Metrics Export
// ============================================================================

// GetAllMetrics returns all current metric values
func (m *System) GetAllMetrics() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	metrics := make(map[string]interface{})

	for name, metric := range m.allMetrics {
		switch m := metric.(type) {
		case *Counter:
			metrics[name] = m.Value()
		case *Gauge:
			metrics[name] = m.Value()
		case *Timer:
			metrics[name] = map[string]interface{}{
				"count":   m.Count(),
				"average": m.Average(),
				"p50":     m.Percentile(50).Milliseconds(),
				"p95":     m.Percentile(95).Milliseconds(),
				"p99":     m.Percentile(99).Milliseconds(),
			}
		}
	}

	return metrics
}

// GetMetricsSnapshot returns a snapshot of all metrics in format
func (m *System) GetMetricsSnapshot(format string) (string, error) {
	if format == "prometheus" {
		return m.formatPrometheus()
	}
	metrics := m.GetAllMetrics()
	if format == "json" {
		return formatMetricsJSON(metrics), nil
	}
	return formatMetricsText(metrics), nil
}

// formatMetricsJSON formats metrics as JSON
func formatMetricsJSON(metrics map[string]interface{}) string {
	data, err := json.Marshal(metrics)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(data)
}

// formatMetricsText formats metrics as plain text
func formatMetricsText(metrics map[string]interface{}) string {
	result := ""
	for key, value := range metrics {
		result += fmt.Sprintf("%s: %v\n", key, value)
	}
	return result
}

// formatPrometheus emits proper Prometheus text exposition format (v0.0.4).
// Counters → TYPE counter. Gauges → TYPE gauge. Timers → TYPE summary with
// p50/p95/p99 quantiles plus _count and _sum.
func (m *System) formatPrometheus() (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var sb strings.Builder

	// Sort names for deterministic output.
	names := make([]string, 0, len(m.allMetrics))
	for name := range m.allMetrics {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		metric := m.allMetrics[name]
		switch v := metric.(type) {
		case *Counter:
			labelStr := prometheusLabels(v.labels)
			fmt.Fprintf(&sb, "# TYPE %s counter\n", name)
			fmt.Fprintf(&sb, "%s%s %d\n", name, labelStr, v.Value())

		case *Gauge:
			labelStr := prometheusLabels(v.labels)
			fmt.Fprintf(&sb, "# TYPE %s gauge\n", name)
			fmt.Fprintf(&sb, "%s%s %g\n", name, labelStr, v.Value())

		case *Timer:
			h := v.histogram
			baseLabels := prometheusLabels(h.labels)
			fmt.Fprintf(&sb, "# TYPE %s summary\n", name)
			for _, q := range []float64{0.5, 0.95, 0.99} {
				qLabels := prometheusLabelsWithExtra(h.labels, "quantile", fmt.Sprintf("%g", q))
				// h.Percentile returns float64 milliseconds.
				fmt.Fprintf(&sb, "%s%s %g\n", name, qLabels, h.Percentile(q))
			}
			fmt.Fprintf(&sb, "%s_sum%s %g\n", name, baseLabels, h.Sum())
			fmt.Fprintf(&sb, "%s_count%s %d\n", name, baseLabels, h.Count())
		}
	}

	return sb.String(), nil
}

// prometheusLabels renders a label map as {key="val",...} or "" if empty.
func prometheusLabels(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	pairs := make([]string, 0, len(keys))
	for _, k := range keys {
		pairs = append(pairs, fmt.Sprintf("%s=%q", k, labels[k]))
	}
	return "{" + strings.Join(pairs, ",") + "}"
}

// prometheusLabelsWithExtra adds one extra label to an existing label map.
func prometheusLabelsWithExtra(labels map[string]string, key, value string) string {
	merged := make(map[string]string, len(labels)+1)
	for k, v := range labels {
		merged[k] = v
	}
	merged[key] = value
	return prometheusLabels(merged)
}

// ResetAllMetrics resets all metrics to zero
func (m *System) ResetAllMetrics() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, metric := range m.allMetrics {
		switch m := metric.(type) {
		case *Counter:
			m.Reset()
		case *Gauge:
			m.Set(0)
		case *Timer:
			m.Reset()
		}
	}

	if m.logger != nil {
		m.logger.Info("All metrics reset", nil)
	}
}

// Close cleans up monitoring system resources
func (m *System) Close() error {
	// Clean up any resources
	// Currently nothing to clean up, but kept for future use
	return nil
}
