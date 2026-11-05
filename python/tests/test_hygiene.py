"""The privacy guarantee, asserted rather than promised.

Everything this library writes is an id, an enum, an integer or a hash. If a
caller's arguments or results ever reach a span attribute, the whole "process
metadata, not content" position is void - so this is a test, not a convention.
"""

import json

from agent_reliability_otel_labels import (
    Binding, Run, bind_authority, canonical_fp, keys, mark_reasoning, mark_run_end, mark_tool_span,
)

SECRET = "patient Jane Doe, SSN 123-45-6789, diagnosis withheld"


def _everything_written(span_data):
    """Every attribute value and every event attribute value, as one string."""
    parts = [json.dumps(dict(span_data.attributes), default=str)]
    for event in span_data.events:
        parts.append(json.dumps(dict(event.attributes), default=str))
    return " ".join(parts)


def test_no_argument_or_result_text_reaches_any_attribute(tracing):
    tracer, exporter = tracing
    run = Run()
    with tracer.start_as_current_span("execute_tool") as span:
        run.mark_start(span)
        mark_tool_span(
            span,
            args={"query": SECRET, "nested": {"note": SECRET}},
            result={"records": [{"body": SECRET}]},
            attempt=1,
            side_effect="read",
            step_index=run.next_step(),
        )
        bind_authority(span, bindings=[
            Binding(id="dspm-prod-v2", type="data_security", sha256="d" * 64,
                    version="2.1.0", required=True, source="entrypoint")])
        mark_run_end(span, exit_reason="final_answer")

    written = _everything_written(exporter.get_finished_spans()[0])
    assert SECRET not in written
    assert "Jane Doe" not in written
    assert "123-45-6789" not in written


def test_no_reasoning_text_reaches_any_attribute(tracing):
    """A chain of thought quotes whatever the model read, so this is the most
    sensitive text the library is ever handed. Only its fingerprint may leave."""
    tracer, exporter = tracing
    with tracer.start_as_current_span("chat") as span:
        mark_reasoning(span, text=SECRET, mode="explicit_cot", reasoning_tokens=12)

    finished = exporter.get_finished_spans()[0]
    written = _everything_written(finished)
    assert SECRET not in written
    assert "Jane Doe" not in written
    assert "123-45-6789" not in written
    assert finished.attributes[keys.COT_TEXT_FP] == canonical_fp(SECRET)


def test_every_attribute_we_write_is_bounded_in_length(tracing):
    """Fixed-length values are what keep a vendor's cardinality bill sane and
    what stop content sneaking in through a long value."""
    tracer, exporter = tracing
    with tracer.start_as_current_span("execute_tool") as span:
        mark_tool_span(span, args={"blob": "x" * 50_000}, result={"blob": "y" * 50_000},
                       attempt=1, side_effect="read", step_index=0)
        mark_reasoning(span, text="z" * 50_000, mode="explicit_cot")

    attrs = exporter.get_finished_spans()[0].attributes
    for key, value in attrs.items():
        if isinstance(value, str):
            assert len(value) <= 64, f"{key} is unbounded"


def test_fingerprints_are_exactly_the_declared_length(tracing):
    tracer, exporter = tracing
    with tracer.start_as_current_span("execute_tool") as span:
        mark_tool_span(span, args={"a": 1}, result={"b": 2},
                       attempt=1, side_effect="read")
        mark_reasoning(span, text="compare both fares")
    attrs = exporter.get_finished_spans()[0].attributes
    assert len(attrs[keys.TOOL_INPUT_FP]) == 16
    assert len(attrs[keys.TOOL_OUTPUT_FP]) == 16
    assert len(attrs[keys.COT_TEXT_FP]) == 16

# The fake secret above is deliberate: these tests assert it NEVER reaches a
# span. A credential scanner flagging it has found the test working, not a leak.
