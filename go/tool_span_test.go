package otellabels

// MarkToolSpan decorates a span somebody else created. These tests pin the
// invariants that make that safe. Mirrors python/tests/test_tool_span.py.

import (
	"context"
	"math"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

func TestSetsTheProcessKeysOnTheSpan(t *testing.T) {
	tracer, exporter := tracing(t)
	withSpan(tracer, "execute_tool", func(span trace.Span) {
		_ = MarkToolSpan(span, ToolCall{
			Args: map[string]any{"to": "JFK"}, Result: map[string]any{"flights": 3},
			Attempt: 3, SideEffect: Read, StepIndex: intPtr(7),
		})
	})
	a := attrs(finished(t, exporter).Attributes)
	if a[KeyToolAttempt].AsInt64() != 3 || a[KeyToolSideEffect].AsString() != "read" || a[KeyStepIndex].AsInt64() != 7 {
		t.Errorf("wrong process keys: %v", a)
	}
	if len(a[KeyToolInputFP].AsString()) != 16 || len(a[KeyToolOutputFP].AsString()) != 16 {
		t.Errorf("fingerprints missing or wrong length: %v", a)
	}
}

func TestOmitsErrorClassWhenThereWasNoError(t *testing.T) {
	// Absence is a claim: an omitted key says "no error observed".
	tracer, exporter := tracing(t)
	withSpan(tracer, "execute_tool", func(span trace.Span) {
		_ = MarkToolSpan(span, ToolCall{Args: map[string]any{}, Result: map[string]any{}, Attempt: 1, SideEffect: Read})
	})
	if _, ok := attrs(finished(t, exporter).Attributes)[KeyToolErrorClass]; ok {
		t.Error("error_class written for a call with no error")
	}
}

func TestOmitsSideEffectWhenTheCallerDoesNotKnowIt(t *testing.T) {
	// Degrade to absent, never to wrong: unknown is left off, not guessed as "pure".
	tracer, exporter := tracing(t)
	withSpan(tracer, "execute_tool", func(span trace.Span) {
		_ = MarkToolSpan(span, ToolCall{Args: map[string]any{}, Result: map[string]any{}, Attempt: 1})
	})
	if _, ok := attrs(finished(t, exporter).Attributes)[KeyToolSideEffect]; ok {
		t.Error("side_effect written although unknown")
	}
}

func TestPureMarksAToolWithNoSideEffect(t *testing.T) {
	tracer, exporter := tracing(t)
	withSpan(tracer, "execute_tool", func(span trace.Span) {
		_ = MarkToolSpan(span, ToolCall{Args: map[string]any{}, Result: map[string]any{}, Attempt: 1, SideEffect: Pure})
	})
	if got := attrs(finished(t, exporter).Attributes)[KeyToolSideEffect].AsString(); got != "pure" {
		t.Errorf("side_effect = %q, want pure", got)
	}
}

func TestRejectsNoneAsASideEffectClass(t *testing.T) {
	// "none" was renamed "pure" so it cannot be confused with an omitted
	// (unknown) key or with bind.method=none.
	tracer, exporter := tracing(t)
	withSpan(tracer, "execute_tool", func(span trace.Span) {
		if err := MarkToolSpan(span, ToolCall{Attempt: 1, SideEffect: "none"}); err == nil {
			t.Error("side effect \"none\" was accepted")
		}
	})
	for key := range attrs(finished(t, exporter).Attributes) {
		if strings.HasPrefix(key, "trustabl.") {
			t.Errorf("%s written despite the invalid enum", key)
		}
	}
}

func TestStampsTheRunIdOnTheToolSpan(t *testing.T) {
	tracer, exporter := tracing(t)
	withSpan(tracer, "execute_tool", func(span trace.Span) {
		_ = MarkToolSpan(span, ToolCall{Args: map[string]any{}, Result: map[string]any{}, Attempt: 1, RunID: "0123456789abcdef"})
	})
	if got := attrs(finished(t, exporter).Attributes)[KeyRunID].AsString(); got != "0123456789abcdef" {
		t.Errorf("run_id = %q", got)
	}
}

func TestOmitsRunIdWhenNoneIsGiven(t *testing.T) {
	tracer, exporter := tracing(t)
	withSpan(tracer, "execute_tool", func(span trace.Span) {
		_ = MarkToolSpan(span, ToolCall{Args: map[string]any{}, Result: map[string]any{}, Attempt: 1})
	})
	if _, ok := attrs(finished(t, exporter).Attributes)[KeyRunID]; ok {
		t.Error("run_id written although none was given")
	}
}

func TestDoesNotOverwriteAToolNameTheInstrumentorSet(t *testing.T) {
	tracer, exporter := tracing(t)
	withSpan(tracer, "execute_tool", func(span trace.Span) {
		span.SetAttributes(attribute.String("gen_ai.tool.name", "search_flights"))
		_ = MarkToolSpan(span, ToolCall{Args: map[string]any{}, Result: map[string]any{}, Attempt: 1, SideEffect: Read, Name: "something_else"})
	})
	if got := attrs(finished(t, exporter).Attributes)["gen_ai.tool.name"].AsString(); got != "search_flights" {
		t.Errorf("gen_ai.tool.name = %q, want search_flights", got)
	}
}

func TestANonRecordingSpanIsASilentNoOp(t *testing.T) {
	span := trace.SpanFromContext(context.Background())
	if err := MarkToolSpan(span, ToolCall{Args: map[string]any{"a": 1}, Result: map[string]any{}, Attempt: 1, SideEffect: Read}); err != nil {
		t.Error(err)
	}
}

func TestASpanWhoseIsRecordingRaisesIsASilentNoOp(t *testing.T) {
	if err := MarkToolSpan(panickySpan{}, ToolCall{Args: map[string]any{"a": 1}, Result: map[string]any{}, Attempt: 1}); err != nil {
		t.Error(err)
	}
}

func TestTheEnumCheckComesAfterTheNoOpCheck(t *testing.T) {
	// A workload with tracing disabled must not be breakable by this library.
	if err := MarkToolSpan(nil, ToolCall{Attempt: 1, SideEffect: "destroys"}); err != nil {
		t.Errorf("no-op returned %v", err)
	}
}

func TestRejectsAnUnknownSideEffectClass(t *testing.T) {
	tracer, _ := tracing(t)
	withSpan(tracer, "execute_tool", func(span trace.Span) {
		if err := MarkToolSpan(span, ToolCall{Attempt: 1, SideEffect: "destroys"}); err == nil {
			t.Error("side effect \"destroys\" was accepted")
		}
	})
}

func TestRejectsAnUnknownErrorClass(t *testing.T) {
	tracer, _ := tracing(t)
	withSpan(tracer, "execute_tool", func(span trace.Span) {
		if err := MarkToolSpan(span, ToolCall{Attempt: 1, ErrorClass: "oops"}); err == nil {
			t.Error("error class \"oops\" was accepted")
		}
	})
}

// wantOnlyFingerprintOmitted checks that exactly one fingerprint is missing
// and everything else was still written.
func wantOnlyFingerprintOmitted(t *testing.T, a map[string]attribute.Value, missing, present string, attempt int64) {
	t.Helper()
	if _, ok := a[missing]; ok {
		t.Errorf("%s written for a value with no canonical form", missing)
	}
	if len(a[present].AsString()) != 16 {
		t.Errorf("%s missing: %v", present, a)
	}
	if a[KeyToolAttempt].AsInt64() != attempt {
		t.Errorf("attempt = %v, want %d", a[KeyToolAttempt], attempt)
	}
}

func TestAnUncanonicalizableArgumentOmitsOnlyTheInputFingerprint(t *testing.T) {
	// Degrade to absent, never to wrong - and never break the workload.
	cases := map[string]any{
		"unsafe integer": int64(1) << 53,
		"time.Time":      time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC),
		"struct":         struct{ X int }{1},
		"NaN":            math.NaN(),
		"function":       func() {},
	}
	for name, value := range cases {
		t.Run(name, func(t *testing.T) {
			tracer, exporter := tracing(t)
			withSpan(tracer, "execute_tool", func(span trace.Span) {
				if err := MarkToolSpan(span, ToolCall{
					Args: map[string]any{"v": value}, Result: map[string]any{"ok": true},
					Attempt: 1, SideEffect: Read, StepIndex: intPtr(0),
				}); err != nil {
					t.Error(err)
				}
			})
			a := attrs(finished(t, exporter).Attributes)
			wantOnlyFingerprintOmitted(t, a, KeyToolInputFP, KeyToolOutputFP, 1)
			if a[KeyToolSideEffect].AsString() != "read" || a[KeyStepIndex].AsInt64() != 0 {
				t.Errorf("other attributes lost: %v", a)
			}
		})
	}
}

