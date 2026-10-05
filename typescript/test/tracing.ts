import type { Span, Tracer } from "@opentelemetry/api";
import {
  BasicTracerProvider,
  InMemorySpanExporter,
  type ReadableSpan,
  SimpleSpanProcessor,
} from "@opentelemetry/sdk-trace-base";
import { expect } from "vitest";

/**
 * A tracer plus the spans it finished. Deliberately not the global provider:
 * these spans stand in for ones a third-party instrumentor created.
 */
export function tracing(): { tracer: Tracer; exporter: InMemorySpanExporter } {
  const exporter = new InMemorySpanExporter();
  const provider = new BasicTracerProvider({
    spanProcessors: [new SimpleSpanProcessor(exporter)],
  });
  return { tracer: provider.getTracer("test-instrumentor"), exporter };
}

/** Python's `with tracer.start_as_current_span(name) as span:`. */
export function withSpan(tracer: Tracer, name: string, fn: (span: Span) => void): void {
  const span = tracer.startSpan(name);
  try {
    fn(span);
  } finally {
    span.end();
  }
}

/** The one span the test finished. */
export function finished(exporter: InMemorySpanExporter): ReadableSpan {
  const spans = exporter.getFinishedSpans();
  expect(spans).toHaveLength(1);
  return spans[0];
}
