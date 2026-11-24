"""The three labels the P1 rules need: expected tools, retrieval result count,
progress marker. Same invariants as the other helpers: absent beats wrong, bad
input is rejected only when tracing is on, and no content reaches a span."""

import pytest
from opentelemetry import trace

from agent_reliability_otel_labels import (
    mark_expected_tools, mark_progress, mark_retrieval_span,
)

SECRET = "patient Jane Doe, SSN 123-45-6789"


def _finished(exporter):
    spans = exporter.get_finished_spans()
    assert len(spans) == 1
    return spans[0]


def test_expected_tools_are_written_on_the_root_span(tracing):
    tracer, exporter = tracing
    with tracer.start_as_current_span("invoke_agent") as root:
        mark_expected_tools(root, tools=["search_flights", "book_flight", "search_flights"])
    assert list(_finished(exporter).attributes["trustabl.expected_tools"]) == [
        "search_flights", "book_flight"]


def test_empty_expected_tools_leave_the_key_off(tracing):
    tracer, exporter = tracing
    with tracer.start_as_current_span("invoke_agent") as root:
        mark_expected_tools(root, tools=[])
    assert "trustabl.expected_tools" not in _finished(exporter).attributes


@pytest.mark.parametrize("bad", ["search", [""], [1], [None]])
def test_expected_tools_rejects_anything_but_a_list_of_names(tracing, bad):
    tracer, _ = tracing
    with tracer.start_as_current_span("invoke_agent") as root:
        with pytest.raises(ValueError):
            mark_expected_tools(root, tools=bad)


def test_retrieval_result_count_is_written_including_zero(tracing):
    tracer, exporter = tracing
    with tracer.start_as_current_span("retrieval") as span:
        mark_retrieval_span(span, result_count=0, step_index=2, run_id="r1")
    attrs = _finished(exporter).attributes
    assert attrs["trustabl.retrieval.result_count"] == 0
    assert attrs["trustabl.step_index"] == 2
    assert attrs["trustabl.run_id"] == "r1"


@pytest.mark.parametrize("bad", [-1, 1.5, "3", True, None])
def test_retrieval_result_count_rejects_anything_but_a_non_negative_integer(tracing, bad):
    tracer, _ = tracing
    with tracer.start_as_current_span("retrieval") as span:
        with pytest.raises(ValueError):
            mark_retrieval_span(span, result_count=bad)


def test_progress_marker_is_a_fingerprint_that_follows_the_state(tracing):
    tracer, exporter = tracing
    with tracer.start_as_current_span("execute_tool") as a:
        mark_progress(a, state={"done": ["flights"], "todo": ["hotel"]})
    with tracer.start_as_current_span("execute_tool") as b:
        mark_progress(b, state={"todo": ["hotel"], "done": ["flights"]})
    with tracer.start_as_current_span("execute_tool") as c:
        mark_progress(c, state={"done": ["flights", "hotel"], "todo": []})
    one, two, three = (s.attributes["trustabl.progress_marker"]
                       for s in exporter.get_finished_spans())
    assert len(one) == 16
    assert one == two
    assert one != three


def test_progress_marker_is_omitted_for_a_state_with_no_canonical_form(tracing):
    tracer, exporter = tracing
    with tracer.start_as_current_span("execute_tool") as span:
        mark_progress(span, state={"bad": float("nan")})
    assert "trustabl.progress_marker" not in _finished(exporter).attributes


def test_no_state_or_tool_text_reaches_the_new_attributes(tracing):
    tracer, exporter = tracing
    with tracer.start_as_current_span("execute_tool") as span:
        mark_progress(span, state={"note": SECRET})
        mark_retrieval_span(span, result_count=3)
    attrs = _finished(exporter).attributes
    assert SECRET not in str(dict(attrs))


def test_the_new_helpers_do_nothing_without_a_recording_span():
    for span in (None, trace.INVALID_SPAN):
        mark_expected_tools(span, tools=["a"])
        mark_retrieval_span(span, result_count=1)
        mark_progress(span, state={"a": 1})
    mark_retrieval_span(trace.INVALID_SPAN, result_count=-5)  # tracing off never raises