func TestAnUncanonicalizableResultOmitsOnlyTheOutputFingerprint(t *testing.T) {
	tracer, exporter := tracing(t)
	withSpan(tracer, "execute_tool", func(span trace.Span) {
		_ = MarkToolSpan(span, ToolCall{
			Args:    map[string]any{"to": "JFK"},
			Result:  map[string]any{"at": time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC)},
			Attempt: 2,
		})
	})
	wantOnlyFingerprintOmitted(t, attrs(finished(t, exporter).Attributes), KeyToolOutputFP, KeyToolInputFP, 2)
}

func TestDoesNotWriteAToolNameWhenAttributesCannotBeRead(t *testing.T) {
	// Unknown is not absent: the instrumentor may have set a name we cannot see.
	span := newOpaqueSpan()
	_ = MarkToolSpan(span, ToolCall{Args: map[string]any{}, Result: map[string]any{}, Attempt: 1, Name: "search_flights"})
	if _, ok := span.written["gen_ai.tool.name"]; ok {
		t.Error("gen_ai.tool.name written on a span whose attributes cannot be read")
	}
	if span.written[KeyToolAttempt].AsInt64() != 1 {
		t.Error("attempt not written")
	}
}

func TestFillsTheToolNameWhenTheInstrumentorLeftItEmpty(t *testing.T) {
	tracer, exporter := tracing(t)
	withSpan(tracer, "execute_tool", func(span trace.Span) {
		_ = MarkToolSpan(span, ToolCall{Args: map[string]any{}, Result: map[string]any{}, Attempt: 1, Name: "search_flights"})
	})
	if got := attrs(finished(t, exporter).Attributes)["gen_ai.tool.name"].AsString(); got != "search_flights" {
		t.Errorf("gen_ai.tool.name = %q", got)
	}
}

