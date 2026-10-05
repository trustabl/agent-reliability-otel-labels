package otellabels

// Every Python test has a Go counterpart named by converting its snake_case
// name to Go style, so a port cannot quietly skip a guarantee:
// test_emits_one_event_per_policy -> TestEmitsOneEventPerPolicy.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func goTestName(python string) string {
	var b strings.Builder
	b.WriteString("Test")
	for _, part := range strings.Split(strings.TrimPrefix(python, "test_"), "_") {
		if part != "" {
			b.WriteString(strings.ToUpper(part[:1]) + part[1:])
		}
	}
	return b.String()
}

func namesIn(t *testing.T, glob string, pattern *regexp.Regexp) map[string]bool {
	t.Helper()
	files, err := filepath.Glob(glob)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range pattern.FindAllStringSubmatch(string(data), -1) {
			names[m[1]] = true
		}
	}
	return names
}

func TestEveryPythonTestHasAGoCounterpart(t *testing.T) {
	python := namesIn(t, "../python/tests/*.py", regexp.MustCompile(`(?m)^\s*def (test_\w+)\(`))
	golang := namesIn(t, "*_test.go", regexp.MustCompile(`(?m)^func (Test\w+)\(t \*testing\.T\)`))
	if len(python) == 0 {
		t.Fatal("found no Python tests; is ../python/tests present?")
	}
	for name := range python {
		if !golang[goTestName(name)] {
			t.Errorf("%s has no Go counterpart %s", name, goTestName(name))
		}
	}
}
