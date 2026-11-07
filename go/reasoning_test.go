package otellabels

// MarkReasoning labels a model call somebody else spanned. These tests pin the
// claims each key makes, because a reasoning label that overclaims is worse
// than none. Mirrors python/tests/test_reasoning.py.

import (
	"reflect"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

func cotKeys(a map[string]attribute.Value) []string {
	var out []string
	for k := range a {
		if strings.HasPrefix(k, "trustabl.cot.") {
			out = append(out, k)
		}
	}
	return out
}

func TestSetsTheReasoningLabelsOnTheSpan(t *testing.T) {
	tracer, exporter := tracing(t)
	withSpan(tracer, "chat", func(span trace.Span) {
		_ = MarkReasoning(span, Reasoning{Text: "compare both fares", Mode: ModeExplicitCoT, ReasoningTokens: intPtr(12)})
	})
	a := attrs(finished(t, exporter).Attributes)
	want, err := CanonicalFP("compare both fares")
	if err != nil {
		t.Fatal(err)
	}
	if !a[KeyCoTPresent].AsBool() || a[KeyCoTTextFP].AsString() != want || a[KeyCoTMode].AsString() != "explicit_cot" {
		t.Errorf("wrong reasoning labels: %v", a)
	}
	if a[KeyGenAIReasoningTokens].AsInt64() != 12 {
		t.Errorf("token count = %v, want 12", a[KeyGenAIReasoningTokens])
	}
}

func TestNoInformationWritesNoKey(t *testing.T) {
	// Absence is a claim: a call that knows nothing must not say false.
	tracer, exporter := tracing(t)
	withSpan(tracer, "chat", func(span trace.Span) { _ = MarkReasoning(span, Reasoning{}) })
	if k := cotKeys(attrs(finished(t, exporter).Attributes)); len(k) != 0 {
		t.Errorf("wrote %v for a call that knows nothing", k)
	}
}

func TestEmptyTextIsNoEvidence(t *testing.T) {
	// A provider that hides its reasoning returns an empty string, which says
	// nothing about whether the model reasoned. Spelled out because in Go it is
	// also the zero value, so a caller cannot tell the two apart.
	tracer, exporter := tracing(t)
	withSpan(tracer, "chat", func(span trace.Span) { _ = MarkReasoning(span, Reasoning{Text: ""}) })
	if k := cotKeys(attrs(finished(t, exporter).Attributes)); len(k) != 0 {
		t.Errorf("wrote %v for an empty text", k)
	}
}

func TestAZeroTokenCountMeansNoReasoning(t *testing.T) {
	tracer, exporter := tracing(t)
	withSpan(tracer, "chat", func(span trace.Span) { _ = MarkReasoning(span, Reasoning{ReasoningTokens: intPtr(0)}) })
	a := attrs(finished(t, exporter).Attributes)
	present, ok := a[KeyCoTPresent]
	if !ok || present.AsBool() {
		t.Errorf("present = %v (written %v), want false", present, ok)
	}
	if v, ok := a[KeyGenAIReasoningTokens]; !ok || v.AsInt64() != 0 {
		t.Errorf("token count = %v (written %v), want 0", v, ok)
	}
	if _, ok := a[KeyCoTTextFP]; ok {
		t.Error("a fingerprint was written for a call with no text")
	}
}

func TestADeclaredNoneMeansNoReasoning(t *testing.T) {
	tracer, exporter := tracing(t)
	withSpan(tracer, "chat", func(span trace.Span) { _ = MarkReasoning(span, Reasoning{Mode: ModeNone}) })
	a := attrs(finished(t, exporter).Attributes)
	present, ok := a[KeyCoTPresent]
	if !ok || present.AsBool() || a[KeyCoTMode].AsString() != "none" {
		t.Errorf("present = %v (written %v), mode = %v", present, ok, a[KeyCoTMode])
	}
}

func TestTokensWithoutTextStillCountAsReasoning(t *testing.T) {
	// Hidden reasoning: the model spent tokens and showed no text.
	tracer, exporter := tracing(t)
	withSpan(tracer, "chat", func(span trace.Span) {
		_ = MarkReasoning(span, Reasoning{Mode: ModeLatent, ReasoningTokens: intPtr(40)})
	})
	a := attrs(finished(t, exporter).Attributes)
	if !a[KeyCoTPresent].AsBool() || a[KeyCoTMode].AsString() != "latent" {
		t.Errorf("wrong labels: %v", a)
	}
	if _, ok := a[KeyCoTTextFP]; ok {
		t.Error("a fingerprint was written for a call with no text")
	}
}

func TestAModeAloneDoesNotClaimThisCallReasoned(t *testing.T) {
	// Mode says how the agent reasons. Whether this call did is a separate fact.
	tracer, exporter := tracing(t)
	withSpan(tracer, "chat", func(span trace.Span) { _ = MarkReasoning(span, Reasoning{Mode: ModeLatent}) })
	a := attrs(finished(t, exporter).Attributes)
	if a[KeyCoTMode].AsString() != "latent" {
		t.Errorf("mode = %v, want latent", a[KeyCoTMode])
	}
	if _, ok := a[KeyCoTPresent]; ok {
		t.Error("present was claimed with no evidence")
	}
}

func TestANoneTheEvidenceContradictsIsOmitted(t *testing.T) {
	// Degrade to absent, never to wrong: the claim the evidence refutes is dropped.
	tracer, exporter := tracing(t)
	withSpan(tracer, "chat", func(span trace.Span) {
		_ = MarkReasoning(span, Reasoning{Text: "it reasoned after all", Mode: ModeNone})
	})
	a := attrs(finished(t, exporter).Attributes)
	if !a[KeyCoTPresent].AsBool() {
		t.Error("present should be true")
	}
	if _, ok := a[KeyCoTMode]; ok {
		t.Error("mode none was written beside evidence of reasoning")
	}
}

func TestPositiveEvidenceBeatsAZeroCount(t *testing.T) {
	tracer, exporter := tracing(t)
	withSpan(tracer, "chat", func(span trace.Span) {
		_ = MarkReasoning(span, Reasoning{Text: "it reasoned", ReasoningTokens: intPtr(0)})
	})
	if !attrs(finished(t, exporter).Attributes)[KeyCoTPresent].AsBool() {
		t.Error("present should be true")
	}
}

func TestALaterCallCannotTakeBackEvidence(t *testing.T) {
	// A span must not argue with itself. Usage often arrives after the text, and a
	// zero count there would otherwise leave present=false beside a fingerprint.
	tracer, exporter := tracing(t)
	withSpan(tracer, "chat", func(span trace.Span) {
		_ = MarkReasoning(span, Reasoning{Text: "compare both fares"})
		_ = MarkReasoning(span, Reasoning{Mode: ModeNone, ReasoningTokens: intPtr(0)})
	})
	a := attrs(finished(t, exporter).Attributes)
	if !a[KeyCoTPresent].AsBool() {
		t.Error("a later call took present back")
	}
	if _, ok := a[KeyCoTMode]; ok {
		t.Error("mode none was written beside evidence of reasoning")
	}
	want, err := CanonicalFP("compare both fares")
	if err != nil {
		t.Fatal(err)
	}
	if a[KeyCoTTextFP].AsString() != want {
		t.Errorf("text_fp = %q, want %q", a[KeyCoTTextFP].AsString(), want)
	}
}

func TestRejectsAnUnknownMode(t *testing.T) {
	tracer, exporter := tracing(t)
	withSpan(tracer, "chat", func(span trace.Span) {
		err := MarkReasoning(span, Reasoning{Text: "compare both fares", Mode: "vibes"})
		if err == nil || !strings.Contains(err.Error(), "mode") {
			t.Errorf("err = %v, want one naming the mode", err)
		}
	})
	if k := cotKeys(attrs(finished(t, exporter).Attributes)); len(k) != 0 {
		t.Errorf("wrote %v for a call it refused", k)
	}
}

func TestReasoningEnumCheckComesAfterTheNoOpCheck(t *testing.T) {
	// A workload with tracing disabled must not be crashable by this library.
	// TestTheEnumCheckComesAfterTheNoOpCheck holds the same promise for tool spans.
	if err := MarkReasoning(nil, Reasoning{Mode: "vibes"}); err != nil {
		t.Error(err)
	}
}

func TestReasoningOnASpanThatCannotRecordIsASilentNoOp(t *testing.T) {
	// A nil span is a subtest of TestNoActiveSpanIsASilentNoOp, one per module.
	if err := MarkReasoning(trace.SpanFromContext(t.Context()), Reasoning{Text: "compare both fares"}); err != nil {
		t.Error(err)
	}
	if err := MarkReasoning(panickySpan{}, Reasoning{Text: "compare both fares"}); err != nil {
		t.Error(err)
	}
}

func TestReadsTheInstrumentorsTokenCount(t *testing.T) {
	tracer, exporter := tracing(t)
	withSpan(tracer, "chat", func(span trace.Span) {
		span.SetAttributes(attribute.Int64(KeyGenAIReasoningTokens, 7))
		_ = MarkReasoning(span, Reasoning{})
	})
	a := attrs(finished(t, exporter).Attributes)
	if !a[KeyCoTPresent].AsBool() || a[KeyGenAIReasoningTokens].AsInt64() != 7 {
		t.Errorf("wrong labels: %v", a)
	}
}

func TestReadsAZeroFromTheInstrumentor(t *testing.T) {
	tracer, exporter := tracing(t)
	withSpan(tracer, "chat", func(span trace.Span) {
		span.SetAttributes(attribute.Int64(KeyGenAIReasoningTokens, 0))
		_ = MarkReasoning(span, Reasoning{})
	})
	present, ok := attrs(finished(t, exporter).Attributes)[KeyCoTPresent]
	if !ok || present.AsBool() {
		t.Errorf("present = %v (written %v), want false", present, ok)
	}
}

func TestNeverOverwritesTheInstrumentorsTokenCount(t *testing.T) {
	tracer, exporter := tracing(t)
	withSpan(tracer, "chat", func(span trace.Span) {
		span.SetAttributes(attribute.Int64(KeyGenAIReasoningTokens, 7))
		_ = MarkReasoning(span, Reasoning{ReasoningTokens: intPtr(99)})
	})
	if v := attrs(finished(t, exporter).Attributes)[KeyGenAIReasoningTokens].AsInt64(); v != 7 {
		t.Errorf("token count = %d, want the instrumentor's 7", v)
	}
}

func TestDoesNotWriteATokenCountWhenAttributesCannotBeRead(t *testing.T) {
	// An unreadable span's value is unknown, and an unknown value is never overwritten.
	span := newOpaqueSpan()
	if err := MarkReasoning(span, Reasoning{ReasoningTokens: intPtr(12)}); err != nil {
		t.Fatal(err)
	}
	if _, ok := span.written[KeyGenAIReasoningTokens]; ok {
		t.Error("a token count was written over an unknown value")
	}
	if !span.written[KeyCoTPresent].AsBool() {
		t.Error("the count is still evidence: present should be true")
	}
}

func TestATextWithNoCanonicalFormOmitsOnlyTheFingerprint(t *testing.T) {
	// One bad byte costs the hash and nothing else.
	tracer, exporter := tracing(t)
	withSpan(tracer, "chat", func(span trace.Span) {
		if err := MarkReasoning(span, Reasoning{Text: "bad \xff byte", Mode: ModeExplicitCoT}); err != nil {
			t.Error(err)
		}
	})
	a := attrs(finished(t, exporter).Attributes)
	if !a[KeyCoTPresent].AsBool() || a[KeyCoTMode].AsString() != "explicit_cot" {
		t.Errorf("wrong labels: %v", a)
	}
	if _, ok := a[KeyCoTTextFP]; ok {
		t.Error("a fingerprint was written for a text with no canonical form")
	}
}

func TestAnInvalidTokenCountIsIgnored(t *testing.T) {
	tracer, exporter := tracing(t)
	withSpan(tracer, "chat", func(span trace.Span) { _ = MarkReasoning(span, Reasoning{ReasoningTokens: intPtr(-1)}) })
	a := attrs(finished(t, exporter).Attributes)
	if _, ok := a[KeyGenAIReasoningTokens]; ok {
		t.Error("a negative token count was written")
	}
	if k := cotKeys(a); len(k) != 0 {
		t.Errorf("wrote %v for a count that means nothing", k)
	}
}

func TestATextThatIsNotAStringIsIgnored(t *testing.T) {
	// Python and TypeScript check this at run time. Here the compiler does: Text
	// is a string. This keeps it that way, because widening it to any would let
	// a struct holding the reasoning through to a code path that was written to
	// refuse it.
	if k := reflect.TypeFor[Reasoning]().Field(0); k.Name != "Text" || k.Type.Kind() != reflect.String {
		t.Errorf("Reasoning.Text is %s %s, want Text string", k.Name, k.Type)
	}
}

func TestTheSameReasoningGivesTheSameFingerprint(t *testing.T) {
	tracer, exporter := tracing(t)
	for _, text := range []string{"compare both fares", "compare both fares", "pick the cheaper fare"} {
		withSpan(tracer, "chat", func(span trace.Span) { _ = MarkReasoning(span, Reasoning{Text: text}) })
	}
	spans := exporter.GetSpans()
	first := attrs(spans[0].Attributes)[KeyCoTTextFP].AsString()
	second := attrs(spans[1].Attributes)[KeyCoTTextFP].AsString()
	third := attrs(spans[2].Attributes)[KeyCoTTextFP].AsString()
	if first != second || first == third {
		t.Errorf("fingerprints %q, %q, %q", first, second, third)
	}
}

func TestKeysStayInTheCotNamespaceAndReuseTheTokenKey(t *testing.T) {
	// The token count is OTel's. Minting trustabl.cot.token_count would give one
	// concept two keys, which the attribute spec's rule 3 forbids.
	tracer, exporter := tracing(t)
	withSpan(tracer, "chat", func(span trace.Span) {
		_ = MarkReasoning(span, Reasoning{Text: "compare both fares", Mode: ModeHybrid, ReasoningTokens: intPtr(12)})
	})
	a := attrs(finished(t, exporter).Attributes)
	if len(a) != 4 {
		t.Fatalf("wrote %d keys, want 4: %v", len(a), a)
	}
	for _, key := range []string{KeyCoTPresent, KeyCoTTextFP, KeyCoTMode, KeyGenAIReasoningTokens} {
		if _, ok := a[key]; !ok {
			t.Errorf("%s was not written", key)
		}
	}
}
