package otellabels

// The spec's acceptance criteria, as tests. Mirrors
// python/tests/test_acceptance.py.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/trace"
)

func TestNoActiveSpanCreatesNoSpan(t *testing.T) {
	// Not merely "does not fail": nothing may reach the exporter either.
	_, exporter := tracing(t)
	_ = MarkToolSpan(nil, ToolCall{Args: map[string]any{"a": 1}, Result: map[string]any{}, Attempt: 1, SideEffect: Read})
	_ = BindAuthority(nil, nil)
	NewRun("").MarkStart(nil)
	if n := len(exporter.GetSpans()); n != 0 {
		t.Errorf("%d spans exported, want 0", n)
	}
}

func TestThePublicApiOffersNoWayToFingerprintACompletion(t *testing.T) {
	// Chat is not supposed to repeat, so a differing completion is not a
	// finding. This fails the moment somebody adds MarkLLMSpan.
	forbidden := []string{"MarkLLMSpan", "MarkLlmSpan", "MarkChatSpan", "MarkCompletion", "FingerprintCompletion", "MarkModelSpan"}
	files, _ := filepath.Glob("*.go")
	fset := token.NewFileSet()
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range f.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && slices.Contains(forbidden, fn.Name.Name) {
				t.Errorf("%s exports %s", name, fn.Name.Name)
			}
		}
	}
}

func TestAChildSpanDoesNotInheritTheParentsBindings(t *testing.T) {
	// Copying the parent's hashes down would report the child as protected when it was not.
	tracer, exporter := tracing(t)
	withSpan(tracer, "invoke_agent", func(parent trace.Span) {
		_ = BindAuthority(parent, []Binding{openshell})
		withSpan(tracer, "invoke_agent", func(child trace.Span) { _ = BindAuthority(child, nil) })
	})
	var counts []int
	for _, s := range exporter.GetSpans() {
		counts = append(counts, len(s.Events))
		if len(s.Events) == 0 && strings.Contains(everythingWritten(s), openshell.SHA256) {
			t.Error("the parent's hash reached the child span")
		}
	}
	slices.Sort(counts)
	if !slices.Equal(counts, []int{0, 1}) {
		t.Errorf("events per span = %v, want [0 1]: the child must carry no binding events", counts)
	}
}
