// Command adk is a live run of otellabels against a real Google ADK for Go
// agent.
//
//	cd go/examples/adk && go run .
//
// The agent loop, tool execution and OpenTelemetry spans are ADK's own. Only
// the model is scripted - a model.LLM that replies with function calls and
// then an answer - so the run is deterministic and costs nothing. While the
// model makes one tool call per turn, as here, the spans are the ones a live
// model would produce; only the model's words are fixed. When a turn makes
// several calls, ADK runs them in parallel goroutines and wraps their
// execute_tool spans in one "execute_tool (merged)" span, which carries no
// labels.
//
// It mirrors python/examples/agent.py: three identical flight searches that
// come back empty, then an answer anyway. Attempt counts calls with the same
// arguments fingerprint within one run, so the three searches are attempts 1,
// 2 and 3, and a search with different arguments would start again at 1. It
// prints every span, then checks that the labels landed where the porting
// guide says they must, and exits 1 if any check fails.
//
// This is its own module so ADK's dependencies never reach the binding's
// go.mod.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"os"
	"regexp"
	"strings"
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/model"
	"google.golang.org/adk/runner"
	"google.golang.org/adk/session"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
	"google.golang.org/genai"

	otellabels "github.com/trustabl/agent-reliability-otel-labels/go"
)

type searchArgs struct {
	From string `json:"from"`
	To   string `json:"to"`
	Date string `json:"date"`
}

type searchResult struct {
	Flights []string `json:"flights"`
	Count   int      `json:"count"`
}

var args = searchArgs{From: "SFO", To: "JFK", Date: "2026-10-01"}

// The policies this agent was started under. Two rails, two hashes.
var bindings = []otellabels.Binding{
	{ID: "openshell-seccomp-v3", Type: otellabels.Sandbox, SHA256: strings.Repeat("a", 64),
		Version: "3.1.0", Required: true, Source: otellabels.Entrypoint},
	{ID: "acs-content-v8", Type: otellabels.ContentSafety, SHA256: strings.Repeat("b", 64),
		Version: "8.0.2", Required: true, Source: otellabels.Entrypoint},
}

// scriptedModel asks for the same search three times, then answers anyway.
type scriptedModel struct{ turn int }

func (m *scriptedModel) Name() string { return "scripted-model" }

func (m *scriptedModel) GenerateContent(_ context.Context, _ *model.LLMRequest, _ bool) iter.Seq2[*model.LLMResponse, error] {
	return func(yield func(*model.LLMResponse, error) bool) {
		m.turn++
		part := genai.NewPartFromText("No flights found.")
		if m.turn <= 3 {
			part = &genai.Part{FunctionCall: &genai.FunctionCall{
				ID: fmt.Sprintf("c%d", m.turn), Name: "search_flights",
				Args: map[string]any{"from": args.From, "to": args.To, "date": args.Date},
			}}
		}
		yield(&model.LLMResponse{Content: genai.NewContentFromParts([]*genai.Part{part}, genai.RoleModel), TurnComplete: true}, nil)
	}
}

