/**
 * Fingerprints: sameness of a tool call without its contents.
 *
 * The canonical form is RFC 8785 (JCS), the same in every binding. For a
 * string, a finite number, a boolean or null, JCS output is by definition what
 * JSON.stringify writes, so the work here is only sorting keys and refusing
 * what has no faithful form. No canonicalization library: the `canonicalize`
 * package ships ESM only and calls toJSON.
 */

import { sha256 } from "@noble/hashes/sha2";
import { bytesToHex, utf8ToBytes } from "@noble/hashes/utils";

export const FP_LENGTH = 16;

/**
 * Field names stripped before hashing: transport and bookkeeping that changes
 * between two otherwise identical calls. Matched EXACTLY and case-insensitively,
 * never as substrings - `customer_id` and `date` are real input. Identical to
 * Python's VOLATILE_KEYS.
 */
export const VOLATILE_KEYS: ReadonlySet<string> = new Set([
  "timestamp", "ts", "_ts", "created_at", "updated_at", "sent_at", "received_at",
  "request_id", "requestid", "req_id",
  "trace_id", "traceid", "span_id", "spanid", "correlation_id",
  "nonce", "uuid", "guid",
  "cursor", "page_token", "next_page_token", "continuation_token",
  "idempotency_key",
]);

/** A value with no canonical form. The caller gets no fingerprint rather than a wrong one. */
export class CanonicalizationError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "CanonicalizationError";
  }
}

function isPlainObject(value: object): value is Record<string, unknown> {
  const proto = Object.getPrototypeOf(value);
  return proto === Object.prototype || proto === null;
}

/** Return value with volatile fields removed, at any depth. Never throws. */
export function stripVolatile(value: unknown): unknown {
  if (Array.isArray(value)) {
    // Array.from visits holes as undefined, which serialize() then refuses.
    return Array.from(value, stripVolatile);
  }
  if (value !== null && typeof value === "object" && isPlainObject(value)) {
    // A null-prototype copy, so a "__proto__" key stays data.
    const out: Record<PropertyKey, unknown> = Object.create(null);
    for (const [key, child] of Object.entries(value)) {
      if (!VOLATILE_KEYS.has(key.toLowerCase())) out[key] = stripVolatile(child);
    }
    // Carried over so serialize() can refuse them; Object.entries skips symbols.
    for (const sym of Object.getOwnPropertySymbols(value)) out[sym] = (value as Record<PropertyKey, unknown>)[sym];
    return out;
  }
  return value;
}

function describe(value: unknown): string {
  if (value === null || typeof value !== "object") return typeof value;
  return (value as { constructor?: { name?: string } }).constructor?.name ?? "object";
}

// RFC 8785 requires I-JSON, which forbids an unpaired surrogate. Node's floor
// here is >=18, so String.prototype.isWellFormed (Node 20+) is not available.
const LONE_SURROGATE = /[\uD800-\uDBFF](?![\uDC00-\uDFFF])|(?<![\uD800-\uDBFF])[\uDC00-\uDFFF]/;

function serialize(value: unknown): string {
  if (typeof value === "string" && LONE_SURROGATE.test(value)) {
    throw new CanonicalizationError(`string contains an unpaired surrogate: ${JSON.stringify(value)}`);
  }
  if (value === null || typeof value === "boolean" || typeof value === "string") {
    return JSON.stringify(value);
  }
  if (typeof value === "number") {
    if (!Number.isFinite(value)) {
      throw new CanonicalizationError(`${value} is not a finite number`);
    }
    // JSON.parse gives 1e16 and 10000000000000000 the same value, so an
    // integral number past 2^53 may already have been rounded. Refuse it.
    if (Number.isInteger(value) && !Number.isSafeInteger(value)) {
      throw new CanonicalizationError(`${value} is beyond the safe integer range ±(2^53-1)`);
    }
    return JSON.stringify(value);
  }
  if (Array.isArray(value)) {
    return `[${Array.from(value, serialize).join(",")}]`;
  }
  if (typeof value === "object" && isPlainObject(value)) {
    if (Object.getOwnPropertySymbols(value).length > 0) {
      throw new CanonicalizationError("object has a symbol key");
    }
    // The default sort compares UTF-16 code units, which is what RFC 8785 requires.
    const members = Object.keys(value)
      .sort()
      .map((key) => {
        if (LONE_SURROGATE.test(key)) {
          throw new CanonicalizationError(`object key contains an unpaired surrogate: ${JSON.stringify(key)}`);
        }
        return `${JSON.stringify(key)}:${serialize(value[key])}`;
      });
    return `{${members.join(",")}}`;
  }
  throw new CanonicalizationError(`cannot canonicalize a value of type ${describe(value)}`);
}

/**
 * Return the exact string that gets hashed: RFC 8785 after volatile fields are
 * stripped. Exposed so a port can diff characters instead of guessing why two
 * hashes disagree. Throws CanonicalizationError for a value with no faithful
 * form.
 */
export function canonicalJson(value: unknown): string {
  return serialize(stripVolatile(value));
}

/** Return the fixed-length fingerprint of value, volatile fields removed. */
export function canonicalFp(value: unknown): string {
  return bytesToHex(sha256(utf8ToBytes(canonicalJson(value)))).slice(0, FP_LENGTH);
}
