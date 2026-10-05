package otellabels

// The attribute names, in one place.
//
// Mirrors python/src/agent_reliability_otel_labels/keys.py and spec/keys.json, which derive
// from the reserved names in the Trustabl attribute spec. Adding a name here
// is a SPEC REVISION, not a code change: amend that spec with a named
// producer, then the manifest, then every binding.

const (
	KeyStepIndex = "trustabl.step_index"
	KeyRunID     = "trustabl.run_id"

	KeyToolInputFP    = "trustabl.tool.input_fp"
	KeyToolOutputFP   = "trustabl.tool.output_fp"
	KeyToolSideEffect = "trustabl.tool.side_effect"
	KeyToolErrorClass = "trustabl.tool.error_class"
	KeyToolAttempt    = "trustabl.tool.attempt"

	KeyHandoffID  = "trustabl.handoff_id"
	KeyExitReason = "trustabl.exit_reason"
)

// Emitted as one EVENT per policy. Parallel-array projections were rejected:
// correlating by index silently mispairs an id with the wrong hash when an
// array is truncated, and this output is signed.
const (
	EventPolicyBinding = "trustabl.policy.binding"
	KeyBindingID       = "trustabl.policy.binding.id"
	KeyBindingType     = "trustabl.policy.binding.type"
	KeyBindingSHA256   = "trustabl.policy.binding.sha256"
	KeyBindingVersion  = "trustabl.policy.binding.version"
	KeyBindingRequired = "trustabl.policy.binding.required"
	KeyBindingSource   = "trustabl.policy.binding.source"
)

// SideEffect classifies what a tool does to the world. "pure", not "none": an
// omitted key already means "unknown", and trustabl.bind.method=none means
// something else again.
type SideEffect string

const (
	Pure         SideEffect = "pure"
	Read         SideEffect = "read"
	Write        SideEffect = "write"
	Irreversible SideEffect = "irreversible"
)

// ErrorClass says how a tool call failed.
type ErrorClass string

const (
	Timeout  ErrorClass = "timeout"
	Class4xx ErrorClass = "4xx"
	Class5xx ErrorClass = "5xx"
	Empty    ErrorClass = "empty"
	Schema   ErrorClass = "schema"
	Auth     ErrorClass = "auth"
)

// ExitReason says how a run ended.
type ExitReason string

const (
	FinalAnswer ExitReason = "final_answer"
	MaxSteps    ExitReason = "max_steps"
	PolicyStop  ExitReason = "policy_stop"
	Error       ExitReason = "error"
)

// BindingType names the kind of policy a Binding attests.
type BindingType string

const (
	Sandbox       BindingType = "sandbox"
	ContentSafety BindingType = "content_safety"
	ToolGrants    BindingType = "tool_grants"
	Retention     BindingType = "retention"
	DataSecurity  BindingType = "data_security"
	Custom        BindingType = "custom"
)

// BindingSource records how a Binding arrived.
type BindingSource string

const (
	Entrypoint BindingSource = "entrypoint"
	Inherited  BindingSource = "inherited"
	Sidecar    BindingSource = "sidecar"
	External   BindingSource = "external"
)

// The valid values of each enum, checked at run time because a value built
// from a string literal compiles whatever it says.
var (
	sideEffects    = []SideEffect{Pure, Read, Write, Irreversible}
	errorClasses   = []ErrorClass{Timeout, Class4xx, Class5xx, Empty, Schema, Auth}
	exitReasons    = []ExitReason{FinalAnswer, MaxSteps, PolicyStop, Error}
	bindingTypes   = []BindingType{Sandbox, ContentSafety, ToolGrants, Retention, DataSecurity, Custom}
	bindingSources = []BindingSource{Entrypoint, Inherited, Sidecar, External}
)
