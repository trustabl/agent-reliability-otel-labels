"""Fingerprints: sameness of a tool call without its contents.

A fingerprint answers "was this the same call?" from a fixed-length hash, so a
loop is detectable without anyone storing what the user asked for.

Canonicalization is the load-bearing half. Two identical calls a second apart
differ in their clocks and request ids; if those reach the hash, every repeat
looks unique and loop detection silently does nothing while appearing healthy.
"""

import hashlib

import rfc8785

FP_LENGTH = 16

#: The largest integer a double holds exactly. JSON numbers are doubles in
#: JavaScript, so an integral value beyond this has already been rounded there.
MAX_SAFE_INTEGER = 2**53 - 1

#: Field names stripped before hashing: transport and bookkeeping that changes
#: between two otherwise identical calls. Matched EXACTLY and case-insensitively,
#: never as substrings - `customer_id` and `date` are real input, and stripping
#: them would collapse genuinely different calls into one fingerprint, which
#: reports loops that never happened. Under-stripping misses a loop; over-
#: stripping invents one. Invented loops are worse, so this list stays narrow.
VOLATILE_KEYS = frozenset({
    "timestamp", "ts", "_ts", "created_at", "updated_at", "sent_at", "received_at",
    "request_id", "requestid", "req_id",
    "trace_id", "traceid", "span_id", "spanid", "correlation_id",
    "nonce", "uuid", "guid",
    "cursor", "page_token", "next_page_token", "continuation_token",
    "idempotency_key",
})


def strip_volatile(obj, volatile=VOLATILE_KEYS):
    """Return obj with volatile fields removed, at any depth."""
    if isinstance(obj, dict):
        return {
            k: strip_volatile(v, volatile)
            for k, v in obj.items()
            if not (isinstance(k, str) and k.lower() in volatile)
        }
    if isinstance(obj, (list, tuple)):
        return [strip_volatile(v, volatile) for v in obj]
    return obj


def _reject_unsafe_integral_floats(obj):
    """Raise for a float that is a whole number beyond ±(2^53-1).

    rfc8785 refuses such an int but lets the float through, because Python can
    tell the two apart. JavaScript cannot - JSON.parse gives 1e16 and
    10000000000000000 the same value - so accepting the float here would give
    Python a fingerprint the TypeScript binding must refuse.
    """
    if isinstance(obj, float):
        if obj.is_integer() and abs(obj) > MAX_SAFE_INTEGER:
            raise rfc8785.IntegerDomainError(int(obj))
    elif isinstance(obj, dict):
        for value in obj.values():
            _reject_unsafe_integral_floats(value)
    elif isinstance(obj, list):
        for value in obj:
            _reject_unsafe_integral_floats(value)


def canonical_json(obj, volatile=VOLATILE_KEYS) -> str:
    """Return the exact string that gets hashed.

    Exposed so a port can diff characters instead of guessing why two hashes
    disagree. The form is RFC 8785 (JCS), identical in every binding: keys
    sorted by UTF-16 code units, no whitespace, UTF-8 raw, and strings and
    numbers exactly as ECMAScript's JSON.stringify writes them - `1.0` is `1`,
    `1e-5` is `0.00001`. json.dumps disagrees with JavaScript on each of those
    numbers, which is why it is not used here.

    Raises rfc8785.CanonicalizationError for a value with no faithful form:
    an integral number beyond ±(2^53-1), NaN, an infinity, a non-string key,
    or any non-JSON type. No fingerprint beats a wrong one.
    """
    stripped = strip_volatile(obj, volatile)
    _reject_unsafe_integral_floats(stripped)
    try:
        return rfc8785.dumps(stripped).decode("utf-8")
    except UnicodeError as err:
        # rfc8785 raises UnicodeEncodeError, not CanonicalizationError, for an
        # unpaired surrogate in an object key; keep the documented contract.
        raise rfc8785.CanonicalizationError(str(err)) from err


def canonical_fp(obj, volatile=VOLATILE_KEYS) -> str:
    """Return the fixed-length fingerprint of obj, volatile fields removed.

    Stripping is the default rather than a separate step the caller must
    remember: forgetting it fails silently, so the safe behaviour is the one
    you get by doing nothing.
    """
    return hashlib.sha256(
        canonical_json(obj, volatile).encode("utf-8")
    ).hexdigest()[:FP_LENGTH]
