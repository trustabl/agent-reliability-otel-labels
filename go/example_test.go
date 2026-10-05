package otellabels_test

import (
	"context"
	"encoding/json"

	"go.opentelemetry.io/otel"

	otellabels "github.com/trustabl/agent-reliability-otel-labels/go"
)

type searchArgs struct {
	From string `json:"from"`
	To   string `json:"to"`
}

func searchFlights(searchArgs) map[string]any { return map[string]any{"flights": []any{}} }

// The README's usage block. It compiles as part of the test suite, so the
// documentation cannot drift from the API.
func Example() {
	tracer := otel.Tracer("my-agent")
	run := otellabels.NewRun("")

	ctx, root := tracer.Start(context.Background(), "invoke_agent")
	run.MarkStart(root)
	_ = otellabels.BindAuthority(root, []otellabels.Binding{
		{ID: "openshell-seccomp-v3", Type: otellabels.Sandbox, SHA256: "...",
			Version: "3.1.0", Required: true, Source: otellabels.Entrypoint},
	})

	_, span := tracer.Start(ctx, "execute_tool")
	args := searchArgs{From: "SFO", To: "JFK"}
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
}
