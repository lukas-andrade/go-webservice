package telemetry

import (
	"context"
	"fmt"
	"net/http"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

type ShutdownFunc func(context.Context) error

// SetupTracing installs a global tracer provider that ships spans over OTLP/HTTP.
// The exporter picks its endpoint from the standard OTEL_EXPORTER_OTLP_*
// variables. When tracing is disabled we still install the W3C propagator,
// so trace headers from upstream callers aren't dropped on the floor.
func SetupTracing(ctx context.Context, serviceName string, enabled bool) (ShutdownFunc, error) {
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{},
	))
	if !enabled {
		return func(context.Context) error { return nil }, nil
	}

	exporter, err := otlptracehttp.New(ctx)
	if err != nil {
		return nil, fmt.Errorf("create otlp exporter: %w", err)
	}
	res, err := resource.New(ctx,
		resource.WithTelemetrySDK(),
		resource.WithAttributes(attribute.String("service.name", serviceName)),
	)
	if err != nil {
		return nil, fmt.Errorf("build otel resource: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)
	return tp.Shutdown, nil
}

func TraceMiddleware(next http.Handler) http.Handler {
	return otelhttp.NewHandler(next, "echo",
		otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
			return "HTTP " + r.Method
		}),
	)
}
