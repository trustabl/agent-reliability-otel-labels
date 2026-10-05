/**
 * Runs the Python, TypeScript and Go bindings over the same inputs and fails
 * on any difference in canonical string or fingerprint.
 *
 *   pnpm cross-check                        # uses ../python/.venv/bin/python and go on PATH
 *   TRUSTABL_PYTHON=python3 TRUSTABL_GO=/usr/local/go/bin/go pnpm cross-check
 *   TRUSTABL_TRACE=trace.json pnpm cross-check   # also check a captured trace's tool calls
 *
 * Each side reads and parses the input files itself. Handing Python a
 * JSON.stringify'd copy would turn 1.0 into 1 before Python saw it, hiding
 * exactly the kind of difference this exists to catch. Go keeps each input as
 * json.RawMessage for the same reason (go/cmd/crosscheck).
 */

import { execFileSync } from "node:child_process";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";

import { CanonicalizationError, canonicalFp, canonicalJson } from "../src/index.js";

const FIXTURES = fileURLToPath(new URL("../../spec/fingerprint-fixtures.json", import.meta.url));
const PYTHON =
  process.env.TRUSTABL_PYTHON ?? fileURLToPath(new URL("../../python/.venv/bin/python", import.meta.url));
const GO = process.env.TRUSTABL_GO ?? "go";
const GO_MODULE = fileURLToPath(new URL("../../go/", import.meta.url));

// The same input extraction as typescriptInputs() below, in Python.
const PY = String.raw`
import json, sys
import rfc8785
from agent_reliability_otel_labels import canonical_fp, canonical_json

fixtures_path, trace_path = sys.argv[1], sys.argv[2]
with open(fixtures_path, encoding="utf-8") as f:
    fixtures = json.load(f)
inputs = [c["input"] for c in fixtures["cases"]]
inputs += [c["input"] for c in fixtures["rejected"]["cases"]]
if trace_path:
    with open(trace_path, encoding="utf-8") as f:
        spans = json.load(f)["spans"]
    for span in spans:
        attrs = span.get("attributes", {})
        for key in ("gen_ai.tool.call.arguments", "gen_ai.tool.call.result"):
            if key in attrs:
                inputs.append(json.loads(attrs[key]))
out = []
for value in inputs:
    try:
        out.append({"canonical": canonical_json(value), "fingerprint": canonical_fp(value)})
    except rfc8785.CanonicalizationError:
        out.append(None)
json.dump(out, sys.stdout, ensure_ascii=False)
`;

type Result = { canonical: string; fingerprint: string } | null;

function typescriptInputs(tracePath: string): unknown[] {
  const fixtures = JSON.parse(readFileSync(FIXTURES, "utf8"));
  const inputs: unknown[] = [
    ...fixtures.cases.map((c: { input: unknown }) => c.input),
    ...fixtures.rejected.cases.map((c: { input: unknown }) => c.input),
  ];
  if (tracePath) {
    for (const span of JSON.parse(readFileSync(tracePath, "utf8")).spans) {
      const attrs = span.attributes ?? {};
      for (const key of ["gen_ai.tool.call.arguments", "gen_ai.tool.call.result"]) {
        if (key in attrs) inputs.push(JSON.parse(attrs[key]));
      }
    }
  }
  return inputs;
}

function typescriptResult(value: unknown): Result {
  try {
    return { canonical: canonicalJson(value), fingerprint: canonicalFp(value) };
  } catch (err) {
    if (err instanceof CanonicalizationError) return null;
    throw err;
  }
}

// Optional: a captured trace whose tool arguments and results are checked too.
const tracePath = process.env.TRUSTABL_TRACE ?? "";

const legs: Record<string, Result[]> = {
  python: JSON.parse(
    execFileSync(PYTHON, ["-c", PY, FIXTURES, tracePath], {
      encoding: "utf8",
      env: { ...process.env, PYTHONIOENCODING: "utf-8" },
    }),
  ),
  typescript: typescriptInputs(tracePath).map(typescriptResult),
  go: JSON.parse(
    execFileSync(GO, ["run", "./cmd/crosscheck", FIXTURES, tracePath], {
      cwd: GO_MODULE,
      encoding: "utf8",
    }),
  ),
};

const counts = Object.entries(legs).map(([name, results]) => `${name} ${results.length}`);
if (new Set(Object.values(legs).map((results) => results.length)).size !== 1) {
  console.error(`input count differs: ${counts.join(", ")}`);
  process.exit(1);
}

let differences = 0;
legs.typescript.forEach((_, i) => {
  const shown = Object.entries(legs).map(([name, results]) => [name, JSON.stringify(results[i])]);
  if (new Set(shown.map(([, value]) => value)).size !== 1) {
    differences++;
    console.error(`#${i}\n${shown.map(([name, value]) => `  ${name.padEnd(10)} ${value}`).join("\n")}`);
  }
});

console.log(`${legs.typescript.length} inputs, ${differences} differences across ${Object.keys(legs).join(", ")}`);
process.exit(differences === 0 ? 0 : 1);
