package http_test

import (
	"errors"
	"net/http"
	"testing"

	"github.com/hibiken/asynq"

	activityhttp "github.com/job-finder/api/internal/activity/interfaces/http"
	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/platform/llm"
	"github.com/job-finder/api/internal/queue"
	"github.com/job-finder/api/internal/testutil"
)

type fakeInspector struct {
	infos map[string]*asynq.QueueInfo
	errs  map[string]error
}

func (f *fakeInspector) CancelProcessing(id string) error  { return nil }
func (f *fakeInspector) DeleteTask(queue, id string) error { return nil }
func (f *fakeInspector) GetQueueInfo(qname string) (*asynq.QueueInfo, error) {
	if err, ok := f.errs[qname]; ok {
		return nil, err
	}
	if info, ok := f.infos[qname]; ok {
		return info, nil
	}
	return &asynq.QueueInfo{Queue: qname}, nil
}

type fakeResolver struct{ class llm.ProviderClass }

func (f fakeResolver) ProviderClass() llm.ProviderClass { return f.class }

func testPolicies() []queue.TaskPolicy {
	return []queue.TaskPolicy{
		{TaskType: queue.TypeIngest, Queue: queue.QueueIngest, LocalConcurrency: 2, HostedConcurrency: 2},
		{TaskType: queue.TypeMatch, Queue: queue.QueueMatch, LocalConcurrency: 1, HostedConcurrency: 3},
		{TaskType: queue.TypeGenerate, Queue: queue.QueueGenerate, LocalConcurrency: 1, HostedConcurrency: 3},
		{TaskType: queue.TypeEnrich, Queue: queue.QueueEnrich, LocalConcurrency: 1, HostedConcurrency: 1},
		{TaskType: queue.TypeSalaryInfer, Queue: queue.QueueSalaryInfer, LocalConcurrency: 1, HostedConcurrency: 3},
		{TaskType: queue.TypeGhostScore, Queue: queue.QueueGhostScore, LocalConcurrency: 1, HostedConcurrency: 3},
	}
}

func TestActivityQueues_FixedOrderingAndProviderClass(t *testing.T) {
	insp := &fakeInspector{infos: map[string]*asynq.QueueInfo{}}
	resolvers := map[string]queue.ClassResolver{
		queue.QueueMatch:       fakeResolver{llm.ProviderClassHosted},
		queue.QueueGenerate:    fakeResolver{llm.ProviderClassLocal},
		queue.QueueSalaryInfer: fakeResolver{llm.ProviderClassHosted},
		queue.QueueGhostScore:  fakeResolver{llm.ProviderClassLocal},
	}
	h := activityhttp.NewActivityHandler(nil, nil, insp, testPolicies(), resolvers)
	r := testutil.SetupRouter(h.Mount)

	w := testutil.DoRequest(r, "GET", "/api/activity/queues", nil, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var out dto.QueueBacklogResponse
	testutil.ParseJSON(w, &out)

	wantOrder := []string{"ingest", "match", "generate", "enrich", "salary:infer", "ghost:score"}
	if len(out.Queues) != len(wantOrder) {
		t.Fatalf("expected %d queues, got %d", len(wantOrder), len(out.Queues))
	}
	for i, q := range out.Queues {
		if q.Queue != wantOrder[i] {
			t.Errorf("queue[%d] = %q, want %q", i, q.Queue, wantOrder[i])
		}
	}

	byName := map[string]dto.QueueBacklogDto{}
	for _, q := range out.Queues {
		byName[q.Queue] = q
	}

	if byName["ingest"].ProviderClass != nil {
		t.Errorf("expected nil providerClass for ingest (non-LLM), got %v", *byName["ingest"].ProviderClass)
	}
	if byName["enrich"].ProviderClass != nil {
		t.Errorf("expected nil providerClass for enrich (non-LLM), got %v", *byName["enrich"].ProviderClass)
	}
	if got := byName["match"].ProviderClass; got == nil || *got != "hosted" {
		t.Errorf("expected match providerClass hosted, got %v", got)
	}
	if got := byName["match"].Concurrency; got != 3 {
		t.Errorf("expected match concurrency 3 (hosted), got %d", got)
	}
	if got := byName["generate"].Concurrency; got != 1 {
		t.Errorf("expected generate concurrency 1 (local), got %d", got)
	}
}

func TestActivityQueues_EtaNilWhenThroughputOrPendingZero(t *testing.T) {
	insp := &fakeInspector{infos: map[string]*asynq.QueueInfo{
		"match":       {Queue: "match", Pending: 100, Processed: 0},
		"ghost:score": {Queue: "ghost:score", Pending: 0, Processed: 50},
	}}
	h := activityhttp.NewActivityHandler(nil, nil, insp, testPolicies(), nil)
	r := testutil.SetupRouter(h.Mount)

	w := testutil.DoRequest(r, "GET", "/api/activity/queues", nil, nil)
	var out dto.QueueBacklogResponse
	testutil.ParseJSON(w, &out)

	for _, q := range out.Queues {
		if q.Queue == "match" && q.EtaSeconds != nil {
			t.Errorf("expected nil etaSeconds when throughput is 0, got %v", *q.EtaSeconds)
		}
		if q.Queue == "ghost:score" && q.EtaSeconds != nil {
			t.Errorf("expected nil etaSeconds when pending is 0, got %v", *q.EtaSeconds)
		}
	}
}

func TestActivityQueues_SingleFailingQueueYieldsPerEntryError(t *testing.T) {
	insp := &fakeInspector{
		infos: map[string]*asynq.QueueInfo{},
		errs:  map[string]error{"match": errors.New("redis timeout")},
	}
	h := activityhttp.NewActivityHandler(nil, nil, insp, testPolicies(), nil)
	r := testutil.SetupRouter(h.Mount)

	w := testutil.DoRequest(r, "GET", "/api/activity/queues", nil, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 despite one queue failing, got %d", w.Code)
	}
	var out dto.QueueBacklogResponse
	testutil.ParseJSON(w, &out)

	for _, q := range out.Queues {
		if q.Queue == "match" {
			if q.Error == nil {
				t.Error("expected per-entry error for match queue")
			}
		} else if q.Error != nil {
			t.Errorf("queue %q: expected no error, got %v", q.Queue, *q.Error)
		}
	}
}

func TestActivityQueues_NilInspectorYields503(t *testing.T) {
	h := activityhttp.NewActivityHandler(nil, nil, nil, testPolicies(), nil)
	r := testutil.SetupRouter(h.Mount)

	w := testutil.DoRequest(r, "GET", "/api/activity/queues", nil, nil)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}
