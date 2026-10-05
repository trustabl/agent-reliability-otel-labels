/**
 * Every Python test has a TypeScript counterpart titled with its exact name,
 * so one grep finds both and a port cannot quietly skip a guarantee.
 * Parametrised Python tests map to one it.each whose title starts with the
 * same name.
 */

import { readdirSync, readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { expect, it } from "vitest";

const PYTHON_TESTS = fileURLToPath(new URL("../../python/tests/", import.meta.url));
const TS_TESTS = fileURLToPath(new URL("./", import.meta.url));

function namesIn(dir: string, suffix: string, pattern: RegExp): Set<string> {
  const names = new Set<string>();
  for (const file of readdirSync(dir).filter((f) => f.endsWith(suffix))) {
    for (const match of readFileSync(dir + file, "utf8").matchAll(pattern)) names.add(match[1]);
  }
  return names;
}

it("test_every_python_test_has_a_typescript_counterpart", () => {
  const python = namesIn(PYTHON_TESTS, ".py", /^\s*def (test_\w+)\(/gm);
  // A test title is a string literal that directly follows it(, it.each(...)(
  // or it.skipIf(...)(  - not any string literal anywhere in the file, which
  // would also count a comment or an unrelated string that merely starts with
  // "test_".
  const typescript = namesIn(TS_TESTS, ".test.ts", /(?:\)\(|\bit\()\s*["'`](test_\w+)/g);
  expect(python.size).toBeGreaterThan(0);
  expect([...python].filter((name) => !typescript.has(name)).sort()).toEqual([]);
});
