// Package toolloop runs a bounded, typed, read-only tool exchange: the model
// may ask for declared lookups, the lookups run, their results come back as
// conversation turns, and the exchange ends in a schema-validated value.
//
// Three properties are the point, and each is enforced rather than intended:
//
//   - It cannot run away. Every bound is fixed before the first request and none
//     of them can be read out of model output or a tool result.
//   - It cannot act. The lookups are reads. That is enforced outside this
//     package by an architecture test over the transitive imports of every
//     package that registers tools — see apps/api/internal/toolfence_test.go,
//     which also documents the three things it cannot catch.
//   - It cannot be steered by what a lookup returns. Tool output is untrusted
//     data, framed and delimited as such, and nothing in it can change the
//     toolset, the bounds, the round count or the answer's schema.
package toolloop

import "time"

// Bounds is what stops an exchange. Every value is fixed at construction and
// none is derivable from a prompt or a model response (FR-026).
type Bounds struct {
	// MaxRounds is how many provider turns the exchange may take.
	MaxRounds int
	// PerToolTimeout bounds one lookup, independently of ctx.
	PerToolTimeout time.Duration
	// MaxResultBytes bounds one lookup's result. Oversized results are
	// truncated and *told* they were truncated.
	MaxResultBytes int
	// MaxTotalCostUSD stops the exchange once accumulated provider cost
	// exceeds it.
	MaxTotalCostUSD float64

	// There is deliberately no overall-deadline field. The caller's context is
	// the single deadline, and Run refuses to start without one — see
	// requireDeadline. A second competing timeout is what produced 030's
	// 830-second hang, and "no second timer" is not the same as "bounded".
}

// Default bounds.
//
// MaxRounds is 4, not 8. Four covers the two-lookup shape the salary consumer
// measures plus one recovery from a refusal; eight was a round number, and the
// cost of a too-generous round limit is paid in wall time on a proxy whose own
// documented worst case is 600s per call.
//
// MaxTotalCostUSD exists because the proxy has always reported per-call cost
// and nothing was accumulating it. A loop that can issue four calls it did not
// individually authorise needs a ceiling on the total, not just on the count.
const (
	DefaultMaxRounds       = 4
	DefaultPerToolTimeout  = 10 * time.Second
	DefaultMaxResultBytes  = 32 * 1024
	DefaultMaxTotalCostUSD = 0.50
)

// DefaultBounds returns the bounds an exchange gets when the caller expresses
// no opinion.
func DefaultBounds() Bounds {
	return Bounds{
		MaxRounds:       DefaultMaxRounds,
		PerToolTimeout:  DefaultPerToolTimeout,
		MaxResultBytes:  DefaultMaxResultBytes,
		MaxTotalCostUSD: DefaultMaxTotalCostUSD,
	}
}

// withDefaults fills unset fields. A zero value means "unspecified", never
// "unbounded": a caller who forgets MaxRounds gets four, not infinity.
func (b Bounds) withDefaults() Bounds {
	d := DefaultBounds()
	if b.MaxRounds <= 0 {
		b.MaxRounds = d.MaxRounds
	}
	if b.PerToolTimeout <= 0 {
		b.PerToolTimeout = d.PerToolTimeout
	}
	if b.MaxResultBytes <= 0 {
		b.MaxResultBytes = d.MaxResultBytes
	}
	if b.MaxTotalCostUSD <= 0 {
		b.MaxTotalCostUSD = d.MaxTotalCostUSD
	}
	return b
}

// StopReason is why an exchange ended. It exists so the bounds are observable
// rather than inferred from an error string.
type StopReason string

const (
	// StopAnswered is the only reason that produces a value.
	StopAnswered StopReason = "answered"
	// StopMaxRounds means the round limit was reached.
	StopMaxRounds StopReason = "max_rounds"
	// StopDeadline means the caller's context expired.
	StopDeadline StopReason = "deadline"
	// StopCostCeiling means accumulated cost passed MaxTotalCostUSD.
	StopCostCeiling StopReason = "cost_ceiling"
	// StopNotToolCapable means the first round — sent with tool_choice
	// "required" — came back with no tool call, which a tool-capable model
	// cannot do. Something in the path dropped the tools array.
	StopNotToolCapable StopReason = "not_tool_capable"
)

// CallOutcome is what happened to one lookup.
type CallOutcome string

const (
	OutcomeOK        CallOutcome = "ok"
	OutcomeRefused   CallOutcome = "refused"
	OutcomeFailed    CallOutcome = "failed"
	OutcomeTimeout   CallOutcome = "timeout"
	OutcomeTruncated CallOutcome = "truncated"
)

// CallRecord is one lookup within a round.
type CallRecord struct {
	Name     string
	Outcome  CallOutcome
	Duration time.Duration
}

// RoundRecord is one provider turn and the lookups it produced.
//
// ServedModel and CostUSD are here because without them a multi-round exchange
// is invisible to exactly the per-tier and per-run visibility features 035 and
// 036 exist to provide — one exchange would show up as several unattributed
// calls.
type RoundRecord struct {
	Round       int
	ServedModel string
	CostUSD     float64
	Calls       []CallRecord
	// SuspectedInjection is set when a result matched the injection heuristic.
	// This is a detector, not a filter: it records, it does not sanitise, and
	// it must not be read as a claim that injected content was removed.
	SuspectedInjection bool
}

// Result is what an exchange produced.
//
// Value is meaningful **only** when StopReason is StopAnswered. Every other
// stop reason returns the zero T and a non-nil error, so no caller can mistake
// a truncated exchange for an answer.
type Result[T any] struct {
	Value        T
	Rounds       []RoundRecord
	TotalCostUSD float64
	StopReason   StopReason
}
