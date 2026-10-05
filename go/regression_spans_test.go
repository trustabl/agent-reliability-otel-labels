package otellabels

// Go-specific: a span implementation that panics must not take the workload
// down with it.

import (
	"testing"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

func TestAPanickingSpanDoesNotReachTheWorkload(t *testing.T) {
	span := &panickyWriteSpan{}
	if err := MarkToolSpan(span, ToolCall{Args: map[string]any{}, Result: map[string]any{}, Attempt: 1, Name: "x"}); err != nil {
		t.Error(err)
	}
	NewRun("").MarkStart(span)
	_ = MarkRunEnd(span, FinalAnswer)
	MarkHandoff(span, "ho-1")
	_ = BindAuthority(span, []Binding{openshell})
}

// panickyWriteSpan records but panics on every write.
type panickyWriteSpan struct{ opaqueSpan }

func (*panickyWriteSpan) IsRecording() bool { return true }

func (*panickyWriteSpan) SetAttributes(...attribute.KeyValue) { panic("write exploded") }

func (*panickyWriteSpan) AddEvent(string, ...trace.EventOption) { panic("event exploded") }
