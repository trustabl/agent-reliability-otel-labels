"""mark_reasoning labels a model call somebody else spanned. These tests pin the
claims each key makes, because a reasoning label that overclaims is worse than
none: absence says "unknown", and a wrong value reads as evidence."""

import pytest
from opentelemetry import trace

from agent_reliability_otel_labels import canonical_fp, keys, mark_reasoning

TOKENS = "gen_ai.usage.reasoning.output_tokens"


def _finished(exporter):
    spans = exporter.get_finished_spans()
    assert len(spans) == 1
    return spans[0]


def _cot_keys(attrs):
    return {k for k in attrs if k.startswith("trustabl.cot.")}


def test_sets_the_reasoning_labels_on_the_span(tracing):
    tracer, exporter = tracing
    with tracer.start_as_current_span("chat") as span:
        mark_reasoning(span, text="compare both fares", mode="explicit_cot", reasoning_tokens=12)
    attrs = _finished(exporter).attributes
    assert attrs["trustabl.cot.present"] is True
    assert attrs["trustabl.cot.text_fp"] == canonical_fp("compare both fares")
    assert attrs["trustabl.cot.mode"] == "explicit_cot"
    assert attrs[TOKENS] == 12


def test_no_information_writes_no_key(tracing):
    """Absence is a claim: a call that knows nothing must not say false."""
    tracer, exporter = tracing
    with tracer.start_as_current_span("chat") as span:
        mark_reasoning(span)
    assert not _cot_keys(_finished(exporter).attributes)


def test_empty_text_is_no_evidence(tracing):
    """A provider that hides its reasoning returns an empty string, which says
    nothing about whether the model reasoned."""
    tracer, exporter = tracing
    with tracer.start_as_current_span("chat") as span:
        mark_reasoning(span, text="")
    assert not _cot_keys(_finished(exporter).attributes)


def test_a_zero_token_count_means_no_reasoning(tracing):
    tracer, exporter = tracing
    with tracer.start_as_current_span("chat") as span:
        mark_reasoning(span, reasoning_tokens=0)
    attrs = _finished(exporter).attributes
    assert attrs["trustabl.cot.present"] is False
    assert attrs[TOKENS] == 0
    assert "trustabl.cot.text_fp" not in attrs


def test_a_declared_none_means_no_reasoning(tracing):
    tracer, exporter = tracing
    with tracer.start_as_current_span("chat") as span:
        mark_reasoning(span, mode="none")
    attrs = _finished(exporter).attributes
    assert attrs["trustabl.cot.present"] is False
    assert attrs["trustabl.cot.mode"] == "none"


def test_tokens_without_text_still_count_as_reasoning(tracing):
    """Hidden reasoning: the model spent tokens and showed no text."""
    tracer, exporter = tracing
    with tracer.start_as_current_span("chat") as span:
        mark_reasoning(span, mode="latent", reasoning_tokens=40)
    attrs = _finished(exporter).attributes
    assert attrs["trustabl.cot.present"] is True
    assert attrs["trustabl.cot.mode"] == "latent"
    assert "trustabl.cot.text_fp" not in attrs


def test_a_mode_alone_does_not_claim_this_call_reasoned(tracing):
    """Mode says how the agent reasons. Whether this call did is a separate
    fact, and nothing here shows it."""
    tracer, exporter = tracing
    with tracer.start_as_current_span("chat") as span:
        mark_reasoning(span, mode="latent")
    attrs = _finished(exporter).attributes
    assert attrs["trustabl.cot.mode"] == "latent"
    assert "trustabl.cot.present" not in attrs


def test_a_none_the_evidence_contradicts_is_omitted(tracing):
    """Degrade to absent, never to wrong: present=true beside mode=none would be
    a span arguing with itself, so the claim the evidence refutes is dropped."""
    tracer, exporter = tracing
    with tracer.start_as_current_span("chat") as span:
        mark_reasoning(span, text="it reasoned after all", mode="none")
    attrs = _finished(exporter).attributes
    assert attrs["trustabl.cot.present"] is True
    assert "trustabl.cot.mode" not in attrs


def test_positive_evidence_beats_a_zero_count(tracing):
    tracer, exporter = tracing
    with tracer.start_as_current_span("chat") as span:
        mark_reasoning(span, text="it reasoned", reasoning_tokens=0)
    assert _finished(exporter).attributes["trustabl.cot.present"] is True


def test_a_later_call_cannot_take_back_evidence(tracing):
    """A span must not argue with itself. Usage often arrives after the text, and
    a zero count there would otherwise leave present=false beside a fingerprint."""
    tracer, exporter = tracing
    with tracer.start_as_current_span("chat") as span:
        mark_reasoning(span, text="compare both fares")
        mark_reasoning(span, reasoning_tokens=0, mode="none")
    attrs = _finished(exporter).attributes
    assert attrs["trustabl.cot.present"] is True
    assert "trustabl.cot.mode" not in attrs
    assert attrs["trustabl.cot.text_fp"] == canonical_fp("compare both fares")


