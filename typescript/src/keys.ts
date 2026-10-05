/**
 * The attribute names, in one place.
 *
 * Mirrors python/src/agent_reliability_otel_labels/keys.py and spec/keys.json, which derive
 * from the reserved names in the Trustabl attribute spec. Adding a name here
 * is a SPEC REVISION, not a code change: amend that spec with a named
 * producer, then the manifest, then every binding.
 */

export const STEP_INDEX = "trustabl.step_index";
export const RUN_ID = "trustabl.run_id";

export const TOOL_INPUT_FP = "trustabl.tool.input_fp";
export const TOOL_OUTPUT_FP = "trustabl.tool.output_fp";
export const TOOL_SIDE_EFFECT = "trustabl.tool.side_effect";
export const TOOL_ERROR_CLASS = "trustabl.tool.error_class";
export const TOOL_ATTEMPT = "trustabl.tool.attempt";

export const HANDOFF_ID = "trustabl.handoff_id";
export const EXIT_REASON = "trustabl.exit_reason";

/**
 * Emitted as one EVENT per policy. Parallel-array projections were rejected:
 * correlating by index silently mispairs an id with the wrong hash when an
 * array is truncated, and this output is signed.
 */
export const POLICY_BINDING_EVENT = "trustabl.policy.binding";
export const BINDING_ID = "trustabl.policy.binding.id";
export const BINDING_TYPE = "trustabl.policy.binding.type";
export const BINDING_SHA256 = "trustabl.policy.binding.sha256";
export const BINDING_VERSION = "trustabl.policy.binding.version";
export const BINDING_REQUIRED = "trustabl.policy.binding.required";
export const BINDING_SOURCE = "trustabl.policy.binding.source";

/**
 * "pure", not "none": an omitted key already means "unknown", and
 * trustabl.bind.method=none means something else again.
 */
const SIDE_EFFECT_VALUES = ["pure", "read", "write", "irreversible"] as const;
const ERROR_CLASS_VALUES = ["timeout", "4xx", "5xx", "empty", "schema", "auth"] as const;
const EXIT_REASON_VALUES = ["final_answer", "max_steps", "policy_stop", "error"] as const;
const BINDING_TYPE_VALUES = [
  "sandbox",
  "content_safety",
  "tool_grants",
  "retention",
  "data_security",
  "custom",
] as const;
const BINDING_SOURCE_VALUES = ["entrypoint", "inherited", "sidecar", "external"] as const;

export type SideEffect = (typeof SIDE_EFFECT_VALUES)[number];
export type ErrorClass = (typeof ERROR_CLASS_VALUES)[number];
export type ExitReason = (typeof EXIT_REASON_VALUES)[number];
export type BindingType = (typeof BINDING_TYPE_VALUES)[number];
export type BindingSource = (typeof BINDING_SOURCE_VALUES)[number];

// Typed as sets of string so a JavaScript caller's unchecked value can be
// tested with .has() without a cast.
export const SIDE_EFFECTS: ReadonlySet<string> = new Set(SIDE_EFFECT_VALUES);
export const ERROR_CLASSES: ReadonlySet<string> = new Set(ERROR_CLASS_VALUES);
export const EXIT_REASONS: ReadonlySet<string> = new Set(EXIT_REASON_VALUES);
export const BINDING_TYPES: ReadonlySet<string> = new Set(BINDING_TYPE_VALUES);
export const BINDING_SOURCES: ReadonlySet<string> = new Set(BINDING_SOURCE_VALUES);
