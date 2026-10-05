/**
 * A fingerprint that does not collapse repeats reports a loop as N distinct
 * calls and silently disables loop detection. Mirrors
 * python/tests/test_fingerprint.py.
 */

import { expect, it } from "vitest";

import { CanonicalizationError, canonicalFp } from "../src/index.js";

it("test_identical_arguments_produce_identical_fingerprints", () => {
  const args = { from: "SFO", to: "JFK", date: "2026-10-01" };
  expect(canonicalFp(args)).toBe(canonicalFp({ ...args }));
});

it("test_key_order_does_not_change_the_fingerprint", () => {
  expect(canonicalFp({ a: 1, b: 2 })).toBe(canonicalFp({ b: 2, a: 1 }));
});

it("test_different_arguments_produce_different_fingerprints", () => {
  expect(canonicalFp({ to: "JFK" })).not.toBe(canonicalFp({ to: "LAX" }));
});

it("test_fingerprint_is_sixteen_hex_characters", () => {
  expect(canonicalFp({ anything: "at all" })).toMatch(/^[0-9a-f]{16}$/);
});

it("test_volatile_fields_do_not_change_the_fingerprint", () => {
  const a = { query: "flights to JFK", request_id: "req-111", timestamp: "2026-09-22T10:00:00Z" };
  const b = { query: "flights to JFK", request_id: "req-222", timestamp: "2026-09-22T10:00:05Z" };
  expect(canonicalFp(a)).toBe(canonicalFp(b));
});

it("test_volatile_fields_are_stripped_at_any_depth", () => {
  const a = { outer: { query: "x", trace_id: "aaa" } };
  const b = { outer: { query: "x", trace_id: "bbb" } };
  expect(canonicalFp(a)).toBe(canonicalFp(b));
});

it("test_volatile_fields_inside_lists_are_stripped", () => {
  const a = { items: [{ sku: "A1", nonce: "n1" }] };
  const b = { items: [{ sku: "A1", nonce: "n2" }] };
  expect(canonicalFp(a)).toBe(canonicalFp(b));
});

it("test_a_meaningful_field_whose_name_contains_id_is_kept", () => {
  expect(canonicalFp({ customer_id: "c1" })).not.toBe(canonicalFp({ customer_id: "c2" }));
});

it("test_canonical_fp_raises_rather_than_hashing_an_uncanonicalizable_value", () => {
  expect(() => canonicalFp({ n: 2 ** 53 })).toThrow(CanonicalizationError);
});
