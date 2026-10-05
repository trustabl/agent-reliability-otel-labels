"""agent-reliability-otel-labels: process-metadata labels for OpenTelemetry agent spans.

This package never creates spans. It adds attributes to spans that some other
instrumentation library already created, so an agent must already be
instrumented for any of this to do anything.
"""

from agent_reliability_otel_labels import keys
from agent_reliability_otel_labels.fingerprint import canonical_fp, canonical_json, strip_volatile
from agent_reliability_otel_labels.authority import Binding, bind_authority
from agent_reliability_otel_labels.run import Run, mark_handoff, mark_run_end
from agent_reliability_otel_labels.spans import mark_tool_span

__all__ = [
    "canonical_fp",
    "canonical_json",
    "strip_volatile",
    "mark_tool_span",
    "Binding",
    "bind_authority",
    "Run",
    "mark_run_end",
    "mark_handoff",
    "keys",
]
