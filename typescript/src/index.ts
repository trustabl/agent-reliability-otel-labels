/**
 * @trustabl/agent-reliability-otel-labels: process-metadata labels for OpenTelemetry agent spans.
 *
 * This package never creates spans. It adds attributes to spans that some
 * other instrumentation library already created, so an agent must already be
 * instrumented for any of this to do anything.
 */

export * as keys from "./keys.js";
export type { BindingSource, BindingType, ErrorClass, ExitReason, SideEffect } from "./keys.js";

export { CanonicalizationError, canonicalFp, canonicalJson, stripVolatile } from "./canonical.js";

export { markToolSpan, type ToolSpanOptions } from "./spans.js";

export { type Binding, bindAuthority } from "./authority.js";
export { Run, markHandoff, markRunEnd } from "./run.js";
