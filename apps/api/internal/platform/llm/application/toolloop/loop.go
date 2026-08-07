package toolloop

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/job-finder/api/internal/platform/llm/domain"
)

// ErrNoDeadline is returned when Run is given a context with no deadline.
var ErrNoDeadline = errors.New("toolloop: the caller's context must carry a deadline")

// Run executes a bounded tool exchange and returns a typed answer.
//
// The exchange ends in CompleteStructuredChat over the accumulated
// conversation, so the answer is the caller's own type, validated by the same
// schema machinery every structured call in this codebase already uses. It
// never returns the model's prose: prose is not an answer, and a loop that
// returned a string would have had no caller here at all.
//
// Value is meaningful only when StopReason is StopAnswered. Every other stop
// reason returns the zero T *and* a non-nil error (C4-16), so a caller cannot
// mistake a truncated exchange for a result.
func Run[T any](
	ctx context.Context,
	p domain.Provider,
	ts *Toolset,
	msgs []domain.Message,
	opts *domain.CompleteOptions,
	b Bounds,
) (Result[T], error) {
	b = b.withDefaults()
	res := Result[T]{}

	// A deadline is required, not merely respected (FR-011a, C4-3a).
	//
	// "The caller's context is the single deadline" is only a bound if there
	// is one. The proxy's own documented worst case is 600 seconds per call;
	// four rounds without a caller deadline is forty minutes of a worker
	// occupied by an exchange nobody is waiting for. Refusing here — before any
	// request is issued — is cheaper than discovering it in production.
	if _, ok := ctx.Deadline(); !ok {
		return res, ErrNoDeadline
	}

	// The conversation is built on a copy: the caller's slice is the caller's
	// (C1-6), and this one grows by two messages per round.
	convo := make([]domain.Message, 0, len(msgs)+2*b.MaxRounds)
	convo = append(convo, domain.Message{Role: string(domain.RoleSystem), Content: systemFraming})
	convo = append(convo, msgs...)

	declarations := ts.Declarations()

	for round := 1; round <= b.MaxRounds; round++ {
		if err := ctx.Err(); err != nil {
			res.StopReason = StopDeadline
			return res, fmt.Errorf("toolloop: %w after %d rounds", err, round-1)
		}
		if res.TotalCostUSD > b.MaxTotalCostUSD {
			res.StopReason = StopCostCeiling
			return res, fmt.Errorf("toolloop: spend ceiling reached: $%.4f of $%.4f after %d rounds",
				res.TotalCostUSD, b.MaxTotalCostUSD, round-1)
		}

		record := RoundRecord{Round: round}

		// Round one asks for a tool call; later rounds allow one.
		//
		// That asymmetry is the whole of the not-tool-capable detection. Under
		// "required" a tool-capable model must emit a call, so its absence is
		// diagnostic. Under "auto" it is not: a model that chose not to look
		// anything up and a model whose tools array was silently dropped in
		// transit produce identical responses, and the proxy's drop_params
		// setting makes the second case a real one.
		choice := "auto"
		if round == 1 {
			choice = "required"
		}

		callOpts := chatOptions(opts, declarations, choice)
		callCtx, served := domain.WithServedModelCapture(ctx)
		callCtx, usage := domain.WithUsageCapture(callCtx)

		reply, err := p.CompleteChat(callCtx, convo, callOpts)
		record.ServedModel = *served
		record.CostUSD = usage.CostUSD
		res.TotalCostUSD += usage.CostUSD
		if err != nil {
			res.Rounds = append(res.Rounds, record)
			return res, fmt.Errorf("toolloop: round %d: %w", round, err)
		}

		if len(reply.ToolCalls) == 0 {
			res.Rounds = append(res.Rounds, record)
			if round == 1 {
				// FR-017. The reason names the task key and the limitation and
				// deliberately does NOT name the serving model: the application
				// never learns which upstream served a request, and
				// reintroducing that knowledge here would undo 030's routing
				// model to improve one error message.
				res.StopReason = StopNotToolCapable
				return res, fmt.Errorf(
					"toolloop: task %q: the model answered without calling a tool on a round that required one — the serving tier cannot call tools, or the tools were dropped in transit; no answer produced",
					taskKeyOf(opts))
			}
			// A later round with no tool calls means the model is done looking
			// things up. Its prose is discarded; the answer comes from the
			// typed terminal.
			value, terr := domain.CompleteStructuredChat[T](ctx, p, convo, terminalOptions(opts))
			if terr != nil {
				// FR-023b: there is no prose fallback. A terminal step that
				// fails after its own retries fails the exchange.
				return res, fmt.Errorf("toolloop: terminal step: %w", terr)
			}
			res.Value = value
			res.StopReason = StopAnswered
			return res, nil
		}

		// Every call of one assistant turn is dispatched, and the whole turn
		// counts as one round (FR-015). Charging per call would let a model
		// with four cheap lookups exhaust the budget a model with one expensive
		// one keeps.
		convo = append(convo, domain.Message{
			Role:      string(domain.RoleAssistant),
			Content:   reply.Content,
			ToolCalls: reply.ToolCalls,
		})
		for _, call := range reply.ToolCalls {
			content, outcome, dur, suspect := dispatchOne(ctx, ts, call, b)
			record.Calls = append(record.Calls, CallRecord{Name: call.Name, Outcome: outcome, Duration: dur})
			record.SuspectedInjection = record.SuspectedInjection || suspect
			convo = append(convo, domain.Message{
				Role:       string(domain.RoleTool),
				ToolCallID: call.ID,
				Name:       call.Name,
				Content:    content,
			})
		}
		res.Rounds = append(res.Rounds, record)
	}

	res.StopReason = StopMaxRounds
	return res, fmt.Errorf("toolloop: stopped after %d rounds without an answer", b.MaxRounds)
}

