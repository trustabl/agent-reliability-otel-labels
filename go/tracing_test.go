package otellabels

// Shared test helpers: Python's conftest.py.

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

// tracing returns a tracer plus the spans it finished. Deliberately not the
// global provider: these spans stand in for ones a third-party instrumentor
// created.
func tracing(t *testing.T) (trace.Tracer, *tracetest.InMemoryExporter) {
	t.Helper()
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })
	return provider.Tracer("test-instrumentor"), exporter
}

// withSpan is Python's `with tracer.start_as_current_span(name) as span:`.
func withSpan(tracer trace.Tracer, name string, fn func(span trace.Span)) {
	_, span := tracer.Start(context.Background(), name)
	defer span.End()
	fn(span)
}

// finished returns the one span the test finished.
func finished(t *testing.T, exporter *tracetest.InMemoryExporter) tracetest.SpanStub {
	t.Helper()
	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("want 1 finished span, got %d", len(spans))
	}
	return spans[0]
}

// attrs indexes a span's or event's attributes by key.
func attrs(kvs []attribute.KeyValue) map[string]attribute.Value {
	out := make(map[string]attribute.Value, len(kvs))
	for _, kv := range kvs {
		out[string(kv.Key)] = kv.Value
	}
	return out
}

func intPtr(n int) *int { return &n }

// opaqueSpan is a recording span that does not expose its attributes, as the
// OpenTelemetry API allows: reading them is an SDK affordance.
type opaqueSpan struct {
	noop.Span
	written map[string]attribute.Value
}

func newOpaqueSpan() *opaqueSpan { return &opaqueSpan{written: map[string]attribute.Value{}} }

func (s *opaqueSpan) IsRecording() bool { return true }

func (s *opaqueSpan) SetAttributes(kvs ...attribute.KeyValue) {
	for _, kv := range kvs {
		s.written[string(kv.Key)] = kv.Value
	}
}

// panickySpan panics when asked whether it records.
type panickySpan struct{ noop.Span }

func (panickySpan) IsRecording() bool { panic("is_recording exploded") }
