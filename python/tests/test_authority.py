"""Policy bindings: which signed policies were in force when the agent ran.

One event per policy. A single flattened hash is the bug this whole shape
exists to prevent - it reports ACS as present when only OpenShell loaded.
"""

import pytest

from agent_reliability_otel_labels import Binding, bind_authority


def _events(exporter):
    spans = exporter.get_finished_spans()
    assert len(spans) == 1
    return list(spans[0].events)


OPENSHELL = Binding(id="openshell-seccomp-v3", type="sandbox",
                    sha256="a" * 64, version="3.1.0", required=True,
                    source="entrypoint")
ACS = Binding(id="acs-content-v8", type="content_safety", sha256="b" * 64,
              version="8.0.2", required=True, source="entrypoint")


def test_emits_one_event_per_policy(tracing):
    tracer, exporter = tracing
    with tracer.start_as_current_span("invoke_agent") as span:
        bind_authority(span, bindings=[OPENSHELL, ACS])
    events = _events(exporter)
    assert len(events) == 2
    assert {e.name for e in events} == {"trustabl.policy.binding"}


def test_each_event_carries_its_own_hash(tracing):
    """Two rails, two hashes, independently stale-able. Never one combined."""
    tracer, exporter = tracing
    with tracer.start_as_current_span("invoke_agent") as span:
        bind_authority(span, bindings=[OPENSHELL, ACS])
    hashes = {e.attributes["trustabl.policy.binding.sha256"] for e in _events(exporter)}
    assert hashes == {"a" * 64, "b" * 64}


def test_event_carries_the_full_binding_record(tracing):
    tracer, exporter = tracing
    with tracer.start_as_current_span("invoke_agent") as span:
        bind_authority(span, bindings=[OPENSHELL])
    attrs = _events(exporter)[0].attributes
    assert attrs["trustabl.policy.binding.id"] == "openshell-seccomp-v3"
    assert attrs["trustabl.policy.binding.type"] == "sandbox"
    assert attrs["trustabl.policy.binding.version"] == "3.1.0"
    assert attrs["trustabl.policy.binding.required"] is True
    assert attrs["trustabl.policy.binding.source"] == "entrypoint"


def test_a_subagent_with_no_bindings_emits_no_events(tracing):
    """A handoff with no bindings is a coverage hole, not an implicit copy of
    the parent. Emitting nothing is what makes the hole visible."""
    tracer, exporter = tracing
    with tracer.start_as_current_span("invoke_agent") as span:
        bind_authority(span, bindings=[])
    assert _events(exporter) == []


def test_no_active_span_is_a_silent_no_op():
    bind_authority(None, bindings=[OPENSHELL])


def test_rejects_an_unknown_binding_type(tracing):
    tracer, _ = tracing
    with tracer.start_as_current_span("invoke_agent") as span:
        with pytest.raises(ValueError):
            bind_authority(span, bindings=[
                Binding(id="x", type="firewall", sha256="c" * 64,
                        version="1", required=True, source="entrypoint")])


def test_rejects_an_unknown_binding_source(tracing):
    tracer, _ = tracing
    with tracer.start_as_current_span("invoke_agent") as span:
        with pytest.raises(ValueError):
            bind_authority(span, bindings=[
                Binding(id="x", type="sandbox", sha256="c" * 64,
                        version="1", required=True, source="guessed")])