// findFlights is the flight backend. It finds nothing.
func findFlights(_ searchArgs) (searchResult, error) {
	return searchResult{Flights: []string{}, Count: 0}, nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	ctx := context.Background()

	// ADK starts its spans on the global tracer provider.
	exporter := tracetest.NewInMemoryExporter()
	options := []sdktrace.TracerProviderOption{
		sdktrace.WithSyncer(exporter),
		// Reads OTEL_SERVICE_NAME from the environment.
		sdktrace.WithResource(resource.Default()),
	}
	// With OTEL_EXPORTER_OTLP_ENDPOINT set, the same spans also go to a trace
	// viewer; the exporter reads the endpoint from the environment.
	otlpEndpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if otlpEndpoint != "" {
		otlp, err := otlptracehttp.New(ctx)
		if err != nil {
			return err
		}
		options = append(options, sdktrace.WithSyncer(otlp))
	}
	provider := sdktrace.NewTracerProvider(options...)
	otel.SetTracerProvider(provider)
	defer func() { _ = provider.Shutdown(ctx) }()

	// The run and the attempt counts are per invocation. This example runs
	// the agent once, so it creates them next to the agent. A server that
	// builds the agent once and runs it many times must create them per run
	// instead (for example in session state), or runs share counters.
	run := otellabels.NewRun("")
	// Calls per arguments fingerprint: an identical repeat is a retry. ADK
	// runs a turn's tool calls in parallel goroutines, so the mutex guards
	// the map; NextStep is already safe for concurrent use.
	var mu sync.Mutex
	attempts := map[string]int{}

	searchFlights, err := functiontool.New(functiontool.Config{
		Name:        "search_flights",
		Description: "Search flights between two airports on a date.",
	}, func(tc agent.ToolContext, a searchArgs) (result searchResult, err error) {
		// Structs are refused: pass the JSON the tool received. `a` is the
		// typed struct, so extra fields are already dropped and missing ones
		// zero-filled.
		rawArgs, _ := json.Marshal(a)
		key, fpErr := otellabels.CanonicalFP(json.RawMessage(rawArgs))
		if fpErr != nil {
			key = "search_flights (no fingerprint)"
		}
		mu.Lock()
		attempts[key]++
		attempt := attempts[key]
		mu.Unlock()
		step := run.NextStep()
		// ADK's execute_tool span is the span in the tool's context.
		span := trace.SpanFromContext(tc)

		// Labelled whether the tool returned or failed. On failure no result
		// is passed, so output_fp is the fingerprint of null, and error_class
		// is left off: the failure class is unknown here.
		defer func() {
			call := otellabels.ToolCall{
				Args: json.RawMessage(rawArgs), Attempt: attempt,
				SideEffect: otellabels.Read, StepIndex: &step,
				RunID: run.RunID(), Name: "search_flights",
			}
			if err == nil {
				rawResult, _ := json.Marshal(result)
				call.Result = json.RawMessage(rawResult)
				if result.Count == 0 {
					call.ErrorClass = otellabels.Empty
				}
			}
			// Only an invalid enum value gets here: a bug in this code, which
			// must not become the tool's error.
			if mErr := otellabels.MarkToolSpan(span, call); mErr != nil {
				fmt.Fprintln(os.Stderr, "trustabl: could not label the tool span:", mErr)
			}
		}()
		return findFlights(a)
	})
	if err != nil {
		return err
	}

	// The before and after agent callbacks run inside ADK's invoke_agent
	// span, so the run labels go on the framework's own agent span.
	travelBooker, err := llmagent.New(llmagent.Config{
		Name:        "travel_booker",
		Description: "Books flights.",
		Model:       &scriptedModel{},
		Tools:       []tool.Tool{searchFlights},
		BeforeAgentCallbacks: []agent.BeforeAgentCallback{func(cc agent.CallbackContext) (*genai.Content, error) {
			root := trace.SpanFromContext(cc)
			run.MarkStart(root)
			return nil, otellabels.BindAuthority(root, bindings)
		}},
		AfterAgentCallbacks: []agent.AfterAgentCallback{func(cc agent.CallbackContext) (*genai.Content, error) {
			// Before the span ends; an ended span drops the write. ADK skips
			// this callback when the consumer stops iterating the run's
			// events, as run() below does on the first error, so a failed run
			// carries no exit_reason. The callback cannot see the error: a
			// consumer that kept iterating past one would record final_answer
			// here, wrongly.
			return nil, otellabels.MarkRunEnd(trace.SpanFromContext(cc), otellabels.FinalAnswer)
		}},
	})
	if err != nil {
		return err
	}

	sessions := session.InMemoryService()
	created, err := sessions.Create(ctx, &session.CreateRequest{AppName: "travel", UserID: "user"})
	if err != nil {
		return err
	}
	r, err := runner.New(runner.Config{AppName: "travel", Agent: travelBooker, SessionService: sessions})
	if err != nil {
		return err
	}
	prompt := genai.NewContentFromText("Find me a flight from SFO to JFK on 2026-10-01.", genai.RoleUser)
	for event, err := range r.Run(ctx, "user", created.Session.ID(), prompt, agent.RunConfig{}) {
		if err != nil {
			return err
		}
		if event.Content != nil {
			for _, p := range event.Content.Parts {
				if p.Text != "" {
					fmt.Printf("agent answered: %s\n\n", p.Text)
				}
			}
		}
	}

	spans := exporter.GetSpans()
	checked := inspect(spans, run.RunID())
	if otlpEndpoint != "" {
		if err := provider.ForceFlush(ctx); err != nil {
			return err
		}
		fmt.Printf("\nsent %d spans for run %s to %s\n", len(spans), run.RunID(), otlpEndpoint)
	}
	return checked
}

// ---- inspect -------------------------------------------------------------

func attrMap(kvs []attribute.KeyValue) map[string]attribute.Value {
	out := make(map[string]attribute.Value, len(kvs))
	for _, kv := range kvs {
		out[string(kv.Key)] = kv.Value
	}
	return out
}

