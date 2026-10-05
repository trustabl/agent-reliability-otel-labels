package otellabels

// Policy bindings: which signed policies were in force.
//
// One EVENT per policy, never a flattened hash and never parallel arrays.
// Contract identity is deliberately NOT emitted here: the deployment stamps it
// through OTEL_RESOURCE_ATTRIBUTES, and a second producer for one key is a bug.

import (
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// Binding is one signed policy artifact attested on a span.
//
// Required says this agent class must carry this rail, so its absence is a
// finding. Source records how it arrived; External marks an artifact this
// contract did not generate.
type Binding struct {
	ID, SHA256, Version string
	Type                BindingType
	Required            bool
	Source              BindingSource
}

// BindAuthority attests every policy in force on this agent or sandbox root.
//
// Every binding is validated before any event is emitted; an invalid Type or
// Source returns an error and emits nothing. An empty slice emits nothing on
// purpose: a handoff carrying no binding is a coverage hole, and making it
// visible is the point.
func BindAuthority(span trace.Span, bindings []Binding) error {
	if unusable(span) {
		return nil
	}
	for _, b := range bindings {
		if err := checkEnum("binding type", b.Type, bindingTypes); err != nil {
			return err
		}
		if err := checkEnum("binding source", b.Source, bindingSources); err != nil {
			return err
		}
	}
	for _, b := range bindings {
		safely(func() {
			span.AddEvent(EventPolicyBinding, trace.WithAttributes(
				attribute.String(KeyBindingID, b.ID),
				attribute.String(KeyBindingType, string(b.Type)),
				attribute.String(KeyBindingSHA256, b.SHA256),
				attribute.String(KeyBindingVersion, b.Version),
				attribute.Bool(KeyBindingRequired, b.Required),
				attribute.String(KeyBindingSource, string(b.Source)),
			))
		})
	}
	return nil
}
