// A live run of @trustabl/agent-reliability-otel-labels against a real Vercel AI SDK agent.
//
//   (cd ../.. && pnpm build) && pnpm install && pnpm start
//
// The agent loop, tool execution and OpenTelemetry spans are the SDK's own
// (ai + @ai-sdk/otel). Only the model is scripted - MockLanguageModelV3 from
// `ai/test` replies with tool calls and then an answer - so the run is
// deterministic and costs nothing. While the model makes one tool call per
// turn, as here, the spans are the ones a live model would produce; only the
// model's words are fixed. When a turn makes several calls, the SDK runs them
// concurrently: each still gets its own execute_tool span, but step_index
// follows the order the calls reach the tool.
//
// It mirrors python/examples/agent.py: three identical flight searches that
// come back empty, then an answer anyway. `attempt` counts calls with the same
// arguments fingerprint within one run, so the three searches are attempts 1,
// 2 and 3, and a search with different arguments would start again at 1. It
// prints every span, then checks that the labels landed where the porting
// guide says they must, and exits 1 if any check fails.

import { context, trace } from "@opentelemetry/api";
import { AsyncLocalStorageContextManager } from "@opentelemetry/context-async-hooks";
import {
  BasicTracerProvider,
  InMemorySpanExporter,
  SimpleSpanProcessor,
} from "@opentelemetry/sdk-trace-base";
import { OTLPTraceExporter } from "@opentelemetry/exporter-trace-otlp-http";
import { resourceFromAttributes } from "@opentelemetry/resources";
import { OpenTelemetry } from "@ai-sdk/otel";
import { generateText, registerTelemetry, stepCountIs, tool } from "ai";
import { MockLanguageModelV3 } from "ai/test";
import { z } from "zod";

import { Run, bindAuthority, canonicalFp, markRunEnd, markToolSpan } from "@trustabl/agent-reliability-otel-labels";

// Without a context manager there is no active span inside a tool, and
// trace.getActiveSpan() returns undefined: every label becomes a silent no-op.
context.setGlobalContextManager(new AsyncLocalStorageContextManager().enable());

const exporter = new InMemorySpanExporter();
const processors = [new SimpleSpanProcessor(exporter)];
// With OTEL_EXPORTER_OTLP_ENDPOINT set, the same spans also go to a trace
// viewer; the exporter reads the endpoint from the environment.
const OTLP_ENDPOINT = process.env.OTEL_EXPORTER_OTLP_ENDPOINT;
if (OTLP_ENDPOINT) processors.push(new SimpleSpanProcessor(new OTLPTraceExporter()));
const provider = new BasicTracerProvider({
  resource: resourceFromAttributes({ "service.name": process.env.OTEL_SERVICE_NAME ?? "travel-agent-typescript" }),
  spanProcessors: processors,
});
const tracer = provider.getTracer("travel-agent");
registerTelemetry(new OpenTelemetry({ tracer }));

// The policies this agent was started under. Two rails, two hashes.
const BINDINGS = [
  { id: "openshell-seccomp-v3", type: "sandbox", sha256: "a".repeat(64), version: "3.1.0", required: true, source: "entrypoint" },
  { id: "acs-content-v8", type: "content_safety", sha256: "b".repeat(64), version: "8.0.2", required: true, source: "entrypoint" },
];

const ARGS = { from: "SFO", to: "JFK", date: "2026-10-01" };
const MAX_STEPS = 5;

// Three turns asking for the same search, then an answer anyway.
let turn = 0;
const model = new MockLanguageModelV3({
  doGenerate: async () => {
    turn++;
    const usage = { inputTokens: 1, outputTokens: 1, totalTokens: 2 };
    if (turn <= 3) {
      return {
        finishReason: { unified: "tool-calls" },
        usage,
        content: [{ type: "tool-call", toolCallId: `c${turn}`, toolName: "search_flights", input: JSON.stringify(ARGS) }],
        warnings: [],
      };
    }
    return { finishReason: { unified: "stop" }, usage, content: [{ type: "text", text: "No flights found." }], warnings: [] };
  },
});

// The flight backend. It finds nothing.
async function findFlights(_args) {
  return { flights: [], count: 0 };
}

// Everything that belongs to one run - the run id, the step counter, the
// attempt counts and the tool that closes over them - is built here, once per
// run, so two runs in one process never share counters.
function buildRun() {
  const run = new Run();
  // Calls per arguments fingerprint: an identical repeat is a retry.
  const attempts = new Map();

  const searchFlights = tool({
    description: "Search flights between two airports on a date.",
    inputSchema: z.object({ from: z.string(), to: z.string(), date: z.string() }),
    execute: async (args) => {
      // `args` is what the tool receives, after zod has validated it.
      let key;
      try {
        key = canonicalFp(args);
      } catch {
        key = "search_flights (no fingerprint)";
      }
      const attempt = (attempts.get(key) ?? 0) + 1;
      attempts.set(key, attempt);
      const stepIndex = run.nextStep();
      // The SDK's execute_tool span is the active span while the tool runs.
      const span = trace.getActiveSpan();

      let result;
      try {
        result = await findFlights(args);
        return result;
      } finally {
        // Labelled whether the tool returned or threw. A throw leaves `result`
        // undefined, so output_fp is the fingerprint of null, and error_class
        // off: the failure class is unknown here.
        try {
          markToolSpan(span, {
            args,
            result,
            attempt,
            sideEffect: "read",
            errorClass: result?.count === 0 ? "empty" : undefined,
            stepIndex,
            runId: run.runId,
            name: "search_flights",
          });
        } catch (err) {
          // Only an invalid enum value gets here: a bug in this code, which
          // must not fail the tool.
          console.error(`trustabl: could not label the tool span: ${err}`);
        }
      }
    },
  });

  return { run, tools: { search_flights: searchFlights } };
}

