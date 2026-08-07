package worker

import (
	"context"
	"strconv"
	"testing"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/queue"
)

type fakeBatchRepo struct {
	jobs      map[string]*fakeJob
	calls     []string
	chunkSize []int
	nextID    int
}

type fakeJob struct {
	id            pgtype.UUID
	seenCount     int
	lastSeenRunID pgtype.UUID
}

func newFakeBatchRepo() *fakeBatchRepo {
	return &fakeBatchRepo{jobs: map[string]*fakeJob{}}
}

func (f *fakeBatchRepo) newID() pgtype.UUID {
	f.nextID++
	var id pgtype.UUID
	id.Valid = true
	copy(id.Bytes[:], []byte(strconv.Itoa(f.nextID)+"----------------"))
	return id
}

func (f *fakeBatchRepo) GetJobsByDedupeKeys(_ context.Context, keys []string) ([]sqlcgen.GetJobsByDedupeKeysRow, error) {
	f.calls = append(f.calls, "GetJobsByDedupeKeys")
	f.chunkSize = append(f.chunkSize, len(keys))
	var out []sqlcgen.GetJobsByDedupeKeysRow
	for _, key := range keys {
		if job, ok := f.jobs[key]; ok {
			out = append(out, sqlcgen.GetJobsByDedupeKeysRow{ID: job.id, DedupeKey: key})
		}
	}
	return out, nil
}

func (f *fakeBatchRepo) FindJobsByCompanies(context.Context, sqlcgen.FindJobsByCompaniesParams) ([]sqlcgen.FindJobsByCompaniesRow, error) {
	f.calls = append(f.calls, "FindJobsByCompanies")
	return nil, nil
}

func (f *fakeBatchRepo) BulkInsertJobs(_ context.Context, arg sqlcgen.BulkInsertJobsParams) ([]sqlcgen.BulkInsertJobsRow, error) {
	f.calls = append(f.calls, "BulkInsertJobs")
	var out []sqlcgen.BulkInsertJobsRow
	for i := len(arg.DedupeKeys) - 1; i >= 0; i-- {
		key := arg.DedupeKeys[i]
		if _, conflict := f.jobs[key]; conflict {
			continue
		}
		job := &fakeJob{id: f.newID(), seenCount: 1}
		f.jobs[key] = job
		out = append(out, sqlcgen.BulkInsertJobsRow{ID: job.id, DedupeKey: key})
	}
	return out, nil
}

func (f *fakeBatchRepo) BulkRecordJobReposts(_ context.Context, arg sqlcgen.BulkRecordJobRepostsParams) (int64, error) {
	f.calls = append(f.calls, "BulkRecordJobReposts")
	var affected int64
	for _, key := range arg.DedupeKeys {
		job, ok := f.jobs[key]
		if !ok {
			continue
		}
		if job.lastSeenRunID.Valid && job.lastSeenRunID.Bytes == arg.RunID.Bytes {
			continue
		}
		job.seenCount++
		job.lastSeenRunID = arg.RunID
		affected++
	}
	return affected, nil
}

func (f *fakeBatchRepo) BulkMergeJobBoards(_ context.Context, arg sqlcgen.BulkMergeJobBoardsParams) (int64, error) {
	f.calls = append(f.calls, "BulkMergeJobBoards")
	return int64(len(arg.Ids)), nil
}

func (f *fakeBatchRepo) BulkInsertActivities(_ context.Context, arg sqlcgen.BulkInsertActivitiesParams) ([]sqlcgen.BulkInsertActivitiesRow, error) {
	f.calls = append(f.calls, "BulkInsertActivities")
	out := make([]sqlcgen.BulkInsertActivitiesRow, 0, len(arg.Ops))
	for i := len(arg.Ops) - 1; i >= 0; i-- {
		out = append(out, sqlcgen.BulkInsertActivitiesRow{ID: f.newID(), JobId: arg.JobIds[i], Op: arg.Ops[i]})
	}
	return out, nil
}

func (f *fakeBatchRepo) FinishSourceRunOk(context.Context, sqlcgen.FinishSourceRunOkParams) error {
	f.calls = append(f.calls, "FinishSourceRunOk")
	return nil
}

func (f *fakeBatchRepo) batchCalls() []string {
	var out []string
	for _, c := range f.calls {
		if c != "FinishSourceRunOk" {
			out = append(out, c)
		}
	}
	return out
}

