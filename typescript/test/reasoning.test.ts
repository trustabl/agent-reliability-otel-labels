/**
 * markReasoning labels a model call somebody else spanned. These tests pin the
 * claims each key makes, because a reasoning label that overclaims is worse than
 * none. Mirrors python/tests/test_reasoning.py.
 */

import { INVALID_SPAN_CONTEXT, type Span, trace } from "@opentelemetry/api";
import { expect, it } from "vitest";

import { canonicalFp, keys, markReasoning, type ReasoningOptions } from "../src/index.js";
import { finished, tracing, withSpan } from "./tracing.js";

const TOKENS = "gen_ai.usage.reasoning.output_tokens";

function cotKeys(attrs: Record<string, unknown>): string[] {
  return Object.keys(attrs).filter((k) => k.startsWith("trustabl.cot.")).sort();
}

it("test_sets_the_reasoning_labels_on_the_span", () => {
  const { tracer, exporter } = tracing();
  withSpan(tracer, "chat", (span) => {
    markReasoning(span, { text: "compare both fares", mode: "explicit_cot", reasoningTokens: 12 });
  });
  const attrs = finished(exporter).attributes;
  expect(attrs["trustabl.cot.present"]).toBe(true);
  expect(attrs["trustabl.cot.text_fp"]).toBe(canonicalFp("compare both fares"));
  expect(attrs["trustabl.cot.mode"]).toBe("explicit_cot");
  expect(attrs[TOKENS]).toBe(12);
});

it("test_no_information_writes_no_key", () => {
  // Absence is a claim: a call that knows nothing must not say false.
  const { tracer, exporter } = tracing();
  withSpan(tracer, "chat", (span) => markReasoning(span));
  expect(cotKeys(finished(exporter).attributes)).toEqual([]);
});

it("test_empty_text_is_no_evidence", () => {
  // A provider that hides its reasoning returns an empty string.
  const { tracer, exporter } = tracing();
  withSpan(tracer, "chat", (span) => markReasoning(span, { text: "" }));
  expect(cotKeys(finished(exporter).attributes)).toEqual([]);
});

it("test_a_zero_token_count_means_no_reasoning", () => {
  const { tracer, exporter } = tracing();
  withSpan(tracer, "chat", (span) => markReasoning(span, { reasoningTokens: 0 }));
  const attrs = finished(exporter).attributes;
  expect(attrs["trustabl.cot.present"]).toBe(false);
  expect(attrs[TOKENS]).toBe(0);
  expect(attrs).not.toHaveProperty(["trustabl.cot.text_fp"]);
});

it("test_a_declared_none_means_no_reasoning", () => {
  const { tracer, exporter } = tracing();
  withSpan(tracer, "chat", (span) => markReasoning(span, { mode: "none" }));
  const attrs = finished(exporter).attributes;
  expect(attrs["trustabl.cot.present"]).toBe(false);
  expect(attrs["trustabl.cot.mode"]).toBe("none");
});

it("test_tokens_without_text_still_count_as_reasoning", () => {
  // Hidden reasoning: the model spent tokens and showed no text.
  const { tracer, exporter } = tracing();
  withSpan(tracer, "chat", (span) => markReasoning(span, { mode: "latent", reasoningTokens: 40 }));
  const attrs = finished(exporter).attributes;
  expect(attrs["trustabl.cot.present"]).toBe(true);
  expect(attrs["trustabl.cot.mode"]).toBe("latent");
  expect(attrs).not.toHaveProperty(["trustabl.cot.text_fp"]);
});

it("test_a_mode_alone_does_not_claim_this_call_reasoned", () => {
  // Mode says how the agent reasons. Whether this call did is a separate fact.
  const { tracer, exporter } = tracing();
  withSpan(tracer, "chat", (span) => markReasoning(span, { mode: "latent" }));
  const attrs = finished(exporter).attributes;
  expect(attrs["trustabl.cot.mode"]).toBe("latent");
  expect(attrs).not.toHaveProperty(["trustabl.cot.present"]);
});

it("test_a_none_the_evidence_contradicts_is_omitted", () => {
  // Degrade to absent, never to wrong: the claim the evidence refutes is dropped.
  const { tracer, exporter } = tracing();
  withSpan(tracer, "chat", (span) => markReasoning(span, { text: "it reasoned after all", mode: "none" }));
  const attrs = finished(exporter).attributes;
  expect(attrs["trustabl.cot.present"]).toBe(true);
  expect(attrs).not.toHaveProperty(["trustabl.cot.mode"]);
});

it("test_positive_evidence_beats_a_zero_count", () => {
  const { tracer, exporter } = tracing();
  withSpan(tracer, "chat", (span) => markReasoning(span, { text: "it reasoned", reasoningTokens: 0 }));
  expect(finished(exporter).attributes["trustabl.cot.present"]).toBe(true);
});

it("test_a_later_call_cannot_take_back_evidence", () => {
  // A span must not argue with itself. Usage often arrives after the text, and a
  // zero count there would otherwise leave present=false beside a fingerprint.
  const { tracer, exporter } = tracing();
  withSpan(tracer, "chat", (span) => {
    markReasoning(span, { text: "compare both fares" });
    markReasoning(span, { reasoningTokens: 0, mode: "none" });
  });
  const attrs = finished(exporter).attributes;
  expect(attrs["trustabl.cot.present"]).toBe(true);
  expect(attrs).not.toHaveProperty(["trustabl.cot.mode"]);
  expect(attrs["trustabl.cot.text_fp"]).toBe(canonicalFp("compare both fares"));
});

