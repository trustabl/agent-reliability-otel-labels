/**
 * Every language binding must emit the same key names.
 *
 * spec/keys.json is the shared source of truth the Python, TypeScript and Go
 * implementations each check themselves against. Test titles are the Python
 * test names verbatim, "python" included, so one grep finds both.
 */

import { existsSync, readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { expect, it } from "vitest";

import { keys } from "../src/index.js";

const MANIFEST = fileURLToPath(new URL("../../spec/keys.json", import.meta.url));

function manifest() {
  return JSON.parse(readFileSync(MANIFEST, "utf8"));
}

function definedStrings(): Set<string> {
  return new Set((Object.values(keys) as unknown[]).filter((v): v is string => typeof v === "string"));
}

it("test_manifest_exists", () => {
  expect(existsSync(MANIFEST), `missing shared key manifest at ${MANIFEST}`).toBe(true);
});

it("test_every_manifest_attribute_is_defined_in_python", () => {
  const defined = definedStrings();
  const missing = (manifest().attributes as string[]).filter((a) => !defined.has(a));
  expect(missing, "TypeScript is missing manifest keys").toEqual([]);
});

it("test_python_defines_no_attribute_the_manifest_does_not_name", () => {
  // Inventing a key in one binding is how the ports diverge. A new key is a
  // spec revision first, then a manifest entry, then code.
  const m = manifest();
  const declared = new Set<string>([
    ...m.attributes,
    ...m.event_attributes,
    m.events.policy_binding,
  ]);
  const extra = [...definedStrings()].filter((v) => v.startsWith("trustabl.") && !declared.has(v));
  expect(extra, "TypeScript defines keys absent from the manifest").toEqual([]);
});

it("test_enums_match_the_manifest", () => {
  const e = manifest().enums;
  const sorted = (values: Iterable<string>) => [...values].sort();
  expect(sorted(keys.SIDE_EFFECTS)).toEqual(sorted(e.side_effect));
  expect(sorted(keys.ERROR_CLASSES)).toEqual(sorted(e.error_class));
  expect(sorted(keys.EXIT_REASONS)).toEqual(sorted(e.exit_reason));
  expect(sorted(keys.BINDING_TYPES)).toEqual(sorted(e.binding_type));
  expect(sorted(keys.BINDING_SOURCES)).toEqual(sorted(e.binding_source));
});

it("test_the_profile_is_frozen_at_1_0", () => {
  expect(manifest().profile_version).toBe("1.0.0");
});
