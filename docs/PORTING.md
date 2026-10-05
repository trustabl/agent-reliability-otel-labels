# Porting to another language

Python, TypeScript and Go ship. This is what a port has to do and where it
will go wrong.

## The contract

Two files in `spec/` are the shared source of truth. A binding that passes both
is conformant; one that does not is a different profile wearing the same name.

| File | What it pins |
|---|---|
| `spec/keys.json` | every attribute and event name, and every enum value |
| `spec/fingerprint-fixtures.json` | the exact canonical string and fingerprint for 27 inputs, and 5 inputs every binding must refuse |

Both are derived from the Trustabl attribute spec, where these names are
**reserved**. Adding a key is three steps in order: amend that spec with a
named producer and bump its version, then this manifest, then code. Never code
first.

## Port order, and why

**TypeScript second.** The Vercel AI SDK is TypeScript-only, and the OpenAI
Agents SDK and MCP have TypeScript implementations people actually ship.

**Go third.** Few agents are written in Go today. It earns its place when a Go
service hosts an agent. Low urgency, not low value.

## What every binding implements

Five entry points, same names, same semantics:

| Entry point | Does |
|---|---|
| `canonicalJson(obj)` | the exact string that gets hashed — **export it**, ports are debugged through it |
| `canonicalFp(obj)` | sha256 of that string, hex, first 16 characters |
| `markToolSpan(span, {...})` | the tool attributes |
| `bindAuthority(span, bindings)` | one `trustabl.policy.binding` event per policy |
| `Run` / `markRunEnd` / `markHandoff` | run id, step order, outcome, delegation |

And five behaviours that are not optional:

1. **Never create a span.** No usable span means return quietly. A span we
   invented is not evidence and it corrupts the trace shape the binder reads.
2. **Never overwrite `gen_ai.*`.** Fill one only if it is absent, and never when
   you cannot read the span's attributes to tell.
3. **Validate enums, but only after the no-op check.** A workload with tracing
   disabled must not be crashable by this library; a bad enum surfaces the first
   time anyone runs with tracing on.
4. **Strip volatile fields before hashing.** Forgetting this fails *silently*,
   which is why stripping is the default rather than a separate call.
5. **`markToolSpan` never lets an exception from fingerprinting reach the
   caller.** Not only `CanonicalizationError` - a circular argument or a
   throwing getter must not escape either. It omits that one fingerprint
   instead.

## The canonicalization contract

Every binding serializes with **RFC 8785, the JSON Canonicalization Scheme
(JCS)**, after volatile fields are stripped. This is where ports used to
diverge, and the divergence is invisible: fingerprints stop matching across
languages and cross-language loop detection quietly degrades. JCS is the IETF
standard that makes every language agree, floats included.

1. **Strip** volatile keys at any depth: the exact, case-insensitive list in
   `VOLATILE_KEYS`.
2. **Serialize** with RFC 8785:
   - object keys sorted by UTF-16 code units, not code points (`"😀"` sorts
     before `"ﬁ"`);
   - no whitespace;
   - strings and numbers exactly as ECMAScript `JSON.stringify` writes a
     primitive: `1.0` → `1`, `1e15` → `1000000000000000`, `1e-5` → `0.00001`,
     `1e-7` → `1e-7`, `-0` → `0`;
   - UTF-8 output, non-ASCII raw, HTML characters unescaped.
3. **Hash**: SHA-256 over the UTF-8 bytes, lowercase hex, first 16 characters.

**A value that cannot be canonicalized produces no fingerprint.** That means
integral numbers beyond ±(2^53−1), whether the language holds them as an
integer or a float, because JavaScript cannot tell `1e16` from
`10000000000000000`. It also covers NaN, ±Infinity, non-string object keys,
and any non-JSON type. `canonicalJson` and `canonicalFp` throw. `markToolSpan`
catches the error per value and omits only `input_fp` or `output_fp`. A hash
over a precision-lost integer could make two different calls look alike:
degrade to absent, never to wrong.

Check `canonical` from the fixtures before you check any hash: a character
diff says what went wrong, a hash diff does not.

| Language | How |
|---|---|
| Python | the `rfc8785` package, plus a pre-check refusing integral floats beyond 2^53−1 (the package accepts them because Python can tell a float from an int) |
| JavaScript | `JSON.stringify` on each primitive, your own key sort, never `toJSON`; refuse integral numbers that fail `Number.isSafeInteger` |
| Go | walk the value yourself; `jcs.NumberToJSON` for floats only; decode JSON text with `UseNumber()` and range-check integers on their text; refuse invalid UTF-8 before decoding, and check each string for unpaired surrogate escapes where it is used, since `encoding/json` silently turns them into U+FFFD |

