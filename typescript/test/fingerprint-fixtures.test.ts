/**
 * Golden fingerprints every language binding must reproduce. Mirrors
 * python/tests/test_fingerprint_fixtures.py and reads the same file.
 */

import { existsSync, readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { expect, it } from "vitest";

import { CanonicalizationError, canonicalFp, canonicalJson } from "../src/index.js";

const FIXTURES = fileURLToPath(new URL("../../spec/fingerprint-fixtures.json", import.meta.url));

interface Fixtures {
  cases: Array<{ name: string; input: unknown; canonical: string; fingerprint: string }>;
  rejected: { cases: Array<{ name: string; input: unknown }> };
}

function fixtures(): Fixtures {
  return JSON.parse(readFileSync(FIXTURES, "utf8"));
}

it("test_fixture_file_exists", () => {
  expect(existsSync(FIXTURES), `missing cross-language fixtures at ${FIXTURES}`).toBe(true);
});

it("test_every_fixture_reproduces_its_canonical_string", () => {
  // Checked before the hash on purpose: a character diff explains itself.
  for (const c of fixtures().cases) {
    expect(canonicalJson(c.input), c.name).toBe(c.canonical);
  }
});

it("test_every_fixture_reproduces_its_recorded_fingerprint", () => {
  for (const c of fixtures().cases) {
    expect(canonicalFp(c.input), c.name).toBe(c.fingerprint);
  }
});

it("test_every_rejected_input_raises", () => {
  for (const c of fixtures().rejected.cases) {
    expect(() => canonicalJson(c.input), c.name).toThrow(CanonicalizationError);
  }
});