// The SDK's own invoke_agent span is not reachable from here, so the run
// labels go on the application's root span. The application may create spans;
// the library never does.
async function runAgent(prompt) {
  const { run, tools } = buildRun();
  return tracer.startActiveSpan("travel-booker", async (root) => {
    root.setAttribute("gen_ai.operation.name", "invoke_agent");
    root.setAttribute("gen_ai.agent.name", "travel-booker");
    run.markStart(root);
    bindAuthority(root, BINDINGS);
    try {
      const result = await generateText({
        model,
        tools,
        stopWhen: stepCountIs(MAX_STEPS),
        prompt,
        telemetry: { isEnabled: true, functionId: "TravelBooker" },
      });
      // Still asking for tools when the step cap stopped it: no final answer.
      const capped = result.finishReason === "tool-calls" && result.steps.length >= MAX_STEPS;
      // Before the root span ends; an ended span drops the write.
      markRunEnd(root, capped ? "max_steps" : "final_answer");
      return { text: result.text, runId: run.runId };
    } catch (err) {
      markRunEnd(root, "error");
      throw err;
    } finally {
      root.end();
    }
  });
}

const { text, runId } = await runAgent("Find me a flight from SFO to JFK on 2026-10-01.");
console.log(`agent answered: ${text}\n`);

// ---- inspect -------------------------------------------------------------

const spans = exporter.getFinishedSpans();
for (const span of spans) {
  const ours = Object.entries(span.attributes).filter(([k]) => k.startsWith("trustabl."));
  console.log(span.name);
  for (const [k, v] of ours) console.log(`    ${k} = ${v}`);
  for (const e of span.events) console.log(`    event ${e.name} ${e.attributes["trustabl.policy.binding.id"]}`);
}

const failures = [];
const check = (ok, what) => {
  console.log(`${ok ? "ok  " : "FAIL"} ${what}`);
  if (!ok) failures.push(what);
};

console.log("");
const tools = spans.filter((s) => s.attributes["gen_ai.operation.name"] === "execute_tool");
const every = (pred) => tools.length > 0 && tools.every(pred);
const root = spans.find((s) => s.name === "travel-booker");
const expectedFp = canonicalFp(ARGS);
check(tools.length === 3, `three execute_tool spans from the SDK (got ${tools.length})`);
check(expectedFp === "c1baebf63d8d7587", "the arguments fingerprint as Python's and Go's do (c1baebf63d8d7587)");
check(every((s) => s.attributes["trustabl.tool.input_fp"] === expectedFp), "every tool span carries that input_fp: the repeat is visible");
check(every((s) => /^[0-9a-f]{16}$/.test(String(s.attributes["trustabl.tool.output_fp"]))), "every tool span carries a 16-hex output_fp");
check(tools.map((s) => s.attributes["trustabl.tool.attempt"]).join(",") === "1,2,3", "attempts are 1, 2, 3");
check(tools.map((s) => s.attributes["trustabl.step_index"]).join(",") === "0,1,2", "step indices are 0, 1, 2");
check(every((s) => s.attributes["trustabl.tool.side_effect"] === "read" && s.attributes["trustabl.tool.error_class"] === "empty"), "side_effect=read and error_class=empty on every tool span");
check(every((s) => s.attributes["gen_ai.tool.name"] === "search_flights"), "the SDK's gen_ai.tool.name is untouched");
check(every((s) => s.attributes["trustabl.run_id"] === runId), "every tool span carries the run id");
check(root?.attributes["trustabl.run_id"] === runId, "the root span carries the run id");
check(root?.attributes["trustabl.exit_reason"] === "final_answer", "the root span carries exit_reason=final_answer");
check(root?.events.filter((e) => e.name === "trustabl.policy.binding").length === 2, "the root span carries two policy binding events");
const others = spans.filter((s) => s !== root && !tools.includes(s));
check(others.every((s) => !Object.keys(s.attributes).some((k) => k.startsWith("trustabl."))), "no trustabl.* key on any other span (chat, step, the SDK's invoke_agent)");
const everything = JSON.stringify(spans.map((s) => Object.entries(s.attributes).filter(([k]) => k.startsWith("trustabl."))));
check(!everything.includes("SFO") && !everything.includes("JFK") && !everything.includes("2026-10-01"), "no argument text in any trustabl.* value");

await provider.shutdown(); // flush the OTLP export before exiting
if (OTLP_ENDPOINT) console.log(`\nsent ${spans.length} spans for run ${runId} to ${OTLP_ENDPOINT}`);

if (failures.length > 0) {
  console.error(`\n${failures.length} check(s) failed`);
  process.exit(1);
}
console.log("\nall checks passed");
