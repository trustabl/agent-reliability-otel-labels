// Package otellabels adds process-metadata labels to OpenTelemetry agent
// spans.
//
// It never creates spans. It adds attributes to spans that some other
// instrumentation library already created, so an agent must already be
// instrumented for any of this to do anything. A nil or non-recording span is
// a silent no-op, and no panic escapes any function.
//
// Tool arguments and results are fingerprinted, never stored: RFC 8785
// canonical JSON, volatile fields stripped, SHA-256, first 16 hex characters.
// Every binding (Python, TypeScript, Go) produces the same fingerprint for
// the same input. Structs are not accepted; marshal them with encoding/json
// and pass json.RawMessage.
package otellabels
