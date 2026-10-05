package otellabels

// Go-specific regressions: the ways a Go canonicalizer silently diverges from
// the other bindings. Each one was observed, not imagined.

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel/trace"
)

func TestIntegersBeyondTwoToThe53AreRejectedInEveryForm(t *testing.T) {
	// jcs.Transform turns 9007199254740993 into 9007199254740992 without
	// complaint. That must never come back, whatever form the integer takes.
	cases := map[string]any{
		"json.Number":     json.Number("9007199254740993"),
		"json.RawMessage": json.RawMessage(`{"n":9007199254740993}`),
		"int64":           int64(9007199254740993),
		"uint64":          uint64(9007199254740993),
		"negative":        json.RawMessage(`-9007199254740993`),
		"huge":            json.RawMessage(`12345678901234567890`),
	}
	for name, value := range cases {
		t.Run(name, func(t *testing.T) { wantRejected(t, value) })
	}
}

func TestHtmlCharactersStayUnescapedInRawMessage(t *testing.T) {
	wantCanonical(t, json.RawMessage(`{"q":"a<b>c&d"}`), `{"q":"a<b>c&d"}`)
}

func TestRawMessageAndMapFingerprintIdentically(t *testing.T) {
	raw := json.RawMessage(`{"to":"JFK","n":[1,2.5,true,null],"nested":{"b":1,"a":"x"}}`)
	m := map[string]any{"to": "JFK", "n": []any{1, 2.5, true, nil}, "nested": map[string]any{"b": 1, "a": "x"}}
	if mustFP(t, raw) != mustFP(t, m) {
		t.Errorf("RawMessage %s and the equivalent map fingerprint differently", mustCanonical(t, raw))
	}
}

func TestAMarshalledStructFingerprintsLikeTheEquivalentMap(t *testing.T) {
	type search struct {
		From string `json:"from"`
		To   string `json:"to"`
		Date string `json:"date"`
	}
	raw, err := json.Marshal(search{From: "SFO", To: "JFK", Date: "2026-10-01"})
	if err != nil {
		t.Fatal(err)
	}
	// The README's example and the first fixture.
	if fp := mustFP(t, json.RawMessage(raw)); fp != "c1baebf63d8d7587" {
		t.Errorf("fingerprint = %s, want c1baebf63d8d7587", fp)
	}
}

func TestAStructIsRefusedWithTheRawMessageFix(t *testing.T) {
	for name, value := range map[string]any{
		"struct":    struct{ X int }{1},
		"time.Time": time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := CanonicalJSON(value)
			if !errors.Is(err, ErrNotCanonical) || !strings.Contains(err.Error(), "json.RawMessage") {
				t.Errorf("error %v should wrap ErrNotCanonical and name the json.RawMessage fix", err)
			}
		})
	}
}

func TestFloat32IsWidenedToFloat64(t *testing.T) {
	wantCanonical(t, map[string]any{"f": float32(1.5)}, `{"f":1.5}`)
	// float32(0.1) is not 0.1; widening shows the value it really holds.
	wantCanonical(t, map[string]any{"f": float32(0.1)}, `{"f":0.10000000149011612}`)
}

func TestVolatileKeysMatchByFullUnicodeLowercasing(t *testing.T) {
	// Python and JavaScript lowercase İ to two code points, so "TİMESTAMP" is
	// real input; the Kelvin sign lowercases to "k" everywhere.
	wantCanonical(t, map[string]any{"TİMESTAMP": 1, "IDEMPOTENCY_\u212aEY": 2}, `{"TİMESTAMP":1}`)
}

func TestAnUnpairedSurrogateEscapeIsRefused(t *testing.T) {
	// encoding/json would quietly decode each of these to U+FFFD.
	for _, raw := range []string{`"\ud800"`, `"\udc00"`, `"\ud800x"`, `"\ud800\n"`, `"\ud800\ud800"`, `"\ude00\ud83d"`} {
		t.Run(raw, func(t *testing.T) { wantRejected(t, json.RawMessage(raw)) })
	}
}

func TestAPairedSurrogateEscapeAndAnEscapedBackslashAreAccepted(t *testing.T) {
	wantCanonical(t, json.RawMessage(`"😀"`), `"😀"`)
	wantCanonical(t, json.RawMessage(`"\\ud800"`), `"\\ud800"`)
}

