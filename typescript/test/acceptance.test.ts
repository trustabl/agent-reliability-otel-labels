/**
 * The spec's acceptance criteria, as tests. Mirrors
 * python/tests/test_acceptance.py.
 */

import { expect, it } from "vitest";

import * as api from "../src/index.js";
import { type Binding, Run, bindAuthority, markToolSpan } from "../src/index.js";
import { tracing, withSpan } from "./tracing.js";

it("test_no_active_span_creates_no_span", () => {
  // Not merely "does not throw": nothing may reach the exporter either.
  const { exporter } = tracing();
  markToolSpan(undefined, { args: { a: 1 }, result: {}, attempt: 1, sideEffect: "read" });
  bindAuthority(undefined, []);
  new Run().markStart(undefined);
  expect(exporter.getFinishedSpans()).toEqual([]);
});

it("test_the_public_api_offers_no_way_to_fingerprint_a_completion", () => {
  // Chat is not supposed to repeat, so a differing completion is not a
  // finding. This fails the moment somebody adds markLlmSpan.
  const forbidden = [
    "markLlmSpan", "markChatSpan", "markCompletion", "fingerprintCompletion", "markModelSpan",
    "mark_llm_span", "mark_chat_span", "mark_completion", "fingerprint_completion", "mark_model_span",
  ];
  expect(Object.keys(api).filter((name) => forbidden.includes(name))).toEqual([]);
});

it("test_a_child_span_does_not_inherit_the_parents_bindings", () => {
  // Copying the parent's hashes down would report the child as protected when it was not.
  const { tracer, exporter } = tracing();
  const parentBinding: Binding = {
    id: "openshell-seccomp-v3", type: "sandbox", sha256: "a".repeat(64),
    version: "3.1.0", required: true, source: "entrypoint",
  };
  withSpan(tracer, "invoke_agent", (parent) => {
    bindAuthority(parent, [parentBinding]);
    withSpan(tracer, "invoke_agent", (child) => bindAuthority(child, []));
  });

  const spans = exporter.getFinishedSpans();
  expect(spans.map((s) => s.events.length).sort(), "the child must carry no binding events")
    .toEqual([0, 1]);
  const child = spans.find((s) => s.events.length === 0)!;
  expect(JSON.stringify(child.events.map((e) => e.attributes))).not.toContain("a".repeat(64));
});
