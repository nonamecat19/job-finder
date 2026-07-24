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
	job              sqlcgen.Job
	updateCalled     bool
	updatedDetail    sqlcgen.UpdateJobDetailParams
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

type enrichFakeEnqueuer struct{ enrichment.Enqueuer }

func (f *enrichFakeEnqueuer) EnqueueContext(ctx context.Context, task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error) {
	return &asynq.TaskInfo{}, nil
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
		adapters.IndeedAdapter{Scraping: scraping.New()}, &enrichFakeEnqueuer{}, 0, nil)

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
		adapters.IndeedAdapter{Scraping: scraping.New()}, &enrichFakeEnqueuer{}, 0, nil)

	payload, _ := json.Marshal(queue.EnrichPayload{JobID: "00000000-0000-0000-0000-000000000001"})
	if err := h.ProcessTask(context.Background(), asynq.NewTask(queue.TypeEnrich, payload)); err != nil {
		t.Fatalf("expected FetchDetail failure to be swallowed (nil returned to asynq), got: %v", err)
	}
	if repo.updateCalled {
		t.Error("expected UpdateJobDetail NOT to be called when FetchDetail fails — existing summary data must be preserved")
	}
}
