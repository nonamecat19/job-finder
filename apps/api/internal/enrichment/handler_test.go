package enrichment_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/enrichment"
	"github.com/job-finder/api/internal/jobsources/adapters"
	"github.com/job-finder/api/internal/queue"
	"github.com/job-finder/api/internal/scraping"
)

type enrichFakeRepo struct {
	enrichment.Repository
	job           sqlcgen.Job
	updateCalled  bool
	updatedDetail sqlcgen.UpdateJobDetailParams
}

func (f *enrichFakeRepo) GetJobByID(ctx context.Context, id pgtype.UUID) (sqlcgen.Job, error) {
	return f.job, nil
}

func (f *enrichFakeRepo) UpdateJobDetail(ctx context.Context, arg sqlcgen.UpdateJobDetailParams) (sqlcgen.Job, error) {
	f.updateCalled = true
	f.updatedDetail = arg
	return f.job, nil
}

// InsertActivityRun no-ops: activity.New calls it unconditionally, and these
// tests don't assert on activity-run bookkeeping.
func (f *enrichFakeRepo) InsertActivityRun(ctx context.Context, arg sqlcgen.InsertActivityRunParams) (sqlcgen.ActivityRun, error) {
	return sqlcgen.ActivityRun{}, nil
}

type enrichFakeEnqueuer struct {
	enrichment.Enqueuer
	enqueued []string
}

func (f *enrichFakeEnqueuer) EnqueueContext(ctx context.Context, task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error) {
	f.enqueued = append(f.enqueued, task.Type())
	return &asynq.TaskInfo{}, nil
}

func (f *enrichFakeEnqueuer) has(taskType string) bool {
	for _, t := range f.enqueued {
		if t == taskType {
			return true
		}
	}
	return false
}

func newIndeedEnrichTestUUID() pgtype.UUID {
	return pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
}

func TestEnrichIndeed_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><body><div id="jobDescriptionText"><p>Full Go role description text long enough to count as real detail content here.</p></div></body></html>`))
	}))
	defer srv.Close()

	repo := &enrichFakeRepo{job: sqlcgen.Job{
		ID:        newIndeedEnrichTestUUID(),
		SourceKey: "indeed",
		Url:       srv.URL + "/viewjob?jk=abc123",
		Title:     "Go Developer",
		Company:   "Acme",
	}}
	h := enrichment.NewHandler(repo, nil, adapters.DjinniAdapter{}, adapters.DouAdapter{}, adapters.WorkUaAdapter{},
		adapters.IndeedAdapter{Scraping: scraping.New()}, adapters.RemoteOKAdapter{}, adapters.GlassdoorAdapter{}, adapters.JobLeadsAdapter{}, &enrichFakeEnqueuer{}, 0, nil)

	payload, _ := json.Marshal(queue.EnrichPayload{JobID: "00000000-0000-0000-0000-000000000001"})
	if err := h.ProcessTask(context.Background(), asynq.NewTask(queue.TypeEnrich, payload)); err != nil {
		t.Fatalf("ProcessTask returned error: %v", err)
	}
	if !repo.updateCalled {
		t.Fatal("expected UpdateJobDetail to be called for an indeed job")
	}
	if repo.updatedDetail.Description == "" {
		t.Error("expected non-empty description to be persisted")
	}
}

func TestEnrichIndeed_FetchDetailFailureDoesNotPropagate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("<html><body>gone</body></html>"))
	}))
	defer srv.Close()

	repo := &enrichFakeRepo{job: sqlcgen.Job{
		ID:        newIndeedEnrichTestUUID(),
		SourceKey: "indeed",
		Url:       srv.URL + "/viewjob?jk=gone",
	}}
	h := enrichment.NewHandler(repo, nil, adapters.DjinniAdapter{}, adapters.DouAdapter{}, adapters.WorkUaAdapter{},
		adapters.IndeedAdapter{Scraping: scraping.New()}, adapters.RemoteOKAdapter{}, adapters.GlassdoorAdapter{}, adapters.JobLeadsAdapter{}, &enrichFakeEnqueuer{}, 0, nil)

	payload, _ := json.Marshal(queue.EnrichPayload{JobID: "00000000-0000-0000-0000-000000000001"})
	if err := h.ProcessTask(context.Background(), asynq.NewTask(queue.TypeEnrich, payload)); err != nil {
		t.Fatalf("expected FetchDetail failure to be swallowed (nil returned to asynq), got: %v", err)
	}
	if repo.updateCalled {
		t.Error("expected UpdateJobDetail NOT to be called when FetchDetail fails — existing summary data must be preserved")
	}
}

func TestEnrichRemoteOK_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"legal": "notice"},
			{"id": "1000001", "position": "Senior Golang Developer", "company": "NovaTech LLC", "location": "", "description": "Full remote Go role description text long enough to count as real detail content.", "url": "https://remoteok.com/remote-jobs/1000001-senior-golang-developer-novatech-llc", "date": "2026-07-20T10:00:00+00:00", "tags": ["golang"]}
		]`))
	}))
	defer srv.Close()

	repo := &enrichFakeRepo{job: sqlcgen.Job{
		ID:        newIndeedEnrichTestUUID(),
		SourceKey: "remoteok",
		Url:       "https://remoteok.com/remote-jobs/1000001-senior-golang-developer-novatech-llc",
		Title:     "Senior Golang Developer",
		Company:   "NovaTech LLC",
	}}
	h := enrichment.NewHandler(repo, nil, adapters.DjinniAdapter{}, adapters.DouAdapter{}, adapters.WorkUaAdapter{},
		adapters.IndeedAdapter{}, adapters.RemoteOKAdapter{Scraping: scraping.New(), APIURL: srv.URL}, adapters.GlassdoorAdapter{}, adapters.JobLeadsAdapter{}, &enrichFakeEnqueuer{}, 0, nil)

	payload, _ := json.Marshal(queue.EnrichPayload{JobID: "00000000-0000-0000-0000-000000000001"})
	if err := h.ProcessTask(context.Background(), asynq.NewTask(queue.TypeEnrich, payload)); err != nil {
		t.Fatalf("ProcessTask returned error: %v", err)
	}
	if !repo.updateCalled {
		t.Fatal("expected UpdateJobDetail to be called for a remoteok job still present in the feed")
	}
	if repo.updatedDetail.Description == "" {
		t.Error("expected non-empty description to be persisted")
	}
}

