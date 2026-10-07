package monitoring

import (
	"context"

	internalmonitoring "github.com/KPO-Tech/seshat/internal/monitoring"
)

type (
	// Logger represents a structured logger with context
	Logger = internalmonitoring.Logger
	// System provides centralized monitoring for Seshat
	System = internalmonitoring.System
)

// InitTracer initialises the global OTel tracer provider.
//
// If OTEL_EXPORTER_OTLP_ENDPOINT is unset it installs a no-op provider and
// returns immediately — callers do not need to special-case the absence.
// The returned shutdown function must be called before the process exits.
//
// By default the gRPC exporter connects without TLS (suitable for a local
// collector). Set OTEL_EXPORTER_OTLP_INSECURE=false to enable TLS.
func InitTracer(ctx context.Context, serviceName string) (func(context.Context) error, error) {
	return internalmonitoring.InitTracer(ctx, serviceName)
}

// NewSystem creates a new monitoring system
func NewSystem(logger *Logger) *System {
	return internalmonitoring.NewSystem(logger)
}
