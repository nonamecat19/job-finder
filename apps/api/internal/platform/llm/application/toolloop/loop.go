package toolloop

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/job-finder/api/internal/platform/llm/domain"
)

var ErrNoDeadline = errors.New("toolloop: the caller's context must carry a deadline")

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

	if _, ok := ctx.Deadline(); !ok {
		return res, ErrNoDeadline
	}

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

				res.StopReason = StopNotToolCapable
				return res, fmt.Errorf(
					"toolloop: task %q: the model answered without calling a tool on a round that required one — the serving tier cannot call tools, or the tools were dropped in transit; no answer produced",
					taskKeyOf(opts))
			}

			value, terr := domain.CompleteStructuredChat[T](ctx, p, convo, terminalOptions(opts))
			if terr != nil {

				return res, fmt.Errorf("toolloop: terminal step: %w", terr)
			}
			res.Value = value
			res.StopReason = StopAnswered
			return res, nil
		}

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

		truncated := out[:b.MaxResultBytes]
		return wrapResult(call.Name, fmt.Sprintf(
			"%s\n\n[TRUNCATED: this result was %d bytes and was cut to %d. You are seeing part of it.]",
			truncated, len(out), b.MaxResultBytes)), OutcomeTruncated, dur, suspect
	}
	return wrapResult(call.Name, out), OutcomeOK, dur, suspect
}

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
