package otellabels

// Run state: order and outcome. step_index exists because OTLP batching can
// reorder a run on the wire. Mirrors python/tests/test_run.py.
// TestNoActiveSpanIsASilentNoOp lives in noop_test.go.

import (
	"math/rand/v2"
	"slices"
	"sort"
	"sync"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

func TestStepIndicesAreMonotonic(t *testing.T) {
	run := NewRun("")
	got := []int{run.NextStep(), run.NextStep(), run.NextStep(), run.NextStep()}
	if !slices.Equal(got, []int{0, 1, 2, 3}) {
		t.Errorf("steps = %v", got)
	}
}

func TestRunIdIsStableAcrossSteps(t *testing.T) {
	run := NewRun("")
	first := run.RunID()
	run.NextStep()
	if run.RunID() != first {
		t.Error("run id changed after a step")
	}
}

func TestTwoRunsGetDifferentIds(t *testing.T) {
	if NewRun("").RunID() == NewRun("").RunID() {
		t.Error("two runs got the same id")
	}
}

func TestRunIdsAreSixteenHexCharacters(t *testing.T) {
	if id := NewRun("").RunID(); !hex16.MatchString(id) {
		t.Errorf("run id %q is not 16 lowercase hex characters", id)
	}
}

func TestKeepsARunIdTheCallerSupplies(t *testing.T) {
	if id := NewRun("0123456789abcdef").RunID(); id != "0123456789abcdef" {
		t.Errorf("run id = %q", id)
	}
}

func TestConcurrentNextStepYieldsEachIndexOnce(t *testing.T) {
	run := NewRun("")
	const goroutines, each = 64, 100
	var mu sync.Mutex
	seen := make(map[int]bool, goroutines*each)
	var wg sync.WaitGroup
	for range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range each {
				step := run.NextStep()
				mu.Lock()
				if seen[step] {
					t.Errorf("step %d handed out twice", step)
				}
				seen[step] = true
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	for i := range goroutines * each {
		if !seen[i] {
			t.Fatalf("step %d never handed out", i)
		}
	}
}

func TestOrderSurvivesSpansArrivingOutOfOrder(t *testing.T) {
	// Shuffle the exported spans, sort by step_index, and the sequence comes back.
	tracer, exporter := tracing(t)
	run := NewRun("")
	called := []string{"alpha", "beta", "gamma", "delta"}
	for _, name := range called {
		withSpan(tracer, "execute_tool", func(span trace.Span) {
			span.SetAttributes(attribute.String("gen_ai.tool.name", name))
			step := run.NextStep()
			_ = MarkToolSpan(span, ToolCall{Args: map[string]any{"n": name}, Result: map[string]any{}, Attempt: 1, SideEffect: Read, StepIndex: &step})
		})
	}
	spans := exporter.GetSpans()
	rand.Shuffle(len(spans), func(i, j int) { spans[i], spans[j] = spans[j], spans[i] })
	sort.Slice(spans, func(i, j int) bool {
		return attrs(spans[i].Attributes)[KeyStepIndex].AsInt64() < attrs(spans[j].Attributes)[KeyStepIndex].AsInt64()
	})
	var recovered []string
	for _, s := range spans {
		recovered = append(recovered, attrs(s.Attributes)["gen_ai.tool.name"].AsString())
	}
	if !slices.Equal(recovered, called) {
		t.Errorf("recovered %v, want %v", recovered, called)
	}
}

func TestMarksTheRunIdOnTheRootSpan(t *testing.T) {
	tracer, exporter := tracing(t)
	run := NewRun("")
	withSpan(tracer, "invoke_agent", run.MarkStart)
	if got := attrs(finished(t, exporter).Attributes)[KeyRunID].AsString(); got != run.RunID() {
		t.Errorf("run_id = %q, want %q", got, run.RunID())
	}
}

func TestMarksHowTheRunEnded(t *testing.T) {
	tracer, exporter := tracing(t)
	withSpan(tracer, "invoke_agent", func(span trace.Span) { _ = MarkRunEnd(span, MaxSteps) })
	if got := attrs(finished(t, exporter).Attributes)[KeyExitReason].AsString(); got != "max_steps" {
		t.Errorf("exit_reason = %q", got)
	}
}

func TestRejectsAnUnknownExitReason(t *testing.T) {
	tracer, exporter := tracing(t)
	withSpan(tracer, "invoke_agent", func(span trace.Span) {
		if err := MarkRunEnd(span, "gave_up"); err == nil {
			t.Error("exit reason \"gave_up\" was accepted")
		}
	})
	if _, ok := attrs(finished(t, exporter).Attributes)[KeyExitReason]; ok {
		t.Error("exit_reason written despite the invalid value")
	}
}

func TestMarksAHandoffToASubagent(t *testing.T) {
	tracer, exporter := tracing(t)
	withSpan(tracer, "invoke_agent", func(span trace.Span) { MarkHandoff(span, "ho-7f31") })
	if got := attrs(finished(t, exporter).Attributes)[KeyHandoffID].AsString(); got != "ho-7f31" {
		t.Errorf("handoff_id = %q", got)
	}
}
