/**
 * The canonical serialization, pinned as a string. Mirrors
 * python/tests/test_canonical_json.py name for name; the values rejected here
 * are JavaScript's own non-JSON types.
 */

import { expect, it } from "vitest";

import { CanonicalizationError, canonicalJson } from "../src/index.js";

it("test_keys_are_sorted_and_separators_are_compact", () => {
  expect(canonicalJson({ b: 2, a: 1 })).toBe('{"a":1,"b":2}');
});

it("test_non_ascii_is_emitted_raw_not_escaped", () => {
  expect(canonicalJson({ q: "Tōkyō" })).toBe('{"q":"Tōkyō"}');
});

it("test_volatile_fields_are_gone_before_serialization", () => {
  expect(canonicalJson({ a: 1, request_id: "r" })).toBe('{"a":1}');
});

it("test_whole_floats_render_as_integers", () => {
  expect(canonicalJson({ f: 1.0 })).toBe('{"f":1}');
});

it("test_integers_have_no_decimal_point", () => {
  expect(canonicalJson({ i: 42 })).toBe('{"i":42}');
});

it("test_html_characters_are_not_escaped", () => {
  expect(canonicalJson({ q: "a<b>c&d" })).toBe('{"q":"a<b>c&d"}');
});

it("test_large_safe_floats_render_positionally", () => {
  expect(canonicalJson({ n: 1e15 })).toBe('{"n":1000000000000000}');
});

it("test_small_floats_render_positionally", () => {
  expect(canonicalJson({ n: 1e-5 })).toBe('{"n":0.00001}');
});

it("test_tiny_floats_use_an_exponent", () => {
  expect(canonicalJson({ n: 1e-7 })).toBe('{"n":1e-7}');
});

it("test_negative_zero_renders_as_zero", () => {
  expect(canonicalJson({ n: -0 })).toBe('{"n":0}');
});

it("test_keys_sort_by_utf16_code_units", () => {
  expect(canonicalJson({ "ﬁ": 1, "\u{1F600}": 2 })).toBe('{"\u{1F600}":2,"ﬁ":1}');
});

it("test_control_characters_are_escaped_as_ecmascript_does", () => {
  expect(canonicalJson({ s: 'a\u0000b\tc\nd\u001fe"f\\g' })).toBe(
    '{"s":"a\\u0000b\\tc\\nd\\u001fe\\"f\\\\g"}',
  );
});

it("test_rejects_an_integer_beyond_the_safe_range", () => {
  expect(() => canonicalJson({ n: 2 ** 53 })).toThrow(CanonicalizationError);
});

it("test_accepts_the_largest_safe_integer", () => {
  expect(canonicalJson({ n: Number.MAX_SAFE_INTEGER })).toBe('{"n":9007199254740991}');
});

it("test_rejects_a_whole_float_beyond_the_safe_range", () => {
  expect(() => canonicalJson({ n: 1e16 })).toThrow(CanonicalizationError);
});

it.each<[string, number]>([
  ["NaN", NaN],
  ["Infinity", Infinity],
  ["-Infinity", -Infinity],
])("test_rejects_nan_and_infinity[%s]", (_label, value) => {
  expect(() => canonicalJson({ n: value })).toThrow(CanonicalizationError);
});

class Point {
  constructor(readonly x = 1) {}
}

it.each<[string, unknown]>([
  ["bigint", { n: 10n }],
  // JSON.stringify would write a Date; Python has no matching rendering.
  ["Date", { at: new Date("2026-09-28T00:00:00Z") }],
  ["Map", { m: new Map([["a", 1]]) }],
  ["Set", { s: new Set([1]) }],
  ["function", { f: () => 1 }],
  ["symbol", { s: Symbol("s") }],
  ["class instance", { p: new Point() }],
  ["Uint8Array", { b: new Uint8Array([1]) }],
  ["undefined in an object", { u: undefined }],
  ["undefined in an array", [undefined]],
  ["array hole", [1, , 3]],
])("test_rejects_a_value_that_is_not_json[%s]", (_label, value) => {
  expect(() => canonicalJson(value)).toThrow(CanonicalizationError);
});

it("test_rejects_a_non_string_key", () => {
  expect(() => canonicalJson({ [Symbol("k")]: "a" })).toThrow(CanonicalizationError);
});

it("test_rejects_a_lone_surrogate_in_a_string", () => {
  expect(() => canonicalJson({ s: "\ud800" })).toThrow(CanonicalizationError);
});

it("test_rejects_a_lone_surrogate_in_a_key", () => {
  expect(() => canonicalJson({ "\udc00": 1 })).toThrow(CanonicalizationError);
});

it("never calls toJSON", () => {
  expect(() => canonicalJson({ v: { toJSON: () => "x" } })).toThrow(CanonicalizationError);
});

it("keeps a __proto__ key as data", () => {
  expect(canonicalJson(JSON.parse('{"__proto__":{"x":1},"a":2}'))).toBe(
    '{"__proto__":{"x":1},"a":2}',
  );
});
