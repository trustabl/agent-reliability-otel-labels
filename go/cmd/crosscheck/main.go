// Command crosscheck is the Go leg of typescript/scripts/cross-check.ts.
//
//	go run ./cmd/crosscheck FIXTURES_PATH [TRACE_PATH]
//
// It extracts the same inputs, in the same order, as the Python and
// TypeScript legs, and prints a JSON array with one entry per input:
// {"canonical": ..., "fingerprint": ...}, or null when the input is refused.
// Inputs stay json.RawMessage, so Go parses each one itself.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	otellabels "github.com/trustabl/agent-reliability-otel-labels/go"
)

type result struct {
	Canonical   string `json:"canonical"`
	Fingerprint string `json:"fingerprint"`
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "crosscheck:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) < 1 || len(args) > 2 {
		return errors.New("usage: crosscheck FIXTURES_PATH [TRACE_PATH]")
	}
	inputs, err := fixtureInputs(args[0])
	if err != nil {
		return err
	}
	if len(args) == 2 && args[1] != "" {
		traced, err := traceInputs(args[1])
		if err != nil {
			return err
		}
		inputs = append(inputs, traced...)
	}
	out := make([]*result, len(inputs))
	for i, input := range inputs {
		canonical, err := otellabels.CanonicalJSON(input)
		if errors.Is(err, otellabels.ErrNotCanonical) {
			continue
		}
		if err != nil {
			return err
		}
		fp, err := otellabels.CanonicalFP(input)
		if err != nil {
			return err
		}
		out[i] = &result{Canonical: canonical, Fingerprint: fp}
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	return enc.Encode(out)
}

func fixtureInputs(path string) ([]json.RawMessage, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var f struct {
		Cases []struct {
			Input json.RawMessage `json:"input"`
		} `json:"cases"`
		Rejected struct {
			Cases []struct {
				Input json.RawMessage `json:"input"`
			} `json:"cases"`
		} `json:"rejected"`
	}
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, err
	}
	var inputs []json.RawMessage
	for _, c := range f.Cases {
		inputs = append(inputs, c.Input)
	}
	for _, c := range f.Rejected.Cases {
		inputs = append(inputs, c.Input)
	}
	return inputs, nil
}

// traceInputs takes gen_ai.tool.call.arguments then gen_ai.tool.call.result
// from each span, in file order, as the other legs do. The attribute values
// are JSON text.
func traceInputs(path string) ([]json.RawMessage, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var capture struct {
		Spans []struct {
			Attributes map[string]json.RawMessage `json:"attributes"`
		} `json:"spans"`
	}
	if err := json.Unmarshal(data, &capture); err != nil {
		return nil, err
	}
	var inputs []json.RawMessage
	for _, span := range capture.Spans {
		for _, key := range []string{"gen_ai.tool.call.arguments", "gen_ai.tool.call.result"} {
			value, ok := span.Attributes[key]
			if !ok {
				continue
			}
			var text string
			if err := json.Unmarshal(value, &text); err != nil {
				return nil, fmt.Errorf("%s is not a string: %w", key, err)
			}
			inputs = append(inputs, json.RawMessage(text))
		}
	}
	return inputs, nil
}
