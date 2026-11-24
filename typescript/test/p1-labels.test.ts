/**
 * The three labels the P1 rules need: expected tools, retrieval result count,
 * progress marker. Mirrors python/tests/test_p1_labels.py.
 */

import { INVALID_SPAN_CONTEXT, type Span, trace } from "@opentelemetry/api";
import { expect, it } from "vitest";

import { markExpectedTools, markProgress, markRetrievalSpan } from "../src/index.js";
import { finished, tracing, withSpan } from "./tracing.js";

const SECRET = "patient Jane Doe, SSN 123-45-6789";

it("test_expected_tools_are_written_on_the_root_span", () => {
  const { tracer, exporter } = tracing();
  withSpan(tracer, "invoke_agent", (root) => {
    markExpectedTools(root, ["search_flights", "book_flight", "search_flights"]);
  });
  expect(finished(exporter).attributes["trustabl.expected_tools"]).toEqual([
    "search_flights",
    "book_flight",
  ]);
});

it("test_empty_expected_tools_leave_the_key_off", () => {
  const { tracer, exporter } = tracing();
  withSpan(tracer, "invoke_agent", (root) => markExpectedTools(root, []));
  expect(finished(exporter).attributes).not.toHaveProperty(["trustabl.expected_tools"]);
});

it.each([["search"], [[""]], [[1]], [[null]]])(
  "test_expected_tools_rejects_anything_but_a_list_of_names %j",
  (bad) => {
    const { tracer } = tracing();
    withSpan(tracer, "invoke_agent", (root) => {
      expect(() => markExpectedTools(root, bad as never)).toThrow(RangeError);
    });
  },
);

it("test_retrieval_result_count_is_written_including_zero", () => {
  const { tracer, exporter } = tracing();
  withSpan(tracer, "retrieval", (span) => {
    markRetrievalSpan(span, { resultCount: 0, stepIndex: 2, runId: "r1" });
  });
  const attrs = finished(exporter).attributes;
  expect(attrs["trustabl.retrieval.result_count"]).toBe(0);
  expect(attrs["trustabl.step_index"]).toBe(2);
  expect(attrs["trustabl.run_id"]).toBe("r1");
});

it.each([[-1], [1.5], ["3"], [true], [null]])(
  "test_retrieval_result_count_rejects_anything_but_a_non_negative_integer %j",
  (bad) => {
    const { tracer } = tracing();
    withSpan(tracer, "retrieval", (span) => {
      expect(() => markRetrievalSpan(span, { resultCount: bad as never })).toThrow(RangeError);
    });
  },
);

it("test_progress_marker_is_a_fingerprint_that_follows_the_state", () => {
  const { tracer, exporter } = tracing();
  withSpan(tracer, "execute_tool", (s) => markProgress(s, { done: ["flights"], todo: ["hotel"] }));
  withSpan(tracer, "execute_tool", (s) => markProgress(s, { todo: ["hotel"], done: ["flights"] }));
  withSpan(tracer, "execute_tool", (s) => markProgress(s, { done: ["flights", "hotel"], todo: [] }));
  const [one, two, three] = exporter
    .getFinishedSpans()
    .map((s) => s.attributes["trustabl.progress_marker"]);
  expect(one).toHaveLength(16);
  expect(one).toBe(two);
  expect(one).not.toBe(three);
});

it("test_progress_marker_is_omitted_for_a_state_with_no_canonical_form", () => {
  const { tracer, exporter } = tracing();
  withSpan(tracer, "execute_tool", (span) => markProgress(span, { bad: Number.NaN }));
  expect(finished(exporter).attributes).not.toHaveProperty(["trustabl.progress_marker"]);
});

it("test_no_state_or_tool_text_reaches_the_new_attributes", () => {
  const { tracer, exporter } = tracing();
  withSpan(tracer, "execute_tool", (span) => {
    markProgress(span, { note: SECRET });
    markRetrievalSpan(span, { resultCount: 3 });
  });
  expect(JSON.stringify(finished(exporter).attributes)).not.toContain(SECRET);
});

it("test_the_new_helpers_do_nothing_without_a_recording_span", () => {
  const nonRecording = trace.wrapSpanContext(INVALID_SPAN_CONTEXT) as Span;
  for (const span of [undefined, nonRecording]) {
    markExpectedTools(span, ["a"]);
    markRetrievalSpan(span, { resultCount: 1 });
    markProgress(span, { a: 1 });
  }
  // tracing off never raises
  markRetrievalSpan(nonRecording, { resultCount: -5 });
});
