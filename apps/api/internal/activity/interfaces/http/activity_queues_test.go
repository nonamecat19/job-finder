package http_test

import (
	"errors"
	"net/http"
	"testing"

	activityhttp "github.com/job-finder/api/internal/activity/interfaces/http"
	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/events"
	"github.com/job-finder/api/internal/platform/llm"
	"github.com/job-finder/api/internal/queue"
	"github.com/job-finder/api/internal/testutil"
)

type fakeInspector struct {
	infos map[string]events.QueueInfo
	errs  map[string]error
}

func (f *fakeInspector) QueueDepth(qname string) (events.QueueInfo, error) {
	if err, ok := f.errs[qname]; ok {
		return events.QueueInfo{}, err
	}
	if info, ok := f.infos[qname]; ok {
		return info, nil
	}
	return events.QueueInfo{}, nil
}

type fakeResolver struct{ class llm.ProviderClass }

func (f fakeResolver) ProviderClass() llm.ProviderClass { return f.class }

func testPolicies() []queue.TaskPolicy {
	return []queue.TaskPolicy{
		{TaskType: queue.TypeIngest, Queue: queue.QueueIngest, Concurrency: 2},
		{TaskType: queue.TypeMatch, Queue: queue.QueueMatch, Concurrency: 3},
		{TaskType: queue.TypeGenerate, Queue: queue.QueueGenerate, Concurrency: 3},
		{TaskType: queue.TypeEnrich, Queue: queue.QueueEnrich, Concurrency: 1},
		{TaskType: queue.TypeSalaryInfer, Queue: queue.QueueSalaryInfer, Concurrency: 3},
		{TaskType: queue.TypeGhostScore, Queue: queue.QueueGhostScore, Concurrency: 3},
	}
}

func TestActivityQueues_FixedOrderingAndProviderClass(t *testing.T) {
	insp := &fakeInspector{infos: map[string]events.QueueInfo{}}
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

	wantOrder := []string{"ingest", "match", "generate", "enrich", "salary", "ghost"}
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
		t.Errorf("expected match concurrency 3, got %d", got)
	}
	if got := byName["generate"].Concurrency; got != 3 {
		t.Errorf("expected generate concurrency 3, got %d", got)
	}
}

func TestActivityQueues_PendingAndActiveFromMessageCounts(t *testing.T) {
	insp := &fakeInspector{infos: map[string]events.QueueInfo{
		"match": {MessagesReady: 100, MessagesUnacked: 3},
	}}
	h := activityhttp.NewActivityHandler(nil, nil, insp, testPolicies(), nil)
	r := testutil.SetupRouter(h.Mount)

	w := testutil.DoRequest(r, "GET", "/api/activity/queues", nil, nil)
	var out dto.QueueBacklogResponse
	testutil.ParseJSON(w, &out)

	for _, q := range out.Queues {
		if q.Queue == "match" {
			if q.Pending != 100 {
				t.Errorf("expected pending 100, got %d", q.Pending)
			}
			if q.Active != 3 {
				t.Errorf("expected active 3, got %d", q.Active)
			}
		}
	}
}

func TestActivityQueues_SingleFailingQueueYieldsPerEntryError(t *testing.T) {
	insp := &fakeInspector{
		infos: map[string]events.QueueInfo{},
		errs:  map[string]error{"match": errors.New("mgmt api timeout")},
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