## Conformance checklist for a new binding

- [ ] Reproduces every `canonical` string in the fixtures, character for character
- [ ] Reproduces every `fingerprint`
- [ ] Refuses every `rejected` input, and each non-JSON value of its own language, writing no fingerprint for it
- [ ] Defines every attribute in `keys.json`, and no `trustabl.*` key it does not name
- [ ] Enum values match the manifest exactly
- [ ] No usable span is a silent no-op, with no exception escaping
- [ ] An existing `gen_ai.tool.name` survives a call that passes a different name
- [ ] Attaches to spans from all three conventions without disturbing their attributes
- [ ] No caller text reaches any attribute — assert over a value with a recognisable secret
- [ ] Every string attribute written is bounded in length
- [ ] Order survives reordering: shuffle exported spans, sort by `step_index`, recover the sequence

Python's `tests/` implements each of these; port the test names as well as the code.

## TypeScript specifics

```
typescript/
  package.json        @trustabl/agent-reliability-otel-labels, ESM + CJS
  src/                one module per Python module
  test/               one file per Python test module, same test names
  scripts/
    cross-check.ts    runs all three bindings over shared inputs, diffs the results
```

- SHA-256 and run ids come from `@noble/hashes`: synchronous on Node,
  browsers and edge runtimes. `node:crypto` is missing on the edge and Web
  Crypto's `digest` is asynchronous, but `markToolSpan` must stay synchronous.
- Never fill `gen_ai.tool.name` from an API `Span`: it exposes no attributes.
  Read the SDK span's `attributes` when present; when it is not, write nothing.
- Peer-depend on `@opentelemetry/api` only. The SDK is a dev dependency for tests.
- Write your own sorted serializer: `JSON.stringify` on each primitive is
  already the JCS form, but its `replacer` does not sort and it calls `toJSON`.
- Refuse integral numbers outside `Number.isSafeInteger`. You cannot tell
  whether the caller meant an integer, which is why every binding refuses both.
- Ship ESM and CJS. Agent code lives in both.

## Go specifics

```
go/
  go.mod              github.com/trustabl/agent-reliability-otel-labels/go
  *.go                one file per Python module; package otellabels
  cmd/crosscheck/     the Go leg of typescript/scripts/cross-check.ts
```

- Depend on `go.opentelemetry.io/otel/trace` and `/attribute` (the API), not
  the SDK. `thindeps_test.go` enforces it.
- Walk values yourself. Never use `jcs.Transform`: it rounds integers beyond
  2^53 without complaint (`9007199254740993` becomes `9007199254740992`).
  Never use `json.Marshal` for output: it escapes `<`, `>` and `&`. Use
  `jcs.NumberToJSON` for floats and nothing else from the library.
- Keep integers exact: decode JSON text with `UseNumber()` and range-check the
  number's text, never a `float64`.
- `encoding/json` replaces invalid UTF-8 and unpaired surrogate escapes with
  U+FFFD instead of failing. Refuse both before decoding, or the shared
  `rejected` fixture passes as a real fingerprint. Check each string where it
  is used, not the whole text up front: a bad escape inside a volatile field
  is stripped with the field, as in the other bindings.
- `strings.ToLower` uses simple case mapping: `İ` (U+0130) becomes `i`, where
  Python and JavaScript produce two code points. A key containing U+0130 is
  never volatile.
- A Go stack overflow is fatal, so bound the nesting depth; a cyclic map is
  otherwise a crashed process, not an error. The limit is 1000 levels of JSON
  nesting; Python's recursion limit and JavaScript's stack put theirs at
  roughly 1000 and 10,000, so the exact refusal depth differs by language.
- Structs are refused, `time.Time` included, as Python refuses `datetime` and
  TypeScript refuses `Date`. Callers marshal with `encoding/json` and pass
  `json.RawMessage`; that fingerprints identically to the equivalent map.

## A port is not done when its tests pass

It is done when it reproduces the fixtures **and** someone has run it against a
real agent in that language and looked at the spans.

## Open follow-up

- **TypeScript and Go have run against real agents, with a scripted model.**
  `typescript/examples/vercel-ai/` (Vercel AI SDK) and `go/examples/adk/`
  (Google ADK for Go) label the frameworks' own spans and check where every
  label landed. The fingerprints on those live spans match Python's.
- **Still open: a run with a real model**, which varies its tool arguments
  where a scripted one does not, and a run against OpenAI Agents TS, whose
  spans come through the OpenInference bridge.
