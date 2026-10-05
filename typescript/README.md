# @trustabl/agent-reliability-otel-labels

Process-metadata labels for OpenTelemetry agent spans.

This library adds about nine attributes and one event type to spans that your
agent **already emits**. It answers questions a trace cannot otherwise answer:
was this the same call as before, was this a retry, does this tool destroy
something, and which signed policies were actually in force.

No API key, no account, no Trustabl service.

## The one prerequisite

**Your agent must already be instrumented.** This library decorates spans; it
never creates them. The Vercel AI SDK's telemetry, OpenLIT, Traceloop,
OpenInference or a framework's native `gen_ai` support all qualify.

## Install

```bash
npm install @trustabl/agent-reliability-otel-labels @opentelemetry/api
```

Node 18 or newer. Ships ESM and CJS with type definitions. `@opentelemetry/api`
is a peer dependency; the OpenTelemetry SDK is not required.

## Use

```typescript
import { Run, bindAuthority, markRunEnd, markToolSpan } from "@trustabl/agent-reliability-otel-labels";

const run = new Run();

await tracer.startActiveSpan("invoke_agent", async (root) => {
  run.markStart(root);
  bindAuthority(root, [
    { id: "openshell-seccomp-v3", type: "sandbox", sha256: "...",
      version: "3.1.0", required: true, source: "entrypoint" },
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

Arguments and results are fingerprinted, never stored: the span carries a
16-character hash, not what the caller asked for. No usable span is a silent
no-op, so a workload with tracing disabled is unaffected.

## What it writes

| Attribute | On | Answers |
|---|---|---|
| `trustabl.step_index` | tool calls | What order did this really happen in? |
| `trustabl.run_id` | root and tool calls | Which spans belong to one job? |
| `trustabl.tool.input_fp` | tool calls | Was this the same call as before? |
| `trustabl.tool.output_fp` | tool calls | Did the same input give the same result? |
| `trustabl.tool.side_effect` | tool calls | `pure`, `read`, `write` or `irreversible`? Omitted when unknown. |
| `trustabl.tool.error_class` | tool calls | Did it actually work? |
| `trustabl.tool.attempt` | tool calls | Is it retrying? |
| `trustabl.handoff_id` | child agent span | Which agent delegated to which? |
| `trustabl.exit_reason` | root, before it ends | How did the run end? |
| `trustabl.policy.binding` (event) | agent / sandbox root | Which signed policies were loaded? |

## More

The full documentation, the Python and Go bindings, and a runnable example on
the Vercel AI SDK (`typescript/examples/vercel-ai/`) are in the repository:
https://github.com/trustabl/agent-reliability-otel-labels

## License

Apache-2.0. See [`LICENSE`](https://github.com/trustabl/agent-reliability-otel-labels/blob/main/LICENSE).
