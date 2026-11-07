/**
 * Labelling a model call's reasoning without carrying its text.
 *
 * A chain of thought quotes whatever the model read, so it is the most sensitive
 * text this library is ever handed. It is hashed and dropped here; only the
 * fingerprint reaches the span.
 *
 * These labels say that a model reasoned and which text it produced. They do not
 * say the reasoning was faithful: present means reasoning occurred, not that it
 * caused the answer.
 */

import type { Span } from "@opentelemetry/api";

import {
  COT_MODE,
  COT_MODES,
  COT_PRESENT,
  COT_TEXT_FP,
  type CotMode,
  GEN_AI_REASONING_TOKENS,
} from "./keys.js";
import { checkEnum, existing, isUnusable, setFingerprint } from "./spans.js";

export interface ReasoningOptions {
  /**
   * Fingerprinted, never stored. An empty string says nothing, because a
   * provider that hides its reasoning returns one.
   */
  text?: string;
  /** How the agent reasons. Leave unset when that is not known: the key is then omitted, which says "unknown". */
  mode?: CotMode;
  /**
   * Fills gen_ai.usage.reasoning.output_tokens only when the instrumentor left
   * it empty. That key is OTel's, and when it is already on the span it decides
   * trustabl.cot.present without being passed here.
   */
  reasoningTokens?: number;
}

/**
 * value when it is a usable count, otherwise undefined. A bad count is ignored
 * rather than thrown: unlike an enum typo, it can come from a provider's response
 * at run time, and a workload must not crash on it.
 */
function tokenCount(value: unknown): number | undefined {
  return Number.isSafeInteger(value) && (value as number) >= 0 ? (value as number) : undefined;
}

/**
 * Attach the reasoning labels to one model-call span.
 *
 * A call that says nothing either way writes nothing: an omitted
 * trustabl.cot.present means "unknown", and false would be a false claim.
 */
export function markReasoning(span: Span | undefined, opts: ReasoningOptions = {}): void {
  if (isUnusable(span)) return;

  // != null (not !==) so a JavaScript caller passing null, like Python's None,
  // also means "not given".
  if (opts.mode != null) checkEnum("mode", opts.mode, COT_MODES);

  const hasText = typeof opts.text === "string" && opts.text !== "";
  const given = tokenCount(opts.reasoningTokens);
  const onSpan = existing(span, GEN_AI_REASONING_TOKENS);
  const counts = [given, tokenCount(onSpan)].filter((c): c is number => c !== undefined);

  // Evidence already on the span stands: usage often arrives after the text, and
  // a zero count must not leave present=false beside a fingerprint.
  let present: boolean | undefined;
  if (hasText || counts.some((c) => c > 0) || existing(span, COT_PRESENT) === true) present = true;
  else if (opts.mode === "none" || counts.length > 0) present = false;

  if (present !== undefined) span.setAttribute(COT_PRESENT, present);
  if (hasText) setFingerprint(span, COT_TEXT_FP, opts.text);
  // A declared "none" the evidence refutes is dropped: present=true beside
  // mode=none would be a span arguing with itself.
  if (opts.mode != null && !(opts.mode === "none" && present === true)) {
    span.setAttribute(COT_MODE, opts.mode);
  }

  // Their key, their value. We fill it only when the instrumentor left it
  // empty, and never when we cannot tell.
  if (given !== undefined && onSpan === undefined) {
    span.setAttribute(GEN_AI_REASONING_TOKENS, given);
  }
}
