# agent-reliability-otel-labels (Python)

Process-metadata labels for OpenTelemetry agent spans.

This library adds about nine attributes and one event type to spans that your
agent **already emits**. It answers questions a trace cannot otherwise answer:
was this the same call as before, was this a retry, does this tool destroy
something, and which signed policies were actually in force.

No API key, no account, no Trustabl service.

## The one prerequisite

**Your agent must already be instrumented.** This library decorates spans; it
never creates them. LangChain's instrumentation, OpenLIT, Traceloop,
OpenInference or a framework's native `gen_ai` support all qualify.

## Install

```bash
pip install agent-reliability-otel-labels
```

Python 3.9 or newer. It depends on `opentelemetry-api` and `rfc8785`, not on
the OpenTelemetry SDK.

## Use

```python
from agent_reliability_otel_labels import Binding, Run, bind_authority, mark_run_end, mark_tool_span

run = Run()

with tracer.start_as_current_span("invoke_agent") as root:
    run.mark_start(root)
    bind_authority(root, bindings=[
        Binding(id="openshell-seccomp-v3", type="sandbox", sha256=...,
                version="3.1.0", required=True, source="entrypoint"),
    ])

    with tracer.start_as_current_span("execute_tool") as span:
        result = search_flights(args)
        mark_tool_span(span, args=args, result=result, attempt=1,
                       side_effect="read", step_index=run.next_step(),
                       run_id=run.run_id)

    # Call before the root span ends; an ended span drops the write.
    mark_run_end(root, exit_reason="final_answer")
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

The full documentation, the TypeScript and Go bindings, and a runnable example
(`python/examples/agent.py`) are in the repository:
https://github.com/trustabl/agent-reliability-otel-labels

## License

Apache-2.0. See [`LICENSE`](https://github.com/trustabl/agent-reliability-otel-labels/blob/main/LICENSE).
