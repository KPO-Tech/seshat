package monitoring

import (
	"context"
	"github.com/stretchr/testify/assert"
	"os"
	"testing"
	"time"
)

func TestDefaultLoggerConfig(t *testing.T) {
	config := DefaultLoggerConfig()

	assert.NotNil(t, config)
	assert.Equal(t, LogLevelInfo, config.Level)
	assert.Equal(t, "stdout", config.Output)
	assert.Equal(t, "text", config.Format)
}

func TestNewLogger(t *testing.T) {
	logger := NewLogger()

	assert.NotNil(t, logger)
	assert.NotNil(t, logger.output)
}

func TestNewLoggerWithConfig(t *testing.T) {
	config := &LoggerConfig{
		Level:  LogLevelDebug,
		Output: "stdout",
		Format: "text",
	}

	logger := NewLoggerWithConfig(config)

	assert.NotNil(t, logger)
	assert.Equal(t, LogLevelDebug, logger.level)
}

func TestLoggerSetLevel(t *testing.T) {
	logger := NewLogger()
	logger.SetLevel(LogLevelDebug)

	assert.Equal(t, LogLevelDebug, logger.level)
}

func TestLoggerSetContext(t *testing.T) {
	config := &LoggerConfig{
		ContextFields: []string{"session_id", "turn_id"},
	}
	logger := NewLoggerWithConfig(config)

	ctx := context.Background()
	ctx = context.WithValue(ctx, "session_id", "test-session-123")
	ctx = context.WithValue(ctx, "turn_id", "turn-456")

	context := logger.SetContext(ctx)

	assert.Contains(t, context, "session_id")
	assert.Equal(t, "test-session-123", context["session_id"])
	assert.Contains(t, context, "turn_id")
	assert.Equal(t, "turn-456", context["turn_id"])
}

func TestLoggerShouldLog(t *testing.T) {
	logger := NewLogger()
	logger.SetLevel(LogLevelInfo)

	assert.True(t, logger.shouldLog(LogLevelInfo))
	assert.True(t, logger.shouldLog(LogLevelWarn))
	assert.True(t, logger.shouldLog(LogLevelError))
	assert.True(t, logger.shouldLog(LogLevelFatal))
	assert.False(t, logger.shouldLog(LogLevelDebug))
}

func TestLoggerDebug(t *testing.T) {
	config := &LoggerConfig{
		Level:  LogLevelDebug,
		Output: "stdout",
		Format: "text",
	}
	logger := NewLoggerWithConfig(config)
	logger.SetLevel(LogLevelDebug)

	// Redirect stdout to buffer
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	defer func() {
		os.Stdout = oldStdout
		w.Close()
	}()

	logger.Debug("test debug message", nil)

	w.Close()
	os.Stdout = oldStdout

	// Restore stdout
	_ = r.Close()
}

func TestLoggerInfo(t *testing.T) {
	logger := NewLogger()
	logger.SetLevel(LogLevelInfo)

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	defer func() {
		os.Stdout = oldStdout
	}()

	logger.Info("test info message", nil)
	logger.InfoWithContext(context.Background(), "context info message", nil)

	w.Close()
	os.Stdout = oldStdout
	_ = r.Close()
}

func TestLoggerWarn(t *testing.T) {
	logger := NewLogger()
	logger.SetLevel(LogLevelWarn)

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	defer func() {
		os.Stdout = oldStdout
	}()

	logger.Warn("test warning message", nil)

	w.Close()
	os.Stdout = oldStdout
	_ = r.Close()
}

func TestLoggerError(t *testing.T) {
	logger := NewLogger()
	logger.SetLevel(LogLevelError)

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	defer func() {
		os.Stdout = oldStdout
	}()

	logger.Error("test error message", nil)
	logger.ErrorWithContext(context.Background(), "context error message", nil)

	w.Close()
	os.Stdout = oldStdout
	_ = r.Close()
}

func TestLoggerLogRequest(t *testing.T) {
	logger := NewLogger()
	logger.SetLevel(LogLevelInfo)

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	defer func() {
		os.Stdout = oldStdout
	}()

	ctx := context.Background()
	logger.LogRequest(ctx, "POST", "https://api.example.com/v1/messages", 200, 150*time.Millisecond)

	w.Close()
	os.Stdout = oldStdout
	_ = r.Close()
}

