"""Decorating tool spans.

Nothing here starts a span. The caller owns the span's lifecycle; we attach
attributes to one that some other instrumentation library already created.
"""

from agent_reliability_otel_labels import keys
from agent_reliability_otel_labels.fingerprint import canonical_fp


def _unusable(span) -> bool:
    """True when there is nothing safe to write to.

    Checked BEFORE argument validation on purpose: a workload running with
    tracing disabled must never be crashed by this library, and a bad enum
    value surfaces the moment anyone runs with tracing on, which is every test
    and every development run.
    """
    if span is None:
        return True
    is_recording = getattr(span, "is_recording", None)
    if not callable(is_recording):
        return True
    try:
        return not span.is_recording()
    except Exception:
        return True


#: What _existing returns when the span's attributes cannot be read at all.
#: Distinct from None, which means the key is absent.
_UNREADABLE = object()


def _existing(span, key):
    """The attribute already on the span, None when absent, or _UNREADABLE.

    Reading a live span's attributes is an SDK affordance, not an API
    guarantee, so an unreadable span degrades to 'unknown' - and an unknown
    value is never overwritten.
    """
    attrs = getattr(span, "attributes", None)
    if attrs is None:
        return _UNREADABLE
    try:
        return attrs.get(key)
    except Exception:
        return _UNREADABLE


def _fingerprint_or_none(value):
    """The fingerprint, or None for a value with no canonical form.

    Caught per value so one bad result does not cost the input fingerprint,
    and so nothing reaches the workload. Broad on purpose: the spec says
    nothing from fingerprinting may reach the caller, and not every failure
    is a CanonicalizationError - a circular value raises RecursionError, and
    an object with a broken __iter__/items/property can raise anything.
    """
    try:
        return canonical_fp(value)
    except Exception:
        return None


def mark_tool_span(span, *, args, result, attempt, side_effect=None,
                   error_class=None, step_index=None, run_id=None, name=None):
    """Attach the process labels to one tool-execution span.

    args and result are fingerprinted, never stored: the hash answers "was this
    the same call?" without carrying what the caller asked for.

    Leave side_effect as None when the tool's class is not known. The key is
    then omitted, which says "unknown"; guessing "pure" would be a false claim.
    """
    if _unusable(span):
        return

    if side_effect is not None and side_effect not in keys.SIDE_EFFECTS:
        raise ValueError(
            f"side_effect must be one of {sorted(keys.SIDE_EFFECTS)}, got {side_effect!r}"
        )
    if error_class is not None and error_class not in keys.ERROR_CLASSES:
        raise ValueError(
            f"error_class must be one of {sorted(keys.ERROR_CLASSES)}, got {error_class!r}"
        )

    input_fp = _fingerprint_or_none(args)
    if input_fp is not None:
        span.set_attribute(keys.TOOL_INPUT_FP, input_fp)
    output_fp = _fingerprint_or_none(result)
    if output_fp is not None:
        span.set_attribute(keys.TOOL_OUTPUT_FP, output_fp)
    span.set_attribute(keys.TOOL_ATTEMPT, attempt)

    if side_effect is not None:
        span.set_attribute(keys.TOOL_SIDE_EFFECT, side_effect)
    if error_class is not None:
        span.set_attribute(keys.TOOL_ERROR_CLASS, error_class)
    if step_index is not None:
        span.set_attribute(keys.STEP_INDEX, step_index)
    if run_id is not None:
        span.set_attribute(keys.RUN_ID, run_id)

    # Their key, their value. We fill it only when the instrumentor left it
    # empty, and never when we cannot tell.
    if name is not None and _existing(span, "gen_ai.tool.name") is None:
        span.set_attribute("gen_ai.tool.name", name)


def mark_retrieval_span(span, *, result_count, step_index=None, run_id=None):
    """Attach the result count to one retrieval span (vector search, RAG lookup).

    Only the count is written, never the documents or the query. Zero is a real
    observation ("the search found nothing"), so it is written, not omitted.
    """
    if _unusable(span):
        return
    if isinstance(result_count, bool) or not isinstance(result_count, int) or result_count < 0:
        raise ValueError(f"result_count must be an integer >= 0, got {result_count!r}")

    span.set_attribute(keys.RETRIEVAL_RESULT_COUNT, result_count)
    if step_index is not None:
        span.set_attribute(keys.STEP_INDEX, step_index)
    if run_id is not None:
        span.set_attribute(keys.RUN_ID, run_id)


def mark_progress(span, *, state):
    """Attach a progress marker: the fingerprint of the task state after this step.

    state is whatever the caller treats as "where the task stands" (a plan, a
    checklist, a working-set summary). It is fingerprinted, never stored. Two
    consecutive steps with the same marker made no progress. A state with no
    canonical form leaves the key off, which says "unknown".
    """
    if _unusable(span):
        return
    fp = _fingerprint_or_none(state)
    if fp is not None:
        span.set_attribute(keys.PROGRESS_MARKER, fp)
