package otellabels

// Golden fingerprints every language binding must reproduce. Mirrors
// python/tests/test_fingerprint_fixtures.py and reads the same file.

import (
	"encoding/json"
	"errors"
	"os"
	"testing"
)

const fixturesPath = "../spec/fingerprint-fixtures.json"

// Inputs stay json.RawMessage so the test harness itself never rounds an
// integer on the way in.
type fixtures struct {
	Cases []struct {
		Name        string          `json:"name"`
		Input       json.RawMessage `json:"input"`
		Canonical   string          `json:"canonical"`
		Fingerprint string          `json:"fingerprint"`
	} `json:"cases"`
	Rejected struct {
		Cases []struct {
			Name  string          `json:"name"`
			Input json.RawMessage `json:"input"`
		} `json:"cases"`
	} `json:"rejected"`
}

func loadFixtures(t *testing.T) fixtures {
	t.Helper()
	data, err := os.ReadFile(fixturesPath)
	if err != nil {
		t.Fatal(err)
	}
	var f fixtures
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatal(err)
	}
	return f
}

func rawJSON(s string) json.RawMessage { return json.RawMessage(s) }

func TestFixtureFileExists(t *testing.T) {
	if _, err := os.Stat(fixturesPath); err != nil {
		t.Fatalf("missing cross-language fixtures at %s: %v", fixturesPath, err)
	}
}

func TestEveryFixtureReproducesItsCanonicalString(t *testing.T) {
	// Checked before the hash on purpose: a character diff explains itself.
	for _, c := range loadFixtures(t).Cases {
		got, err := CanonicalJSON(c.Input)
		if err != nil || got != c.Canonical {
			t.Errorf("%s: got %q, %v; want %q", c.Name, got, err, c.Canonical)
		}
	}
}

func TestEveryFixtureReproducesItsRecordedFingerprint(t *testing.T) {
	for _, c := range loadFixtures(t).Cases {
		got, err := CanonicalFP(c.Input)
		if err != nil || got != c.Fingerprint {
			t.Errorf("%s: got %q, %v; want %q", c.Name, got, err, c.Fingerprint)
		}
	}
}

func TestEveryRejectedInputRaises(t *testing.T) {
	for _, c := range loadFixtures(t).Rejected.Cases {
		if got, err := CanonicalJSON(c.Input); !errors.Is(err, ErrNotCanonical) {
			t.Errorf("%s: got %q, %v; want ErrNotCanonical", c.Name, got, err)
		}
	}
}