func TestLoggerLogToolExecution(t *testing.T) {
	logger := NewLogger()
	logger.SetLevel(LogLevelInfo)

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	defer func() {
		os.Stdout = oldStdout
	}()

	ctx := context.Background()
	logger.LogToolExecution(ctx, "test_tool", 50*time.Millisecond, true)
	logger.LogToolExecution(ctx, "failing_tool", 100*time.Millisecond, false)

	w.Close()
	os.Stdout = oldStdout
	_ = r.Close()
}

func TestLoggerLogCircuitBreakerState(t *testing.T) {
	logger := NewLogger()
	logger.SetLevel(LogLevelInfo)

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	defer func() {
		os.Stdout = oldStdout
	}()

	ctx := context.Background()
	logger.LogCircuitBreakerState(ctx, "closed", "open", "failure threshold reached")

	w.Close()
	os.Stdout = oldStdout
	_ = r.Close()
}

func TestLoggerJSONFormat(t *testing.T) {
	config := &LoggerConfig{
		Level:  LogLevelInfo,
		Output: "stdout",
		Format: "json",
	}
	logger := NewLoggerWithConfig(config)

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	defer func() {
		os.Stdout = oldStdout
	}()

	logger.Info("test json message", nil)

	w.Close()
	os.Stdout = oldStdout
	_ = r.Close()
}

func TestLoggerTextFormat(t *testing.T) {
	config := &LoggerConfig{
		Level:  LogLevelInfo,
		Output: "stdout",
		Format: "text",
	}
	logger := NewLoggerWithConfig(config)

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	defer func() {
		os.Stdout = oldStdout
	}()

	logger.Info("test text message", nil)

	w.Close()
	os.Stdout = oldStdout
	_ = r.Close()
}

func TestLoggerWithFileOutput(t *testing.T) {
	tempFile := "/tmp/test_nexus.log"
	defer os.Remove(tempFile)

	config := &LoggerConfig{
		Level:    LogLevelInfo,
		Output:   "file",
		FilePath: tempFile,
		Format:   "text",
	}
	logger := NewLoggerWithConfig(config)

	logger.Info("test file message", nil)

	// Read the file
	content, err := os.ReadFile(tempFile)
	assert.NoError(t, err)
	assert.Contains(t, string(content), "test file message")
}

func TestLoggerContextFields(t *testing.T) {
	config := &LoggerConfig{
		ContextFields: []string{"session_id", "turn_id", "request_id"},
	}
	logger := NewLoggerWithConfig(config)

	ctx := context.Background()
	ctx = context.WithValue(ctx, "session_id", "session-123")
	ctx = context.WithValue(ctx, "turn_id", "turn-456")
	ctx = context.WithValue(ctx, "request_id", "req-789")

	context := logger.SetContext(ctx)

	assert.Equal(t, "session-123", context["session_id"])
	assert.Equal(t, "turn-456", context["turn_id"])
	assert.Equal(t, "req-789", context["request_id"])
}

func TestLoggerWithFields(t *testing.T) {
	logger := NewLogger()
	logger.SetLevel(LogLevelInfo)

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	defer func() {
		os.Stdout = oldStdout
	}()

	fields := map[string]interface{}{
		"user_id": "user-123",
		"action":  "test_action",
	}
	logger.Info("message with fields", fields)

	w.Close()
	os.Stdout = oldStdout
	_ = r.Close()
}

func TestLoggerErrorWithStack(t *testing.T) {
	logger := NewLogger()
	logger.SetLevel(LogLevelError)

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	defer func() {
		os.Stdout = oldStdout
	}()

	err := assert.AnError
	logger.ErrorWithStack("error with stack trace", err, nil)

	w.Close()
	os.Stdout = oldStdout
	_ = r.Close()
}

