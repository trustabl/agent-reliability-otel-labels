"""The canonical serialization, pinned as a string.

A port that only compares hashes gets "they differ" and no clue why. Pinning
the exact bytes each language must produce turns a port bug into a visible
character-level diff.

The form is RFC 8785 (JCS): numbers and strings exactly as ECMAScript's
JSON.stringify writes them, keys sorted by UTF-16 code units, UTF-8 raw. Every
binding has a maintained implementation of it, so none has to imitate another.
"""

import datetime

import pytest
import rfc8785

from agent_reliability_otel_labels import canonical_json


def test_keys_are_sorted_and_separators_are_compact():
    assert canonical_json({"b": 2, "a": 1}) == '{"a":1,"b":2}'


def test_non_ascii_is_emitted_raw_not_escaped():
    assert canonical_json({"q": "Tōkyō"}) == '{"q":"Tōkyō"}'


def test_volatile_fields_are_gone_before_serialization():
    assert canonical_json({"a": 1, "request_id": "r"}) == '{"a":1}'


def test_whole_floats_render_as_integers():
    """JavaScript has one number type: after JSON.parse, 1.0 IS 1. A Python-only
    `1.0` would make the same call fingerprint differently per language."""
    assert canonical_json({"f": 1.0}) == '{"f":1}'


def test_integers_have_no_decimal_point():
    assert canonical_json({"i": 42}) == '{"i":42}'


def test_html_characters_are_not_escaped():
    """Go's encoding/json escapes <, > and & unless told not to."""
    assert canonical_json({"q": "a<b>c&d"}) == '{"q":"a<b>c&d"}'


def test_large_safe_floats_render_positionally():
    """json.dumps writes 1e+15; ECMAScript writes every digit below 1e21."""
    assert canonical_json({"n": 1e15}) == '{"n":1000000000000000}'


def test_small_floats_render_positionally():
    """json.dumps writes 1e-05; ECMAScript writes 0.00001."""
    assert canonical_json({"n": 1e-5}) == '{"n":0.00001}'


def test_tiny_floats_use_an_exponent():
    assert canonical_json({"n": 1e-7}) == '{"n":1e-7}'


def test_negative_zero_renders_as_zero():
    assert canonical_json({"n": -0.0}) == '{"n":0}'


def test_keys_sort_by_utf16_code_units():
    """Code-point order puts U+FB01 first; UTF-16 order puts U+1F600 first,
    because its high surrogate D83D sorts below FB01. JavaScript sorts by
    UTF-16, so RFC 8785 does too."""
    assert canonical_json({"ﬁ": 1, "\U0001F600": 2}) == '{"\U0001F600":2,"ﬁ":1}'


def test_control_characters_are_escaped_as_ecmascript_does():
    assert (canonical_json({"s": 'a\x00b\tc\nd\x1fe"f\\g'})
            == '{"s":"a\\u0000b\\tc\\nd\\u001fe\\"f\\\\g"}')


def test_rejects_an_integer_beyond_the_safe_range():
    """Past 2^53 a double cannot hold every integer, so JavaScript would have
    rounded it already. Two different calls could then hash alike."""
    with pytest.raises(rfc8785.CanonicalizationError):
        canonical_json({"n": 2**53})


def test_accepts_the_largest_safe_integer():
    assert canonical_json({"n": 2**53 - 1}) == '{"n":9007199254740991}'


def test_rejects_a_whole_float_beyond_the_safe_range():
    """JSON.parse gives 1e16 and 10000000000000000 the same value, so the
    TypeScript binding must refuse both. Python refuses both too, or it would
    fingerprint what TypeScript cannot."""
    with pytest.raises(rfc8785.CanonicalizationError):
        canonical_json({"n": 1e16})


@pytest.mark.parametrize("value", [float("nan"), float("inf"), float("-inf")],
                         ids=["nan", "inf", "-inf"])
def test_rejects_nan_and_infinity(value):
    with pytest.raises(rfc8785.CanonicalizationError):
        canonical_json({"n": value})


@pytest.mark.parametrize(
    "value",
    [datetime.datetime(2026, 9, 28), {1, 2}, b"bytes", object()],
    ids=["datetime", "set", "bytes", "object"],
)
def test_rejects_a_value_that_is_not_json(value):
    """The old default=str fallback rendered these however Python's str()
    happened to, which no other language reproduces."""
    with pytest.raises(rfc8785.CanonicalizationError):
        canonical_json({"v": value})


def test_rejects_a_non_string_key():
    with pytest.raises(rfc8785.CanonicalizationError):
        canonical_json({1: "a"})


def test_rejects_a_lone_surrogate_in_a_string():
    """RFC 8785 requires I-JSON, which forbids an unpaired surrogate. rfc8785
    itself raises CanonicalizationError here."""
    with pytest.raises(rfc8785.CanonicalizationError):
        canonical_json({"s": "\ud800"})


def test_rejects_a_lone_surrogate_in_a_key():
    """rfc8785 raises UnicodeEncodeError for this one, not
    CanonicalizationError - canonical_json wraps it so the documented contract
    (canonical_json always raises CanonicalizationError) holds."""
    with pytest.raises(rfc8785.CanonicalizationError):
        canonical_json({"\udc00": 1})
