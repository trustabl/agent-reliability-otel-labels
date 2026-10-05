"""mark_tool_span decorates a span somebody else created. These tests pin the
invariants that make that safe."""

import datetime

import pytest
from opentelemetry import trace

from agent_reliability_otel_labels import mark_tool_span


def _finished(exporter):
    spans = exporter.get_finished_spans()
    assert len(spans) == 1
    return spans[0]


def test_sets_the_process_keys_on_the_span(tracing):
    tracer, exporter = tracing
    with tracer.start_as_current_span("execute_tool") as span:
        mark_tool_span(span, args={"to": "JFK"}, result={"flights": 3},
                       attempt=3, side_effect="read", step_index=7)
    attrs = _finished(exporter).attributes
    assert attrs["trustabl.tool.attempt"] == 3
    assert attrs["trustabl.tool.side_effect"] == "read"
    assert attrs["trustabl.step_index"] == 7
    assert len(attrs["trustabl.tool.input_fp"]) == 16
    assert len(attrs["trustabl.tool.output_fp"]) == 16


def test_omits_error_class_when_there_was_no_error(tracing):
    """Absence is a claim: an omitted key says 'no error observed', where
    error_class='none' would invent an observation nobody made."""
    tracer, exporter = tracing
    with tracer.start_as_current_span("execute_tool") as span:
        mark_tool_span(span, args={}, result={}, attempt=1, side_effect="read")
    assert "trustabl.tool.error_class" not in _finished(exporter).attributes


def test_omits_side_effect_when_the_caller_does_not_know_it(tracing):
    """Degrade to absent, never to wrong: an unknown side-effect class is left
    off the span rather than guessed as 'pure'."""
    tracer, exporter = tracing
    with tracer.start_as_current_span("execute_tool") as span:
        mark_tool_span(span, args={}, result={}, attempt=1)
    assert "trustabl.tool.side_effect" not in _finished(exporter).attributes


def test_pure_marks_a_tool_with_no_side_effect(tracing):
    tracer, exporter = tracing
    with tracer.start_as_current_span("execute_tool") as span:
        mark_tool_span(span, args={}, result={}, attempt=1, side_effect="pure")
    assert _finished(exporter).attributes["trustabl.tool.side_effect"] == "pure"


def test_rejects_none_as_a_side_effect_class(tracing):
    """'none' was renamed to 'pure' so it cannot be confused with an omitted
    (unknown) key or with bind.method=none."""
    tracer, _ = tracing
    with tracer.start_as_current_span("execute_tool") as span:
        with pytest.raises(ValueError):
            mark_tool_span(span, args={}, result={}, attempt=1, side_effect="none")


def test_stamps_the_run_id_on_the_tool_span(tracing):
    """Process rules group tool spans by run; without run_id here every
    consumer has to walk up to the root to find it."""
    tracer, exporter = tracing
    with tracer.start_as_current_span("execute_tool") as span:
        mark_tool_span(span, args={}, result={}, attempt=1, run_id="0123456789abcdef")
    assert _finished(exporter).attributes["trustabl.run_id"] == "0123456789abcdef"


def test_omits_run_id_when_none_is_given(tracing):
    tracer, exporter = tracing
    with tracer.start_as_current_span("execute_tool") as span:
        mark_tool_span(span, args={}, result={}, attempt=1)
    assert "trustabl.run_id" not in _finished(exporter).attributes


def test_does_not_overwrite_a_tool_name_the_instrumentor_set(tracing):
    tracer, exporter = tracing
    with tracer.start_as_current_span("execute_tool") as span:
        span.set_attribute("gen_ai.tool.name", "search_flights")
        mark_tool_span(span, args={}, result={}, attempt=1,
                       side_effect="read", name="something_else")
    assert _finished(exporter).attributes["gen_ai.tool.name"] == "search_flights"


def test_no_active_span_is_a_silent_no_op():
    """A span we invented is not evidence of anything, and it corrupts the trace
    shape the binder depends on. Do nothing instead."""
    mark_tool_span(None, args={"a": 1}, result={}, attempt=1, side_effect="read")


def test_a_non_recording_span_is_a_silent_no_op():
    mark_tool_span(trace.INVALID_SPAN, args={"a": 1}, result={},
                   attempt=1, side_effect="read")


