/**
 * Policy bindings: which signed policies were in force.
 *
 * One EVENT per policy, never a flattened hash and never parallel arrays.
 * Contract identity is deliberately NOT emitted here: the deployment stamps it
 * through OTEL_RESOURCE_ATTRIBUTES, and a second producer for one key is a bug.
 */

import type { Span } from "@opentelemetry/api";

import {
  BINDING_ID,
  BINDING_REQUIRED,
  BINDING_SHA256,
  BINDING_SOURCE,
  BINDING_SOURCES,
  BINDING_TYPE,
  BINDING_TYPES,
  BINDING_VERSION,
  type BindingSource,
  type BindingType,
  POLICY_BINDING_EVENT,
} from "./keys.js";
import { checkEnum, isUnusable } from "./spans.js";

/**
 * One signed policy artifact attested on a span.
 *
 * `required` says this agent class must carry this rail, so its absence is a
 * finding. `source` records how it arrived; `external` marks an artifact this
 * contract did not generate.
 */
export interface Binding {
  readonly id: string;
  readonly type: BindingType;
  readonly sha256: string;
  readonly version: string;
  readonly required: boolean;
  readonly source: BindingSource;
}

/**
 * Attest every policy in force on this agent or sandbox root.
 *
 * Every binding is validated before any event is emitted. An empty list emits
 * nothing on purpose: a handoff carrying no binding is a coverage hole, and
 * making it visible is the point.
 */
export function bindAuthority(span: Span | undefined, bindings: readonly Binding[]): void {
  if (isUnusable(span)) return;

  for (const binding of bindings) {
    checkEnum("binding type", binding.type, BINDING_TYPES);
    checkEnum("binding source", binding.source, BINDING_SOURCES);
  }

  for (const binding of bindings) {
    span.addEvent(POLICY_BINDING_EVENT, {
      [BINDING_ID]: binding.id,
      [BINDING_TYPE]: binding.type,
      [BINDING_SHA256]: binding.sha256,
      [BINDING_VERSION]: binding.version,
      [BINDING_REQUIRED]: binding.required,
      [BINDING_SOURCE]: binding.source,
    });
  }
}
