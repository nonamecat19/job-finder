package http_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/ghostjob/application"
	ghostjobhttp "github.com/job-finder/api/internal/ghostjob/interfaces/http"
	"github.com/job-finder/api/internal/testutil"
)

type fakeGhostProvider struct {
	out   dto.JobSignalDto
	err   error
	calls int32
}

func (f *fakeGhostProvider) ScoreJob(ctx context.Context, jobID string) (dto.JobSignalDto, error) {
	atomic.AddInt32(&f.calls, 1)
	return f.out, f.err
}

func TestGhostScore_HappyPath(t *testing.T) {
	fake := &fakeGhostProvider{out: dto.JobSignalDto{ID: "sig-1", JobID: "job-1", Kind: "ghost", Score: 82}}
	h := &ghostjobhttp.GhostJobHandler{Ghost: fake}
	r := testutil.SetupRouter(h.Mount)

	w := testutil.DoRequestJSON(r, "POST", "/api/jobs/job-1/ghost-score", nil, map[string]string{"id": "job-1"})
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var out dto.JobSignalDto
	testutil.ParseJSON(w, &out)
	if out.Score != 82 {
		t.Fatalf("expected score 82, got %d", out.Score)
	}
}

// Story 3, scenario 4: the scoring model is unreachable -> an error is
// shown, and (by construction — the service never persists on error) the
// previously stored score is left intact.
func TestGhostScore_ModelUnreachableReturnsError(t *testing.T) {
	fake := &fakeGhostProvider{err: errors.New("ollama: chat request failed: connection refused")}
	h := &ghostjobhttp.GhostJobHandler{Ghost: fake}
	r := testutil.SetupRouter(h.Mount)

	w := testutil.DoRequestJSON(r, "POST", "/api/jobs/job-1/ghost-score", nil, map[string]string{"id": "job-1"})
	if w.Code != 500 {
		t.Fatalf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGhostScore_DeclinedToScoreReturns422(t *testing.T) {
	fake := &fakeGhostProvider{err: application.ErrDeclinedToScore}
	h := &ghostjobhttp.GhostJobHandler{Ghost: fake}
	r := testutil.SetupRouter(h.Mount)

	w := testutil.DoRequestJSON(r, "POST", "/api/jobs/job-1/ghost-score", nil, map[string]string{"id": "job-1"})
	if w.Code != 422 {
		t.Fatalf("expected 422, got %d: %s", w.Code, w.Body.String())
	}
}

// Story 3, scenario 3: a concurrent double-invocation is idempotent via the
// (jobId, kind) upsert — at the handler layer this just means two
// concurrent requests each get a normal response; the exactly-one-row
// guarantee lives in the SQL unique constraint + ON CONFLICT (see
// ghostjob/service_test.go's TestScoreJob_SecondUpsertReplacesFirst).
func TestGhostScore_ConcurrentInvocationsBothSucceed(t *testing.T) {
	fake := &fakeGhostProvider{out: dto.JobSignalDto{ID: "sig-1", JobID: "job-1", Kind: "ghost", Score: 60}}
	h := &ghostjobhttp.GhostJobHandler{Ghost: fake}
	r := testutil.SetupRouter(h.Mount)

	done := make(chan int, 2)
	for i := 0; i < 2; i++ {
		go func() {
			w := testutil.DoRequestJSON(r, "POST", "/api/jobs/job-1/ghost-score", nil, map[string]string{"id": "job-1"})
			done <- w.Code
		}()
	}
	for i := 0; i < 2; i++ {
		if code := <-done; code != 200 {
			t.Errorf("expected 200, got %d", code)
		}
	}
	if fake.calls != 2 {
		t.Fatalf("expected 2 calls into the service, got %d", fake.calls)
	}
}
