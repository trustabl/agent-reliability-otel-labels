"""We attach to spans we did not shape.

The corpus captured three incompatible attribute conventions. A span from any
of them must take our labels without us disturbing theirs.
"""

import pytest

from agent_reliability_otel_labels import mark_tool_span

CONVENTIONS = [
    ("genai", {"gen_ai.operation.name": "execute_tool",
               "gen_ai.tool.name": "search_flights"}),
    ("openinference", {"openinference.span.kind": "TOOL",
                       "tool.name": "search_flights"}),
    ("traceloop", {"traceloop.span.kind": "tool",
                   "traceloop.entity.name": "search_flights"}),
]


@pytest.mark.parametrize("convention,attrs", CONVENTIONS,
                         ids=[c for c, _ in CONVENTIONS])
def test_labels_attach_to_a_span_from_any_convention(tracing, convention, attrs):
    tracer, exporter = tracing
    with tracer.start_as_current_span("tool") as span:
        for key, value in attrs.items():
            span.set_attribute(key, value)
        mark_tool_span(span, args={"to": "JFK"}, result={},
                       attempt=1, side_effect="read", step_index=0)

    written = exporter.get_finished_spans()[0].attributes
    assert written["trustabl.tool.side_effect"] == "read"
    for key, value in attrs.items():
        assert written[key] == value, f"{convention}: we disturbed {key}"
