package application

import (
	"context"
	"sync"
	"testing"

	"github.com/job-finder/api/internal/activity"
	"github.com/job-finder/api/internal/generation/domain"
	"github.com/job-finder/api/internal/platform/llm"
)

// traceSpy wraps a stage provider and records the trace id each call saw on its
// context. The trace never travels on CompleteOptions — it is stamped once at
// the top of a run and read back out by the gateway adapter — so a call site
// that "forgets" to pass it cannot exist, and what this spy is really checking
// is that no intermediate frame drops or replaces the context.
type traceSpy struct {
	inner  *stageProvider
	mu     sync.Mutex
	traces []string
}

func spyOn(p *stageProvider) *traceSpy { return &traceSpy{inner: p} }

func (s *traceSpy) record(ctx context.Context) {
	s.mu.Lock()
	s.traces = append(s.traces, llm.TraceIDFrom(ctx))
	s.mu.Unlock()
}

func (s *traceSpy) ModelName() string { return s.inner.ModelName() }

func (s *traceSpy) Complete(ctx context.Context, prompt string, opts *llm.CompleteOptions) (string, error) {
	s.record(ctx)
	return s.inner.Complete(ctx, prompt, opts)
}

func (s *traceSpy) CompleteJSON(ctx context.Context, prompt string, opts *llm.CompleteOptions) (string, error) {
	s.record(ctx)
	return s.inner.CompleteJSON(ctx, prompt, opts)
}

func (s *traceSpy) CompleteChat(ctx context.Context, msgs []llm.Message, opts *llm.CompleteOptions) (llm.ChatResult, error) {
	s.record(ctx)
	return s.inner.CompleteChat(ctx, msgs, opts)
}

func (s *traceSpy) Embed(ctx context.Context, text string) ([]float32, error) {
	return s.inner.Embed(ctx, text)
}

func (s *traceSpy) seen() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.traces...)
}

// ungroundedThenGrounded answers the summary stage with a claim the master
// cannot support on the first call and a safe one afterwards, which is what
// forces the FR-008 re-prompt. The retry is the interesting call here: it is
// emitted from inside summarize(), several frames below where the trace was
// stamped, so it is exactly the call a threading-based design would drop.
func ungroundedThenGrounded(t *testing.T) func(string) string {
	t.Helper()
	var mu sync.Mutex
	calls := 0
	return func(string) string {
		mu.Lock()
		calls++
		n := calls
		mu.Unlock()
		if n == 1 {
			return mustJSON(t, domain.TailoredSummary{Summary: "Led engineering at Globex and holds a PhD in cryptography."})
		}
		return mustJSON(t, domain.TailoredSummary{Summary: "Backend engineer who shipped the payments service."})
	}
}

// tracedService builds the escalating pipeline of escalation_test.go with every
// stage wrapped in a spy, so one run exercises the analyze stage, two economy
// selections, one premium escalation, and a summary that re-prompts — eleven
// call sites' worth of behaviour in one pass.
func tracedService(t *testing.T) (*Service, []*traceSpy) {
	t.Helper()
	analyze := spyOn(&stageProvider{name: "generation-analyze", reply: func(string) string {
		return mustJSON(t, domain.VacancyAnalysis{
			RequiredSkills:   []string{"Go", "Docker"},
			NiceToHaveSkills: []string{"React"},
			ExperienceLevel:  "senior",
		})
	}})
	sel := spyOn(&stageProvider{name: "generation-select", reply: truncatedSelection(t)})
	prem := spyOn(&stageProvider{name: "generation-select-premium", reply: completeSelection(t)})
	summary := spyOn(&stageProvider{name: "generation-summary", reply: ungroundedThenGrounded(t)})

	svc := &Service{llm: GenerationRouters{
		Analyze: analyze, Select: sel, Premium: prem, Summary: summary, Cover: summary,
	}}
	return svc, []*traceSpy{analyze, sel, prem, summary}
}

func traceTestConfig() domain.ShapeConfig {
	cfg := domain.DefaultShapeConfig()
	cfg.ExperienceBulletsMin = 2
	return cfg
}

