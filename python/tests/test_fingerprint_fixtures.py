"""Golden fingerprints every language binding must reproduce.

Canonicalization is where ports silently diverge: Python's json.dumps,
JavaScript's JSON.stringify and Go's encoding/json disagree about unicode
escaping, float formatting and empty containers. If they disagree, the same
call fingerprints differently per language and cross-language loop detection
quietly stops working.

These values were produced by the Python implementation and are now the
contract. A change here is a deliberate, reviewed break, not a refactor.
"""

import json
import pathlib

import pytest
import rfc8785

from agent_reliability_otel_labels import canonical_fp, canonical_json

FIXTURES = pathlib.Path(__file__).resolve().parents[2] / "spec" / "fingerprint-fixtures.json"


def test_fixture_file_exists():
    assert FIXTURES.is_file(), f"missing cross-language fixtures at {FIXTURES}"


def test_every_fixture_reproduces_its_canonical_string():
    """Checked before the hash on purpose: a character diff explains itself."""
    for case in json.loads(FIXTURES.read_text())["cases"]:
        assert canonical_json(case["input"]) == case["canonical"], case["name"]


def test_every_fixture_reproduces_its_recorded_fingerprint():
    for case in json.loads(FIXTURES.read_text())["cases"]:
        assert canonical_fp(case["input"]) == case["fingerprint"], case["name"]


def test_every_rejected_input_raises():
    for case in json.loads(FIXTURES.read_text())["rejected"]["cases"]:
        with pytest.raises(rfc8785.CanonicalizationError):
            canonical_json(case["input"])