// dispatchOne runs one lookup and turns every possible outcome into text the
// model can react to.
//
// Nothing here returns an error to the caller. A refused call, a failed call, a
// timed-out call and a truncated result are all things a model can recover from
// by asking differently; aborting the exchange on any of them would turn a
// recoverable turn into a failed run.
func dispatchOne(ctx context.Context, ts *Toolset, call domain.ToolCall, b Bounds) (content string, outcome CallOutcome, dur time.Duration, suspect bool) {
	callCtx, cancel := context.WithTimeout(ctx, b.PerToolTimeout)
	defer cancel()

	started := time.Now()
	out, err := ts.Dispatch(callCtx, call)
	dur = time.Since(started)

	switch {
	case err != nil:
		var ref refusal
		if errors.As(err, &ref) {
			return wrapResult(call.Name, "REFUSED: "+ref.reason), OutcomeRefused, dur, false
		}
		if callCtx.Err() != nil && ctx.Err() == nil {
			return wrapResult(call.Name, fmt.Sprintf("TIMED OUT after %s. Try a narrower request or answer without this lookup.", b.PerToolTimeout)),
				OutcomeTimeout, dur, false
		}
		return wrapResult(call.Name, "FAILED: "+err.Error()), OutcomeFailed, dur, false
	}

	suspect = looksInjected(out)
	if len(out) > b.MaxResultBytes {
		// FR-014: say so. A silently truncated result is one the model reasons
		// over as if it were complete, which is how a partial lookup becomes a
		// confident wrong answer.
		truncated := out[:b.MaxResultBytes]
		return wrapResult(call.Name, fmt.Sprintf(
			"%s\n\n[TRUNCATED: this result was %d bytes and was cut to %d. You are seeing part of it.]",
			truncated, len(out), b.MaxResultBytes)), OutcomeTruncated, dur, suspect
	}
	return wrapResult(call.Name, out), OutcomeOK, dur, suspect
}

// chatOptions builds the per-round options: the caller's own settings plus this
// round's tool declarations and choice.
//
// It copies, because the caller's options belong to the caller and the round's
// tool choice changes between rounds. JSONOutput is deliberately off: a round
// that may return tool calls must not be constrained to JSON, or the model has
// to choose between calling a tool and satisfying the format.
func chatOptions(opts *domain.CompleteOptions, tools []domain.ToolDef, choice string) *domain.CompleteOptions {
	var cp domain.CompleteOptions
	if opts != nil {
		cp = *opts
	}
	cp.Tools = tools
	cp.ToolChoice = choice
	cp.JSONOutput = false
	cp.ResponseMode = domain.ResponseModeJSON
	cp.JSONSchema = ""
	return &cp
}

// terminalOptions builds the options for the typed terminal step.
//
// The caller's strictness is preserved exactly (C4-19): ResponseModeStrict and
// the Validator hook apply to the loop's answer the way they apply to a single
// structured call today. Tools are cleared — the answer turn is not a round
// that may look anything else up.
func terminalOptions(opts *domain.CompleteOptions) *domain.CompleteOptions {
	var cp domain.CompleteOptions
	if opts != nil {
		cp = *opts
	}
	cp.Tools = nil
	cp.ToolChoice = ""
	return &cp
}

func taskKeyOf(opts *domain.CompleteOptions) string {
	if k := opts.Task(); k != "" {
		return k
	}
	return "(unnamed)"
}
