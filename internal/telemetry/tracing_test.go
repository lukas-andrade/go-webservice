package telemetry

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

func TestSetupTracingDisabledIsNoop(t *testing.T) {
	shutdown, err := SetupTracing(context.Background(), "echo-test", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := shutdown(context.Background()); err != nil {
		t.Errorf("shutdown: %v", err)
	}
}

func TestSetupTracingExportsToOTLPEndpoint(t *testing.T) {
	received := make(chan string, 1)
	collector := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case received <- r.URL.Path:
		default:
		}
	}))
	defer collector.Close()
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", collector.URL)

	previous := otel.GetTracerProvider()
	t.Cleanup(func() { otel.SetTracerProvider(previous) })

	shutdown, err := SetupTracing(context.Background(), "echo-test", true)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	_, span := otel.Tracer("test").Start(context.Background(), "work")
	span.End()
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown: %v", err)
	}

	select {
	case path := <-received:
		if path != "/v1/traces" {
			t.Errorf("exported to %s, want /v1/traces", path)
		}
	default:
		t.Fatal("shutdown should flush pending spans to the collector")
	}
}

func TestTraceMiddlewareContinuesUpstreamTrace(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	t.Cleanup(func() { otel.SetTracerProvider(previous) })
	if _, err := SetupTracing(context.Background(), "echo-test", false); err != nil {
		t.Fatal(err)
	}

	var seen trace.SpanContext
	h := TraceMiddleware(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		seen = trace.SpanContextFromContext(r.Context())
	}))

	req := httptest.NewRequest(http.MethodPost, "/x", nil)
	req.Header.Set("traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	h.ServeHTTP(httptest.NewRecorder(), req)

	spans := recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("got %d spans, want 1", len(spans))
	}
	if spans[0].Name() != "HTTP POST" {
		t.Errorf("span name = %q", spans[0].Name())
	}
	if got := seen.TraceID().String(); got != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Errorf("trace id = %s, want the one from traceparent", got)
	}
}