func TestACircularArgumentOmitsOnlyTheInputFingerprint(t *testing.T) {
	// A Go stack overflow is fatal, so the depth bound is what keeps this a
	// missing fingerprint instead of a crashed process.
	cyclic := map[string]any{"a": 1}
	cyclic["self"] = cyclic
	tracer, exporter := tracing(t)
	withSpan(tracer, "execute_tool", func(span trace.Span) {
		_ = MarkToolSpan(span, ToolCall{Args: cyclic, Result: map[string]any{"ok": true}, Attempt: 1})
	})
	wantOnlyFingerprintOmitted(t, attrs(finished(t, exporter).Attributes), KeyToolInputFP, KeyToolOutputFP, 1)
}

func TestAThrowingValueOmitsOnlyTheOutputFingerprint(t *testing.T) {
	// The Go walker calls no user code - no MarshalJSON, no String - so no
	// value can throw from inside it. The Go counterpart is a value the walker
	// cannot handle at all.
	tracer, exporter := tracing(t)
	withSpan(tracer, "execute_tool", func(span trace.Span) {
		_ = MarkToolSpan(span, ToolCall{Args: map[string]any{"to": "JFK"}, Result: map[string]any{"c": make(chan int)}, Attempt: 1})
	})
	wantOnlyFingerprintOmitted(t, attrs(finished(t, exporter).Attributes), KeyToolOutputFP, KeyToolInputFP, 1)
}