func inspect(spans tracetest.SpanStubs, runID string) error {
	for _, s := range spans {
		fmt.Println(s.Name)
		for _, kv := range s.Attributes {
			if strings.HasPrefix(string(kv.Key), "trustabl.") {
				fmt.Printf("    %s = %s\n", kv.Key, kv.Value.Emit())
			}
		}
		for _, e := range s.Events {
			fmt.Printf("    event %s %s\n", e.Name, attrMap(e.Attributes)[otellabels.KeyBindingID].Emit())
		}
	}

	failures := 0
	check := func(ok bool, what string) {
		mark := "ok  "
		if !ok {
			mark, failures = "FAIL", failures+1
		}
		fmt.Printf("%s %s\n", mark, what)
	}
	fmt.Println()

	var tools []map[string]attribute.Value
	var root map[string]attribute.Value
	var rootEvents []sdktrace.Event
	others := 0
	for _, s := range spans {
		a := attrMap(s.Attributes)
		switch {
		// ADK's "execute_tool (merged)" span, around a turn's several calls,
		// shares the operation name but is not a tool call.
		case a["gen_ai.operation.name"].AsString() == "execute_tool" && s.Name != "execute_tool (merged)":
			tools = append(tools, a)
		// One agent here; with sub-agents, match the root by name or parent.
		case strings.HasPrefix(s.Name, "invoke_agent"):
			root, rootEvents = a, s.Events
		default:
			for k := range a {
				if strings.HasPrefix(k, "trustabl.") {
					others++
				}
			}
		}
	}
	rawArgs, _ := json.Marshal(args)
	expectedFP, _ := otellabels.CanonicalFP(json.RawMessage(rawArgs))
	hex16 := regexp.MustCompile(`^[0-9a-f]{16}$`)
	every := func(pred func(map[string]attribute.Value) bool) bool {
		for _, t := range tools {
			if !pred(t) {
				return false
			}
		}
		return len(tools) > 0
	}
	column := func(key string) string {
		var vals []string
		for _, t := range tools {
			vals = append(vals, t[key].Emit())
		}
		return strings.Join(vals, ",")
	}

	check(len(tools) == 3, fmt.Sprintf("three execute_tool spans from ADK (got %d)", len(tools)))
	check(expectedFP == "c1baebf63d8d7587", "the arguments fingerprint as Python's and TypeScript's do (c1baebf63d8d7587)")
	check(every(func(t map[string]attribute.Value) bool {
		return t[otellabels.KeyToolInputFP].AsString() == expectedFP
	}), "every tool span carries that input_fp: the repeat is visible")
	check(every(func(t map[string]attribute.Value) bool {
		return hex16.MatchString(t[otellabels.KeyToolOutputFP].AsString())
	}), "every tool span carries a 16-hex output_fp")
	check(column(otellabels.KeyToolAttempt) == "1,2,3", "attempts are 1, 2, 3")
	check(column(otellabels.KeyStepIndex) == "0,1,2", "step indices are 0, 1, 2")
	check(every(func(t map[string]attribute.Value) bool {
		return t[otellabels.KeyToolSideEffect].AsString() == "read" && t[otellabels.KeyToolErrorClass].AsString() == "empty"
	}), "side_effect=read and error_class=empty on every tool span")
	check(every(func(t map[string]attribute.Value) bool { return t["gen_ai.tool.name"].AsString() == "search_flights" }), "ADK's gen_ai.tool.name is untouched")
	check(every(func(t map[string]attribute.Value) bool { return t[otellabels.KeyRunID].AsString() == runID }), "every tool span carries the run id")
	check(root != nil && root[otellabels.KeyRunID].AsString() == runID, "ADK's invoke_agent span carries the run id")
	check(root != nil && root[otellabels.KeyExitReason].AsString() == "final_answer", "ADK's invoke_agent span carries exit_reason=final_answer")
	bindingEvents := 0
	for _, e := range rootEvents {
		if e.Name == otellabels.EventPolicyBinding {
			bindingEvents++
		}
	}
	check(bindingEvents == 2, "ADK's invoke_agent span carries two policy binding events")
	check(others == 0, "no trustabl.* key on any other span (generate_content; also execute_tool (merged) when a turn makes several calls)")
	var ours []string
	for _, s := range spans {
		for _, kv := range s.Attributes {
			if strings.HasPrefix(string(kv.Key), "trustabl.") {
				ours = append(ours, kv.Value.Emit())
			}
		}
	}
	joined := strings.Join(ours, " ")
	check(!strings.Contains(joined, "SFO") && !strings.Contains(joined, "JFK") && !strings.Contains(joined, "2026-10-01"), "no argument text in any trustabl.* value")

	if failures > 0 {
		return fmt.Errorf("\n%d check(s) failed", failures)
	}
	fmt.Println("\nall checks passed")
	return nil
}
