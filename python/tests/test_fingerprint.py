"""The fingerprint is the value of the tool keys: a fingerprint that does not
collapse repeats reports a loop as N distinct calls and silently disables loop
detection. These tests are the guard against that failing quietly."""

import pytest
import rfc8785

from agent_reliability_otel_labels import canonical_fp


def test_identical_arguments_produce_identical_fingerprints():
    args = {"from": "SFO", "to": "JFK", "date": "2026-10-01"}
    assert canonical_fp(args) == canonical_fp(dict(args))


def test_key_order_does_not_change_the_fingerprint():
    assert canonical_fp({"a": 1, "b": 2}) == canonical_fp({"b": 2, "a": 1})


def test_different_arguments_produce_different_fingerprints():
    assert canonical_fp({"to": "JFK"}) != canonical_fp({"to": "LAX"})


def test_fingerprint_is_sixteen_hex_characters():
    fp = canonical_fp({"anything": "at all"})
    assert len(fp) == 16
    assert all(c in "0123456789abcdef" for c in fp)


def test_volatile_fields_do_not_change_the_fingerprint():
    """Two identical calls a second apart must collapse. If they do not, a loop
    reports as N distinct calls and detection silently does nothing."""
    a = {"query": "flights to JFK", "request_id": "req-111",
         "timestamp": "2026-09-22T10:00:00Z"}
    b = {"query": "flights to JFK", "request_id": "req-222",
         "timestamp": "2026-09-22T10:00:05Z"}
    assert canonical_fp(a) == canonical_fp(b)


def test_volatile_fields_are_stripped_at_any_depth():
    a = {"outer": {"query": "x", "trace_id": "aaa"}}
    b = {"outer": {"query": "x", "trace_id": "bbb"}}
    assert canonical_fp(a) == canonical_fp(b)


def test_volatile_fields_inside_lists_are_stripped():
    a = {"items": [{"sku": "A1", "nonce": "n1"}]}
    b = {"items": [{"sku": "A1", "nonce": "n2"}]}
    assert canonical_fp(a) == canonical_fp(b)


def test_a_meaningful_field_whose_name_contains_id_is_kept():
    """customer_id is real input, not plumbing. Stripping every name containing
    'id' would collapse genuinely different calls into one fingerprint."""
    assert canonical_fp({"customer_id": "c1"}) != canonical_fp({"customer_id": "c2"})


def test_canonical_fp_raises_rather_than_hashing_an_uncanonicalizable_value():
    """A direct caller sees why there is no fingerprint."""
    with pytest.raises(rfc8785.CanonicalizationError):
        canonical_fp({"n": 2**53})
