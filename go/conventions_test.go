package otellabels

// We attach to spans we did not shape. A span from any of the three captured
// conventions must take our labels without us disturbing theirs. Mirrors
// python/tests/test_conventions.py.

import (
	"testing"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

func TestLabelsAttachToASpanFromAnyConvention(t *testing.T) {
	conventions := map[string]map[string]string{
		"genai":         {"gen_ai.operation.name": "execute_tool", "gen_ai.tool.name": "search_flights"},
		"openinference": {"openinference.span.kind": "TOOL", "tool.name": "search_flights"},
		"traceloop":     {"traceloop.span.kind": "tool", "traceloop.entity.name": "search_flights"},
	}
	for convention, theirs := range conventions {
		t.Run(convention, func(t *testing.T) {
			tracer, exporter := tracing(t)
			withSpan(tracer, "tool", func(span trace.Span) {
				for key, value := range theirs {
					span.SetAttributes(attribute.String(key, value))
				}
				_ = MarkToolSpan(span, ToolCall{Args: map[string]any{"to": "JFK"}, Result: map[string]any{}, Attempt: 1, SideEffect: Read, StepIndex: intPtr(0)})
			})
			written := attrs(finished(t, exporter).Attributes)
			if written[KeyToolSideEffect].AsString() != "read" {
				t.Error("side_effect not written")
			}
			for key, value := range theirs {
				if written[key].AsString() != value {
					t.Errorf("%s: we disturbed %s", convention, key)
				}
			}
		})
	}
}
