/**
 * Run identity, step order, and how a run ended.
 *
 * stepIndex is assigned in process, in call order, because OTLP batching can
 * split and reorder a run on the wire. A consumer that sorts by step_index
 * gets the true sequence back.
 */

import type { Span } from "@opentelemetry/api";
import { bytesToHex, randomBytes } from "@noble/hashes/utils";

import { EXIT_REASON, EXIT_REASONS, type ExitReason, HANDOFF_ID, RUN_ID } from "./keys.js";
import { checkEnum, isUnusable } from "./spans.js";

/**
 * One user task, and the step counter for it.
 *
 * Not safe to share across concurrent logical runs: an agent fanning out
 * should carry one Run per logical run, or the step order it records is not
 * the order anything happened in.
 */
export class Run {
  readonly runId: string;
  #steps = 0;

  constructor(runId?: string) {
    // 8 random bytes, 16 hex characters: the same length as Python's.
    // randomBytes uses node:crypto on Node 18 (no global crypto there) and
    // Web Crypto elsewhere.
    this.runId = runId || bytesToHex(randomBytes(8));
  }

  nextStep(): number {
    return this.#steps++;
  }

  /** Stamp the run identity on the root span. */
  markStart(span: Span | undefined): void {
    if (isUnusable(span)) return;
    span.setAttribute(RUN_ID, this.runId);
  }
}

/**
 * Record how the run finished.
 *
 * Call this before the root span ends. An ended span stops recording, so a
 * late call is a silent no-op and the run has no exit_reason.
 */
export function markRunEnd(span: Span | undefined, exitReason: ExitReason): void {
  if (isUnusable(span)) return;
  checkEnum("exitReason", exitReason, EXIT_REASONS);
  span.setAttribute(EXIT_REASON, exitReason);
}

/**
 * Record that this agent span came from a delegation. The handoff id lets a
 * consumer ask whether the child carried any policy of its own.
 */
export function markHandoff(span: Span | undefined, handoffId: string): void {
  if (isUnusable(span)) return;
  span.setAttribute(HANDOFF_ID, handoffId);
}
