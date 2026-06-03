package monitoring

import (
	"context"

	internalmonitoring "github.com/EngineerProjects/nexus-engine/internal/monitoring"
)

type (
	Logger = internalmonitoring.Logger
	System = internalmonitoring.System
)

func InitTracer(ctx context.Context, serviceName string) (func(context.Context) error, error) {
	return internalmonitoring.InitTracer(ctx, serviceName)
}

func NewSystem(logger *Logger) *System {
	return internalmonitoring.NewSystem(logger)
}
