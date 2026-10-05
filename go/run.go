package otellabels

// Run identity, step order, and how a run ended.
//
// The step index is assigned in process, in call order, because OTLP
// batching can split and reorder a run on the wire. A consumer that sorts by
// trustabl.step_index gets the true sequence back.

import (
	"crypto/rand"
	"encoding/hex"
	"sync/atomic"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// Run is one user task and the step counter for it.
//
// NextStep is safe for concurrent use, so no index is ever handed out twice.
// That does not make one Run shareable across concurrent logical runs: the
// order it would record is not the order anything happened in. Carry one Run
// per logical run.
type Run struct {
	runID string
	steps atomic.Int64
}

// NewRun starts a run. An empty runID gets a random 16-hex-character id, the
// same length as the other bindings'.
func NewRun(runID string) *Run {
	if runID == "" {
		var b [8]byte
		_, _ = rand.Read(b[:]) // crypto/rand.Read never fails as of Go 1.24
		runID = hex.EncodeToString(b[:])
	}
	return &Run{runID: runID}
}

// RunID returns the run's id.
func (r *Run) RunID() string { return r.runID }

// NextStep returns the next step index, starting at 0.
func (r *Run) NextStep() int { return int(r.steps.Add(1) - 1) }

// MarkStart stamps the run identity on the root span.
func (r *Run) MarkStart(span trace.Span) {
	if unusable(span) {
		return
	}
	safely(func() { span.SetAttributes(attribute.String(KeyRunID, r.runID)) })
}

// MarkRunEnd records how the run finished.
//
// Call it before the root span ends. An ended span stops recording, so a late
// call is a silent no-op and the run has no exit_reason. An invalid reason
// returns an error and writes nothing.
func MarkRunEnd(span trace.Span, reason ExitReason) error {
	if unusable(span) {
		return nil
	}
	if err := checkEnum("exit reason", reason, exitReasons); err != nil {
		return err
	}
	safely(func() { span.SetAttributes(attribute.String(KeyExitReason, string(reason))) })
	return nil
}

// MarkHandoff records that this agent span came from a delegation. The
// handoff id lets a consumer ask whether the child carried any policy of its
// own - a handoff with no bindings is a coverage hole.
func MarkHandoff(span trace.Span, handoffID string) {
	if unusable(span) {
		return
	}
	safely(func() { span.SetAttributes(attribute.String(KeyHandoffID, handoffID)) })
}