class _RaisingIsRecording:
    def is_recording(self):
        raise RuntimeError("boom")


def test_a_span_whose_is_recording_raises_is_a_silent_no_op():
    mark_tool_span(_RaisingIsRecording(), args={"a": 1}, result={}, attempt=1)


def test_rejects_an_unknown_side_effect_class(tracing):
    tracer, _ = tracing
    with tracer.start_as_current_span("execute_tool") as span:
        with pytest.raises(ValueError):
            mark_tool_span(span, args={}, result={}, attempt=1, side_effect="destroys")


def test_an_uncanonicalizable_argument_omits_only_the_input_fingerprint(tracing):
    """Degrade to absent, never to wrong - and never crash the workload."""
    tracer, exporter = tracing
    with tracer.start_as_current_span("execute_tool") as span:
        mark_tool_span(span, args={"n": 2**53}, result={"ok": True}, attempt=1,
                       side_effect="read", step_index=0)
    attrs = _finished(exporter).attributes
    assert "trustabl.tool.input_fp" not in attrs
    assert len(attrs["trustabl.tool.output_fp"]) == 16
    assert attrs["trustabl.tool.attempt"] == 1
    assert attrs["trustabl.tool.side_effect"] == "read"
    assert attrs["trustabl.step_index"] == 0


def test_an_uncanonicalizable_result_omits_only_the_output_fingerprint(tracing):
    tracer, exporter = tracing
    with tracer.start_as_current_span("execute_tool") as span:
        mark_tool_span(span, args={"to": "JFK"},
                       result={"at": datetime.datetime(2026, 9, 28)}, attempt=2)
    attrs = _finished(exporter).attributes
    assert "trustabl.tool.output_fp" not in attrs
    assert len(attrs["trustabl.tool.input_fp"]) == 16
    assert attrs["trustabl.tool.attempt"] == 2


class _OpaqueSpan:
    """A recording span that does not expose its attributes, which the
    OpenTelemetry API allows: reading them is an SDK affordance."""

    def __init__(self):
        self.written = {}

    def is_recording(self):
        return True

    def set_attribute(self, key, value):
        self.written[key] = value


def test_does_not_write_a_tool_name_when_attributes_cannot_be_read():
    """Unknown is not absent. The instrumentor may have set a name we cannot
    see, and overwriting it would replace their value with ours."""
    span = _OpaqueSpan()
    mark_tool_span(span, args={}, result={}, attempt=1, name="search_flights")
    assert "gen_ai.tool.name" not in span.written
    assert span.written["trustabl.tool.attempt"] == 1


def test_fills_the_tool_name_when_the_instrumentor_left_it_empty(tracing):
    tracer, exporter = tracing
    with tracer.start_as_current_span("execute_tool") as span:
        mark_tool_span(span, args={}, result={}, attempt=1, name="search_flights")
    assert _finished(exporter).attributes["gen_ai.tool.name"] == "search_flights"


def test_a_circular_argument_omits_only_the_input_fingerprint(tracing):
    """A cycle raises RecursionError, not CanonicalizationError. The spec says
    nothing may reach the workload, so mark_tool_span must catch this too."""
    tracer, exporter = tracing
    args = {}
    args["self"] = args
    with tracer.start_as_current_span("execute_tool") as span:
        mark_tool_span(span, args=args, result={"ok": True}, attempt=1)
    attrs = _finished(exporter).attributes
    assert "trustabl.tool.input_fp" not in attrs
    assert len(attrs["trustabl.tool.output_fp"]) == 16


class _BadItemsDict(dict):
    """A dict subclass whose .items() raises, so canonical_fp raises something
    that is genuinely not a CanonicalizationError."""

    def items(self):
        raise RuntimeError("boom")


def test_a_throwing_value_omits_only_the_output_fingerprint(tracing):
    tracer, exporter = tracing
    with tracer.start_as_current_span("execute_tool") as span:
        mark_tool_span(span, args={"to": "JFK"}, result=_BadItemsDict({"a": 1}), attempt=1)
    attrs = _finished(exporter).attributes
    assert "trustabl.tool.output_fp" not in attrs
    assert len(attrs["trustabl.tool.input_fp"]) == 16
