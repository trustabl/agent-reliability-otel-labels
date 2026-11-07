package otellabels

// Labelling a model call's reasoning without carrying its text.
//
// A chain of thought quotes whatever the model read, so it is the most
// sensitive text this library is ever handed. It is hashed and dropped here;
// only the fingerprint reaches the span.
//
// These labels say that a model reasoned and which text it produced. They do
// not say the reasoning was faithful: present means reasoning occurred, not
// that it caused the answer.

import (
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// Reasoning is one model call's reasoning to label.
type Reasoning struct {
	// Text is fingerprinted, never stored. "" says nothing, because a provider
	// that hides its reasoning returns an empty string.
	Text string
	// Mode says how the agent reasons. "" means not known: the key is then
	// omitted, which says so.
	Mode CoTMode
	// ReasoningTokens fills gen_ai.usage.reasoning.output_tokens only when the
	// instrumentor left it empty. That key is OTel's, and when it is already on
	// the span it decides trustabl.cot.present without being passed here. A
	// pointer because 0 is a real count; nil means unknown.
	ReasoningTokens *int
}

// MarkReasoning attaches the reasoning labels to one model-call span.
//
// It returns an error only for an invalid Mode, and then writes nothing. A nil
// or non-recording span, or a Text with no canonical form, returns nil: the
// fingerprint is omitted and every other attribute is still written. A negative
// count is ignored rather than refused: unlike an enum typo, it can come from a
// provider's response at run time, and a workload must not fail on it.
//
// A call that says nothing either way writes nothing: an omitted
// trustabl.cot.present means "unknown", and false would be a false claim.
func MarkReasoning(span trace.Span, r Reasoning) error {
	if unusable(span) {
		return nil
	}
	if r.Mode != "" {
		if err := checkEnum("mode", r.Mode, cotModes); err != nil {
			return err
		}
	}

	onSpan, found, readable := existing(span, KeyGenAIReasoningTokens)
	var counted, positive bool
	count := func(n int64) {
		if n >= 0 {
			counted = true
			positive = positive || n > 0
		}
	}
	if r.ReasoningTokens != nil {
		count(int64(*r.ReasoningTokens))
	}
	if found && onSpan.Type() == attribute.INT64 {
		count(onSpan.AsInt64())
	}

	// Evidence already on the span stands: usage often arrives after the text, and
	// a zero count must not leave present=false beside a fingerprint.
	earlier, stated, _ := existing(span, KeyCoTPresent)
	attrs := make([]attribute.KeyValue, 0, 4)
	evidence := r.Text != "" || positive || (stated && earlier.Type() == attribute.BOOL && earlier.AsBool())
	switch {
	case evidence:
		attrs = append(attrs, attribute.Bool(KeyCoTPresent, true))
	case r.Mode == ModeNone || counted:
		attrs = append(attrs, attribute.Bool(KeyCoTPresent, false))
	}
	if r.Text != "" {
		if fp, ok := fingerprint(r.Text); ok {
			attrs = append(attrs, attribute.String(KeyCoTTextFP, fp))
		}
	}
	// A declared "none" the evidence refutes is dropped: present=true beside
	// mode=none would be a span arguing with itself.
	if r.Mode != "" && !(r.Mode == ModeNone && evidence) {
		attrs = append(attrs, attribute.String(KeyCoTMode, string(r.Mode)))
	}
	// Their key, their value. We fill it only when the instrumentor left it
	// empty, and never when we cannot tell.
	if r.ReasoningTokens != nil && *r.ReasoningTokens >= 0 && readable && !found {
		attrs = append(attrs, attribute.Int64(KeyGenAIReasoningTokens, int64(*r.ReasoningTokens)))
	}
	if len(attrs) > 0 {
		safely(func() { span.SetAttributes(attrs...) })
	}
	return nil
}
