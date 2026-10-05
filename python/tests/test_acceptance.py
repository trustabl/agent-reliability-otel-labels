"""The spec's acceptance criteria, as tests."""

import json

import agent_reliability_otel_labels
from agent_reliability_otel_labels import Binding, Run, bind_authority, mark_tool_span


def test_no_active_span_creates_no_span(tracing):
    """Not merely 'does not raise': nothing may reach the exporter either."""
    _, exporter = tracing
    mark_tool_span(None, args={"a": 1}, result={}, attempt=1, side_effect="read")
    bind_authority(None, bindings=[])
    Run().mark_start(None)
    assert exporter.get_finished_spans() == ()


def test_the_public_api_offers_no_way_to_fingerprint_a_completion():
    """Chat is not supposed to repeat, so a differing completion is not a
    finding. The guarantee is structural: no entry point exists that would
    fingerprint one. This fails the moment somebody adds mark_llm_span."""
    surface = set(agent_reliability_otel_labels.__all__)
    forbidden = {"mark_llm_span", "mark_chat_span", "mark_completion",
                 "fingerprint_completion", "mark_model_span"}
    assert surface & forbidden == set()


def test_a_child_span_does_not_inherit_the_parents_bindings(tracing):
    """A handoff carrying no binding is a coverage hole. Copying the parent's
    hashes down would report the child as protected when it was not."""
    tracer, exporter = tracing
    parent_binding = Binding(id="openshell-seccomp-v3", type="sandbox",
                             sha256="a" * 64, version="3.1.0", required=True,
                             source="entrypoint")
    with tracer.start_as_current_span("invoke_agent") as parent:
        bind_authority(parent, bindings=[parent_binding])
        with tracer.start_as_current_span("invoke_agent") as child:
            bind_authority(child, bindings=[])

    by_name = {}
    for span in exporter.get_finished_spans():
        by_name.setdefault(len(span.events), []).append(span)

    events_per_span = sorted(len(s.events) for s in exporter.get_finished_spans())
    assert events_per_span == [0, 1], "the child must carry no binding events"

    child_span = min(exporter.get_finished_spans(), key=lambda s: len(s.events))
    written = json.dumps([dict(e.attributes) for e in child_span.events])
    assert "a" * 64 not in written
