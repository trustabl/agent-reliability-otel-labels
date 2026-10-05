/**
 * markToolSpan decorates a span somebody else created. These tests pin the
 * invariants that make that safe. Mirrors python/tests/test_tool_span.py.
 */

import { INVALID_SPAN_CONTEXT, type Span, trace } from "@opentelemetry/api";
import { expect, it } from "vitest";

import { canonicalFp, markToolSpan, type ToolSpanOptions } from "../src/index.js";
import { finished, tracing, withSpan } from "./tracing.js";

it("test_sets_the_process_keys_on_the_span", () => {
  const { tracer, exporter } = tracing();
  withSpan(tracer, "execute_tool", (span) => {
    markToolSpan(span, {
      args: { to: "JFK" }, result: { flights: 3 }, attempt: 3, sideEffect: "read", stepIndex: 7,
    });
  });
  const attrs = finished(exporter).attributes;
  expect(attrs["trustabl.tool.attempt"]).toBe(3);
  expect(attrs["trustabl.tool.side_effect"]).toBe("read");
  expect(attrs["trustabl.step_index"]).toBe(7);
  expect(attrs["trustabl.tool.input_fp"]).toHaveLength(16);
  expect(attrs["trustabl.tool.output_fp"]).toHaveLength(16);
});

it("test_omits_error_class_when_there_was_no_error", () => {
  // Absence is a claim: an omitted key says "no error observed".
  const { tracer, exporter } = tracing();
  withSpan(tracer, "execute_tool", (span) => {
    markToolSpan(span, { args: {}, result: {}, attempt: 1, sideEffect: "read" });
  });
  expect(finished(exporter).attributes).not.toHaveProperty(["trustabl.tool.error_class"]);
});

it("test_omits_side_effect_when_the_caller_does_not_know_it", () => {
  // Degrade to absent, never to wrong: unknown is left off, not guessed as "pure".
  const { tracer, exporter } = tracing();
  withSpan(tracer, "execute_tool", (span) => {
    markToolSpan(span, { args: {}, result: {}, attempt: 1 });
  });
  expect(finished(exporter).attributes).not.toHaveProperty(["trustabl.tool.side_effect"]);
});

it("test_pure_marks_a_tool_with_no_side_effect", () => {
  const { tracer, exporter } = tracing();
  withSpan(tracer, "execute_tool", (span) => {
    markToolSpan(span, { args: {}, result: {}, attempt: 1, sideEffect: "pure" });
  });
  expect(finished(exporter).attributes["trustabl.tool.side_effect"]).toBe("pure");
});

it("test_rejects_none_as_a_side_effect_class", () => {
  const { tracer } = tracing();
  withSpan(tracer, "execute_tool", (span) => {
    expect(() =>
      markToolSpan(span, {
        args: {}, result: {}, attempt: 1,
        // @ts-expect-error - a JavaScript caller gets no compile-time check
        sideEffect: "none",
      }),
    ).toThrow(RangeError);
  });
});

it("test_stamps_the_run_id_on_the_tool_span", () => {
  const { tracer, exporter } = tracing();
  withSpan(tracer, "execute_tool", (span) => {
    markToolSpan(span, { args: {}, result: {}, attempt: 1, runId: "0123456789abcdef" });
  });
  expect(finished(exporter).attributes["trustabl.run_id"]).toBe("0123456789abcdef");
});

it("test_omits_run_id_when_none_is_given", () => {
  const { tracer, exporter } = tracing();
  withSpan(tracer, "execute_tool", (span) => {
    markToolSpan(span, { args: {}, result: {}, attempt: 1 });
  });
  expect(finished(exporter).attributes).not.toHaveProperty(["trustabl.run_id"]);
});

it("test_does_not_overwrite_a_tool_name_the_instrumentor_set", () => {
  const { tracer, exporter } = tracing();
  withSpan(tracer, "execute_tool", (span) => {
    span.setAttribute("gen_ai.tool.name", "search_flights");
    markToolSpan(span, {
      args: {}, result: {}, attempt: 1, sideEffect: "read", name: "something_else",
    });
  });
  expect(finished(exporter).attributes["gen_ai.tool.name"]).toBe("search_flights");
});

it("test_no_active_span_is_a_silent_no_op", () => {
  markToolSpan(undefined, { args: { a: 1 }, result: {}, attempt: 1, sideEffect: "read" });
});

it("test_a_non_recording_span_is_a_silent_no_op", () => {
  markToolSpan(trace.wrapSpanContext(INVALID_SPAN_CONTEXT), {
    args: { a: 1 }, result: {}, attempt: 1, sideEffect: "read",
  });
});

it("test_a_span_whose_is_recording_raises_is_a_silent_no_op", () => {
  const span = {
    isRecording: () => {
      throw new Error("boom");
    },
  } as unknown as Span;
  markToolSpan(span, { args: { a: 1 }, result: {}, attempt: 1 });
});

it("the enum check comes after the no-op check", () => {
  // A workload with tracing disabled must not be crashable by this library.
  expect(() =>
    markToolSpan(undefined, {
      args: {}, result: {}, attempt: 1,
      // @ts-expect-error - a JavaScript caller gets no compile-time check
      sideEffect: "destroys",
    }),
  ).not.toThrow();
});

it("test_rejects_an_unknown_side_effect_class", () => {
  const { tracer } = tracing();
  withSpan(tracer, "execute_tool", (span) => {
    expect(() =>
      markToolSpan(span, {
        args: {}, result: {}, attempt: 1,
        // @ts-expect-error - a JavaScript caller gets no compile-time check
        sideEffect: "destroys",
      }),
    ).toThrow(RangeError);
  });
});

class Point {
  constructor(readonly x = 1) {}
}

