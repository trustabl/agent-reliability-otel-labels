package otellabels

// One Go test for the Python test of this name in test_tool_span.py,
// test_run.py and test_authority.py: Go test names are package-wide, so each
// Python module is a subtest.

import (
	"strings"
	"testing"
)

func TestNoActiveSpanIsASilentNoOp(t *testing.T) {
	binding := Binding{ID: "openshell-seccomp-v3", Type: Sandbox, SHA256: strings.Repeat("a", 64), Version: "3.1.0", Required: true, Source: Entrypoint}
	t.Run("tool_span", func(t *testing.T) {
		if err := MarkToolSpan(nil, ToolCall{Args: map[string]any{"a": 1}, Result: map[string]any{}, Attempt: 1, SideEffect: Read}); err != nil {
			t.Error(err)
		}
	})
	t.Run("run", func(t *testing.T) {
		NewRun("").MarkStart(nil)
		if err := MarkRunEnd(nil, FinalAnswer); err != nil {
			t.Error(err)
		}
		MarkHandoff(nil, "ho-1")
	})
	t.Run("authority", func(t *testing.T) {
		if err := BindAuthority(nil, []Binding{binding}); err != nil {
			t.Error(err)
		}
	})
}