func TestInvalidJsonTextIsRefused(t *testing.T) {
	for name, raw := range map[string]string{
		"truncated":     `{`,
		"trailing data": `1 2`,
		"empty":         ``,
		"invalid UTF-8": "\"\xff\"",
	} {
		t.Run(name, func(t *testing.T) { wantRejected(t, json.RawMessage(raw)) })
	}
}

func TestNilMapsSlicesAndPointersAreNull(t *testing.T) {
	// The same as encoding/json.Marshal, so a caller who marshals first and
	// one who does not get the same fingerprint.
	var p *int
	wantCanonical(t, map[string]any{"m": map[string]any(nil), "s": []any(nil), "p": p}, `{"m":null,"p":null,"s":null}`)
}

func TestPointersAndArraysAreFollowed(t *testing.T) {
	n := 7
	wantCanonical(t, map[string]any{"p": &n, "a": [2]string{"x", "y"}}, `{"a":["x","y"],"p":7}`)
}

func TestDeepNestingIsRefusedNotFatal(t *testing.T) {
	var v any = "leaf"
	for range 2 * maxDepth {
		v = []any{v}
	}
	wantRejected(t, v)
}

func TestALoneSurrogateInsideAVolatileFieldIsStrippedNotRefused(t *testing.T) {
	// Python and TypeScript strip the field before they look at its strings.
	wantCanonical(t, json.RawMessage(`{"timestamp":{"s":"\udc00"},"a":1}`), `{"a":1}`)
	wantCanonical(t, json.RawMessage(`{"timestamp":{"\ud800":1}}`), `{}`)
}

func TestALoneSurrogateOutsideAVolatileFieldIsStillRefused(t *testing.T) {
	for _, raw := range []string{`{"x":{"s":"\udc00"}}`, `{"x":{"\ud800":1}}`} {
		t.Run(raw, func(t *testing.T) {
			_, err := CanonicalJSON(json.RawMessage(raw))
			if !errors.Is(err, ErrNotCanonical) || !strings.Contains(err.Error(), "unpaired surrogate escape") {
				t.Errorf("error %v should wrap ErrNotCanonical and name the unpaired surrogate escape", err)
			}
		})
	}
}

func TestNestingJustUnderTheLimitIsAccepted(t *testing.T) {
	// The limit counts JSON levels, however the value reached the walker.
	raw := strings.Repeat("[", maxDepth-1) + strings.Repeat("]", maxDepth-1)
	if _, err := CanonicalJSON(json.RawMessage(raw)); err != nil {
		t.Errorf("%d nested arrays as json.RawMessage: %v", maxDepth-1, err)
	}
	var v any = []any{}
	for range maxDepth - 2 {
		v = []any{v}
	}
	if _, err := CanonicalJSON(v); err != nil {
		t.Errorf("%d nested []any: %v", maxDepth-1, err)
	}
}

func TestAPointerCycleIsRefused(t *testing.T) {
	var v any
	v = &v
	wantRejected(t, v)
}

func TestInvalidJsonNumberTextIsRefused(t *testing.T) {
	for _, text := range []string{"", "+1", "007", "1.", ".5", "0x1.8p1"} {
		t.Run(text, func(t *testing.T) { wantRejected(t, json.Number(text)) })
	}
	wantCanonical(t, json.Number("-0"), `0`)
	wantRejected(t, json.Number("1E400"))
}

func TestANilRawMessageIsNull(t *testing.T) {
	// encoding/json.Marshal renders a nil json.RawMessage as null.
	wantCanonical(t, map[string]any{"r": json.RawMessage(nil)}, `{"r":null}`)
	tracer, exporter := tracing(t)
	withSpan(tracer, "execute_tool", func(span trace.Span) {
		_ = MarkToolSpan(span, ToolCall{Args: map[string]any{}, Result: json.RawMessage(nil), Attempt: 1})
	})
	if got, want := attrs(finished(t, exporter).Attributes)[KeyToolOutputFP].AsString(), mustFP(t, nil); got != want {
		t.Errorf("output fingerprint = %q, want %q (null)", got, want)
	}
}
