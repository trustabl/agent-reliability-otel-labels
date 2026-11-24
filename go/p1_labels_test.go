package otellabels

// The three labels the P1 rules need: expected tools, retrieval result count,
// progress marker. Mirrors python/tests/test_p1_labels.py.

import (
	"slices"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

func TestExpectedToolsAreWrittenOnTheRootSpan(t *testing.T) {
	tracer, exporter := tracing(t)
	withSpan(tracer, "invoke_agent", func(root trace.Span) {
		_ = MarkExpectedTools(root, []string{"search_flights", "book_flight", "search_flights"})
	})
	got := attrs(finished(t, exporter).Attributes)[KeyExpectedTools].AsStringSlice()
	if !slices.Equal(got, []string{"search_flights", "book_flight"}) {
		t.Errorf("expected_tools = %v", got)
	}
}

func TestEmptyExpectedToolsLeaveTheKeyOff(t *testing.T) {
	tracer, exporter := tracing(t)
	withSpan(tracer, "invoke_agent", func(root trace.Span) { _ = MarkExpectedTools(root, nil) })
	if _, ok := attrs(finished(t, exporter).Attributes)[KeyExpectedTools]; ok {
		t.Error("expected_tools written for an empty list")
	}
}

func TestExpectedToolsRejectsAnythingButAListOfNames(t *testing.T) {
	tracer, exporter := tracing(t)
	withSpan(tracer, "invoke_agent", func(root trace.Span) {
		if err := MarkExpectedTools(root, []string{"ok", ""}); err == nil {
			t.Error("an empty tool name was accepted")
		}
	})
	if _, ok := attrs(finished(t, exporter).Attributes)[KeyExpectedTools]; ok {
		t.Error("a rejected call still wrote the key")
	}
}

func TestRetrievalResultCountIsWrittenIncludingZero(t *testing.T) {
	tracer, exporter := tracing(t)
	withSpan(tracer, "retrieval", func(span trace.Span) {
		_ = MarkRetrievalSpan(span, RetrievalCall{ResultCount: 0, StepIndex: intPtr(2), RunID: "r1"})
	})
	a := attrs(finished(t, exporter).Attributes)
	v, ok := a[KeyRetrievalResultCount]
	if !ok || v.AsInt64() != 0 || a[KeyStepIndex].AsInt64() != 2 || a[KeyRunID].AsString() != "r1" {
		t.Errorf("wrong retrieval keys: %v", a)
	}
}

func TestRetrievalResultCountRejectsAnythingButANonNegativeInteger(t *testing.T) {
	tracer, exporter := tracing(t)
	withSpan(tracer, "retrieval", func(span trace.Span) {
		if err := MarkRetrievalSpan(span, RetrievalCall{ResultCount: -1}); err == nil {
			t.Error("a negative count was accepted")
		}
	})
	if _, ok := attrs(finished(t, exporter).Attributes)[KeyRetrievalResultCount]; ok {
		t.Error("a rejected call still wrote the key")
	}
}

func TestProgressMarkerIsAFingerprintThatFollowsTheState(t *testing.T) {
	tracer, exporter := tracing(t)
	for _, state := range []map[string]any{
		{"done": []any{"flights"}, "todo": []any{"hotel"}},
		{"todo": []any{"hotel"}, "done": []any{"flights"}},
		{"done": []any{"flights", "hotel"}, "todo": []any{}},
	} {
		withSpan(tracer, "execute_tool", func(span trace.Span) { MarkProgress(span, state) })
	}
	spans := exporter.GetSpans()
	if len(spans) != 3 {
		t.Fatalf("got %d spans", len(spans))
	}
	m := func(i int) string { return attrs(spans[i].Attributes)[KeyProgressMarker].AsString() }
	if len(m(0)) != 16 || m(0) != m(1) || m(0) == m(2) {
		t.Errorf("markers = %q %q %q", m(0), m(1), m(2))
	}
}

func TestProgressMarkerIsOmittedForAStateWithNoCanonicalForm(t *testing.T) {
	tracer, exporter := tracing(t)
	withSpan(tracer, "execute_tool", func(span trace.Span) {
		MarkProgress(span, map[string]any{"bad": make(chan int)})
	})
	if _, ok := attrs(finished(t, exporter).Attributes)[KeyProgressMarker]; ok {
		t.Error("progress_marker written for an uncanonicalizable state")
	}
}

func TestNoStateOrToolTextReachesTheNewAttributes(t *testing.T) {
	const secret = "patient Jane Doe, SSN 123-45-6789"
	tracer, exporter := tracing(t)
	withSpan(tracer, "execute_tool", func(span trace.Span) {
		MarkProgress(span, map[string]any{"note": secret})
		_ = MarkRetrievalSpan(span, RetrievalCall{ResultCount: 3})
	})
	for _, kv := range finished(t, exporter).Attributes {
		if strings.Contains(kv.Value.Emit(), secret) {
			t.Errorf("%s carries caller text", kv.Key)
		}
	}
}

func TestTheNewHelpersDoNothingWithoutARecordingSpan(t *testing.T) {
	for _, span := range []trace.Span{nil, noop.Span{}} {
		_ = MarkExpectedTools(span, []string{"a"})
		_ = MarkRetrievalSpan(span, RetrievalCall{ResultCount: 1})
		MarkProgress(span, map[string]any{"a": 1})
	}
	// tracing off never errors
	if err := MarkRetrievalSpan(noop.Span{}, RetrievalCall{ResultCount: -5}); err != nil {
		t.Errorf("non-recording span returned %v", err)
	}
}
