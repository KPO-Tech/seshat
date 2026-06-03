package monitoring

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/EngineerProjects/nexus-engine/internal/types"
)

// LogLevel represents severity of log messages
type LogLevel string

const (
	LogLevelDebug LogLevel = "debug"
	LogLevelInfo  LogLevel = "info"
	LogLevelWarn  LogLevel = "warn"
	LogLevelError LogLevel = "error"
	LogLevelFatal LogLevel = "fatal"
)

// LoggerConfig represents logger configuration
type LoggerConfig struct {
	// Level is minimum log level to output
	Level LogLevel `json:"level"`

	// Output is where to write logs (stdout, stderr, file)
	Output string `json:"output"`

	// FilePath is file path when Output is "file"
	FilePath string `json:"file_path"`

	// Format is log format (json, text)
	Format string `json:"format"`

	// ContextFields are additional fields to include in all log entries
	ContextFields []string `json:"context_fields"`
}

// DefaultLoggerConfig returns default logger configuration
func DefaultLoggerConfig() *LoggerConfig {
	return &LoggerConfig{
		Level:         LogLevelInfo, // Default to info level
		Output:        "stdout",
		Format:        "text",
		ContextFields: []string{"session_id", "turn_id", "request_id"},
	}
}

// Logger represents a structured logger with context
type Logger struct {
	config     *LoggerConfig
	level      LogLevel
	mu         sync.RWMutex
	output     io.Writer
	jsonFormat bool
}

// NewLogger creates a new logger with default configuration
func NewLogger() *Logger {
	return NewLoggerWithConfig(DefaultLoggerConfig())
}

// NewLoggerWithConfig creates a new logger with custom configuration
func NewLoggerWithConfig(config *LoggerConfig) *Logger {
	if config == nil {
		config = DefaultLoggerConfig()
	}

	// Validate and set defaults
	if config.Level == "" {
		config.Level = LogLevelInfo
	}

	var output io.Writer
	switch config.Output {
	case "stderr":
		output = os.Stderr
	case "file":
		if config.FilePath == "" {
			config.FilePath = "nexus_engine.log"
		}
		file, err := os.OpenFile(config.FilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			// Fallback to stderr so error logs don't mix with stdout program output.
			log.Printf("Failed to open log file %q, falling back to stderr: %v", config.FilePath, err)
			output = os.Stderr
		} else {
			output = file
		}
	default: // stdout
		output = os.Stdout
	}

	return &Logger{
		config:     config,
		level:      config.Level,
		output:     output,
		jsonFormat: config.Format == "json",
	}
}

// SetLevel updates minimum log level
func (l *Logger) SetLevel(level LogLevel) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.level = level
}

// SetContext extracts context fields from context
func (l *Logger) SetContext(ctx context.Context) map[string]string {
	context := make(map[string]string)

	for _, field := range l.config.ContextFields {
		switch field {
		case "session_id":
			if sessionID := ctx.Value(types.ContextKeySessionID); sessionID != nil {
				context["session_id"] = fmt.Sprintf("%v", sessionID)
			}
		case "turn_id":
			if turnID := ctx.Value(types.ContextKeyTurnID); turnID != nil {
				context["turn_id"] = fmt.Sprintf("%v", turnID)
			}
		case "request_id":
			if requestID := ctx.Value(types.ContextKeyRequestID); requestID != nil {
				context["request_id"] = fmt.Sprintf("%v", requestID)
			}
		case "trace_id":
			if traceID := ctx.Value(types.ContextKeyTraceID); traceID != nil {
				context["trace_id"] = fmt.Sprintf("%v", traceID)
			}
		}
	}

	return context
}

// formatMessage formats a log message with context
func (l *Logger) formatMessage(level LogLevel, msg string, context map[string]string, fields map[string]interface{}) string {
	timestamp := time.Now().UTC().Format(time.RFC3339)

	var formatted string
	if l.jsonFormat {
		// JSON format
		logData := map[string]interface{}{
			"timestamp":   timestamp,
			"level":       level,
			"message":     msg,
			"environment": getEnvironment(),
			"hostname":    getHostname(),
		}

		// Add context
		for key, value := range context {
			logData[key] = value
		}

		// Add additional fields
		for key, value := range fields {
			logData[key] = value
		}

		jsonBytes, _ := json.Marshal(logData)
		formatted = string(jsonBytes)
	} else {
		// Text format
		formatted = fmt.Sprintf("[%s] [%s] %s", timestamp, level, msg)

		// Add context
		if len(context) > 0 {
			formatted += fmt.Sprintf(" | context: %v", context)
		}

		// Add additional fields
		if len(fields) > 0 {
			formatted += fmt.Sprintf(" | fields: %v", fields)
		}
	}

	return formatted
}

// writeMessage writes a formatted log message
func (l *Logger) writeMessage(level LogLevel, msg string, context map[string]string, fields map[string]interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Check if this level should be logged
	if !l.shouldLog(level) {
		return
	}

	formatted := l.formatMessage(level, msg, context, fields)
	fmt.Fprintln(l.output, formatted)
}

// shouldLog checks if given level should be logged
func (l *Logger) shouldLog(level LogLevel) bool {
	switch l.level {
	case LogLevelDebug:
		return level == LogLevelDebug || level == LogLevelInfo || level == LogLevelWarn || level == LogLevelError || level == LogLevelFatal
	case LogLevelInfo:
		return level == LogLevelInfo || level == LogLevelWarn || level == LogLevelError || level == LogLevelFatal
	case LogLevelWarn:
		return level == LogLevelWarn || level == LogLevelError || level == LogLevelFatal
	case LogLevelError:
		return level == LogLevelError || level == LogLevelFatal
	case LogLevelFatal:
		return level == LogLevelFatal
	default:
		return true
	}
}