// recorderWithID returns a recorder carrying the given run id and no store, so
// its Step calls are inert while ID() still answers. That is the shape the
// trace stamp actually depends on.
func recorderWithID(t *testing.T, id string) *activity.Recorder {
	t.Helper()
	rec := activity.FromID(nil, id)
	if rec == nil {
		t.Fatalf("activity.FromID(%q) returned nil", id)
	}
	return rec
}

// runTraced runs one tailoring pass under the given run id and returns every
// trace id observed across every stage, in call order per stage.
func runTraced(t *testing.T, runID string) ([]string, error) {
	t.Helper()
	svc, spies := tracedService(t)
	rec := recorderWithID(t, runID)
	_, _, err := svc.tailorRendercvResume(context.Background(), escalationMaster(), "Go role", domain.GroundingModerate, traceTestConfig(), nil, rec, &runProvenance{})
	var all []string
	for _, s := range spies {
		all = append(all, s.seen()...)
	}
	return all, err
}

// 036 FR-009: every call of one run carries the same trace id — including the
// selection retries, the premium escalation and the summary re-prompt, none of
// which are visible from the call site that stamped it.
func TestEveryStageOfARunSharesOneTrace(t *testing.T) {
	const runID = "11111111-2222-3333-4444-555555555555"

	traces, err := runTraced(t, runID)
	if err != nil {
		t.Fatalf("run should complete via escalation: %v", err)
	}

	if len(traces) < 5 {
		t.Fatalf("expected at least 5 LLM calls (analyze + 2 economy selects + escalation + summary), got %d", len(traces))
	}
	for i, got := range traces {
		if got != runID {
			t.Errorf("call %d carried trace %q, want the run id %q — a partially correlated run looks complete and is not", i, got, runID)
		}
	}
}

// The re-prompted summary is called out on its own because it is the call most
// easily lost: it is emitted from inside summarize() after a grounding failure,
// which no call site knows about in advance.
func TestSummaryRepromptKeepsTheRunTrace(t *testing.T) {
	const runID = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"

	svc, spies := tracedService(t)
	summary := spies[len(spies)-1]
	rec := recorderWithID(t, runID)

	if _, _, err := svc.tailorRendercvResume(context.Background(), escalationMaster(), "Go role", domain.GroundingModerate, traceTestConfig(), nil, rec, &runProvenance{}); err != nil {
		t.Fatalf("tailorRendercvResume: %v", err)
	}

	seen := summary.seen()
	if len(seen) < 2 {
		t.Fatalf("summary stage made %d calls, want the first plus its re-prompt", len(seen))
	}
	for i, got := range seen {
		if got != runID {
			t.Errorf("summary call %d carried trace %q, want %q", i, got, runID)
		}
	}
}

// 036 FR-011: concurrent runs must not cross-attribute. Ten at once, each with
// its own id, and no call may see an id belonging to another run.
func TestConcurrentRunsCarryDistinctTraces(t *testing.T) {
	ids := []string{
		"00000000-0000-0000-0000-00000000000a",
		"00000000-0000-0000-0000-00000000000b",
		"00000000-0000-0000-0000-00000000000c",
		"00000000-0000-0000-0000-00000000000d",
		"00000000-0000-0000-0000-00000000000e",
		"00000000-0000-0000-0000-00000000000f",
		"00000000-0000-0000-0000-000000000010",
		"00000000-0000-0000-0000-000000000011",
		"00000000-0000-0000-0000-000000000012",
		"00000000-0000-0000-0000-000000000013",
	}

	var wg sync.WaitGroup
	results := make([][]string, len(ids))
	errs := make([]error, len(ids))
	for i, id := range ids {
		wg.Add(1)
		go func(i int, id string) {
			defer wg.Done()
			results[i], errs[i] = runTraced(t, id)
		}(i, id)
	}
	wg.Wait()

	for i, id := range ids {
		if errs[i] != nil {
			t.Fatalf("run %s failed: %v", id, errs[i])
		}
		if len(results[i]) == 0 {
			t.Fatalf("run %s made no LLM calls", id)
		}
		for j, got := range results[i] {
			if got != id {
				t.Errorf("run %s call %d carried trace %q — cross-attribution between concurrent runs", id, j, got)
			}
		}
	}
}