func TestEnrichRemoteOK_RotatedOutDoesNotUpdate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"legal": "notice"}]`))
	}))
	defer srv.Close()

	repo := &enrichFakeRepo{job: sqlcgen.Job{
		ID:        newIndeedEnrichTestUUID(),
		SourceKey: "remoteok",
		Url:       "https://remoteok.com/remote-jobs/9999999-gone",
	}}
	h := enrichment.NewHandler(repo, nil, adapters.DjinniAdapter{}, adapters.DouAdapter{}, adapters.WorkUaAdapter{},
		adapters.IndeedAdapter{}, adapters.RemoteOKAdapter{Scraping: scraping.New(), APIURL: srv.URL}, adapters.GlassdoorAdapter{}, adapters.JobLeadsAdapter{}, &enrichFakeEnqueuer{}, 0, nil)

	payload, _ := json.Marshal(queue.EnrichPayload{JobID: "00000000-0000-0000-0000-000000000001"})
	if err := h.ProcessTask(context.Background(), asynq.NewTask(queue.TypeEnrich, payload)); err != nil {
		t.Fatalf("expected rotated-out listing to be swallowed (nil returned to asynq), got: %v", err)
	}
	if repo.updateCalled {
		t.Error("expected UpdateJobDetail NOT to be called when the listing has rotated out — existing summary data must be preserved")
	}
}

// enrichJobLeadsFakeSession always returns a fixed cookie, bypassing the
// real login flow so these tests only exercise FetchDetail/enrichment.
type enrichJobLeadsFakeSession struct{ cookie string }

func (s *enrichJobLeadsFakeSession) Ensure(_ context.Context) (string, error)  { return s.cookie, nil }
func (s *enrichJobLeadsFakeSession) Refresh(_ context.Context) (string, error) { return s.cookie, nil }

func TestEnrichJobLeads_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html><body><article class="job-detail">
			<div class="job-detail__description"><p>Full Go role description text long enough to count as real detail content here.</p></div>
			<time class="job-detail__date" datetime="2026-07-20T00:00:00Z"></time>
		</article></body></html>`))
	}))
	defer srv.Close()

	repo := &enrichFakeRepo{job: sqlcgen.Job{
		ID:        newIndeedEnrichTestUUID(),
		SourceKey: "jobleads",
		Url:       srv.URL + "/job/senior-golang-engineer-abc123",
		Title:     "Senior Golang Engineer",
		Company:   "NovaTech LLC",
	}}
	h := enrichment.NewHandler(repo, nil, adapters.DjinniAdapter{}, adapters.DouAdapter{}, adapters.WorkUaAdapter{},
		adapters.IndeedAdapter{}, adapters.RemoteOKAdapter{}, adapters.GlassdoorAdapter{},
		adapters.JobLeadsAdapter{Scraping: scraping.New(), Session: &enrichJobLeadsFakeSession{cookie: "cookie-xyz"}}, &enrichFakeEnqueuer{}, 0, nil)

	payload, _ := json.Marshal(queue.EnrichPayload{JobID: "00000000-0000-0000-0000-000000000001"})
	if err := h.ProcessTask(context.Background(), asynq.NewTask(queue.TypeEnrich, payload)); err != nil {
		t.Fatalf("ProcessTask returned error: %v", err)
	}
	if !repo.updateCalled {
		t.Fatal("expected UpdateJobDetail to be called for a jobleads job still available")
	}
	if repo.updatedDetail.Description == "" {
		t.Error("expected non-empty description to be persisted")
	}
}

