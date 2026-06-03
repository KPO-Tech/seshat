package monitoring

import (
	"context"
	"os"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.27.0"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

const nexusTracerName = "nexus-engine"

// InitTracer initialises the global OTel tracer provider.
//
// If OTEL_EXPORTER_OTLP_ENDPOINT is unset it installs a no-op provider and
// returns immediately — callers do not need to special-case the absence.
// The returned shutdown function must be called before the process exits.
//
// By default the gRPC exporter connects without TLS (suitable for a local
// collector). Set OTEL_EXPORTER_OTLP_INSECURE=false to enable TLS.
func InitTracer(ctx context.Context, serviceName string) (shutdown func(context.Context) error, err error) {
	endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if endpoint == "" {
		otel.SetTracerProvider(noop.NewTracerProvider())
		return func(context.Context) error { return nil }, nil
	}

	opts := []otlptracegrpc.Option{
		otlptracegrpc.WithEndpoint(endpoint),
	}
	if os.Getenv("OTEL_EXPORTER_OTLP_INSECURE") != "false" {
		opts = append(opts, otlptracegrpc.WithInsecure())
	}

	exporter, err := otlptracegrpc.New(ctx, opts...)
	if err != nil {
		return nil, err
	}

	res, _ := resource.New(ctx,
		resource.WithAttributes(semconv.ServiceName(serviceName)),
		resource.WithAttributes(attribute.String("service.version", "nexus-engine-v2")),
	)

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)

	otel.SetTracerProvider(tp)
	return tp.Shutdown, nil
}

// Tracer returns the global named tracer for Nexus Engine.
// Safe to call before InitTracer — returns a no-op tracer until the provider
// is initialised.
func Tracer() trace.Tracer {
	return otel.Tracer(nexusTracerName)
}
