/**
 * We attach to spans we did not shape. A span from any of the three captured
 * conventions must take our labels without us disturbing theirs. Mirrors
 * python/tests/test_conventions.py.
 */

import { expect, it } from "vitest";

import { markToolSpan } from "../src/index.js";
import { tracing, withSpan } from "./tracing.js";

const CONVENTIONS: Array<[string, Record<string, string>]> = [
  ["genai", { "gen_ai.operation.name": "execute_tool", "gen_ai.tool.name": "search_flights" }],
  ["openinference", { "openinference.span.kind": "TOOL", "tool.name": "search_flights" }],
  ["traceloop", { "traceloop.span.kind": "tool", "traceloop.entity.name": "search_flights" }],
];

it.each(CONVENTIONS)("test_labels_attach_to_a_span_from_any_convention[%s]", (convention, attrs) => {
  const { tracer, exporter } = tracing();
  withSpan(tracer, "tool", (span) => {
    for (const [key, value] of Object.entries(attrs)) span.setAttribute(key, value);
    markToolSpan(span, { args: { to: "JFK" }, result: {}, attempt: 1, sideEffect: "read", stepIndex: 0 });
  });
  const written = exporter.getFinishedSpans()[0].attributes;
  expect(written["trustabl.tool.side_effect"]).toBe("read");
  for (const [key, value] of Object.entries(attrs)) {
    expect(written[key], `${convention}: we disturbed ${key}`).toBe(value);
  }
});