func TestEnrichJobLeads_UnavailableDoesNotUpdate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html><body><p>This job has been removed and is no longer available.</p></body></html>`))
	}))
	defer srv.Close()

	repo := &enrichFakeRepo{job: sqlcgen.Job{
		ID:        newIndeedEnrichTestUUID(),
		SourceKey: "jobleads",
		Url:       srv.URL + "/job/gone",
	}}
	h := enrichment.NewHandler(repo, nil, adapters.DjinniAdapter{}, adapters.DouAdapter{}, adapters.WorkUaAdapter{},
		adapters.IndeedAdapter{}, adapters.RemoteOKAdapter{}, adapters.GlassdoorAdapter{},
		adapters.JobLeadsAdapter{Scraping: scraping.New(), Session: &enrichJobLeadsFakeSession{cookie: "cookie-xyz"}}, &enrichFakeEnqueuer{}, 0, nil)

	payload, _ := json.Marshal(queue.EnrichPayload{JobID: "00000000-0000-0000-0000-000000000001"})
	if err := h.ProcessTask(context.Background(), asynq.NewTask(queue.TypeEnrich, payload)); err != nil {
		t.Fatalf("expected unavailable listing to be swallowed (nil returned to asynq), got: %v", err)
	}
	if repo.updateCalled {
		t.Error("expected UpdateJobDetail NOT to be called when the listing is unavailable — existing summary data must be preserved")
	}
}

// Ingestion no longer enqueues match/ghost for a NeedsDetail source — it
// hands both to this handler. So enrichment owes every job it touches a match
// and a ghost task, including on the give-up paths where the detail fetch
// failed: otherwise the job carries no score at all and never surfaces in the
// score-sorted feed.
func TestEnrich_EnqueuesDownstreamEvenWhenFetchDetailFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("<html><body>gone</body></html>"))
	}))
	defer srv.Close()

	repo := &enrichFakeRepo{job: sqlcgen.Job{
		ID:        newIndeedEnrichTestUUID(),
		SourceKey: "indeed",
		Url:       srv.URL + "/viewjob?jk=gone",
	}}
	enq := &enrichFakeEnqueuer{}
	h := enrichment.NewHandler(repo, nil, adapters.DjinniAdapter{}, adapters.DouAdapter{}, adapters.WorkUaAdapter{},
		adapters.IndeedAdapter{Scraping: scraping.New()}, adapters.RemoteOKAdapter{}, adapters.GlassdoorAdapter{}, adapters.JobLeadsAdapter{}, enq, 0, nil)

	payload, _ := json.Marshal(queue.EnrichPayload{JobID: "00000000-0000-0000-0000-000000000001"})
	if err := h.ProcessTask(context.Background(), asynq.NewTask(queue.TypeEnrich, payload)); err != nil {
		t.Fatalf("ProcessTask returned error: %v", err)
	}
	if !enq.has(queue.TypeMatch) {
		t.Error("expected a match task even though the detail fetch failed")
	}
	if !enq.has(queue.TypeGhostScore) {
		t.Error("expected a ghost_score task even though the detail fetch failed")
	}
}

// A source with no enrich branch must not strand the job either: ingestion
// skipped its match/ghost on the promise that this handler would run them.
func TestEnrich_UnknownSourceStillEnqueuesDownstream(t *testing.T) {
	repo := &enrichFakeRepo{job: sqlcgen.Job{
		ID:        newIndeedEnrichTestUUID(),
		SourceKey: "some-future-source",
		Url:       "https://example.test/job/1",
	}}
	enq := &enrichFakeEnqueuer{}
	h := enrichment.NewHandler(repo, nil, adapters.DjinniAdapter{}, adapters.DouAdapter{}, adapters.WorkUaAdapter{},
		adapters.IndeedAdapter{}, adapters.RemoteOKAdapter{}, adapters.GlassdoorAdapter{}, adapters.JobLeadsAdapter{}, enq, 0, nil)

	payload, _ := json.Marshal(queue.EnrichPayload{JobID: "00000000-0000-0000-0000-000000000001"})
	if err := h.ProcessTask(context.Background(), asynq.NewTask(queue.TypeEnrich, payload)); err != nil {
		t.Fatalf("ProcessTask returned error: %v", err)
	}
	if repo.updateCalled {
		t.Error("expected no detail update for a source with no enrich branch")
	}
	if !enq.has(queue.TypeMatch) || !enq.has(queue.TypeGhostScore) {
		t.Errorf("expected match + ghost_score to still be enqueued, got %v", enq.enqueued)
	}
}
