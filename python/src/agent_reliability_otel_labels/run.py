"""Run identity, step order, and how a run ended.

step_index is assigned in process, in call order, because OTLP batching can
split and reorder a run on the wire. A consumer that sorts by step_index gets
the true sequence back; without it, "the agent answered before it searched"
is indistinguishable from a reordered export.
"""

import itertools
import uuid

from agent_reliability_otel_labels import keys
from agent_reliability_otel_labels.spans import _unusable


class Run:
    """One user task, and the step counter for it.

    Not thread-safe by design: a run is a single logical sequence. An agent
    fanning out concurrently should carry one Run per logical run, not share
    one across them, or the step order it records is not the order anything
    happened in.
    """

    def __init__(self, run_id=None):
        self.run_id = run_id or uuid.uuid4().hex[:16]
        self._steps = itertools.count()

    def next_step(self) -> int:
        return next(self._steps)

    def mark_start(self, span) -> None:
        """Stamp the run identity on the root span."""
        if _unusable(span):
            return
        span.set_attribute(keys.RUN_ID, self.run_id)


def mark_run_end(span, *, exit_reason) -> None:
    """Record how the run finished.

    Call this before the root span ends. An ended span stops recording, so a
    late call is a silent no-op and the run has no exit_reason.

    `max_steps` is the one worth the trouble: an agent quietly hitting its
    limit every time looks successful from the outside.
    """
    if _unusable(span):
        return
    if exit_reason not in keys.EXIT_REASONS:
        raise ValueError(
            f"exit_reason must be one of {sorted(keys.EXIT_REASONS)}, got {exit_reason!r}"
        )
    span.set_attribute(keys.EXIT_REASON, exit_reason)


def mark_handoff(span, *, handoff_id) -> None:
    """Record that this agent span came from a delegation.

    The handoff id is what lets a consumer ask whether the child carried any
    policy of its own - a handoff with no bindings is a coverage hole.
    """
    if _unusable(span):
        return
    span.set_attribute(keys.HANDOFF_ID, handoff_id)