func TestLoggerWithDifferentLevels(t *testing.T) {
	tests := []struct {
		name      string
		setLevel  LogLevel
		testLevel LogLevel
		shouldLog bool
	}{
		{"debug level logs debug", LogLevelDebug, LogLevelDebug, true},
		{"debug level logs info", LogLevelDebug, LogLevelInfo, true},
		{"info level logs info", LogLevelInfo, LogLevelInfo, true},
		{"info level skips debug", LogLevelInfo, LogLevelDebug, false},
		{"warn level logs warn", LogLevelWarn, LogLevelWarn, true},
		{"warn level logs error", LogLevelWarn, LogLevelError, true},
		{"error level logs error", LogLevelError, LogLevelError, true},
		{"error level skips info", LogLevelError, LogLevelInfo, false},
		{"fatal level logs fatal", LogLevelFatal, LogLevelFatal, true},
		{"fatal level skips warn", LogLevelFatal, LogLevelWarn, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := NewLogger()
			logger.SetLevel(tt.setLevel)

			shouldLog := logger.shouldLog(tt.testLevel)
			assert.Equal(t, tt.shouldLog, shouldLog)
		})
	}
}

func TestLoggerFormatMessage(t *testing.T) {
	config := &LoggerConfig{
		Level:  LogLevelInfo,
		Output: "stdout",
		Format: "text",
	}
	logger := NewLoggerWithConfig(config)

	msg := "test message"
	fields := map[string]interface{}{
		"key": "value",
	}
	formatted := logger.formatMessage(LogLevelInfo, msg, nil, fields)

	assert.Contains(t, formatted, msg)
	assert.Contains(t, formatted, "fields:")
	assert.Contains(t, formatted, "value")
}

func TestLoggerWritableAfterInit(t *testing.T) {
	logger := NewLogger()
	assert.NotNil(t, logger)
	assert.NotNil(t, logger.output)
	assert.NotPanics(t, func() {
		logger.Info("writable after init", nil)
	})
}

func TestLoggerFallbackOnUnwritableFile(t *testing.T) {
	config := &LoggerConfig{
		Level:    LogLevelInfo,
		Output:   "file",
		FilePath: "/proc/nonexistent_readonly_dir/nexus_test.log",
		Format:   "text",
	}
	logger := NewLoggerWithConfig(config)
	assert.NotNil(t, logger)
	assert.NotNil(t, logger.output)
	assert.Equal(t, os.Stderr, logger.output, "should fall back to stderr when file is unwritable")
	assert.NotPanics(t, func() {
		logger.Info("fallback message", nil)
	})
}

func TestLoggerContextIntegration(t *testing.T) {
	config := &LoggerConfig{
		Level:         LogLevelInfo,
		Output:        "stdout",
		Format:        "text",
		ContextFields: []string{"session_id"},
	}
	logger := NewLoggerWithConfig(config)

	ctx := context.Background()
	ctx = context.WithValue(ctx, "session_id", "test-session")

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	defer func() {
		os.Stdout = oldStdout
	}()

	logger.InfoWithContext(ctx, "contextual message", nil)

	w.Close()
	os.Stdout = oldStdout
	_ = r.Close()
}

func TestNewCounter(t *testing.T) {
	labels := map[string]string{"subsystem": "test", "type": "counter"}
	counter := NewCounter("test_counter", labels)

	assert.NotNil(t, counter)
	assert.Equal(t, "test_counter", counter.name)
	assert.Equal(t, uint64(0), counter.Value())
}

func TestCounterAdd(t *testing.T) {
	counter := NewCounter("test_counter", nil)

	counter.Add(5)
	assert.Equal(t, uint64(5), counter.Value())

	counter.Add(3)
	assert.Equal(t, uint64(8), counter.Value())
}

func TestCounterInc(t *testing.T) {
	counter := NewCounter("test_counter", nil)

	for i := 0; i < 5; i++ {
		counter.Inc()
	}

	assert.Equal(t, uint64(5), counter.Value())
}

func TestCounterReset(t *testing.T) {
	counter := NewCounter("test_counter", nil)

	counter.Add(10)
	counter.Reset()

	assert.Equal(t, uint64(0), counter.Value())
}

func TestCounterToMetric(t *testing.T) {
	labels := map[string]string{"subsystem": "test"}
	counter := NewCounter("test_counter", labels)

	counter.Add(5)
	metric := counter.ToMetric()

	assert.Equal(t, "test_counter", metric.Name)
	assert.Equal(t, MetricTypeCounter, metric.Type)
	assert.Equal(t, float64(5), metric.Value)
	assert.Equal(t, labels, metric.Labels)
}

func TestNewGauge(t *testing.T) {
	labels := map[string]string{"subsystem": "test", "type": "gauge"}
	gauge := NewGauge("test_gauge", labels)

	assert.NotNil(t, gauge)
	assert.Equal(t, "test_gauge", gauge.name)
	assert.Equal(t, float64(0), gauge.Value())
}

