/**
 * Run state: order and outcome. step_index exists because OTLP batching can
 * reorder a run on the wire. Mirrors python/tests/test_run.py.
 */

import type { ReadableSpan } from "@opentelemetry/sdk-trace-base";
import { expect, it } from "vitest";

import { Run, markHandoff, markRunEnd, markToolSpan } from "../src/index.js";
import { finished, tracing, withSpan } from "./tracing.js";

function shuffle<T>(items: T[]): T[] {
  for (let i = items.length - 1; i > 0; i--) {
    const j = Math.floor(Math.random() * (i + 1));
    [items[i], items[j]] = [items[j], items[i]];
  }
  return items;
}

it("test_step_indices_are_monotonic", () => {
  const run = new Run();
  expect([run.nextStep(), run.nextStep(), run.nextStep(), run.nextStep()]).toEqual([0, 1, 2, 3]);
});

it("test_run_id_is_stable_across_steps", () => {
  const run = new Run();
  const first = run.runId;
  run.nextStep();
  expect(run.runId).toBe(first);
});

it("test_two_runs_get_different_ids", () => {
  expect(new Run().runId).not.toBe(new Run().runId);
});

it("run ids are sixteen hex characters, like Python's", () => {
  expect(new Run().runId).toMatch(/^[0-9a-f]{16}$/);
});

it("keeps a run id the caller supplies", () => {
  expect(new Run("0123456789abcdef").runId).toBe("0123456789abcdef");
});

it("test_order_survives_spans_arriving_out_of_order", () => {
  // Shuffle the exported spans, sort by step_index, and the sequence comes back.
  const { tracer, exporter } = tracing();
  const run = new Run();
  const called = ["alpha", "beta", "gamma", "delta"];
  for (const name of called) {
    withSpan(tracer, "execute_tool", (span) => {
      span.setAttribute("gen_ai.tool.name", name);
      markToolSpan(span, {
        args: { n: name }, result: {}, attempt: 1, sideEffect: "read", stepIndex: run.nextStep(),
      });
    });
  }
  const step = (s: ReadableSpan) => s.attributes["trustabl.step_index"] as number;
  const recovered = shuffle([...exporter.getFinishedSpans()])
    .sort((a, b) => step(a) - step(b))
    .map((s) => s.attributes["gen_ai.tool.name"]);
  expect(recovered).toEqual(called);
});

it("test_marks_the_run_id_on_the_root_span", () => {
  const { tracer, exporter } = tracing();
  const run = new Run();
  withSpan(tracer, "invoke_agent", (span) => run.markStart(span));
  expect(finished(exporter).attributes["trustabl.run_id"]).toBe(run.runId);
});

it("test_marks_how_the_run_ended", () => {
  const { tracer, exporter } = tracing();
  withSpan(tracer, "invoke_agent", (span) => markRunEnd(span, "max_steps"));
  expect(finished(exporter).attributes["trustabl.exit_reason"]).toBe("max_steps");
});

it("test_rejects_an_unknown_exit_reason", () => {
  const { tracer } = tracing();
  withSpan(tracer, "invoke_agent", (span) => {
    // @ts-expect-error - a JavaScript caller gets no compile-time check
    expect(() => markRunEnd(span, "gave_up")).toThrow(RangeError);
  });
});

it("test_marks_a_handoff_to_a_subagent", () => {
  const { tracer, exporter } = tracing();
  withSpan(tracer, "invoke_agent", (span) => markHandoff(span, "ho-7f31"));
  expect(finished(exporter).attributes["trustabl.handoff_id"]).toBe("ho-7f31");
});

it("test_no_active_span_is_a_silent_no_op", () => {
  new Run().markStart(undefined);
  markRunEnd(undefined, "final_answer");
  markHandoff(undefined, "ho-1");
});
