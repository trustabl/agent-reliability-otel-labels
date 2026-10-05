/**
 * Policy bindings: one event per policy. A single flattened hash is the bug
 * this shape exists to prevent. Mirrors python/tests/test_authority.py.
 */

import type { InMemorySpanExporter } from "@opentelemetry/sdk-trace-base";
import { expect, it } from "vitest";

import { type Binding, bindAuthority } from "../src/index.js";
import { finished, tracing, withSpan } from "./tracing.js";

function events(exporter: InMemorySpanExporter) {
  return finished(exporter).events;
}

const OPENSHELL: Binding = {
  id: "openshell-seccomp-v3", type: "sandbox", sha256: "a".repeat(64),
  version: "3.1.0", required: true, source: "entrypoint",
};
const ACS: Binding = {
  id: "acs-content-v8", type: "content_safety", sha256: "b".repeat(64),
  version: "8.0.2", required: true, source: "entrypoint",
};

it("test_emits_one_event_per_policy", () => {
  const { tracer, exporter } = tracing();
  withSpan(tracer, "invoke_agent", (span) => bindAuthority(span, [OPENSHELL, ACS]));
  const evs = events(exporter);
  expect(evs).toHaveLength(2);
  expect(new Set(evs.map((e) => e.name))).toEqual(new Set(["trustabl.policy.binding"]));
});

it("test_each_event_carries_its_own_hash", () => {
  // Two rails, two hashes, independently stale-able. Never one combined.
  const { tracer, exporter } = tracing();
  withSpan(tracer, "invoke_agent", (span) => bindAuthority(span, [OPENSHELL, ACS]));
  const hashes = new Set(events(exporter).map((e) => e.attributes?.["trustabl.policy.binding.sha256"]));
  expect(hashes).toEqual(new Set(["a".repeat(64), "b".repeat(64)]));
});

it("test_event_carries_the_full_binding_record", () => {
  const { tracer, exporter } = tracing();
  withSpan(tracer, "invoke_agent", (span) => bindAuthority(span, [OPENSHELL]));
  const attrs = events(exporter)[0].attributes ?? {};
  expect(attrs["trustabl.policy.binding.id"]).toBe("openshell-seccomp-v3");
  expect(attrs["trustabl.policy.binding.type"]).toBe("sandbox");
  expect(attrs["trustabl.policy.binding.version"]).toBe("3.1.0");
  expect(attrs["trustabl.policy.binding.required"]).toBe(true);
  expect(attrs["trustabl.policy.binding.source"]).toBe("entrypoint");
});

it("test_a_subagent_with_no_bindings_emits_no_events", () => {
  // A handoff with no bindings is a coverage hole; emitting nothing makes it visible.
  const { tracer, exporter } = tracing();
  withSpan(tracer, "invoke_agent", (span) => bindAuthority(span, []));
  expect(events(exporter)).toEqual([]);
});

it("test_no_active_span_is_a_silent_no_op", () => {
  bindAuthority(undefined, [OPENSHELL]);
});

it("test_rejects_an_unknown_binding_type", () => {
  const { tracer } = tracing();
  withSpan(tracer, "invoke_agent", (span) => {
    expect(() =>
      bindAuthority(span, [{
        ...OPENSHELL,
        id: "x",
        // @ts-expect-error - a JavaScript caller gets no compile-time check
        type: "firewall",
      }]),
    ).toThrow(RangeError);
  });
});

it("test_rejects_an_unknown_binding_source", () => {
  const { tracer } = tracing();
  withSpan(tracer, "invoke_agent", (span) => {
    expect(() =>
      bindAuthority(span, [{
        ...OPENSHELL,
        id: "x",
        // @ts-expect-error - a JavaScript caller gets no compile-time check
        source: "guessed",
      }]),
    ).toThrow(RangeError);
  });
});

it("validates every binding before emitting any event", () => {
  const { tracer, exporter } = tracing();
  withSpan(tracer, "invoke_agent", (span) => {
    expect(() =>
      // @ts-expect-error - a JavaScript caller gets no compile-time check
      bindAuthority(span, [OPENSHELL, { ...ACS, source: "guessed" }]),
    ).toThrow(RangeError);
  });
  expect(events(exporter)).toEqual([]);
});
