package toolloop

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/job-finder/api/internal/platform/llm/domain"
)

type band struct {
	Min int `json:"min"`
	Max int `json:"max"`
}

func (b *band) Validate() error {
	if b.Min <= 0 || b.Max < b.Min {
		return errors.New("band: min must be positive and max must not be below min")
	}
	return nil
}

type turn struct {
	content   string
	toolCalls []domain.ToolCall
	err       error
	usage     *domain.Usage
	served    string
	delay     time.Duration
}

type scriptedProvider struct {
	turns   []turn
	calls   int
	seen    [][]domain.Message
	choices []string
	tools   [][]domain.ToolDef
}

func (s *scriptedProvider) ModelName() string { return "scripted" }

func (s *scriptedProvider) CompleteChat(ctx context.Context, msgs []domain.Message, opts *domain.CompleteOptions) (domain.ChatResult, error) {
	snapshot := make([]domain.Message, len(msgs))
	copy(snapshot, msgs)
	s.seen = append(s.seen, snapshot)
	if opts != nil {
		s.choices = append(s.choices, opts.ToolChoice)
		s.tools = append(s.tools, opts.Tools)
	}

	i := s.calls
	s.calls++
	if i >= len(s.turns) {

		return domain.ChatResult{}, errors.New("scriptedProvider: no turn scripted for this call")
	}
	tn := s.turns[i]
	if tn.delay > 0 {
		select {
		case <-time.After(tn.delay):
		case <-ctx.Done():
			return domain.ChatResult{}, ctx.Err()
		}
	}
	if tn.served != "" {
		domain.ReportServedModel(ctx, tn.served)
	}
	if tn.usage != nil {
		domain.ReportUsage(ctx, *tn.usage)
	}
	if tn.err != nil {
		return domain.ChatResult{}, tn.err
	}
	return domain.ChatResult{Content: tn.content, ToolCalls: tn.toolCalls}, nil
}

func (s *scriptedProvider) Complete(ctx context.Context, prompt string, opts *domain.CompleteOptions) (string, error) {
	res, err := s.CompleteChat(ctx, domain.PromptMessages(opts.SystemPrompt(), prompt), opts)
	return res.Content, err
}

func (s *scriptedProvider) CompleteJSON(ctx context.Context, prompt string, opts *domain.CompleteOptions) (string, error) {
	return s.Complete(ctx, prompt, opts)
}

func (s *scriptedProvider) Embed(context.Context, string) ([]float32, error) { return nil, nil }

type lookupArgs struct {
	Bucket string `json:"bucket"`
}

func testToolset(t *testing.T, reply func(args lookupArgs) (string, error)) (*Toolset, *[]lookupArgs) {
	t.Helper()
	var seen []lookupArgs
	tool := domain.NewTool("lookup_comparable_bands", "Look up comparable salary bands for a bucket.",
		func(ctx context.Context, args lookupArgs) (string, error) {
			seen = append(seen, args)
			return reply(args)
		})
	ts, err := NewToolset(tool)
	if err != nil {
		t.Fatalf("NewToolset: %v", err)
	}
	return ts, &seen
}

func call(id, name string, args any) domain.ToolCall {
	raw, _ := json.Marshal(args)
	return domain.ToolCall{ID: id, Name: name, Arguments: raw}
}

func boundedCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func userTurn(text string) []domain.Message {
	return []domain.Message{{Role: string(domain.RoleUser), Content: text}}
}

