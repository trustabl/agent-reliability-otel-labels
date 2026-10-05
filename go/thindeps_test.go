package otellabels

// The library depends on the OpenTelemetry API and nothing heavier: an agent
// that imports it must not pull in the SDK. Tests may use anything.

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

var allowedImports = []string{
	"go.opentelemetry.io/otel/attribute",
	"go.opentelemetry.io/otel/trace",
	"github.com/gowebpki/jcs",
}

func TestTheLibraryImportsOnlyTheApiAndJcs(t *testing.T) {
	files, _ := filepath.Glob("*.go")
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(token.NewFileSet(), name, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, spec := range f.Imports {
			path, _ := strconv.Unquote(spec.Path.Value)
			standard := !strings.Contains(strings.SplitN(path, "/", 2)[0], ".")
			allowed := false
			for _, a := range allowedImports {
				allowed = allowed || path == a
			}
			if !standard && !allowed {
				t.Errorf("%s imports %s; the library may use only the standard library, the OpenTelemetry API and jcs", name, path)
			}
		}
	}
}