func TestGaugeSet(t *testing.T) {
	gauge := NewGauge("test_gauge", nil)

	gauge.Set(42.5)
	assert.Equal(t, float64(42.5), gauge.Value())

	gauge.Set(-10.3)
	assert.Equal(t, float64(-10.3), gauge.Value())
}

func TestGaugeAdd(t *testing.T) {
	gauge := NewGauge("test_gauge", nil)

	gauge.Add(10.0)
	assert.Equal(t, float64(10.0), gauge.Value())

	gauge.Add(-3.5)
	assert.Equal(t, float64(6.5), gauge.Value())
}

func TestGaugeToMetric(t *testing.T) {
	labels := map[string]string{"subsystem": "test"}
	gauge := NewGauge("test_gauge", labels)

	gauge.Set(42.5)
	metric := gauge.ToMetric()

	assert.Equal(t, "test_gauge", metric.Name)
	assert.Equal(t, MetricTypeGauge, metric.Type)
	assert.Equal(t, float64(42.5), metric.Value)
	assert.Equal(t, labels, metric.Labels)
}

func TestNewHistogram(t *testing.T) {
	labels := map[string]string{"subsystem": "test"}
	buckets := []float64{1, 5, 10, 25, 50, 100}
	histogram := NewHistogram("test_histogram", labels, buckets)

	assert.NotNil(t, histogram)
	assert.Equal(t, "test_histogram", histogram.name)
	assert.Equal(t, uint64(0), histogram.Count())
	assert.Equal(t, float64(0), histogram.Sum())
}

func TestHistogramObserve(t *testing.T) {
	buckets := []float64{10, 20, 30}
	histogram := NewHistogram("test_histogram", nil, buckets)

	histogram.Observe(5)
	histogram.Observe(15)
	histogram.Observe(25)
	histogram.Observe(35)

	assert.Equal(t, uint64(4), histogram.Count())
	assert.Equal(t, float64(80), histogram.Sum())
}

func TestHistogramAverage(t *testing.T) {
	buckets := []float64{10, 20, 30}
	histogram := NewHistogram("test_histogram", nil, buckets)

	histogram.Observe(10)
	histogram.Observe(20)
	histogram.Observe(30)

	assert.Equal(t, float64(20), histogram.Average())
}

func TestHistogramPercentile(t *testing.T) {
	buckets := []float64{10, 20, 30, 40, 50}
	histogram := NewHistogram("test_histogram", nil, buckets)

	// Add values across buckets
	for i := 1; i <= 50; i++ {
		histogram.Observe(float64(i))
	}

	assert.Less(t, float64(0), histogram.Percentile(50))
	assert.LessOrEqual(t, float64(25), histogram.Percentile(50))
	assert.LessOrEqual(t, histogram.Percentile(95), float64(50))
}

func TestHistogramReset(t *testing.T) {
	buckets := []float64{10, 20, 30}
	histogram := NewHistogram("test_histogram", nil, buckets)

	histogram.Observe(10)
	histogram.Observe(20)
	histogram.Observe(30)

	histogram.Reset()

	assert.Equal(t, uint64(0), histogram.Count())
	assert.Equal(t, float64(0), histogram.Sum())
}

func TestHistogramToMetric(t *testing.T) {
	labels := map[string]string{"subsystem": "test"}
	buckets := []float64{10, 20, 30}
	histogram := NewHistogram("test_histogram", labels, buckets)

	histogram.Observe(10)
	histogram.Observe(20)

	metric := histogram.ToMetric()

	assert.Equal(t, "test_histogram", metric.Name)
	assert.Equal(t, MetricTypeHistogram, metric.Type)
	assert.Equal(t, float64(15), metric.Value) // Average
	assert.Equal(t, labels, metric.Labels)
}

func TestNewTimer(t *testing.T) {
	labels := map[string]string{"subsystem": "test"}
	timer := NewTimer("test_timer", labels)

	assert.NotNil(t, timer)
	assert.Equal(t, "test_timer", timer.name)
	assert.Equal(t, uint64(0), timer.Count())
}

