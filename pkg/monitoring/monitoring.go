package monitoring

import (
	"context"

	internalmonitoring "github.com/KPO-Tech/seshat/internal/monitoring"
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
