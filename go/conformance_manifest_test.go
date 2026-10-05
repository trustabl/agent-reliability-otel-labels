package otellabels

// Every language binding must emit the same key names. spec/keys.json is the
// shared source of truth. Test names are the Python names converted to Go
// style, "Python" included, so one search finds both.

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
)

const manifestPath = "../spec/keys.json"

type manifest struct {
	ProfileVersion  string              `json:"profile_version"`
	Attributes      []string            `json:"attributes"`
	EventAttributes []string            `json:"event_attributes"`
	Events          map[string]string   `json:"events"`
	Enums           map[string][]string `json:"enums"`
}

func loadManifest(t *testing.T) manifest {
	t.Helper()
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var m manifest
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

// definedStrings returns every string constant declared in keys.go - Go's
// analogue of Python's vars(keys), read from source because Go cannot list a
// package's constants at run time.
func definedStrings(t *testing.T) []string {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), "keys.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	ast.Inspect(f, func(n ast.Node) bool {
		if lit, ok := n.(*ast.BasicLit); ok && lit.Kind == token.STRING {
			if s, err := strconv.Unquote(lit.Value); err == nil {
				out = append(out, s)
			}
		}
		return true
	})
	return out
}

func TestManifestExists(t *testing.T) {
	if _, err := os.Stat(manifestPath); err != nil {
		t.Fatalf("missing shared key manifest at %s: %v", manifestPath, err)
	}
}

func TestEveryManifestAttributeIsDefinedInPython(t *testing.T) {
	defined := definedStrings(t)
	for _, key := range loadManifest(t).Attributes {
		if !slices.Contains(defined, key) {
			t.Errorf("Go is missing manifest key %s", key)
		}
	}
}

func TestPythonDefinesNoAttributeTheManifestDoesNotName(t *testing.T) {
	// Inventing a key in one binding is how the ports diverge. A new key is a
	// spec revision first, then a manifest entry, then code.
	m := loadManifest(t)
	declared := append(append(slices.Clone(m.Attributes), m.EventAttributes...), m.Events["policy_binding"])
	for _, s := range definedStrings(t) {
		if strings.HasPrefix(s, "trustabl.") && !slices.Contains(declared, s) {
			t.Errorf("Go defines %s, absent from the manifest", s)
		}
	}
}

func sortedStrings[T ~string](values []T) []string {
	out := make([]string, len(values))
	for i, v := range values {
		out[i] = string(v)
	}
	slices.Sort(out)
	return out
}

func TestEnumsMatchTheManifest(t *testing.T) {
	enums := loadManifest(t).Enums
	for name, got := range map[string][]string{
		"side_effect":    sortedStrings(sideEffects),
		"error_class":    sortedStrings(errorClasses),
		"exit_reason":    sortedStrings(exitReasons),
		"binding_type":   sortedStrings(bindingTypes),
		"binding_source": sortedStrings(bindingSources),
	} {
		want := slices.Sorted(slices.Values(enums[name]))
		if !slices.Equal(got, want) {
			t.Errorf("%s = %v, manifest has %v", name, got, want)
		}
	}
}

func TestTheProfileIsFrozenAt10(t *testing.T) {
	if v := loadManifest(t).ProfileVersion; v != "1.0.0" {
		t.Errorf("profile_version = %q, want 1.0.0", v)
	}
}