func TestTimerRecord(t *testing.T) {
	timer := NewTimer("test_timer", nil)

	timer.Record(100 * time.Millisecond)
	timer.Record(200 * time.Millisecond)
	timer.Record(300 * time.Millisecond)

	assert.Equal(t, uint64(3), timer.Count())
	assert.Equal(t, float64(200), timer.Average())
}

func TestTimerStart(t *testing.T) {
	timer := NewTimer("test_timer", nil)

	stop := timer.Start()
	time.Sleep(50 * time.Millisecond)
	stop()

	assert.Equal(t, uint64(1), timer.Count())
	assert.Greater(t, timer.Average(), float64(40)) // Should be around 50ms
	assert.Less(t, timer.Average(), float64(100))   // But not >100ms
}

func TestTimerPercentile(t *testing.T) {
	timer := NewTimer("test_timer", nil)

	// Record various durations
	for i := 1; i <= 100; i++ {
		duration := time.Duration(i) * time.Millisecond
		timer.Record(duration)
	}

	p50 := timer.Percentile(50)
	p95 := timer.Percentile(95)
	p99 := timer.Percentile(99)

	assert.Greater(t, p50.Milliseconds(), int64(40))
	assert.Less(t, p50.Milliseconds(), int64(60))

	assert.Greater(t, p95.Milliseconds(), int64(90))
	assert.LessOrEqual(t, p95.Milliseconds(), int64(100))

	assert.Greater(t, p99.Milliseconds(), int64(95))
	assert.LessOrEqual(t, p99.Milliseconds(), int64(100))
}

func TestTimerReset(t *testing.T) {
	timer := NewTimer("test_timer", nil)

	timer.Record(100 * time.Millisecond)
	timer.Record(200 * time.Millisecond)

	timer.Reset()

	assert.Equal(t, uint64(0), timer.Count())
	assert.Equal(t, float64(0), timer.Average())
}

func TestTimerToMetric(t *testing.T) {
	labels := map[string]string{"subsystem": "test"}
	timer := NewTimer("test_timer", labels)

	timer.Record(100 * time.Millisecond)
	timer.Record(200 * time.Millisecond)

	metric := timer.ToMetric()

	assert.Equal(t, "test_timer", metric.Name)
	assert.Equal(t, MetricTypeHistogram, metric.Type)
	assert.Equal(t, float64(150), metric.Value) // Average
	assert.Equal(t, labels, metric.Labels)
}

func TestNewSystem(t *testing.T) {
	system := NewSystem(nil)

	assert.NotNil(t, system)
	assert.NotNil(t, system.GetLogger())
}

func TestRecordAPIRequest(t *testing.T) {
	system := NewSystem(nil)

	system.RecordAPIRequest()
	system.RecordAPIRequest()
	system.RecordAPIRequest()

	metrics := system.GetAllMetrics()
	requestsTotal, ok := metrics["api_requests_total"].(uint64)
	assert.True(t, ok)
	assert.Equal(t, uint64(3), requestsTotal)
}

func TestRecordAPISuccess(t *testing.T) {
	system := NewSystem(nil)

	system.RecordAPISuccess(100 * time.Millisecond)
	system.RecordAPISuccess(200 * time.Millisecond)

	metrics := system.GetAllMetrics()
	successCount, ok := metrics["api_requests_success"].(uint64)
	assert.True(t, ok)
	assert.Equal(t, uint64(2), successCount)
}

func TestRecordAPIFailure(t *testing.T) {
	system := NewSystem(nil)

	err := assert.AnError
	system.RecordAPIFailure(err, 150*time.Millisecond)

	metrics := system.GetAllMetrics()
	failureCount, ok := metrics["api_requests_failure"].(uint64)
	assert.True(t, ok)
	assert.Equal(t, uint64(1), failureCount)
}

func TestRecordToolCall(t *testing.T) {
	system := NewSystem(nil)

	system.RecordToolCall("test_tool")
	system.RecordToolCall("test_tool")
	system.RecordToolCall("another_tool")

	metrics := system.GetAllMetrics()
	toolCalls, ok := metrics["tool_calls_total"].(uint64)
	assert.True(t, ok)
	assert.Equal(t, uint64(3), toolCalls)
}

func TestRecordToolSuccess(t *testing.T) {
	system := NewSystem(nil)

	system.RecordToolSuccess("test_tool", 50*time.Millisecond)
	system.RecordToolSuccess("test_tool", 100*time.Millisecond)

	metrics := system.GetAllMetrics()
	successCount, ok := metrics["tool_calls_success"].(uint64)
	assert.True(t, ok)
	assert.Equal(t, uint64(2), successCount)
}