it("test_rejects_an_unknown_mode", () => {
  const { tracer, exporter } = tracing();
  withSpan(tracer, "chat", (span) => {
    expect(() =>
      markReasoning(span, {
        text: "compare both fares",
        // @ts-expect-error - a JavaScript caller gets no compile-time check
        mode: "vibes",
      }),
    ).toThrow(RangeError);
  });
  expect(cotKeys(finished(exporter).attributes)).toEqual([]);
});

it("test_the_enum_check_comes_after_the_no_op_check", () => {
  // A workload with tracing disabled must not be crashable by this library.
  expect(() =>
    markReasoning(undefined, {
      // @ts-expect-error - a JavaScript caller gets no compile-time check
      mode: "vibes",
    }),
  ).not.toThrow();
});

it("test_no_active_span_is_a_silent_no_op", () => {
  markReasoning(undefined, { text: "compare both fares", mode: "explicit_cot" });
});

it("test_a_non_recording_span_is_a_silent_no_op", () => {
  markReasoning(trace.wrapSpanContext(INVALID_SPAN_CONTEXT), { text: "compare both fares", mode: "explicit_cot" });
});

it("test_reads_the_instrumentors_token_count", () => {
  const { tracer, exporter } = tracing();
  withSpan(tracer, "chat", (span) => {
    span.setAttribute(TOKENS, 7);
    markReasoning(span);
  });
  const attrs = finished(exporter).attributes;
  expect(attrs["trustabl.cot.present"]).toBe(true);
  expect(attrs[TOKENS]).toBe(7);
});

it("test_reads_a_zero_from_the_instrumentor", () => {
  const { tracer, exporter } = tracing();
  withSpan(tracer, "chat", (span) => {
    span.setAttribute(TOKENS, 0);
    markReasoning(span);
  });
  expect(finished(exporter).attributes["trustabl.cot.present"]).toBe(false);
});

it("test_never_overwrites_the_instrumentors_token_count", () => {
  const { tracer, exporter } = tracing();
  withSpan(tracer, "chat", (span) => {
    span.setAttribute(TOKENS, 7);
    markReasoning(span, { reasoningTokens: 99 });
  });
  expect(finished(exporter).attributes[TOKENS]).toBe(7);
});

/** A recording span that does not expose its attributes, as the API's Span does not. */
function opaqueSpan(): { span: Span; written: Record<string, unknown> } {
  const written: Record<string, unknown> = {};
  const span = {
    isRecording: () => true,
    setAttribute(key: string, value: unknown) {
      written[key] = value;
      return span;
    },
  } as unknown as Span;
  return { span, written };
}

it("test_does_not_write_a_token_count_when_attributes_cannot_be_read", () => {
  // An unreadable span's value is unknown, and an unknown value is never overwritten.
  const { span, written } = opaqueSpan();
  markReasoning(span, { reasoningTokens: 12 });
  expect(written).not.toHaveProperty([TOKENS]);
  expect(written["trustabl.cot.present"]).toBe(true);
});

it("test_a_text_with_no_canonical_form_omits_only_the_fingerprint", () => {
  // One bad character costs the hash and nothing else.
  const { tracer, exporter } = tracing();
  withSpan(tracer, "chat", (span) => markReasoning(span, { text: "bad \ud800 surrogate", mode: "explicit_cot" }));
  const attrs = finished(exporter).attributes;
  expect(attrs["trustabl.cot.present"]).toBe(true);
  expect(attrs["trustabl.cot.mode"]).toBe("explicit_cot");
  expect(attrs).not.toHaveProperty(["trustabl.cot.text_fp"]);
});

it.each([[-1], [true], [1.5], [Number.NaN], ["12"]])("test_an_invalid_token_count_is_ignored %s", (count) => {
  const { tracer, exporter } = tracing();
  withSpan(tracer, "chat", (span) => {
    markReasoning(span, { reasoningTokens: count as unknown as number });
  });
  const attrs = finished(exporter).attributes;
  expect(attrs).not.toHaveProperty([TOKENS]);
  expect(cotKeys(attrs)).toEqual([]);
});

it("test_a_text_that_is_not_a_string_is_ignored", () => {
  const { tracer, exporter } = tracing();
  withSpan(tracer, "chat", (span) => {
    markReasoning(span, { text: ["compare both fares"] } as unknown as ReasoningOptions);
  });
  expect(cotKeys(finished(exporter).attributes)).toEqual([]);
});

it("test_the_same_reasoning_gives_the_same_fingerprint", () => {
  const { tracer, exporter } = tracing();
  for (const text of ["compare both fares", "compare both fares", "pick the cheaper fare"]) {
    withSpan(tracer, "chat", (span) => markReasoning(span, { text }));
  }
  const [first, second, third] = exporter.getFinishedSpans().map((s) => s.attributes["trustabl.cot.text_fp"]);
  expect(first).toBe(second);
  expect(first).not.toBe(third);
});

it("test_keys_stay_in_the_cot_namespace_and_reuse_the_token_key", () => {
  // The token count is OTel's. Minting trustabl.cot.token_count would give one
  // concept two keys, which the attribute spec's rule 3 forbids.
  const { tracer, exporter } = tracing();
  withSpan(tracer, "chat", (span) => {
    markReasoning(span, { text: "compare both fares", mode: "hybrid", reasoningTokens: 12 });
  });
  const attrs = finished(exporter).attributes;
  expect(cotKeys(attrs)).toEqual([keys.COT_MODE, keys.COT_PRESENT, keys.COT_TEXT_FP]);
  expect(Object.keys(attrs).filter((k) => !k.startsWith("trustabl.cot."))).toEqual([keys.GEN_AI_REASONING_TOKENS]);
});
