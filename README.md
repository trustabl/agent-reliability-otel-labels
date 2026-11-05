# agent-reliability-otel-labels

Process-metadata labels for OpenTelemetry agent spans.

This library adds about a dozen attributes and one event type to spans that your
agent **already emits**. It answers questions a trace cannot otherwise answer:
was this the same call as before, was this a retry, does this tool destroy
something, and which signed policies were actually in force.

Apache-2.0. No API key, no account, no Trustabl service. If the Trustabl binary
is absent, this still works — that is a design requirement, not a side effect.

## The one prerequisite

**Your agent must already be instrumented.** This library decorates spans; it
never creates them. LangChain's instrumentation, OpenLIT, Traceloop,
OpenInference or a framework's native `gen_ai` support all qualify.

An agent emitting no telemetry gets nothing from this, and the honest fix there
is to add an instrumentation library, not this one.

## Install

```bash
pip install agent-reliability-otel-labels                     # Python 3.9+
npm install @trustabl/agent-reliability-otel-labels           # Node 18+, ESM and CJS
go get github.com/trustabl/agent-reliability-otel-labels/go   # Go 1.25+
```

## Use

```python
from agent_reliability_otel_labels import (
    Binding, Run, bind_authority, canonical_fp, mark_run_end, mark_tool_span,
)


def run_agent(args):
    run = Run()      # one per run: never share a Run between runs
    attempts = {}    # calls so far in this run, per arguments fingerprint

    with tracer.start_as_current_span("invoke_agent") as root:
        run.mark_start(root)
        bind_authority(root, bindings=[
            Binding(id="openshell-seccomp-v3", type="sandbox", sha256=...,
                    version="3.1.0", required=True, source="entrypoint"),
            Binding(id="acs-content-v8", type="content_safety", sha256=...,
                    version="8.0.2", required=True, source="entrypoint"),
        ])
        try:
            with tracer.start_as_current_span("execute_tool") as span:
                key = canonical_fp(args)
                attempts[key] = attempts.get(key, 0) + 1
                step = run.next_step()
                result = None
                try:
                    result = search_flights(args)
                finally:
                    # In finally, so a call that raises is labelled too.
                    mark_tool_span(span, args=args, result=result,
                                   attempt=attempts[key], side_effect="read",
                                   step_index=step, run_id=run.run_id)

            # Call before the root span ends; an ended span drops the write.
            mark_run_end(root, exit_reason="final_answer")
        except Exception:
            mark_run_end(root, exit_reason="error")
            raise
```

`python/examples/agent.py` runs this end to end and prints what an exporter
receives. `typescript/examples/vercel-ai/` and `go/examples/adk/` do the same
against real agent frameworks and check where every label landed.

```typescript
import { Run, bindAuthority, canonicalFp, markRunEnd, markToolSpan } from "@trustabl/agent-reliability-otel-labels";

async function runAgent(args: Record<string, unknown>) {
  const run = new Run();                      // one per run: never share a Run between runs
  const attempts = new Map<string, number>(); // calls so far in this run, per arguments fingerprint

  await tracer.startActiveSpan("invoke_agent", async (root) => {
    run.markStart(root);
    bindAuthority(root, [
      { id: "openshell-seccomp-v3", type: "sandbox", sha256: "...",
        version: "3.1.0", required: true, source: "entrypoint" },
      { id: "acs-content-v8", type: "content_safety", sha256: "...",
        version: "8.0.2", required: true, source: "entrypoint" },
    ]);
    try {
      await tracer.startActiveSpan("execute_tool", async (span) => {
        const key = canonicalFp(args);
        const attempt = (attempts.get(key) ?? 0) + 1;
        attempts.set(key, attempt);
        const stepIndex = run.nextStep();
        let result: unknown;
        try {
          result = await searchFlights(args);
        } finally {
          // In finally, so a call that throws is labelled too.
          markToolSpan(span, {
            args, result, attempt, sideEffect: "read",
            stepIndex, runId: run.runId,
          });
          span.end();
        }
      });

      // Call before the root span ends; an ended span drops the write.
      markRunEnd(root, "final_answer");
    } catch (err) {
      markRunEnd(root, "error");
      throw err;
    } finally {
      root.end();
    }
  });
}
```

