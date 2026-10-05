"""Every language binding must emit the same key names.

The manifest at the repository root is the shared source of truth the Python,
TypeScript and Go implementations each check themselves against. Without it,
three ports drift and a consumer sees three vocabularies for one profile.
"""

import json
import pathlib

from agent_reliability_otel_labels import keys

MANIFEST = pathlib.Path(__file__).resolve().parents[2] / "spec" / "keys.json"


def _manifest():
    return json.loads(MANIFEST.read_text())


def test_manifest_exists():
    assert MANIFEST.is_file(), f"missing shared key manifest at {MANIFEST}"


def test_every_manifest_attribute_is_defined_in_python():
    defined = {v for k, v in vars(keys).items()
               if isinstance(v, str) and not k.startswith("_")}
    missing = set(_manifest()["attributes"]) - defined
    assert not missing, f"Python is missing manifest keys: {sorted(missing)}"


def test_python_defines_no_attribute_the_manifest_does_not_name():
    """Inventing a key in one binding is how the three ports diverge. A new key
    is a spec revision first, then a manifest entry, then code."""
    declared = set(_manifest()["attributes"]) | set(_manifest()["event_attributes"])
    declared.add(_manifest()["events"]["policy_binding"])
    defined = {v for k, v in vars(keys).items()
               if isinstance(v, str) and v.startswith("trustabl.")}
    extra = defined - declared
    assert not extra, f"Python defines keys absent from the manifest: {sorted(extra)}"


def test_enums_match_the_manifest():
    m = _manifest()["enums"]
    assert set(m["side_effect"]) == set(keys.SIDE_EFFECTS)
    assert set(m["error_class"]) == set(keys.ERROR_CLASSES)
    assert set(m["exit_reason"]) == set(keys.EXIT_REASONS)
    assert set(m["binding_type"]) == set(keys.BINDING_TYPES)
    assert set(m["binding_source"]) == set(keys.BINDING_SOURCES)


def test_the_profile_is_frozen_at_1_0():
    assert _manifest()["profile_version"] == "1.0.0"
