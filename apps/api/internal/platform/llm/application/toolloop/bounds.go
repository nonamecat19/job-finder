package toolloop

import "time"

type Bounds struct {
	MaxRounds int

	PerToolTimeout time.Duration

	MaxResultBytes int

	MaxTotalCostUSD float64
}

const (
	DefaultMaxRounds       = 4
	DefaultPerToolTimeout  = 10 * time.Second
	DefaultMaxResultBytes  = 32 * 1024
	DefaultMaxTotalCostUSD = 0.50
)

func DefaultBounds() Bounds {
	return Bounds{
		MaxRounds:       DefaultMaxRounds,
		PerToolTimeout:  DefaultPerToolTimeout,
		MaxResultBytes:  DefaultMaxResultBytes,
		MaxTotalCostUSD: DefaultMaxTotalCostUSD,
	}
}

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

type StopReason string

const (
	StopAnswered StopReason = "answered"

	StopMaxRounds StopReason = "max_rounds"

	StopDeadline StopReason = "deadline"

	StopCostCeiling StopReason = "cost_ceiling"

	StopNotToolCapable StopReason = "not_tool_capable"
)

type CallOutcome string

const (
	OutcomeOK        CallOutcome = "ok"
	OutcomeRefused   CallOutcome = "refused"
	OutcomeFailed    CallOutcome = "failed"
	OutcomeTimeout   CallOutcome = "timeout"
	OutcomeTruncated CallOutcome = "truncated"
)

type CallRecord struct {
	Name     string
	Outcome  CallOutcome
	Duration time.Duration
}

type RoundRecord struct {
	Round       int
	ServedModel string
	CostUSD     float64
	Calls       []CallRecord

	SuspectedInjection bool
}

type Result[T any] struct {
	Value        T
	Rounds       []RoundRecord
	TotalCostUSD float64
	StopReason   StopReason
}