In Go, structs are refused rather than converted silently: marshal them and
pass `json.RawMessage`. `go/example_test.go` compiles this block.

```go
run := otellabels.NewRun("")

ctx, root := tracer.Start(ctx, "invoke_agent")
run.MarkStart(root)
_ = otellabels.BindAuthority(root, []otellabels.Binding{
	{ID: "openshell-seccomp-v3", Type: otellabels.Sandbox, SHA256: "...",
		Version: "3.1.0", Required: true, Source: otellabels.Entrypoint},
})

_, span := tracer.Start(ctx, "execute_tool")
result := searchFlights(args)
raw, _ := json.Marshal(args) // structs are refused: pass JSON text
step := run.NextStep()
_ = otellabels.MarkToolSpan(span, otellabels.ToolCall{
	Args: json.RawMessage(raw), Result: result, Attempt: 1,
	SideEffect: otellabels.Read, StepIndex: &step, RunID: run.RunID(),
})
span.End()

// Call before the root span ends; an ended span drops the write.
_ = otellabels.MarkRunEnd(root, otellabels.FinalAnswer)
root.End()
```

## Label a model call's reasoning

If your model reasons, label the span of the model call with the same care:

```python
with tracer.start_as_current_span("chat") as span:
    response = call_model(prompt)
    # Only the fingerprint leaves the process. Pass the reasoning, not the answer.
    mark_reasoning(span, text=response.thinking, mode="latent",
                   reasoning_tokens=response.usage.reasoning_tokens)
```

```typescript
markReasoning(span, { text: response.thinking, mode: "latent", reasoningTokens: response.usage.reasoningTokens });
```

```go
_ = otellabels.MarkReasoning(span, otellabels.Reasoning{
	Text: thinking, Mode: otellabels.ModeLatent, ReasoningTokens: &tokens,
})
```

Every argument is optional. A model that hides its reasoning still reports
tokens, so pass what you have: tokens with no text says the model reasoned and
showed nothing. A call that says nothing either way writes nothing, because an
omitted `trustabl.cot.present` means "unknown" and false would be a false claim.

Pass the text your provider returns, and declare the mode for what that text is.
Some providers return a summary and never the raw chain of thought: the fingerprint
then proves which summary the span saw and no more, and `explicit_cot`, which says
the text is the reasoning itself, would overclaim. `latent` is the honest mode there.

These labels say that a model reasoned and which text it produced. They do not
say the reasoning was faithful: `present` means reasoning occurred, not that it
caused the answer.

## What it writes

| Attribute | On | Answers |
|---|---|---|
| `trustabl.step_index` | tool calls | What order did this really happen in? |
| `trustabl.run_id` | root and tool calls | Which spans belong to one job? One job inside a `gen_ai.conversation.id` thread. |
| `trustabl.tool.input_fp` | tool calls | Was this the same call as before? |
| `trustabl.tool.output_fp` | tool calls | Did the same input give the same result? |
| `trustabl.tool.side_effect` | tool calls | `pure`, `read`, `write` or `irreversible`? Omitted when unknown. |
| `trustabl.tool.error_class` | tool calls | Did it actually work? |
| `trustabl.tool.attempt` | tool calls | Is it retrying? |
| `trustabl.handoff_id` | child agent span | Which agent delegated to which? |
| `trustabl.exit_reason` | root, before it ends | How did the run end? |
| `trustabl.policy.binding` (event) | agent / sandbox root | Which signed policies were loaded? |
| `trustabl.cot.present` | model calls | Did this call reason? Omitted when nothing says either way. |
| `trustabl.cot.text_fp` | model calls | Was this the same reasoning text? The text itself is never stored. |
| `trustabl.cot.mode` | model calls | `explicit_cot`, `latent`, `symbolic`, `hybrid` or `none`, as you declare it. Omitted when unknown. |

