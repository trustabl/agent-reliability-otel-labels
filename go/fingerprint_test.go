package otellabels

// A fingerprint that does not collapse repeats reports a loop as N distinct
// calls and silently disables loop detection. Mirrors
// python/tests/test_fingerprint.py.

import (
	"errors"
	"maps"
	"regexp"
	"testing"
)

func mustFP(t *testing.T, v any) string {
	t.Helper()
	fp, err := CanonicalFP(v)
	if err != nil {
		t.Fatalf("CanonicalFP(%#v): %v", v, err)
	}
	return fp
}

var hex16 = regexp.MustCompile(`^[0-9a-f]{16}$`)

func TestIdenticalArgumentsProduceIdenticalFingerprints(t *testing.T) {
	args := map[string]any{"from": "SFO", "to": "JFK", "date": "2026-10-01"}
	if mustFP(t, args) != mustFP(t, maps.Clone(args)) {
		t.Error("identical arguments fingerprinted differently")
	}
}

func TestKeyOrderDoesNotChangeTheFingerprint(t *testing.T) {
	// Go maps have no order; the JSON text form is where order could leak.
	a := mustFP(t, rawJSON(`{"a":1,"b":2}`))
	b := mustFP(t, rawJSON(`{"b":2,"a":1}`))
	if a != b {
		t.Error("key order changed the fingerprint")
	}
}

func TestDifferentArgumentsProduceDifferentFingerprints(t *testing.T) {
	if mustFP(t, map[string]any{"to": "JFK"}) == mustFP(t, map[string]any{"to": "LAX"}) {
		t.Error("different arguments fingerprinted alike")
	}
}

func TestFingerprintIsSixteenHexCharacters(t *testing.T) {
	if fp := mustFP(t, map[string]any{"anything": "at all"}); !hex16.MatchString(fp) {
		t.Errorf("fingerprint %q is not 16 lowercase hex characters", fp)
	}
}

func TestVolatileFieldsDoNotChangeTheFingerprint(t *testing.T) {
	a := map[string]any{"query": "flights to JFK", "request_id": "req-111", "timestamp": "2026-09-22T10:00:00Z"}
	b := map[string]any{"query": "flights to JFK", "request_id": "req-222", "timestamp": "2026-09-22T10:00:05Z"}
	if mustFP(t, a) != mustFP(t, b) {
		t.Error("volatile fields changed the fingerprint")
	}
}

func TestVolatileFieldsAreStrippedAtAnyDepth(t *testing.T) {
	a := map[string]any{"outer": map[string]any{"query": "x", "trace_id": "aaa"}}
	b := map[string]any{"outer": map[string]any{"query": "x", "trace_id": "bbb"}}
	if mustFP(t, a) != mustFP(t, b) {
		t.Error("a nested volatile field changed the fingerprint")
	}
}

func TestVolatileFieldsInsideListsAreStripped(t *testing.T) {
	a := map[string]any{"items": []any{map[string]any{"sku": "A1", "nonce": "n1"}}}
	b := map[string]any{"items": []any{map[string]any{"sku": "A1", "nonce": "n2"}}}
	if mustFP(t, a) != mustFP(t, b) {
		t.Error("a volatile field inside a list changed the fingerprint")
	}
}

func TestAMeaningfulFieldWhoseNameContainsIdIsKept(t *testing.T) {
	if mustFP(t, map[string]any{"customer_id": "c1"}) == mustFP(t, map[string]any{"customer_id": "c2"}) {
		t.Error("customer_id was stripped")
	}
}

func TestCanonicalFpRaisesRatherThanHashingAnUncanonicalizableValue(t *testing.T) {
	fp, err := CanonicalFP(map[string]any{"n": int64(1) << 53})
	if !errors.Is(err, ErrNotCanonical) || fp != "" {
		t.Errorf("CanonicalFP = %q, %v; want \"\", ErrNotCanonical", fp, err)
	}
}
