package otellabels

// The privacy guarantee, asserted rather than promised. Everything this
// library writes is an id, an enum, an integer or a hash. Mirrors
// python/tests/test_hygiene.py.

import (
	"strings"
	"testing"

	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

// Deliberately fake. These tests assert it NEVER reaches a span; a credential
// scanner flagging it has found the test working, not a leak.
const secret = "patient Jane Doe, SSN 123-45-6789, diagnosis withheld"

// everythingWritten is every attribute value and every event attribute
// value, as one string.
func everythingWritten(s tracetest.SpanStub) string {
	var parts []string
	for _, kv := range s.Attributes {
		parts = append(parts, kv.Value.Emit())
	}
	for _, e := range s.Events {
		for _, kv := range e.Attributes {
			parts = append(parts, kv.Value.Emit())
		}
	}
	return strings.Join(parts, " ")
}

func TestNoArgumentOrResultTextReachesAnyAttribute(t *testing.T) {
	tracer, exporter := tracing(t)
	run := NewRun("")
	withSpan(tracer, "execute_tool", func(span trace.Span) {
		run.MarkStart(span)
		step := run.NextStep()
		_ = MarkToolSpan(span, ToolCall{
			Args:    map[string]any{"query": secret, "nested": map[string]any{"note": secret}},
			Result:  map[string]any{"records": []any{map[string]any{"body": secret}}},
			Attempt: 1, SideEffect: Read, StepIndex: &step,
		})
		_ = BindAuthority(span, []Binding{{ID: "dspm-prod-v2", Type: DataSecurity, SHA256: strings.Repeat("d", 64), Version: "2.1.0", Required: true, Source: Entrypoint}})
		_ = MarkRunEnd(span, FinalAnswer)
	})
	written := everythingWritten(finished(t, exporter))
	for _, leak := range []string{secret, "Jane Doe", "123-45-6789"} {
		if strings.Contains(written, leak) {
			t.Errorf("%q reached a span attribute", leak)
		}
	}
}

func TestEveryAttributeWeWriteIsBoundedInLength(t *testing.T) {
	tracer, exporter := tracing(t)
	withSpan(tracer, "execute_tool", func(span trace.Span) {
		_ = MarkToolSpan(span, ToolCall{
			Args: map[string]any{"blob": strings.Repeat("x", 50_000)}, Result: map[string]any{"blob": strings.Repeat("y", 50_000)},
			Attempt: 1, SideEffect: Read, StepIndex: intPtr(0),
		})
	})
	for _, kv := range finished(t, exporter).Attributes {
		if s := kv.Value.AsString(); len(s) > 64 {
			t.Errorf("%s is unbounded (%d characters)", kv.Key, len(s))
		}
	}
}

func TestFingerprintsAreExactlyTheDeclaredLength(t *testing.T) {
	tracer, exporter := tracing(t)
	withSpan(tracer, "execute_tool", func(span trace.Span) {
		_ = MarkToolSpan(span, ToolCall{Args: map[string]any{"a": 1}, Result: map[string]any{"b": 2}, Attempt: 1, SideEffect: Read})
	})
	a := attrs(finished(t, exporter).Attributes)
	for _, key := range []string{KeyToolInputFP, KeyToolOutputFP} {
		if !hex16.MatchString(a[key].AsString()) {
			t.Errorf("%s = %q, want 16 hex characters", key, a[key].AsString())
		}
	}
}
