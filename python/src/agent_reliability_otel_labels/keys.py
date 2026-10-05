"""The attribute names, in one place.

Mirrors spec/keys.json, which derives from the reserved names in the Trustabl
attribute spec. Adding a name here is a SPEC REVISION, not a code change:
amend that spec with a named producer, then the manifest, then every binding.
"""

STEP_INDEX = "trustabl.step_index"
RUN_ID = "trustabl.run_id"

TOOL_INPUT_FP = "trustabl.tool.input_fp"
TOOL_OUTPUT_FP = "trustabl.tool.output_fp"
TOOL_SIDE_EFFECT = "trustabl.tool.side_effect"
TOOL_ERROR_CLASS = "trustabl.tool.error_class"
TOOL_ATTEMPT = "trustabl.tool.attempt"

HANDOFF_ID = "trustabl.handoff_id"
EXIT_REASON = "trustabl.exit_reason"

#: Emitted as one EVENT per policy. Parallel-array projections were considered
#: and explicitly rejected: correlating by index silently mispairs an id with
#: the wrong hash when an array is truncated, and this output is signed.
POLICY_BINDING_EVENT = "trustabl.policy.binding"
BINDING_ID = "trustabl.policy.binding.id"
BINDING_TYPE = "trustabl.policy.binding.type"
BINDING_SHA256 = "trustabl.policy.binding.sha256"
BINDING_VERSION = "trustabl.policy.binding.version"
BINDING_REQUIRED = "trustabl.policy.binding.required"
BINDING_SOURCE = "trustabl.policy.binding.source"

#: "pure", not "none": an omitted key already means "unknown", and
#: trustabl.bind.method=none means something else again.
SIDE_EFFECTS = frozenset({"pure", "read", "write", "irreversible"})
ERROR_CLASSES = frozenset({"timeout", "4xx", "5xx", "empty", "schema", "auth"})
EXIT_REASONS = frozenset({"final_answer", "max_steps", "policy_stop", "error"})
BINDING_TYPES = frozenset({
    "sandbox", "content_safety", "tool_grants", "retention", "data_security", "custom",
})
BINDING_SOURCES = frozenset({"entrypoint", "inherited", "sidecar", "external"})
