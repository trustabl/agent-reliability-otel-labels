package otellabels

// The canonical serialization, pinned as a string. Mirrors
// python/tests/test_canonical_json.py name for name; the values rejected here
// are Go's own non-JSON types.

import (
	"encoding/json"
	"errors"
	"math"
	"testing"
	"time"
)

func mustCanonical(t *testing.T, v any) string {
	t.Helper()
	s, err := CanonicalJSON(v)
	if err != nil {
		t.Fatalf("CanonicalJSON(%#v): %v", v, err)
	}
	return s
}

func wantCanonical(t *testing.T, v any, want string) {
	t.Helper()
	if got := mustCanonical(t, v); got != want {
		t.Errorf("CanonicalJSON = %s, want %s", got, want)
	}
}

func wantRejected(t *testing.T, v any) {
	t.Helper()
	s, err := CanonicalJSON(v)
	if !errors.Is(err, ErrNotCanonical) {
		t.Errorf("CanonicalJSON(%#v) = %q, %v; want ErrNotCanonical", v, s, err)
	}
}

func TestKeysAreSortedAndSeparatorsAreCompact(t *testing.T) {
	wantCanonical(t, map[string]any{"b": 2, "a": 1}, `{"a":1,"b":2}`)
}

func TestNonAsciiIsEmittedRawNotEscaped(t *testing.T) {
	wantCanonical(t, map[string]any{"q": "Tōkyō"}, `{"q":"Tōkyō"}`)
}

func TestVolatileFieldsAreGoneBeforeSerialization(t *testing.T) {
	wantCanonical(t, map[string]any{"a": 1, "request_id": "r"}, `{"a":1}`)
}

func TestWholeFloatsRenderAsIntegers(t *testing.T) {
	wantCanonical(t, map[string]any{"f": 1.0}, `{"f":1}`)
}

func TestIntegersHaveNoDecimalPoint(t *testing.T) {
	wantCanonical(t, map[string]any{"i": 42}, `{"i":42}`)
}

func TestHtmlCharactersAreNotEscaped(t *testing.T) {
	// encoding/json.Marshal would write <, > and & as < etc.
	wantCanonical(t, map[string]any{"q": "a<b>c&d"}, `{"q":"a<b>c&d"}`)
}

func TestLargeSafeFloatsRenderPositionally(t *testing.T) {
	wantCanonical(t, map[string]any{"n": 1e15}, `{"n":1000000000000000}`)
}

func TestSmallFloatsRenderPositionally(t *testing.T) {
	wantCanonical(t, map[string]any{"n": 1e-5}, `{"n":0.00001}`)
}

func TestTinyFloatsUseAnExponent(t *testing.T) {
	wantCanonical(t, map[string]any{"n": 1e-7}, `{"n":1e-7}`)
}

func TestNegativeZeroRendersAsZero(t *testing.T) {
	wantCanonical(t, map[string]any{"n": math.Copysign(0, -1)}, `{"n":0}`)
}

func TestKeysSortByUtf16CodeUnits(t *testing.T) {
	// Code-point order (and Go's byte order) puts U+FB01 first; UTF-16 order
	// puts U+1F600 first, because its high surrogate D83D sorts below FB01.
	wantCanonical(t, map[string]any{"ﬁ": 1, "\U0001F600": 2}, `{"😀":2,"ﬁ":1}`)
}

func TestControlCharactersAreEscapedAsEcmascriptDoes(t *testing.T) {
	wantCanonical(t, map[string]any{"s": "a\x00b\tc\nd\x1fe\"f\\g"}, `{"s":"a\u0000b\tc\nd\u001fe\"f\\g"}`)
}

func TestRejectsAnIntegerBeyondTheSafeRange(t *testing.T) {
	wantRejected(t, map[string]any{"n": int64(1) << 53})
}

func TestAcceptsTheLargestSafeInteger(t *testing.T) {
	wantCanonical(t, map[string]any{"n": int64(1)<<53 - 1}, `{"n":9007199254740991}`)
}

func TestRejectsAWholeFloatBeyondTheSafeRange(t *testing.T) {
	// JSON.parse gives 1e16 and 10000000000000000 the same value, so every
	// binding refuses both.
	wantRejected(t, map[string]any{"n": 1e16})
}

func TestRejectsNanAndInfinity(t *testing.T) {
	for name, value := range map[string]float64{"nan": math.NaN(), "inf": math.Inf(1), "-inf": math.Inf(-1)} {
		t.Run(name, func(t *testing.T) { wantRejected(t, map[string]any{"n": value}) })
	}
}

func TestRejectsAValueThatIsNotJson(t *testing.T) {
	cases := map[string]any{
		"time.Time":  time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC),
		"struct":     struct{ X int }{1},
		"[]byte":     []byte("bytes"),
		"channel":    make(chan int),
		"function":   func() {},
		"complex128": complex(1, 2),
	}
	for name, value := range cases {
		t.Run(name, func(t *testing.T) { wantRejected(t, map[string]any{"v": value}) })
	}
}

func TestRejectsANonStringKey(t *testing.T) {
	wantRejected(t, map[int]string{1: "a"})
}

func TestRejectsALoneSurrogateInAString(t *testing.T) {
	// A Go string cannot hold a UTF-16 surrogate; it arrives as a JSON escape
	// or as its (invalid) UTF-8 encoding.
	wantRejected(t, json.RawMessage(`{"s":"\ud800"}`))
	wantRejected(t, map[string]any{"s": "\xed\xa0\x80"})
}

func TestRejectsALoneSurrogateInAKey(t *testing.T) {
	wantRejected(t, json.RawMessage(`{"\udc00":1}`))
	wantRejected(t, map[string]any{"\xed\xb0\x80": 1})
}