func TestRecordToolFailure(t *testing.T) {
	system := NewSystem(nil)

	err := assert.AnError
	system.RecordToolFailure("test_tool", err, 75*time.Millisecond)

	metrics := system.GetAllMetrics()
	failureCount, ok := metrics["tool_calls_failure"].(uint64)
	assert.True(t, ok)
	assert.Equal(t, uint64(1), failureCount)
}

func TestRecordToolPermissionDenial(t *testing.T) {
	system := NewSystem(nil)

	system.RecordToolPermissionDenial("restricted_tool")
	system.RecordToolPermissionDenial("another_restricted_tool")

	metrics := system.GetAllMetrics()
	denialCount, ok := metrics["tool_permission_denials"].(uint64)
	assert.True(t, ok)
	assert.Equal(t, uint64(2), denialCount)
}

func TestRecordQueryTurn(t *testing.T) {
	system := NewSystem(nil)

	system.RecordQueryTurn()
	system.RecordQueryTurn()
	system.RecordQueryTurn()

	metrics := system.GetAllMetrics()
	turns, ok := metrics["query_turns_total"].(uint64)
	assert.True(t, ok)
	assert.Equal(t, uint64(3), turns)
}

func TestRecordQueryIteration(t *testing.T) {
	system := NewSystem(nil)

	for i := 0; i < 10; i++ {
		system.RecordQueryIteration()
	}

	metrics := system.GetAllMetrics()
	iterations, ok := metrics["query_iterations_total"].(uint64)
	assert.True(t, ok)
	assert.Equal(t, uint64(10), iterations)
}

func TestRecordQuerySuccess(t *testing.T) {
	system := NewSystem(nil)

	system.RecordQuerySuccess(500 * time.Millisecond)
	system.RecordQuerySuccess(1000 * time.Millisecond)

	metrics := system.GetAllMetrics()
	// Should have timer data, not just count
	latencyData, ok := metrics["query_latency_ms"].(map[string]interface{})
	assert.True(t, ok)
	assert.Contains(t, latencyData, "count")
	assert.Contains(t, latencyData, "average")
}

func TestRecordQueryFailure(t *testing.T) {
	system := NewSystem(nil)

	err := assert.AnError
	system.RecordQueryFailure(err, 750*time.Millisecond)

	metrics := system.GetAllMetrics()
	errorCount, ok := metrics["query_errors"].(uint64)
	assert.True(t, ok)
	assert.Equal(t, uint64(1), errorCount)
}

func TestRecordCircuitBreakerStateTransition(t *testing.T) {
	system := NewSystem(nil)

	system.RecordCircuitBreakerStateTransition("closed", "open", "failure threshold reached")
	system.RecordCircuitBreakerStateTransition("open", "half-open", "reset timeout")
	system.RecordCircuitBreakerStateTransition("half-open", "closed", "recovery successful")

	metrics := system.GetAllMetrics()
	transitions, ok := metrics["circuit_breaker_state_transitions"].(uint64)
	assert.True(t, ok)
	assert.Equal(t, uint64(3), transitions)
}

func TestSetCircuitBreakerState(t *testing.T) {
	system := NewSystem(nil)

	system.SetCircuitBreakerState("closed")
	metrics := system.GetAllMetrics()
	state, ok := metrics["circuit_breaker_current_state"].(float64)
	assert.True(t, ok)
	assert.Equal(t, float64(0), state)

	system.SetCircuitBreakerState("open")
	metrics = system.GetAllMetrics()
	state, ok = metrics["circuit_breaker_current_state"].(float64)
	assert.True(t, ok)
	assert.Equal(t, float64(1), state)

	system.SetCircuitBreakerState("half-open")
	metrics = system.GetAllMetrics()
	state, ok = metrics["circuit_breaker_current_state"].(float64)
	assert.True(t, ok)
	assert.Equal(t, float64(2), state)
}