// Debug logs a debug message
func (l *Logger) Debug(msg string, fields map[string]interface{}) {
	l.writeMessage(LogLevelDebug, msg, nil, fields)
}

// DebugWithContext logs a debug message with context
func (l *Logger) DebugWithContext(ctx context.Context, msg string, fields map[string]interface{}) {
	context := l.SetContext(ctx)
	l.writeMessage(LogLevelDebug, msg, context, fields)
}

// Info logs an info message
func (l *Logger) Info(msg string, fields map[string]interface{}) {
	l.writeMessage(LogLevelInfo, msg, nil, fields)
}

// InfoWithContext logs an info message with context
func (l *Logger) InfoWithContext(ctx context.Context, msg string, fields map[string]interface{}) {
	context := l.SetContext(ctx)
	l.writeMessage(LogLevelInfo, msg, context, fields)
}

// Warn logs a warning message
func (l *Logger) Warn(msg string, fields map[string]interface{}) {
	l.writeMessage(LogLevelWarn, msg, nil, fields)
}

// WarnWithContext logs a warning message with context
func (l *Logger) WarnWithContext(ctx context.Context, msg string, fields map[string]interface{}) {
	context := l.SetContext(ctx)
	l.writeMessage(LogLevelWarn, msg, context, fields)
}

// Error logs an error message
func (l *Logger) Error(msg string, fields map[string]interface{}) {
	l.writeMessage(LogLevelError, msg, nil, fields)
}

// ErrorWithContext logs an error message with context
func (l *Logger) ErrorWithContext(ctx context.Context, msg string, fields map[string]interface{}) {
	context := l.SetContext(ctx)
	l.writeMessage(LogLevelError, msg, context, fields)
}

// ErrorWithStack logs an error message with stack trace
func (l *Logger) ErrorWithStack(msg string, err error, fields map[string]interface{}) {
	stack := getStackTrace()
	if fields == nil {
		fields = make(map[string]interface{})
	}
	fields["error"] = err.Error()
	fields["stack"] = stack
	l.writeMessage(LogLevelError, msg, nil, fields)
}

// Fatal logs a fatal message and exits
func (l *Logger) Fatal(msg string, fields map[string]interface{}) {
	l.writeMessage(LogLevelFatal, msg, nil, fields)
	os.Exit(1)
}

// FatalWithContext logs a fatal message with context and exits
func (l *Logger) FatalWithContext(ctx context.Context, msg string, fields map[string]interface{}) {
	context := l.SetContext(ctx)
	l.writeMessage(LogLevelFatal, msg, context, fields)
	os.Exit(1)
}

// getStackTrace captures current stack trace
func getStackTrace() string {
	buf := make([]byte, 4096)
	n := runtime.Stack(buf, false)
	return string(buf[:n])
}

// getEnvironment returns current environment
func getEnvironment() string {
	env := os.Getenv("ENVIRONMENT")
	if env == "" {
		env = "development"
	}
	return env
}

// getHostname returns current hostname
func getHostname() string {
	hostname, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return hostname
}

// LogRequest logs HTTP request information
func (l *Logger) LogRequest(ctx context.Context, method, url string, statusCode int, duration time.Duration) {
	fields := map[string]interface{}{
		"http_method": method,
		"http_url":    url,
		"http_status": statusCode,
		"duration_ms": duration.Milliseconds(),
	}
	l.InfoWithContext(ctx, fmt.Sprintf("HTTP %s %s", method, url), fields)
}

// LogToolExecution logs tool execution information
func (l *Logger) LogToolExecution(ctx context.Context, toolName string, duration time.Duration, success bool) {
	fields := map[string]interface{}{
		"tool_name":   toolName,
		"duration_ms": duration.Milliseconds(),
		"success":     success,
	}
	status := "success"
	if !success {
		status = "error"
	}
	l.InfoWithContext(ctx, fmt.Sprintf("Tool execution %s: %s", toolName, status), fields)
}

// LogCircuitBreakerState logs circuit breaker state changes
func (l *Logger) LogCircuitBreakerState(ctx context.Context, from, to string, reason string) {
	fields := map[string]interface{}{
		"from_state": from,
		"to_state":   to,
		"reason":     reason,
	}
	l.InfoWithContext(ctx, "Circuit breaker state transition", fields)
}

// LogError logs application errors with context
func (l *Logger) LogError(ctx context.Context, err error, operation string) {
	fields := map[string]interface{}{
		"operation": operation,
		"error":     err.Error(),
	}
	l.ErrorWithContext(ctx, fmt.Sprintf("Operation failed: %s", operation), fields)
}

// RequestLogger wraps HTTP client with logging
type RequestLogger struct {
	logger *Logger
}

// NewRequestLogger creates a new request logger
func NewRequestLogger(logger *Logger) *RequestLogger {
	return &RequestLogger{
		logger: logger,
	}
}

// LogRequest logs HTTP request details
func (rl *RequestLogger) LogRequest(ctx context.Context, req interface{}, resp interface{}, err error, duration time.Duration) {
	rl.logger.InfoWithContext(ctx, "HTTP request", map[string]interface{}{
		"duration_ms":  duration.Milliseconds(),
		"has_response": resp != nil,
		"has_error":    err != nil,
	})

	if err != nil {
		rl.logger.LogError(ctx, err, "http_request")
	}

	if resp != nil {
		// Try to extract common response fields
		if r, ok := resp.(*http.Response); ok {
			fields := map[string]interface{}{
				"status_code": r.StatusCode,
			}
			rl.logger.InfoWithContext(ctx, "HTTP response", fields)
		}
	}
}