it.each<[string, unknown]>([
  ["unsafe integer", 2 ** 53],
  ["bigint", 10n],
  ["Date", new Date("2026-09-28T00:00:00Z")],
  ["NaN", NaN],
  ["Infinity", Infinity],
  ["undefined", undefined],
  ["class instance", new Point()],
])("test_an_uncanonicalizable_argument_omits_only_the_input_fingerprint[%s]", (_label, value) => {
  // Degrade to absent, never to wrong - and never crash the workload.
  const { tracer, exporter } = tracing();
  withSpan(tracer, "execute_tool", (span) => {
    markToolSpan(span, {
      args: { v: value }, result: { ok: true }, attempt: 1, sideEffect: "read", stepIndex: 0,
    });
  });
  const attrs = finished(exporter).attributes;
  expect(attrs).not.toHaveProperty(["trustabl.tool.input_fp"]);
  expect(attrs["trustabl.tool.output_fp"]).toHaveLength(16);
  expect(attrs["trustabl.tool.attempt"]).toBe(1);
  expect(attrs["trustabl.tool.side_effect"]).toBe("read");
  expect(attrs["trustabl.step_index"]).toBe(0);
});

it("test_an_uncanonicalizable_result_omits_only_the_output_fingerprint", () => {
  const { tracer, exporter } = tracing();
  withSpan(tracer, "execute_tool", (span) => {
    markToolSpan(span, {
      args: { to: "JFK" }, result: { at: new Date("2026-09-28T00:00:00Z") }, attempt: 2,
    });
  });
  const attrs = finished(exporter).attributes;
  expect(attrs).not.toHaveProperty(["trustabl.tool.output_fp"]);
  expect(attrs["trustabl.tool.input_fp"]).toHaveLength(16);
  expect(attrs["trustabl.tool.attempt"]).toBe(2);
});

/** A recording span that does not expose its attributes, as the API allows. */
function opaqueSpan(): { span: Span; written: Record<string, unknown> } {
  const written: Record<string, unknown> = {};
  const span = {
    isRecording: () => true,
    setAttribute: (key: string, value: unknown) => {
      written[key] = value;
      return span;
    },
  } as unknown as Span;
  return { span, written };
}

it("test_does_not_write_a_tool_name_when_attributes_cannot_be_read", () => {
  const { span, written } = opaqueSpan();
  markToolSpan(span, { args: {}, result: {}, attempt: 1, name: "search_flights" });
  expect(written).not.toHaveProperty(["gen_ai.tool.name"]);
  expect(written["trustabl.tool.attempt"]).toBe(1);
});

it("a void tool's undefined result fingerprints as null, like Python's None", () => {
  const { tracer, exporter } = tracing();
  withSpan(tracer, "execute_tool", (span) => {
    markToolSpan(span, { args: { to: "JFK" }, result: undefined, attempt: 1 });
  });
  const attrs = finished(exporter).attributes;
  expect(attrs["trustabl.tool.output_fp"]).toBe(canonicalFp(null));
});

it("null option values mean 'not given', like Python's None", () => {
  const { tracer, exporter } = tracing();
  withSpan(tracer, "execute_tool", (span) => {
    expect(() =>
      markToolSpan(span, {
        args: {},
        result: {},
        attempt: 1,
        sideEffect: null,
        errorClass: null,
        stepIndex: null,
        runId: null,
        name: null,
      } as unknown as ToolSpanOptions),
    ).not.toThrow();
  });
  const attrs = finished(exporter).attributes;
  expect(attrs).not.toHaveProperty(["trustabl.tool.side_effect"]);
  expect(attrs).not.toHaveProperty(["trustabl.tool.error_class"]);
  expect(attrs).not.toHaveProperty(["trustabl.step_index"]);
  expect(attrs).not.toHaveProperty(["trustabl.run_id"]);
  expect(attrs).not.toHaveProperty(["gen_ai.tool.name"]);
});

it("test_fills_the_tool_name_when_the_instrumentor_left_it_empty", () => {
  const { tracer, exporter } = tracing();
  withSpan(tracer, "execute_tool", (span) => {
    markToolSpan(span, { args: {}, result: {}, attempt: 1, name: "search_flights" });
  });
  expect(finished(exporter).attributes["gen_ai.tool.name"]).toBe("search_flights");
});

it("test_a_circular_argument_omits_only_the_input_fingerprint", () => {
  // A cycle raises RangeError, not CanonicalizationError. The spec says
  // nothing may reach the workload, so markToolSpan must catch this too.
  const { tracer, exporter } = tracing();
  const args: Record<string, unknown> = {};
  args.self = args;
  withSpan(tracer, "execute_tool", (span) => {
    markToolSpan(span, { args, result: { ok: true }, attempt: 1 });
  });
  const attrs = finished(exporter).attributes;
  expect(attrs).not.toHaveProperty(["trustabl.tool.input_fp"]);
  expect(attrs["trustabl.tool.output_fp"]).toHaveLength(16);
});

it("test_a_throwing_value_omits_only_the_output_fingerprint", () => {
  // A getter that throws is not a CanonicalizationError either.
  const { tracer, exporter } = tracing();
  const result: Record<string, unknown> = {};
  Object.defineProperty(result, "bad", {
    enumerable: true,
    get(): unknown {
      throw new TypeError("boom");
    },
  });
  withSpan(tracer, "execute_tool", (span) => {
    markToolSpan(span, { args: { to: "JFK" }, result, attempt: 1 });
  });
  const attrs = finished(exporter).attributes;
  expect(attrs).not.toHaveProperty(["trustabl.tool.output_fp"]);
  expect(attrs["trustabl.tool.input_fp"]).toHaveLength(16);
});
