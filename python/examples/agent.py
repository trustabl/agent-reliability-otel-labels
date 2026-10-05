"""A worked example: what the labels look like on a real run.

Run it with the dev extras installed:

    python examples/agent.py

It prints the spans an exporter would receive. Nothing here talks to Trustabl,
and no Trustabl binary is running - that is the point.

To send the spans to a trace viewer instead, install the OTLP exporter and set
the standard OpenTelemetry variables:

    pip install opentelemetry-exporter-otlp-proto-http
    OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318 \
    OTEL_SERVICE_NAME=travel-agent-python python examples/agent.py
"""

import os

from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import (
    ConsoleSpanExporter,
    SimpleSpanProcessor,
)

from agent_reliability_otel_labels import Binding, Run, bind_authority, mark_run_end, mark_tool_span

# The resource picks up OTEL_SERVICE_NAME from the environment.
provider = TracerProvider()
if os.environ.get("OTEL_EXPORTER_OTLP_ENDPOINT"):
    # Imported only here: the exporter is an optional dependency.
    from opentelemetry.exporter.otlp.proto.http.trace_exporter import OTLPSpanExporter

    provider.add_span_processor(SimpleSpanProcessor(OTLPSpanExporter()))
else:
    provider.add_span_processor(SimpleSpanProcessor(ConsoleSpanExporter()))
tracer = provider.get_tracer("example-agent")

# The policies this agent was started under. Two rails, two hashes: either can
# go stale without the other, and one combined hash could not say so.
BINDINGS = [
    Binding(id="openshell-seccomp-v3", type="sandbox", sha256="a" * 64,
            version="3.1.0", required=True, source="entrypoint"),
    Binding(id="acs-content-v8", type="content_safety", sha256="b" * 64,
            version="8.0.2", required=True, source="entrypoint"),
]


def search_flights(args):
    return {"flights": [], "count": 0}


def main():
    run = Run()

    with tracer.start_as_current_span("invoke_agent travel-booker") as root:
        root.set_attribute("gen_ai.operation.name", "invoke_agent")
        root.set_attribute("gen_ai.agent.name", "travel-booker")
        run.mark_start(root)
        bind_authority(root, bindings=BINDINGS)

        # Three identical calls: same input_fp three times, which is what a
        # loop rule matches on. The arguments themselves never leave the process.
        args = {"from": "SFO", "to": "JFK", "date": "2026-10-01"}
        for attempt in (1, 2, 3):
            with tracer.start_as_current_span("execute_tool search_flights") as span:
                span.set_attribute("gen_ai.operation.name", "execute_tool")
                span.set_attribute("gen_ai.tool.name", "search_flights")
                result = search_flights(args)
                mark_tool_span(
                    span,
                    args={**args, "request_id": f"req-{attempt}"},  # stripped
                    result=result,
                    attempt=attempt,
                    side_effect="read",
                    error_class="empty" if result["count"] == 0 else None,
                    step_index=run.next_step(),
                    run_id=run.run_id,
                )

        # Answered anyway, after three empty searches. error_class=empty then
        # exit_reason=final_answer is the "made something up" pattern.
        mark_run_end(root, exit_reason="final_answer")

    provider.shutdown()  # flush before exit
    if os.environ.get("OTEL_EXPORTER_OTLP_ENDPOINT"):
        print(f"sent 4 spans for run {run.run_id} to {os.environ['OTEL_EXPORTER_OTLP_ENDPOINT']}")


if __name__ == "__main__":
    main()
