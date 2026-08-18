package toolloop

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/job-finder/api/internal/platform/llm/domain"
)

func TestRunRefusesADeadlelessContext(t *testing.T) {
	ts, _ := testToolset(t, func(lookupArgs) (string, error) { return "ok", nil })
	p := &scriptedProvider{turns: []turn{{content: "should never be reached"}}}

	_, err := Run[band](context.Background(), p, ts, userTurn("estimate"), nil, Bounds{})
	if !errors.Is(err, ErrNoDeadline) {
		t.Fatalf("error = %v, want ErrNoDeadline", err)
	}
	if p.calls != 0 {
		t.Errorf("provider was called %d times; the refusal must happen before any request is issued", p.calls)
	}
}

func TestRunawayStopsAtExactlyMaxRounds(t *testing.T) {
	ts, _ := testToolset(t, func(lookupArgs) (string, error) { return "more data", nil })

	turns := make([]turn, 20)
	for i := range turns {
		turns[i] = turn{toolCalls: []domain.ToolCall{call("c", "lookup_comparable_bands", lookupArgs{Bucket: "a"})}}
	}
	p := &scriptedProvider{turns: turns}

	res, err := Run[band](boundedCtx(t), p, ts, userTurn("estimate"), nil, Bounds{MaxRounds: 3})
	if err == nil {
		t.Fatal("a runaway exchange returned no error")
	}
	if res.StopReason != StopMaxRounds {
		t.Errorf("StopReason = %q, want %q", res.StopReason, StopMaxRounds)
	}
	if p.calls != 3 {
		t.Errorf("provider called %d times, want exactly 3", p.calls)
	}
	if len(res.Rounds) != 3 {
		t.Errorf("recorded %d rounds, want exactly 3", len(res.Rounds))
	}
	if res.Value != (band{}) {
		t.Errorf("Value = %+v on a non-answered stop, want the zero value", res.Value)
	}
}

func TestDefaultRoundLimitIsFour(t *testing.T) {
	if DefaultBounds().MaxRounds != 4 {
		t.Errorf("default MaxRounds = %d, want 4", DefaultBounds().MaxRounds)
	}
	if DefaultBounds().MaxTotalCostUSD != 0.50 {
		t.Errorf("default MaxTotalCostUSD = %v, want 0.50", DefaultBounds().MaxTotalCostUSD)
	}

	got := Bounds{}.withDefaults()
	if got.MaxRounds != 4 || got.PerToolTimeout != 10*time.Second || got.MaxResultBytes != 32*1024 {
		t.Errorf("zero Bounds resolved to %+v, want the documented defaults", got)
	}
}

