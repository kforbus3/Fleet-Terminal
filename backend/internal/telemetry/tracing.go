package telemetry

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// TracerShutdown flushes and stops the tracer provider.
type TracerShutdown func(context.Context) error

// SetupTracing configures OpenTelemetry tracing. When tracing is disabled or no
// OTLP endpoint is configured, it installs a context-propagating no-op provider
// so instrumentation code is always safe to call. Returns a shutdown func.
func SetupTracing(ctx context.Context, enabled bool, otlpEndpoint, serviceName, version string, log *slog.Logger) (TracerShutdown, error) {
	// Propagation is always enabled so trace context flows across services.
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{},
	))

	if !enabled || otlpEndpoint == "" {
		log.Info("tracing disabled (no exporter)")
		return func(context.Context) error { return nil }, nil
	}

	exp, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpoint(otlpEndpoint),
		otlptracehttp.WithInsecure(),
	)
	if err != nil {
		return nil, err
	}
	res, err := resource.Merge(resource.Default(), resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceName(serviceName),
		semconv.ServiceVersion(version),
	))
	if err != nil {
		return nil, err
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)
	log.Info("tracing enabled", "endpoint", otlpEndpoint)
	return tp.Shutdown, nil
}
