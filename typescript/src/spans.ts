/**
 * Decorating tool spans.
 *
 * Nothing here starts a span. The caller owns the span's lifecycle; we attach
 * attributes to one that some other instrumentation library already created.
 */

import type { Span } from "@opentelemetry/api";

import { canonicalFp } from "./canonical.js";
import {
  ERROR_CLASSES,
  type ErrorClass,
  RUN_ID,
  SIDE_EFFECTS,
  type SideEffect,
  STEP_INDEX,
  TOOL_ATTEMPT,
  TOOL_ERROR_CLASS,
  TOOL_INPUT_FP,
  TOOL_OUTPUT_FP,
  TOOL_SIDE_EFFECT,
} from "./keys.js";

/**
 * True when there is nothing safe to write to.
 *
 * Checked BEFORE argument validation on purpose: a workload running with
 * tracing disabled must never be crashed by this library, and a bad enum
 * value surfaces the moment anyone runs with tracing on.
 *
 * Typed as a guard for its false branch: a span that is not unusable is a Span.
 */
export function isUnusable(span: Span | undefined): span is undefined {
  if (span == null) return true;
  try {
    return typeof span.isRecording !== "function" || !span.isRecording();
  } catch {
    return true;
  }
}

/** Throw RangeError unless value is one of allowed. Python raises ValueError. */
export function checkEnum(label: string, value: string, allowed: ReadonlySet<string>): void {
  if (!allowed.has(value)) {
    throw new RangeError(
      `${label} must be one of ${JSON.stringify([...allowed].sort())}, got ${JSON.stringify(value)}`,
    );
  }
}

/** What existing() returns when the span's attributes cannot be read at all. */
export const UNREADABLE: unique symbol = Symbol("unreadable");

/**
 * The attribute already on the span, undefined when absent, or UNREADABLE.
 *
 * The API's Span exposes no attributes; the SDK's span does. An unreadable
 * span degrades to "unknown", and an unknown value is never overwritten.
 */
export function existing(span: Span, key: string): unknown {
  try {
    const attrs = (span as { attributes?: unknown }).attributes;
    if (attrs === null || typeof attrs !== "object") return UNREADABLE;
    return (attrs as Record<string, unknown>)[key];
  } catch {
    return UNREADABLE;
  }
}

export interface ToolSpanOptions {
  /** Fingerprinted, never stored. */
  args: unknown;
  /** Fingerprinted, never stored. */
  result: unknown;
  attempt: number;
  /** Leave unset when the tool's class is not known: the key is then omitted, which says "unknown". */
  sideEffect?: SideEffect;
  errorClass?: ErrorClass;
  stepIndex?: number;
  runId?: string;
  name?: string;
}

/**
 * Set the fingerprint of value, or nothing when value has no canonical form.
 *
 * Catches everything, not only CanonicalizationError: the spec says nothing
 * from fingerprinting may reach the caller, and not every failure is a
 * CanonicalizationError - a circular value throws RangeError, and a throwing
 * getter throws whatever it throws. canonicalJson/canonicalFp still raise for
 * direct callers.
 */
function setFingerprint(span: Span, key: string, value: unknown): void {
  let fp: string;
  try {
    fp = canonicalFp(value);
  } catch {
    return;
  }
  span.setAttribute(key, fp);
}

/**
 * Attach the process labels to one tool-execution span.
 *
 * args and result are fingerprinted, never stored: the hash answers "was this
 * the same call?" without carrying what the caller asked for.
 */
export function markToolSpan(span: Span | undefined, opts: ToolSpanOptions): void {
  if (isUnusable(span)) return;

  // != null (not !==) so a JavaScript caller passing null, like Python's None,
  // also means "not given" rather than a compile-time-unreachable enum value.
  if (opts.sideEffect != null) checkEnum("sideEffect", opts.sideEffect, SIDE_EFFECTS);
  if (opts.errorClass != null) checkEnum("errorClass", opts.errorClass, ERROR_CLASSES);

  // A top-level undefined args/result is a void call, not "no value" - fingerprint
  // it as null so it matches Python's None (undefined nested inside stays rejected).
  setFingerprint(span, TOOL_INPUT_FP, opts.args === undefined ? null : opts.args);
  setFingerprint(span, TOOL_OUTPUT_FP, opts.result === undefined ? null : opts.result);
  span.setAttribute(TOOL_ATTEMPT, opts.attempt);

  if (opts.sideEffect != null) span.setAttribute(TOOL_SIDE_EFFECT, opts.sideEffect);
  if (opts.errorClass != null) span.setAttribute(TOOL_ERROR_CLASS, opts.errorClass);
  if (opts.stepIndex != null) span.setAttribute(STEP_INDEX, opts.stepIndex);
  if (opts.runId != null) span.setAttribute(RUN_ID, opts.runId);

  // Their key, their value. We fill it only when the instrumentor left it
  // empty, and never when we cannot tell.
  if (opts.name != null && existing(span, "gen_ai.tool.name") === undefined) {
    span.setAttribute("gen_ai.tool.name", opts.name);
  }
}