func runID(b byte) pgtype.UUID {
	id := pgtype.UUID{Valid: true}
	id.Bytes[0] = b
	return id
}

func posting(company, title string) dto.NormalizedJob {
	return dto.NormalizedJob{SourceKey: "adzuna", Company: company, Title: title, URL: "https://x/" + title}
}

func postings(n int) []dto.NormalizedJob {
	out := make([]dto.NormalizedJob, 0, n)
	for i := range n {
		out = append(out, posting("Acme", "role-"+strconv.Itoa(i)))
	}
	return out
}

func persist(t *testing.T, repo *fakeBatchRepo, jobs []dto.NormalizedJob, run pgtype.UUID, chunkSize int) PersistResult {
	t.Helper()
	batch := NewPostingBatch(jobs, pgtype.UUID{}, run, false)
	result, err := persistBatch(context.Background(), repo, batch, chunkSize)
	if err != nil {
		t.Fatalf("persistBatch: %v", err)
	}
	return result
}

func TestRepostGuardCountsOncePerRun(t *testing.T) {
	repo := newFakeBatchRepo()
	jobs := postings(1)
	key := DedupeKey(jobs[0].Company, jobs[0].Title, jobs[0].URL)

	persist(t, repo, jobs, runID(1), 0)
	if got := repo.jobs[key].seenCount; got != 1 {
		t.Fatalf("after insert seenCount = %d, want 1", got)
	}

	persist(t, repo, jobs, runID(1), 0)
	persist(t, repo, jobs, runID(1), 0)
	if got := repo.jobs[key].seenCount; got != 2 {
		t.Fatalf("after retries seenCount = %d, want 2", got)
	}

	persist(t, repo, jobs, runID(2), 0)
	if got := repo.jobs[key].seenCount; got != 3 {
		t.Fatalf("after second run seenCount = %d, want 3", got)
	}
}

func TestRepostGuardCountsRowWithNullLastSeenRun(t *testing.T) {
	repo := newFakeBatchRepo()
	jobs := postings(1)
	key := DedupeKey(jobs[0].Company, jobs[0].Title, jobs[0].URL)
	repo.jobs[key] = &fakeJob{id: repo.newID(), seenCount: 1}

	result := persist(t, repo, jobs, runID(1), 0)

	if result.Reposted != 1 {
		t.Fatalf("Reposted = %d, want 1", result.Reposted)
	}
	if got := repo.jobs[key].seenCount; got != 2 {
		t.Fatalf("seenCount = %d, want 2 — NULL must be DISTINCT FROM the run id", got)
	}
}

func TestPersistBatchEmptyRunIssuesNoBatchStatements(t *testing.T) {
	repo := newFakeBatchRepo()
	result := persist(t, repo, nil, runID(1), 0)

	if calls := repo.batchCalls(); len(calls) != 0 {
		t.Fatalf("batch statements = %v, want none", calls)
	}
	if result.Statements != 0 {
		t.Fatalf("Statements = %d, want 0", result.Statements)
	}
}

func TestPersistBatchChunking(t *testing.T) {
	tests := []struct {
		name       string
		count      int
		chunkSize  int
		wantChunks []int
	}{
		{"under one chunk", 3, 5, []int{3}},
		{"exactly one chunk", 5, 5, []int{5}},
		{"exact multiple", 10, 5, []int{5, 5}},
		{"one over", 11, 5, []int{5, 5, 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newFakeBatchRepo()
			result := persist(t, repo, postings(tt.count), runID(1), tt.chunkSize)

			if len(repo.chunkSize) != len(tt.wantChunks) {
				t.Fatalf("chunks = %v, want %v", repo.chunkSize, tt.wantChunks)
			}
			for i, want := range tt.wantChunks {
				if repo.chunkSize[i] != want {
					t.Fatalf("chunks = %v, want %v", repo.chunkSize, tt.wantChunks)
				}
			}
			if len(result.Inserted) != tt.count {
				t.Fatalf("Inserted = %d, want %d", len(result.Inserted), tt.count)
			}
			if want := 3 * len(tt.wantChunks); result.Statements != want {
				t.Fatalf("Statements = %d, want %d", result.Statements, want)
			}
		})
	}
}