func TestUpdateActiveSessions(t *testing.T) {
	system := NewSystem(nil)

	system.UpdateActiveSessions(5.0)
	metrics := system.GetAllMetrics()
	sessions, ok := metrics["active_sessions"].(float64)
	assert.True(t, ok)
	assert.Equal(t, float64(5.0), sessions)

	system.UpdateActiveSessions(10.0)
	metrics = system.GetAllMetrics()
	sessions, ok = metrics["active_sessions"].(float64)
	assert.True(t, ok)
	assert.Equal(t, float64(10.0), sessions)
}

func TestUpdateActiveConnections(t *testing.T) {
	system := NewSystem(nil)

	system.UpdateActiveConnections(3.0)
	metrics := system.GetAllMetrics()
	connections, ok := metrics["active_connections"].(float64)
	assert.True(t, ok)
	assert.Equal(t, float64(3.0), connections)
}

func TestUpdateMemoryUsage(t *testing.T) {
	system := NewSystem(nil)

	system.UpdateMemoryUsage(512.0)
	metrics := system.GetAllMetrics()
	memory, ok := metrics["memory_usage_mb"].(float64)
	assert.True(t, ok)
	assert.Equal(t, float64(512.0), memory)
}

func TestRecordError(t *testing.T) {
	system := NewSystem(nil)

	err := assert.AnError
	system.RecordError(err, "test_operation")

	metrics := system.GetAllMetrics()
	errors, ok := metrics["total_errors"].(uint64)
	assert.True(t, ok)
	assert.Equal(t, uint64(1), errors)
}

func TestGetAllMetrics(t *testing.T) {
	system := NewSystem(nil)

	// Record various metrics
	system.RecordAPIRequest()
	system.RecordToolCall("test_tool")
	system.RecordQueryTurn()
	system.RecordCircuitBreakerStateTransition("closed", "open", "test")
	system.UpdateActiveSessions(5.0)

	metrics := system.GetAllMetrics()

	// Verify we have all expected metrics
	assert.Contains(t, metrics, "api_requests_total")
	assert.Contains(t, metrics, "tool_calls_total")
	assert.Contains(t, metrics, "query_turns_total")
	assert.Contains(t, metrics, "circuit_breaker_state_transitions")
	assert.Contains(t, metrics, "active_sessions")

	// Verify types
	_, ok := metrics["api_requests_total"].(uint64)
	assert.True(t, ok)

	_, ok = metrics["active_sessions"].(float64)
	assert.True(t, ok)

	_, ok = metrics["api_latency_ms"].(map[string]interface{})
	assert.True(t, ok)
}

func TestGetMetricsSnapshotJSON(t *testing.T) {
	system := NewSystem(nil)

	system.RecordAPIRequest()
	system.RecordToolCall("test_tool")

	snapshot, err := system.GetMetricsSnapshot("json")
	assert.NoError(t, err)
	assert.Contains(t, snapshot, "api_requests_total")
	assert.Contains(t, snapshot, "tool_calls_total")
	assert.Contains(t, snapshot, "{") // JSON format
	assert.Contains(t, snapshot, "}")
}

func TestGetMetricsSnapshotPrometheus(t *testing.T) {
	system := NewSystem(nil)

	system.RecordAPIRequest()
	system.RecordToolCall("test_tool")

	snapshot, err := system.GetMetricsSnapshot("prometheus")
	assert.NoError(t, err)
	assert.Contains(t, snapshot, "api_requests_total")
	assert.Contains(t, snapshot, "tool_calls_total")
}

func TestGetMetricsSnapshotText(t *testing.T) {
	system := NewSystem(nil)

	system.RecordAPIRequest()
	system.RecordToolCall("test_tool")

	snapshot, err := system.GetMetricsSnapshot("text")
	assert.NoError(t, err)
	assert.Contains(t, snapshot, "api_requests_total")
	assert.Contains(t, snapshot, "tool_calls_total")
}

func TestResetAllMetrics(t *testing.T) {
	system := NewSystem(nil)

	// Record some metrics
	system.RecordAPIRequest()
	system.RecordToolCall("test_tool")
	system.RecordQueryTurn()
	system.RecordError(assert.AnError, "test")

	// Reset everything
	system.ResetAllMetrics()

	// Verify everything is reset
	metrics := system.GetAllMetrics()

	for key, value := range metrics {
		switch v := value.(type) {
		case uint64:
			assert.Equal(t, uint64(0), v, "Metric %s should be reset to 0", key)
		case float64:
			assert.Equal(t, float64(0), v, "Metric %s should be reset to 0", key)
		case map[string]interface{}:
			// Timer metrics contain maps, verify they're reset
			count, ok := v["count"].(uint64)
			assert.True(t, ok, "Timer %s count should be 0", key)
			assert.Equal(t, uint64(0), count, "Timer %s count should be 0", key)
		}
	}
}

