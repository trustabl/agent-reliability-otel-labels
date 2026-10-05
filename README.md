# agent-reliability-otel-labels

Process-metadata labels for OpenTelemetry agent spans.

This library adds about nine attributes and one event type to spans that your
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
from agent_reliability_otel_labels import Binding, Run, bind_authority, mark_run_end, mark_tool_span

run = Run()

with tracer.start_as_current_span("invoke_agent") as root:
    run.mark_start(root)
    bind_authority(root, bindings=[
        Binding(id="openshell-seccomp-v3", type="sandbox", sha256=...,
                version="3.1.0", required=True, source="entrypoint"),
        Binding(id="acs-content-v8", type="content_safety", sha256=...,
                version="8.0.2", required=True, source="entrypoint"),
    ])

    with tracer.start_as_current_span("execute_tool") as span:
        result = search_flights(args)
        mark_tool_span(span, args=args, result=result, attempt=1,
                       side_effect="read", step_index=run.next_step(),
                       run_id=run.run_id)

    # Call before the root span ends; an ended span drops the write.

    mark_run_end(root, exit_reason="final_answer")
```

`python/examples/agent.py` runs this end to end and prints what an exporter
receives. `typescript/examples/vercel-ai/` and `go/examples/adk/` do the same
against real agent frameworks and check where every label landed.

```typescript
import { Run, bindAuthority, markRunEnd, markToolSpan } from "@trustabl/agent-reliability-otel-labels";

const run = new Run();

await tracer.startActiveSpan("invoke_agent", async (root) => {
  run.markStart(root);
  bindAuthority(root, [
    { id: "openshell-seccomp-v3", type: "sandbox", sha256: "...",
      version: "3.1.0", required: true, source: "entrypoint" },
    { id: "acs-content-v8", type: "content_safety", sha256: "...",
      version: "8.0.2", required: true, source: "entrypoint" },
  ]);

  await tracer.startActiveSpan("execute_tool", async (span) => {
    const result = await searchFlights(args);
    markToolSpan(span, {
      args, result, attempt: 1, sideEffect: "read",
      stepIndex: run.nextStep(), runId: run.runId,
    });
    span.end();
  });

  // Call before the root span ends; an ended span drops the write.
  markRunEnd(root, "final_answer");
  root.end();
});
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

One event per policy. Never a flattened hash, never parallel arrays: a single
combined hash reports a content policy as present when only the sandbox loaded.

## What it deliberately does not do

- **Does not create spans.** No active span is a silent no-op, never a span we invented.
- **Does not auto-instrument.** No monkey-patching, no client wrapping, no import hooks.
- **Does not carry content.** Arguments and results are fingerprinted, never stored.
  A test asserts no caller text reaches any attribute.
- **Does not fingerprint model completions.** Chat is not supposed to repeat.
- **Does not re-assert `gen_ai.*` keys** your instrumentation already set.
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

## Status

**Pre-release.** The attribute names are *reserved* in the
Trustabl attribute spec, meaning nothing else may claim them, and they become a
spec addition when this ships.

| Binding | State |
|---|---|
| Python | implemented, 86 tests |
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