One event per policy. Never a flattened hash, never parallel arrays: a single
combined hash reports a content policy as present when only the sandbox loaded.

## What it deliberately does not do

- **Does not create spans.** No active span is a silent no-op, never a span we invented.
- **Does not auto-instrument.** No monkey-patching, no client wrapping, no import hooks.
- **Does not carry content.** Arguments, results and reasoning text are fingerprinted, never stored.
  A test asserts no caller text reaches any attribute.
- **Does not fingerprint model answers.** Chat is not supposed to repeat, so a
  differing answer is not a finding. Reasoning text is hashed for a different
  reason, to match a text captured elsewhere to its span, and a differing
  reasoning fingerprint is not a finding either.
- **Does not re-assert `gen_ai.*` keys** your instrumentation already set, and
  mints no `trustabl.*` key for a concept they name:
  `gen_ai.usage.reasoning.output_tokens` is the reasoning token count, and it is
  filled only when it is empty.
- **Does not emit keys produced elsewhere** — contract identity arrives through
  `OTEL_RESOURCE_ATTRIBUTES`, and bind quality comes from the collector processor.

## Fingerprints and privacy

Tool arguments are hashed, not recorded:

```
{"from": "SFO", "to": "JFK", "date": "2026-10-01"}  →  c1baebf63d8d7587
```

Same arguments give the same code; different arguments give a different one;
the code cannot be turned back into the arguments. Seeing one code twelve times
proves a repeat without anyone storing what was asked.

Clocks, request ids, trace ids and nonces are stripped before hashing —
otherwise every repeat looks unique and loop detection silently does nothing.
The strip list is deliberately narrow: missing a loop is bad, inventing one is
worse.

Reasoning text is hashed the same way, and it needs more care than a tool's
arguments, because a chain of thought quotes whatever the model read. The hash
is one-way but not secret: anyone already holding a candidate text can hash it
and confirm the match. No OTel convention for hashing reasoning exists yet, so
this default is this library's own design rather than a standard.

## Status

**v0.1.1** for Python and TypeScript (v0.1.0 for Go; v0.1.0 was the first
public release), on
[PyPI](https://pypi.org/project/agent-reliability-otel-labels/),
[npm](https://www.npmjs.com/package/@trustabl/agent-reliability-otel-labels) and
[pkg.go.dev](https://pkg.go.dev/github.com/trustabl/agent-reliability-otel-labels/go).
Before 1.0, a minor version may change the API. The attribute names are *reserved* in the
Trustabl attribute spec, meaning nothing else may claim them, and they become a
spec addition when this ships.

| Binding | State |
|---|---|
| Python | implemented, 114 tests |
| TypeScript | implemented; run against a real Vercel AI SDK agent with a scripted model |
| Go | implemented; run against a real Google ADK agent with a scripted model |

## Repository layout

```
spec/keys.json                  every key name, shared by all bindings
spec/fingerprint-fixtures.json  golden canonicalization every binding must reproduce
python/                         the Python binding
typescript/                     the TypeScript binding, @trustabl/agent-reliability-otel-labels
go/                             the Go binding, github.com/trustabl/agent-reliability-otel-labels/go
docs/PORTING.md                 how to add a language
```

`spec/` is the contract between bindings. Each one carries a test asserting it
matches, so a port cannot quietly invent a key or drift on canonicalization.

## Development

```bash
cd python
python3 -m venv .venv && .venv/bin/pip install -e '.[dev]'
.venv/bin/python -m pytest

cd ../typescript
pnpm install
pnpm test && pnpm typecheck && pnpm build
pnpm cross-check    # all three bindings over the same inputs; needs python/.venv and go

cd ../go
go vet ./... && go test -race ./...
```

## License

Apache-2.0. See [`LICENSE`](LICENSE).