func TestIntegratedMetricsFlow(t *testing.T) {
	system := NewSystem(nil)

	// Simulate a complete flow
	// 1. API Request
	system.RecordAPIRequest()
	system.RecordAPISuccess(100 * time.Millisecond)

	// 2. Tool Call
	system.RecordToolCall("file_read")
	system.RecordToolSuccess("file_read", 50*time.Millisecond)

	// 3. Query Turn
	system.RecordQueryTurn()
	system.RecordQuerySuccess(200 * time.Millisecond)

	// 4. Circuit Breaker
	system.RecordCircuitBreakerStateTransition("closed", "open", "API failures")
	system.SetCircuitBreakerState("open")

	// Verify all metrics are recorded
	metrics := system.GetAllMetrics()

	assert.Equal(t, uint64(1), metrics["api_requests_total"])
	assert.Equal(t, uint64(1), metrics["api_requests_success"])
	assert.Equal(t, uint64(1), metrics["tool_calls_total"])
	assert.Equal(t, uint64(1), metrics["tool_calls_success"])
	assert.Equal(t, uint64(1), metrics["query_turns_total"])
	assert.Equal(t, uint64(1), metrics["circuit_breaker_state_transitions"])
	assert.Equal(t, float64(1), metrics["circuit_breaker_current_state"])

	// Verify latency metrics
	apiLatency := metrics["api_latency_ms"].(map[string]interface{})
	assert.Equal(t, uint64(1), apiLatency["count"])

	toolLatency := metrics["tool_latency_ms"].(map[string]interface{})
	assert.Equal(t, uint64(1), toolLatency["count"])

	queryLatency := metrics["query_latency_ms"].(map[string]interface{})
	assert.Equal(t, uint64(1), queryLatency["count"])
}

func TestConcurrentMetricUpdates(t *testing.T) {
	system := NewSystem(nil)

	// Update multiple metrics concurrently
	done := make(chan bool, 20)

	for i := 0; i < 10; i++ {
		go func(idx int) {
			system.RecordAPIRequest()
			system.RecordToolCall("tool_" + string(rune('A'+idx)))
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		go func(idx int) {
			system.UpdateActiveSessions(float64(idx))
			done <- true
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < 20; i++ {
		<-done
	}

	// Verify metrics are consistent
	metrics := system.GetAllMetrics()
	requests, ok := metrics["api_requests_total"].(uint64)
	assert.True(t, ok)
	assert.Equal(t, uint64(10), requests)

	tools, ok := metrics["tool_calls_total"].(uint64)
	assert.True(t, ok)
	assert.Equal(t, uint64(10), tools)
}

func TestRecordCacheStats(t *testing.T) {
	system := NewSystem(nil)

	system.RecordCacheStats(400, 100)
	system.RecordCacheStats(380, 0)

	metrics := system.GetAllMetrics()

	readTokens, ok := metrics["prompt_cache_read_tokens_total"].(uint64)
	assert.True(t, ok)
	assert.Equal(t, uint64(780), readTokens)

	creationTokens, ok := metrics["prompt_cache_creation_tokens_total"].(uint64)
	assert.True(t, ok)
	assert.Equal(t, uint64(100), creationTokens)

	hits, ok := metrics["prompt_cache_hits_total"].(uint64)
	assert.True(t, ok)
	assert.Equal(t, uint64(2), hits)

	writes, ok := metrics["prompt_cache_writes_total"].(uint64)
	assert.True(t, ok)
	assert.Equal(t, uint64(1), writes)
}

func TestRecordCacheStats_ZeroFieldsNoOp(t *testing.T) {
	system := NewSystem(nil)
	system.RecordCacheStats(0, 0)

	metrics := system.GetAllMetrics()
	hits, _ := metrics["prompt_cache_hits_total"].(uint64)
	writes, _ := metrics["prompt_cache_writes_total"].(uint64)
	assert.Equal(t, uint64(0), hits)
	assert.Equal(t, uint64(0), writes)
}
