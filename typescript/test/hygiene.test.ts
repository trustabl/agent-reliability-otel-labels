/**
 * The privacy guarantee, asserted rather than promised. Everything this
 * library writes is an id, an enum, an integer or a hash. Mirrors
 * python/tests/test_hygiene.py.
 */

import type { ReadableSpan } from "@opentelemetry/sdk-trace-base";
import { expect, it } from "vitest";

import { Run, bindAuthority, keys, markRunEnd, markToolSpan } from "../src/index.js";
import { finished, tracing, withSpan } from "./tracing.js";

// Deliberately fake. These tests assert it NEVER reaches a span; a credential
// scanner flagging it has found the test working, not a leak.
const SECRET = "patient Jane Doe, SSN 123-45-6789, diagnosis withheld";

/** Every attribute value and every event attribute value, as one string. */
function everythingWritten(span: ReadableSpan): string {
  const parts = [JSON.stringify(span.attributes)];
  for (const event of span.events) parts.push(JSON.stringify(event.attributes ?? {}));
  return parts.join(" ");
}

it("test_no_argument_or_result_text_reaches_any_attribute", () => {
  const { tracer, exporter } = tracing();
  const run = new Run();
  withSpan(tracer, "execute_tool", (span) => {
    run.markStart(span);
    markToolSpan(span, {
      args: { query: SECRET, nested: { note: SECRET } },
      result: { records: [{ body: SECRET }] },
      attempt: 1,
      sideEffect: "read",
      stepIndex: run.nextStep(),
    });
    bindAuthority(span, [{
      id: "dspm-prod-v2", type: "data_security", sha256: "d".repeat(64),
      version: "2.1.0", required: true, source: "entrypoint",
    }]);
    markRunEnd(span, "final_answer");
  });

  const written = everythingWritten(finished(exporter));
  expect(written).not.toContain(SECRET);
  expect(written).not.toContain("Jane Doe");
  expect(written).not.toContain("123-45-6789");
});

it("test_every_attribute_we_write_is_bounded_in_length", () => {
  const { tracer, exporter } = tracing();
  withSpan(tracer, "execute_tool", (span) => {
    markToolSpan(span, {
      args: { blob: "x".repeat(50_000) }, result: { blob: "y".repeat(50_000) },
      attempt: 1, sideEffect: "read", stepIndex: 0,
    });
  });
  for (const [key, value] of Object.entries(finished(exporter).attributes)) {
    if (typeof value === "string") expect(value.length, `${key} is unbounded`).toBeLessThanOrEqual(64);
  }
});

it("test_fingerprints_are_exactly_the_declared_length", () => {
  const { tracer, exporter } = tracing();
  withSpan(tracer, "execute_tool", (span) => {
    markToolSpan(span, { args: { a: 1 }, result: { b: 2 }, attempt: 1, sideEffect: "read" });
  });
  const attrs = finished(exporter).attributes;
  expect(attrs[keys.TOOL_INPUT_FP]).toMatch(/^[0-9a-f]{16}$/);
  expect(attrs[keys.TOOL_OUTPUT_FP]).toMatch(/^[0-9a-f]{16}$/);
});
