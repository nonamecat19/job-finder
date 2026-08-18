package worker

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/jobsources/application/ingest"
	"github.com/job-finder/api/internal/queue"
)

func runID(b byte) pgtype.UUID {
	id := pgtype.UUID{Valid: true}
	id.Bytes[0] = b
	return id
}

func posting(company, title string) dto.NormalizedJob {
	return dto.NormalizedJob{SourceKey: "adzuna", Company: company, Title: title, URL: "https://x/" + title}
}

type recordingEnqueuer struct {
	types []string
}

func (r *recordingEnqueuer) EnqueueContext(_ context.Context, workType string, _ []byte) error {
	r.types = append(r.types, workType)
	return nil
}

func TestEnqueueInsertedRouting(t *testing.T) {
	tests := []struct {
		name        string
		needsDetail bool
		result      ingest.PersistResult
		want        []string
	}{
		{
			name:        "full postings queue match and ghost",
			needsDetail: false,
			result:      ingest.PersistResult{Inserted: []ingest.InsertedJob{{JobID: runID(9), Posting: posting("Acme", "Engineer")}}},
			want:        []string{queue.TypeMatch, queue.TypeGhostScore},
		},
		{
			name:        "list-only postings queue enrich only",
			needsDetail: true,
			result:      ingest.PersistResult{Inserted: []ingest.InsertedJob{{JobID: runID(9), Posting: posting("Acme", "Engineer")}}},
			want:        []string{queue.TypeEnrich},
		},
		{
			name:   "a rolled-back run queues nothing",
			result: ingest.PersistResult{},
		},
		{
			name:   "reposted and merged postings queue nothing",
			result: ingest.PersistResult{Reposted: 7, Merged: 3, Skipped: 2},
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
