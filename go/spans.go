package otellabels

// Decorating tool spans.
//
// Nothing here starts a span. The caller owns the span's lifecycle; we attach
// attributes to one that some other instrumentation library already created.

import (
	"fmt"
	"slices"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// unusable reports whether there is nothing safe to write to.
//
// Checked BEFORE argument validation on purpose: a workload running with
// tracing disabled must never be broken by this library, and a bad enum
// value surfaces the moment anyone runs with tracing on.
func unusable(span trace.Span) (u bool) {
	defer func() {
		if recover() != nil {
			u = true
		}
	}()
	return span == nil || !span.IsRecording()
}

// safely runs a write to the span; a panicking span implementation is
// absorbed rather than passed to the workload.
func safely(write func()) {
	defer func() { _ = recover() }()
	write()
}

// existing returns the attribute already on the span. readable is false when
// the span does not expose its attributes: the API's trace.Span does not, the
// SDK's span does. An unknown value is never overwritten.
func existing(span trace.Span, key attribute.Key) (value attribute.Value, found, readable bool) {
	defer func() {
		if recover() != nil {
			value, found, readable = attribute.Value{}, false, false
		}
	}()
	ro, ok := span.(interface{ Attributes() []attribute.KeyValue })
	if !ok {
		return attribute.Value{}, false, false
	}
	for _, kv := range ro.Attributes() {
		if kv.Key == key {
			return kv.Value, true, true
		}
	}
	return attribute.Value{}, false, true
}

// checkEnum returns an error unless value is one of allowed. Python raises
// ValueError and TypeScript throws RangeError for the same mistake.
func checkEnum[T ~string](label string, value T, allowed []T) error {
	if slices.Contains(allowed, value) {
		return nil
	}
	names := make([]string, len(allowed))
	for i, a := range allowed {
		names[i] = string(a)
	}
	slices.Sort(names)
	return fmt.Errorf("otellabels: %s must be one of %q, got %q", label, names, string(value))
}

// ToolCall is one tool execution to label.
type ToolCall struct {
	// Args and Result are fingerprinted, never stored. Structs are refused:
	// marshal them with encoding/json and pass json.RawMessage.
	Args, Result any
	Attempt      int
	// SideEffect "" means unknown: the key is then omitted, which says so.
	SideEffect SideEffect
	// ErrorClass "" means no error was observed: the key is omitted.
	ErrorClass ErrorClass
	// StepIndex nil means omitted; it is a pointer because 0 is a real step.
	StepIndex *int
	// RunID "" means omitted.
	RunID string
	// Name fills gen_ai.tool.name only when the span's attributes are
	// readable and the instrumentor left it empty.
	Name string
}

// fingerprint returns the fingerprint of v, or false when v has none. Any
// failure omits the key; nothing reaches the workload.
func fingerprint(v any) (fp string, ok bool) {
	defer func() {
		if recover() != nil {
			fp, ok = "", false
		}
	}()
	fp, err := CanonicalFP(v)
	return fp, err == nil
}

// MarkToolSpan attaches the process labels to one tool-execution span.
//
// It returns an error only for an invalid SideEffect or ErrorClass, and then
// writes nothing. A nil or non-recording span, or an Args or Result with no
// canonical form, returns nil: the affected fingerprint is omitted and every
// other attribute is still written.
func MarkToolSpan(span trace.Span, c ToolCall) error {
	if unusable(span) {
		return nil
	}
	if c.SideEffect != "" {
		if err := checkEnum("side effect", c.SideEffect, sideEffects); err != nil {
			return err
		}
	}
	if c.ErrorClass != "" {
		if err := checkEnum("error class", c.ErrorClass, errorClasses); err != nil {
			return err
		}
	}

	attrs := make([]attribute.KeyValue, 0, 8)
	if fp, ok := fingerprint(c.Args); ok {
		attrs = append(attrs, attribute.String(KeyToolInputFP, fp))
	}
	if fp, ok := fingerprint(c.Result); ok {
		attrs = append(attrs, attribute.String(KeyToolOutputFP, fp))
	}
	attrs = append(attrs, attribute.Int64(KeyToolAttempt, int64(c.Attempt)))
	if c.SideEffect != "" {
		attrs = append(attrs, attribute.String(KeyToolSideEffect, string(c.SideEffect)))
	}
	if c.ErrorClass != "" {
		attrs = append(attrs, attribute.String(KeyToolErrorClass, string(c.ErrorClass)))
	}
	if c.StepIndex != nil {
		attrs = append(attrs, attribute.Int64(KeyStepIndex, int64(*c.StepIndex)))
	}
	if c.RunID != "" {
		attrs = append(attrs, attribute.String(KeyRunID, c.RunID))
	}
	safely(func() { span.SetAttributes(attrs...) })

	// Their key, their value. We fill it only when the instrumentor left it
	// empty, and never when we cannot tell.
	if c.Name != "" {
		if _, found, readable := existing(span, "gen_ai.tool.name"); readable && !found {
			safely(func() { span.SetAttributes(attribute.String("gen_ai.tool.name", c.Name)) })
		}
	}
	return nil
}
