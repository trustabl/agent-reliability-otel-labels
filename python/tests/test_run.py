"""Run state: order and outcome.

step_index exists because OTLP batching can reorder a run on the wire. Assigning
it in-process is what lets a consumer rebuild the true sequence.
"""

import random

import pytest

from agent_reliability_otel_labels import Run, mark_handoff, mark_run_end, mark_tool_span


def test_step_indices_are_monotonic():
    run = Run()
    assert [run.next_step() for _ in range(4)] == [0, 1, 2, 3]


def test_run_id_is_stable_across_steps():
    run = Run()
    first = run.run_id
    run.next_step()
    assert run.run_id == first


def test_two_runs_get_different_ids():
    assert Run().run_id != Run().run_id


def test_order_survives_spans_arriving_out_of_order(tracing):
    """The property that matters: shuffle the exported spans, sort by
    step_index, and the original sequence comes back."""
    tracer, exporter = tracing
    run = Run()
    called = ["alpha", "beta", "gamma", "delta"]
    for name in called:
        with tracer.start_as_current_span("execute_tool") as span:
            span.set_attribute("gen_ai.tool.name", name)
            mark_tool_span(span, args={"n": name}, result={}, attempt=1,
                           side_effect="read", step_index=run.next_step())

    shuffled = list(exporter.get_finished_spans())
    random.shuffle(shuffled)
    recovered = [s.attributes["gen_ai.tool.name"]
                 for s in sorted(shuffled, key=lambda s: s.attributes["trustabl.step_index"])]
    assert recovered == called


def test_marks_the_run_id_on_the_root_span(tracing):
    tracer, exporter = tracing
    run = Run()
    with tracer.start_as_current_span("invoke_agent") as span:
        run.mark_start(span)
    assert exporter.get_finished_spans()[0].attributes["trustabl.run_id"] == run.run_id


def test_marks_how_the_run_ended(tracing):
    tracer, exporter = tracing
    with tracer.start_as_current_span("invoke_agent") as span:
        mark_run_end(span, exit_reason="max_steps")
    assert exporter.get_finished_spans()[0].attributes["trustabl.exit_reason"] == "max_steps"


def test_rejects_an_unknown_exit_reason(tracing):
    tracer, _ = tracing
    with tracer.start_as_current_span("invoke_agent") as span:
        with pytest.raises(ValueError):
            mark_run_end(span, exit_reason="gave_up")


def test_marks_a_handoff_to_a_subagent(tracing):
    tracer, exporter = tracing
    with tracer.start_as_current_span("invoke_agent") as span:
        mark_handoff(span, handoff_id="ho-7f31")
    assert exporter.get_finished_spans()[0].attributes["trustabl.handoff_id"] == "ho-7f31"


def test_no_active_span_is_a_silent_no_op():
    Run().mark_start(None)
    mark_run_end(None, exit_reason="final_answer")
    mark_handoff(None, handoff_id="ho-1")