func TestDeclaredLookupProducesATypedAnswer(t *testing.T) {
	ts, seen := testToolset(t, func(args lookupArgs) (string, error) {
		return `{"p50": 120000}`, nil
	})
	p := &scriptedProvider{turns: []turn{
		{toolCalls: []domain.ToolCall{call("call_1", "lookup_comparable_bands", lookupArgs{Bucket: "berlin"})}},
		{content: "I have what I need."},
		{content: `{"min":100000,"max":140000}`},
	}}

	res, err := Run[band](boundedCtx(t), p, ts, userTurn("estimate this"), nil, Bounds{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.StopReason != StopAnswered {
		t.Fatalf("StopReason = %q, want %q", res.StopReason, StopAnswered)
	}
	if res.Value.Min != 100000 || res.Value.Max != 140000 {
		t.Errorf("Value = %+v, want the typed band from the terminal step", res.Value)
	}
	if len(*seen) != 1 || (*seen)[0].Bucket != "berlin" {
		t.Errorf("handler saw %+v, want one call with bucket berlin", *seen)
	}

	second := p.seen[1]
	last := second[len(second)-1]
	if last.Role != string(domain.RoleTool) {
		t.Fatalf("last message before round 2 has role %q, want tool", last.Role)
	}
	if last.ToolCallID != "call_1" {
		t.Errorf("tool message carries id %q, want call_1", last.ToolCallID)
	}
	if !strings.Contains(last.Content, "120000") {
		t.Errorf("tool message does not carry the result: %q", last.Content)
	}
}

func TestContentWithToolCallsIsNotFinal(t *testing.T) {
	ts, seen := testToolset(t, func(lookupArgs) (string, error) { return "ok", nil })
	p := &scriptedProvider{turns: []turn{
		{
			content:   "Let me check the comparable bands first.",
			toolCalls: []domain.ToolCall{call("call_1", "lookup_comparable_bands", lookupArgs{Bucket: "berlin"})},
		},
		{content: "done"},
		{content: `{"min":1,"max":2}`},
	}}

	res, err := Run[band](boundedCtx(t), p, ts, userTurn("estimate"), nil, Bounds{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.StopReason != StopAnswered {
		t.Errorf("StopReason = %q, want answered", res.StopReason)
	}
	if len(*seen) != 1 {
		t.Errorf("handler ran %d times, want 1 — the prose was treated as an answer and the lookup skipped", len(*seen))
	}
}

func TestFirstRoundRequiresAToolCallAndLaterRoundsDoNot(t *testing.T) {
	ts, _ := testToolset(t, func(lookupArgs) (string, error) { return "ok", nil })
	p := &scriptedProvider{turns: []turn{
		{toolCalls: []domain.ToolCall{call("c1", "lookup_comparable_bands", lookupArgs{Bucket: "a"})}},
		{toolCalls: []domain.ToolCall{call("c2", "lookup_comparable_bands", lookupArgs{Bucket: "b"})}},
		{content: "done"},
		{content: `{"min":1,"max":2}`},
	}}

	if _, err := Run[band](boundedCtx(t), p, ts, userTurn("estimate"), nil, Bounds{}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(p.choices) < 3 {
		t.Fatalf("provider saw %d calls, want at least 3", len(p.choices))
	}
	if p.choices[0] != "required" {
		t.Errorf("round 1 tool_choice = %q, want required — without it, a dropped tools array is indistinguishable from a model that chose not to look anything up", p.choices[0])
	}
	for i, c := range p.choices[1:3] {
		if c != "auto" {
			t.Errorf("round %d tool_choice = %q, want auto", i+2, c)
		}
	}
}

func TestProseOnTheRequiredFirstRoundIsNotToolCapable(t *testing.T) {
	ts, seen := testToolset(t, func(lookupArgs) (string, error) { return "ok", nil })
	p := &scriptedProvider{turns: []turn{
		{content: "The salary is probably around 120k."},
	}}

	res, err := Run[band](boundedCtx(t), p, ts, userTurn("estimate"), &domain.CompleteOptions{TaskKey: "default"}, Bounds{})
	if err == nil {
		t.Fatal("Run returned nil error on a non-tool-capable tier; the prose would have been passed off as an answer")
	}
	if res.StopReason != StopNotToolCapable {
		t.Errorf("StopReason = %q, want %q", res.StopReason, StopNotToolCapable)
	}
	if res.Value != (band{}) {
		t.Errorf("Value = %+v, want the zero value", res.Value)
	}
	if !strings.Contains(err.Error(), "default") {
		t.Errorf("error %q does not name the task key", err)
	}

	if strings.Contains(err.Error(), "scripted") {
		t.Errorf("error %q names the serving model; the application does not know it and must not claim to", err)
	}
	if len(*seen) != 0 {
		t.Errorf("a handler ran despite no tool call being made")
	}
	if p.calls != 1 {
		t.Errorf("provider called %d times, want 1 — the exchange must stop immediately", p.calls)
	}
}

func TestRoundsRecordServedModelAndCost(t *testing.T) {
	ts, _ := testToolset(t, func(lookupArgs) (string, error) { return "ok", nil })
	p := &scriptedProvider{turns: []turn{
		{
			toolCalls: []domain.ToolCall{call("c1", "lookup_comparable_bands", lookupArgs{Bucket: "a"})},
			served:    "tier-one/model",
			usage:     &domain.Usage{CostUSD: 0.01},
		},
		{content: "done", served: "tier-two/model", usage: &domain.Usage{CostUSD: 0.02}},
		{content: `{"min":1,"max":2}`},
	}}

	res, err := Run[band](boundedCtx(t), p, ts, userTurn("estimate"), nil, Bounds{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(res.Rounds) != 2 {
		t.Fatalf("recorded %d rounds, want 2", len(res.Rounds))
	}
	if res.Rounds[0].ServedModel != "tier-one/model" || res.Rounds[1].ServedModel != "tier-two/model" {
		t.Errorf("served models = %q / %q, want tier-one/model / tier-two/model",
			res.Rounds[0].ServedModel, res.Rounds[1].ServedModel)
	}
	if res.Rounds[0].CostUSD != 0.01 || res.Rounds[1].CostUSD != 0.02 {
		t.Errorf("per-round cost = %v / %v, want 0.01 / 0.02", res.Rounds[0].CostUSD, res.Rounds[1].CostUSD)
	}
	if res.TotalCostUSD < 0.0299 || res.TotalCostUSD > 0.0301 {
		t.Errorf("TotalCostUSD = %v, want ~0.03", res.TotalCostUSD)
	}
	if len(res.Rounds[0].Calls) != 1 || res.Rounds[0].Calls[0].Outcome != OutcomeOK {
		t.Errorf("round 1 calls = %+v, want one ok call", res.Rounds[0].Calls)
	}
}

func TestTerminalFailureFailsTheExchange(t *testing.T) {
	ts, _ := testToolset(t, func(lookupArgs) (string, error) { return "ok", nil })
	p := &scriptedProvider{turns: []turn{
		{toolCalls: []domain.ToolCall{call("c1", "lookup_comparable_bands", lookupArgs{Bucket: "a"})}},
		{content: "done"},

		{content: `{"min":100,"max":1}`},
		{content: `{"min":100,"max":1}`},
		{content: `{"min":100,"max":1}`},
	}}

	res, err := Run[band](boundedCtx(t), p, ts, userTurn("estimate"), nil, Bounds{})
	if err == nil {
		t.Fatal("Run succeeded despite a terminal step that never produced a valid value")
	}
	if res.Value != (band{}) {
		t.Errorf("Value = %+v, want the zero value", res.Value)
	}
	if !strings.Contains(err.Error(), "terminal step") {
		t.Errorf("error %q does not identify the terminal step", err)
	}
}
