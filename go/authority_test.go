package otellabels

// Policy bindings: one event per policy. A single flattened hash is the bug
// this shape exists to prevent. Mirrors python/tests/test_authority.py.
// TestNoActiveSpanIsASilentNoOp lives in noop_test.go.

import (
	"strings"
	"testing"

	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

var (
	openshell = Binding{ID: "openshell-seccomp-v3", Type: Sandbox, SHA256: strings.Repeat("a", 64), Version: "3.1.0", Required: true, Source: Entrypoint}
	acs       = Binding{ID: "acs-content-v8", Type: ContentSafety, SHA256: strings.Repeat("b", 64), Version: "8.0.2", Required: true, Source: Entrypoint}
)

func bindingEvents(t *testing.T, exporter *tracetest.InMemoryExporter) []map[string]string {
	t.Helper()
	var out []map[string]string
	for _, e := range finished(t, exporter).Events {
		if e.Name != EventPolicyBinding {
			t.Errorf("unexpected event %q", e.Name)
		}
		m := map[string]string{}
		for key, value := range attrs(e.Attributes) {
			m[key] = value.Emit()
		}
		out = append(out, m)
	}
	return out
}

func TestEmitsOneEventPerPolicy(t *testing.T) {
	tracer, exporter := tracing(t)
	withSpan(tracer, "invoke_agent", func(span trace.Span) { _ = BindAuthority(span, []Binding{openshell, acs}) })
	if n := len(bindingEvents(t, exporter)); n != 2 {
		t.Errorf("%d events, want 2", n)
	}
}

func TestEachEventCarriesItsOwnHash(t *testing.T) {
	// Two rails, two hashes, independently stale-able. Never one combined.
	tracer, exporter := tracing(t)
	withSpan(tracer, "invoke_agent", func(span trace.Span) { _ = BindAuthority(span, []Binding{openshell, acs}) })
	hashes := map[string]bool{}
	for _, e := range bindingEvents(t, exporter) {
		hashes[e[KeyBindingSHA256]] = true
	}
	if !hashes[strings.Repeat("a", 64)] || !hashes[strings.Repeat("b", 64)] || len(hashes) != 2 {
		t.Errorf("hashes = %v", hashes)
	}
}

func TestEventCarriesTheFullBindingRecord(t *testing.T) {
	tracer, exporter := tracing(t)
	withSpan(tracer, "invoke_agent", func(span trace.Span) { _ = BindAuthority(span, []Binding{openshell}) })
	e := bindingEvents(t, exporter)[0]
	want := map[string]string{
		KeyBindingID: "openshell-seccomp-v3", KeyBindingType: "sandbox", KeyBindingVersion: "3.1.0",
		KeyBindingRequired: "true", KeyBindingSource: "entrypoint",
	}
	for key, value := range want {
		if e[key] != value {
			t.Errorf("%s = %q, want %q", key, e[key], value)
		}
	}
}

func TestASubagentWithNoBindingsEmitsNoEvents(t *testing.T) {
	// A handoff with no bindings is a coverage hole; emitting nothing makes it visible.
	tracer, exporter := tracing(t)
	withSpan(tracer, "invoke_agent", func(span trace.Span) { _ = BindAuthority(span, nil) })
	if n := len(bindingEvents(t, exporter)); n != 0 {
		t.Errorf("%d events, want 0", n)
	}
}

func TestRejectsAnUnknownBindingType(t *testing.T) {
	tracer, _ := tracing(t)
	withSpan(tracer, "invoke_agent", func(span trace.Span) {
		bad := openshell
		bad.ID, bad.Type = "x", "firewall"
		if err := BindAuthority(span, []Binding{bad}); err == nil {
			t.Error("binding type \"firewall\" was accepted")
		}
	})
}

func TestRejectsAnUnknownBindingSource(t *testing.T) {
	tracer, _ := tracing(t)
	withSpan(tracer, "invoke_agent", func(span trace.Span) {
		bad := openshell
		bad.ID, bad.Source = "x", "guessed"
		if err := BindAuthority(span, []Binding{bad}); err == nil {
			t.Error("binding source \"guessed\" was accepted")
		}
	})
}

func TestValidatesEveryBindingBeforeEmittingAnyEvent(t *testing.T) {
	tracer, exporter := tracing(t)
	withSpan(tracer, "invoke_agent", func(span trace.Span) {
		bad := acs
		bad.Source = "guessed"
		if err := BindAuthority(span, []Binding{openshell, bad}); err == nil {
			t.Error("invalid binding accepted")
		}
	})
	if n := len(bindingEvents(t, exporter)); n != 0 {
		t.Errorf("%d events emitted before validation failed, want 0", n)
	}
}