func TestNewPostingBatchKeepsFirstOccurrence(t *testing.T) {
	first := posting("Acme", "Engineer")
	first.Description = "first"
	second := posting("Acme", "Engineer")
	second.Description = "second"

	batch := NewPostingBatch([]dto.NormalizedJob{first, second, posting("Acme", "Designer")}, pgtype.UUID{}, runID(1), false)

	if len(batch.Postings) != 2 {
		t.Fatalf("Postings = %d, want 2", len(batch.Postings))
	}
	if batch.Postings[0].Description != "first" {
		t.Fatalf("kept %q, want the first occurrence — adapters return source-ranked order", batch.Postings[0].Description)
	}
	if batch.Skipped != 1 {
		t.Fatalf("Skipped = %d, want 1", batch.Skipped)
	}
	if batch.Found != 3 {
		t.Fatalf("Found = %d, want 3", batch.Found)
	}
}

func TestPersistBatchCollapsesInBatchDuplicates(t *testing.T) {
	repo := newFakeBatchRepo()
	dup := posting("Acme", "Engineer")

	result := persist(t, repo, []dto.NormalizedJob{dup, dup, posting("Acme", "Designer")}, runID(1), 0)

	if len(result.Inserted) != 2 {
		t.Fatalf("Inserted = %d, want 2", len(result.Inserted))
	}
	if result.Skipped != 1 {
		t.Fatalf("Skipped = %d, want 1", result.Skipped)
	}
	if result.Reposted != 0 {
		t.Fatalf("Reposted = %d, want 0 — an in-batch duplicate is not a re-sighting", result.Reposted)
	}
}

func TestPersistBatchCorrelatesInsertsByKey(t *testing.T) {
	repo := newFakeBatchRepo()
	jobs := []dto.NormalizedJob{posting("Acme", "Engineer"), posting("Acme", "Designer")}

	result := persist(t, repo, jobs, runID(1), 0)

	for _, ins := range result.Inserted {
		if want := DedupeKey(ins.Posting.Company, ins.Posting.Title, ins.Posting.URL); ins.DedupeKey != want {
			t.Fatalf("posting %q paired with key of another row", ins.Posting.Title)
		}
		if id, ok := ins.ActivityIDs[opMatch]; !ok || !id.Valid {
			t.Fatalf("posting %q has no match activity id", ins.Posting.Title)
		}
		if id, ok := ins.ActivityIDs[opGhost]; !ok || !id.Valid {
			t.Fatalf("posting %q has no ghost activity id", ins.Posting.Title)
		}
	}
}

type recordingEnqueuer struct {
	types []string
}

func (r *recordingEnqueuer) EnqueueContext(_ context.Context, task *asynq.Task, _ ...asynq.Option) (*asynq.TaskInfo, error) {
	r.types = append(r.types, task.Type())
	return &asynq.TaskInfo{}, nil
}

func TestEnqueueInsertedRouting(t *testing.T) {
	tests := []struct {
		name        string
		needsDetail bool
		result      PersistResult
		want        []string
	}{
		{
			name:        "full postings queue match and ghost",
			needsDetail: false,
			result:      PersistResult{Inserted: []InsertedJob{{JobID: runID(9), Posting: posting("Acme", "Engineer")}}},
			want:        []string{queue.TypeMatch, queue.TypeGhostScore},
		},
		{
			name:        "list-only postings queue enrich only",
			needsDetail: true,
			result:      PersistResult{Inserted: []InsertedJob{{JobID: runID(9), Posting: posting("Acme", "Engineer")}}},
			want:        []string{queue.TypeEnrich},
		},
		{
			name:   "a rolled-back run queues nothing",
			result: PersistResult{},
		},
		{
			name:   "reposted and merged postings queue nothing",
			result: PersistResult{Reposted: 7, Merged: 3, Skipped: 2},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			enq := &recordingEnqueuer{}
			h := NewHandler(nil, nil, nil, enq)

			h.enqueueInserted(context.Background(), tt.result, tt.needsDetail)

			if len(enq.types) != len(tt.want) {
				t.Fatalf("enqueued %v, want %v", enq.types, tt.want)
			}
			for i, want := range tt.want {
				if enq.types[i] != want {
					t.Fatalf("enqueued %v, want %v", enq.types, tt.want)
				}
			}
		})
	}
}