def test_rejects_an_unknown_mode(tracing):
    tracer, exporter = tracing
    with tracer.start_as_current_span("chat") as span:
        with pytest.raises(ValueError, match="mode"):
            mark_reasoning(span, text="compare both fares", mode="vibes")
    assert not _cot_keys(_finished(exporter).attributes)


def test_the_enum_check_comes_after_the_no_op_check():
    """A workload with tracing disabled must not be crashable by this library."""
    mark_reasoning(None, mode="vibes")


def test_no_active_span_is_a_silent_no_op():
    mark_reasoning(None, text="compare both fares", mode="explicit_cot")


def test_a_non_recording_span_is_a_silent_no_op():
    mark_reasoning(trace.INVALID_SPAN, text="compare both fares", mode="explicit_cot")


def test_reads_the_instrumentors_token_count(tracing):
    tracer, exporter = tracing
    with tracer.start_as_current_span("chat") as span:
        span.set_attribute(TOKENS, 7)
        mark_reasoning(span)
    attrs = _finished(exporter).attributes
    assert attrs["trustabl.cot.present"] is True
    assert attrs[TOKENS] == 7


def test_reads_a_zero_from_the_instrumentor(tracing):
    tracer, exporter = tracing
    with tracer.start_as_current_span("chat") as span:
        span.set_attribute(TOKENS, 0)
        mark_reasoning(span)
    assert _finished(exporter).attributes["trustabl.cot.present"] is False


def test_never_overwrites_the_instrumentors_token_count(tracing):
    tracer, exporter = tracing
    with tracer.start_as_current_span("chat") as span:
        span.set_attribute(TOKENS, 7)
        mark_reasoning(span, reasoning_tokens=99)
    assert _finished(exporter).attributes[TOKENS] == 7


class _OpaqueSpan:
    """A recording span that does not expose its attributes, which the
    OpenTelemetry API allows: reading them is an SDK affordance."""

    def __init__(self):
        self.written = {}

    def is_recording(self):
        return True

    def set_attribute(self, key, value):
        self.written[key] = value


def test_does_not_write_a_token_count_when_attributes_cannot_be_read():
    """An unknown value is never overwritten, and an unreadable span's value is
    unknown. The labels still land: the count is still evidence."""
    span = _OpaqueSpan()
    mark_reasoning(span, reasoning_tokens=12)
    assert TOKENS not in span.written
    assert span.written["trustabl.cot.present"] is True


def test_a_text_with_no_canonical_form_omits_only_the_fingerprint(tracing):
    """A lone surrogate has no JSON form. Reasoning text is still evidence that
    the model reasoned, so one bad character costs the hash and nothing else."""
    tracer, exporter = tracing
    with tracer.start_as_current_span("chat") as span:
        mark_reasoning(span, text="bad \ud800 surrogate", mode="explicit_cot")
    attrs = _finished(exporter).attributes
    assert attrs["trustabl.cot.present"] is True
    assert attrs["trustabl.cot.mode"] == "explicit_cot"
    assert "trustabl.cot.text_fp" not in attrs


@pytest.mark.parametrize("count", [-1, True, 1.5, "12"])
def test_an_invalid_token_count_is_ignored(tracing, count):
    tracer, exporter = tracing
    with tracer.start_as_current_span("chat") as span:
        mark_reasoning(span, reasoning_tokens=count)
    attrs = _finished(exporter).attributes
    assert TOKENS not in attrs
    assert not _cot_keys(attrs)


def test_a_text_that_is_not_a_string_is_ignored(tracing):
    tracer, exporter = tracing
    with tracer.start_as_current_span("chat") as span:
        mark_reasoning(span, text=["compare both fares"])
    assert not _cot_keys(_finished(exporter).attributes)


def test_the_same_reasoning_gives_the_same_fingerprint(tracing):
    tracer, exporter = tracing
    for text in ("compare both fares", "compare both fares", "pick the cheaper fare"):
        with tracer.start_as_current_span("chat") as span:
            mark_reasoning(span, text=text)
    first, second, third = [s.attributes["trustabl.cot.text_fp"] for s in exporter.get_finished_spans()]
    assert first == second
    assert first != third


def test_keys_stay_in_the_cot_namespace_and_reuse_the_token_key(tracing):
    """The token count is OTel's. Minting trustabl.cot.token_count would give
    one concept two keys, which the attribute spec's rule 3 forbids."""
    tracer, exporter = tracing
    with tracer.start_as_current_span("chat") as span:
        mark_reasoning(span, text="compare both fares", mode="hybrid", reasoning_tokens=12)
    attrs = _finished(exporter).attributes
    assert _cot_keys(attrs) == {keys.COT_PRESENT, keys.COT_TEXT_FP, keys.COT_MODE}
    assert set(attrs) - _cot_keys(attrs) == {keys.GEN_AI_REASONING_TOKENS}