func TestExpiredContextStopsWithoutStartingAnotherLookup(t *testing.T) {
	var handlerRuns int
	tool := domain.NewTool("lookup_comparable_bands", "look up",
		func(ctx context.Context, args lookupArgs) (string, error) {
			handlerRuns++
			return "ok", nil
		})
	ts, err := NewToolset(tool)
	if err != nil {
		t.Fatalf("NewToolset: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()

	p := &scriptedProvider{turns: []turn{
		{toolCalls: []domain.ToolCall{call("c1", "lookup_comparable_bands", lookupArgs{Bucket: "a"})}},

		{delay: 200 * time.Millisecond, toolCalls: []domain.ToolCall{call("c2", "lookup_comparable_bands", lookupArgs{Bucket: "b"})}},
		{content: "unreachable"},
	}}

	res, rerr := Run[band](ctx, p, ts, userTurn("estimate"), nil, Bounds{MaxRounds: 4})
	if rerr == nil {
		t.Fatal("an expired exchange returned no error")
	}
	if handlerRuns > 1 {
		t.Errorf("handler ran %d times; no further lookup may start once the context is done", handlerRuns)
	}
	if res.StopReason != StopDeadline && !strings.Contains(rerr.Error(), "context") {
		t.Errorf("StopReason = %q and error = %v; expected the deadline to be identifiable", res.StopReason, rerr)
	}
}

func TestHangingLookupTimesOutAndTheExchangeContinues(t *testing.T) {
	tool := domain.NewTool("lookup_comparable_bands", "look up",
		func(ctx context.Context, args lookupArgs) (string, error) {
			<-ctx.Done()
			return "", ctx.Err()
		})
	ts, err := NewToolset(tool)
	if err != nil {
		t.Fatalf("NewToolset: %v", err)
	}

	p := &scriptedProvider{turns: []turn{
		{toolCalls: []domain.ToolCall{call("c1", "lookup_comparable_bands", lookupArgs{Bucket: "a"})}},
		{content: "giving up on the lookup"},
		{content: `{"min":1,"max":2}`},
	}}

	res, rerr := Run[band](boundedCtx(t), p, ts, userTurn("estimate"), nil,
		Bounds{MaxRounds: 4, PerToolTimeout: 30 * time.Millisecond})
	if rerr != nil {
		t.Fatalf("a timed-out lookup must not fail the exchange: %v", rerr)
	}
	if res.StopReason != StopAnswered {
		t.Fatalf("StopReason = %q, want answered", res.StopReason)
	}
	if len(res.Rounds[0].Calls) != 1 || res.Rounds[0].Calls[0].Outcome != OutcomeTimeout {
		t.Errorf("round 1 calls = %+v, want one timeout", res.Rounds[0].Calls)
	}

	second := p.seen[1]
	if !strings.Contains(second[len(second)-1].Content, "TIMED OUT") {
		t.Errorf("the timeout was not reported to the model: %q", second[len(second)-1].Content)
	}
}

func TestOversizedResultIsTruncatedAndSaysSo(t *testing.T) {
	big := strings.Repeat("x", 5000)
	ts, _ := testToolset(t, func(lookupArgs) (string, error) { return big, nil })
	p := &scriptedProvider{turns: []turn{
		{toolCalls: []domain.ToolCall{call("c1", "lookup_comparable_bands", lookupArgs{Bucket: "a"})}},
		{content: "done"},
		{content: `{"min":1,"max":2}`},
	}}

	res, err := Run[band](boundedCtx(t), p, ts, userTurn("estimate"), nil,
		Bounds{MaxRounds: 4, MaxResultBytes: 100})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Rounds[0].Calls[0].Outcome != OutcomeTruncated {
		t.Errorf("outcome = %q, want truncated", res.Rounds[0].Calls[0].Outcome)
	}
	delivered := p.seen[1][len(p.seen[1])-1].Content
	if !strings.Contains(delivered, "TRUNCATED") {
		t.Errorf("truncation was silent; the model cannot tell it saw a partial result: %q", delivered)
	}
	if !strings.Contains(delivered, "5000") {
		t.Errorf("the truncation notice does not say how much was cut: %q", delivered)
	}
}

func TestFailingLookupBecomesAMessage(t *testing.T) {
	ts, _ := testToolset(t, func(lookupArgs) (string, error) {
		return "", errors.New("database unavailable")
	})
	p := &scriptedProvider{turns: []turn{
		{toolCalls: []domain.ToolCall{call("c1", "lookup_comparable_bands", lookupArgs{Bucket: "a"})}},
		{content: "answering without it"},
		{content: `{"min":1,"max":2}`},
	}}

	res, err := Run[band](boundedCtx(t), p, ts, userTurn("estimate"), nil, Bounds{})
	if err != nil {
		t.Fatalf("a failing lookup must not abort the exchange: %v", err)
	}
	if res.Rounds[0].Calls[0].Outcome != OutcomeFailed {
		t.Errorf("outcome = %q, want failed", res.Rounds[0].Calls[0].Outcome)
	}
	if !strings.Contains(p.seen[1][len(p.seen[1])-1].Content, "database unavailable") {
		t.Error("the failure was not reported to the model")
	}
}

func TestFourCallsInOneTurnCountAsOneRound(t *testing.T) {
	ts, seen := testToolset(t, func(lookupArgs) (string, error) { return "ok", nil })
	p := &scriptedProvider{turns: []turn{
		{toolCalls: []domain.ToolCall{
			call("c1", "lookup_comparable_bands", lookupArgs{Bucket: "a"}),
			call("c2", "lookup_comparable_bands", lookupArgs{Bucket: "b"}),
			call("c3", "lookup_comparable_bands", lookupArgs{Bucket: "c"}),
			call("c4", "lookup_comparable_bands", lookupArgs{Bucket: "d"}),
		}},
		{content: "done"},
		{content: `{"min":1,"max":2}`},
	}}

	res, err := Run[band](boundedCtx(t), p, ts, userTurn("estimate"), nil, Bounds{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(*seen) != 4 {
		t.Errorf("handler ran %d times, want 4 — all calls of a turn are dispatched", len(*seen))
	}
	if len(res.Rounds) != 2 {
		t.Errorf("recorded %d rounds, want 2 — four calls in one turn are one round", len(res.Rounds))
	}
	if len(res.Rounds[0].Calls) != 4 {
		t.Errorf("round 1 recorded %d calls, want 4", len(res.Rounds[0].Calls))
	}
}

func TestAccumulatedCostStopsTheExchange(t *testing.T) {
	ts, _ := testToolset(t, func(lookupArgs) (string, error) { return "ok", nil })
	expensive := turn{
		toolCalls: []domain.ToolCall{call("c", "lookup_comparable_bands", lookupArgs{Bucket: "a"})},
		usage:     &domain.Usage{CostUSD: 0.30},
	}
	p := &scriptedProvider{turns: []turn{expensive, expensive, expensive, expensive}}

	res, err := Run[band](boundedCtx(t), p, ts, userTurn("estimate"), nil,
		Bounds{MaxRounds: 8, MaxTotalCostUSD: 0.50})
	if err == nil {
		t.Fatal("the exchange ran past its spend ceiling without error")
	}
	if res.StopReason != StopCostCeiling {
		t.Fatalf("StopReason = %q, want %q", res.StopReason, StopCostCeiling)
	}
	if p.calls != 2 {
		t.Errorf("provider called %d times, want 2 — the ceiling is checked between rounds, so the third must not be issued", p.calls)
	}
	if res.Value != (band{}) {
		t.Errorf("Value = %+v on a cost-ceiling stop, want the zero value", res.Value)
	}
}

func TestNonAnsweredStopsReturnZeroValueAndAnError(t *testing.T) {
	ts, _ := testToolset(t, func(lookupArgs) (string, error) { return "ok", nil })
	loop := turn{toolCalls: []domain.ToolCall{call("c", "lookup_comparable_bands", lookupArgs{Bucket: "a"})}}

	for name, tc := range map[string]struct {
		bounds Bounds
		turns  []turn
		want   StopReason
	}{
		"max rounds": {
			bounds: Bounds{MaxRounds: 2},
			turns:  []turn{loop, loop, loop},
			want:   StopMaxRounds,
		},
		"cost ceiling": {
			bounds: Bounds{MaxRounds: 8, MaxTotalCostUSD: 0.01},
			turns: []turn{
				{toolCalls: loop.toolCalls, usage: &domain.Usage{CostUSD: 1}},
				loop,
			},
			want: StopCostCeiling,
		},
		"not tool capable": {
			bounds: Bounds{},
			turns:  []turn{{content: "just prose"}},
			want:   StopNotToolCapable,
		},
	} {
		t.Run(name, func(t *testing.T) {
			p := &scriptedProvider{turns: tc.turns}
			res, err := Run[band](boundedCtx(t), p, ts, userTurn("estimate"), nil, tc.bounds)
			if err == nil {
				t.Fatalf("stop reason %q returned no error", tc.want)
			}
			if res.StopReason != tc.want {
				t.Errorf("StopReason = %q, want %q", res.StopReason, tc.want)
			}
			if res.Value != (band{}) {
				t.Errorf("Value = %+v, want the zero value", res.Value)
			}
		})
	}
}

func TestBoundsHasNoOverallDeadlineField(t *testing.T) {
	tp := reflect.TypeOf(Bounds{})
	for i := 0; i < tp.NumField(); i++ {
		name := strings.ToLower(tp.Field(i).Name)
		if strings.Contains(name, "deadline") || name == "timeout" || strings.Contains(name, "totaltime") {
			t.Errorf("Bounds has field %q; ctx is the single deadline and a second one is forbidden by design", tp.Field(i).Name)
		}
	}
}

func TestToolsetIsImmutableAfterConstruction(t *testing.T) {
	ts, _ := testToolset(t, func(lookupArgs) (string, error) { return "ok", nil })

	decls := ts.Declarations()
	if len(decls) != 1 {
		t.Fatalf("got %d declarations, want 1", len(decls))
	}
	decls[0].Name = "mutated"
	decls = append(decls, domain.ToolDef{Name: "smuggled"})
	_ = decls

	after := ts.Declarations()
	if len(after) != 1 || after[0].Name != "lookup_comparable_bands" {
		t.Errorf("toolset changed through its own declarations: %+v", after)
	}
	if names := ts.Names(); len(names) != 1 || names[0] != "lookup_comparable_bands" {
		t.Errorf("Names() = %v, want the single registered tool", names)
	}
}
