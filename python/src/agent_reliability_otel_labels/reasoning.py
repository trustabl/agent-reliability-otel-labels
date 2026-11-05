"""Labelling a model call's reasoning without carrying its text.

A chain of thought quotes whatever the model read, so it is the most sensitive
text this library is ever handed. It is hashed and dropped here; only the
fingerprint reaches the span.

These labels say that a model reasoned and which text it produced. They do not
say the reasoning was faithful: present means reasoning occurred, not that it
caused the answer.
"""

from agent_reliability_otel_labels import keys
from agent_reliability_otel_labels.spans import _existing, _fingerprint_or_none, _unusable


def _token_count(value):
    """value when it is a usable count, otherwise None.

    A bad count is ignored rather than raised: unlike an enum typo, it can come
    from a provider's response at run time, and a workload must not crash on it.
    """
    if isinstance(value, bool) or not isinstance(value, int) or value < 0:
        return None
    return value


def mark_reasoning(span, *, text=None, mode=None, reasoning_tokens=None):
    """Attach the reasoning labels to one model-call span.

    text is fingerprinted, never stored. Pass the reasoning as a string, not the
    answer: the hash exists to match a text captured elsewhere to this span, and
    a differing hash is not a finding. An empty string says nothing, because a
    provider that hides its reasoning returns one.

    mode says how the agent reasons. Leave it as None when that is not known:
    the key is then omitted, which says "unknown".

    reasoning_tokens fills gen_ai.usage.reasoning.output_tokens only when the
    instrumentor left it empty. That key is OTel's, and when it is already on
    the span it decides trustabl.cot.present without being passed here.

    A call that says nothing either way writes nothing: an omitted
    trustabl.cot.present means "unknown", and false would be a false claim.
    """
    if _unusable(span):
        return

    if mode is not None and mode not in keys.COT_MODES:
        raise ValueError(f"mode must be one of {sorted(keys.COT_MODES)}, got {mode!r}")

    has_text = isinstance(text, str) and text != ""
    given = _token_count(reasoning_tokens)
    on_span = _existing(span, keys.GEN_AI_REASONING_TOKENS)
    counts = [c for c in (given, _token_count(on_span)) if c is not None]

    # Evidence already on the span stands: usage often arrives after the text, and
    # a zero count must not leave present=false beside a fingerprint.
    present = None
    if has_text or any(c > 0 for c in counts) or _existing(span, keys.COT_PRESENT) is True:
        present = True
    elif mode == "none" or counts:
        present = False

    if present is not None:
        span.set_attribute(keys.COT_PRESENT, present)
    if has_text:
        fingerprint = _fingerprint_or_none(text)
        if fingerprint is not None:
            span.set_attribute(keys.COT_TEXT_FP, fingerprint)
    # A declared "none" the evidence refutes is dropped: present=true beside
    # mode=none would be a span arguing with itself.
    if mode is not None and not (mode == "none" and present):
        span.set_attribute(keys.COT_MODE, mode)

    # Their key, their value. We fill it only when the instrumentor left it
    # empty, and never when we cannot tell.
    if given is not None and on_span is None:
        span.set_attribute(keys.GEN_AI_REASONING_TOKENS, given)
