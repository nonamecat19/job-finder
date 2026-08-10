package ingest

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbutil"
	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/jobsources/domain"
)

const DefaultChunkSize = 500

const (
	OpMatch  = "match"
	OpGhost  = "ghost_score"
	OpEnrich = "enrich"
)

type PostingBatch struct {
	Postings       []dto.NormalizedJob
	Keys           []string
	Found          int
	Skipped        int
	SubscriptionID pgtype.UUID
	RunID          pgtype.UUID
	NeedsDetail    bool
}

type Classification struct {
	Known        map[string]pgtype.UUID
	NewPostings  []dto.NormalizedJob
	MergeTargets map[string]pgtype.UUID
}

type InsertedJob struct {
	JobID       pgtype.UUID
	DedupeKey   string
	Posting     dto.NormalizedJob
	ActivityIDs map[string]pgtype.UUID
}

type PersistResult struct {
	Inserted   []InsertedJob
	Reposted   int
	Merged     int
	Skipped    int
	Statements int
}

func NewPostingBatch(jobs []dto.NormalizedJob, subscriptionID, runID pgtype.UUID, needsDetail bool) PostingBatch {
	batch := PostingBatch{
		Found:          len(jobs),
		SubscriptionID: subscriptionID,
		RunID:          runID,
		NeedsDetail:    needsDetail,
	}
	seen := make(map[string]struct{}, len(jobs))
	for _, j := range jobs {
		key := DedupeKey(j.Company, j.Title, j.URL)
		if _, dup := seen[key]; dup {
			batch.Skipped++
			continue
		}
		seen[key] = struct{}{}
		batch.Postings = append(batch.Postings, j)
		batch.Keys = append(batch.Keys, key)
	}
	return batch
}

func PersistBatch(ctx context.Context, q domain.BatchRepository, batch PostingBatch, chunkSize int) (PersistResult, error) {
	if chunkSize <= 0 {
		chunkSize = DefaultChunkSize
	}

	result := PersistResult{Skipped: batch.Skipped}
	for start := 0; start < len(batch.Postings); start += chunkSize {
		end := min(start+chunkSize, len(batch.Postings))
		if err := persistChunk(ctx, q, batch, batch.Postings[start:end], batch.Keys[start:end], &result); err != nil {
			return PersistResult{}, err
		}
	}

	if err := q.FinishSourceRunOk(ctx, sqlcgen.FinishSourceRunOkParams{
		ID:    batch.RunID,
		Found: int32(batch.Found),
		New:   int32(len(result.Inserted)),
	}); err != nil {
		return PersistResult{}, fmt.Errorf("ingestion: finish source run: %w", err)
	}
	return result, nil
}

func persistChunk(ctx context.Context, q domain.BatchRepository, batch PostingBatch, postings []dto.NormalizedJob, keys []string, result *PersistResult) error {
	classified, statements, err := classify(ctx, q, postings, keys)
	if err != nil {
		return err
	}
	result.Statements += statements

	inserted, statements, err := insertNew(ctx, q, batch, classified)
	if err != nil {
		return err
	}
	result.Statements += statements

	if len(classified.Known) > 0 {
		repostKeys := make([]string, 0, len(classified.Known))
		for key := range classified.Known {
			repostKeys = append(repostKeys, key)
		}
		affected, err := q.BulkRecordJobReposts(ctx, sqlcgen.BulkRecordJobRepostsParams{
			DedupeKeys:     repostKeys,
			SubscriptionID: batch.SubscriptionID,
			RunID:          batch.RunID,
		})
		if err != nil {
			return fmt.Errorf("ingestion: record reposts: %w", err)
		}
		result.Statements++
		result.Reposted += int(affected)
	}

	if len(classified.MergeTargets) > 0 {
		merged, err := mergeBoards(ctx, q, postings, classified.MergeTargets)
		if err != nil {
			return err
		}
		result.Statements++
		result.Merged += merged
	}

	if len(inserted) > 0 {
		if err := attachActivities(ctx, q, batch, inserted); err != nil {
			return err
		}
		result.Statements++
	}

	result.Inserted = append(result.Inserted, inserted...)
	return nil
}

func classify(ctx context.Context, q domain.BatchRepository, postings []dto.NormalizedJob, keys []string) (Classification, int, error) {
	rows, err := q.GetJobsByDedupeKeys(ctx, keys)
	if err != nil {
		return Classification{}, 0, fmt.Errorf("ingestion: classify postings: %w", err)
	}
	statements := 1

	known := make(map[string]pgtype.UUID, len(rows))
	for _, row := range rows {
		known[row.DedupeKey] = row.ID
	}

	var newPostings []dto.NormalizedJob
	for i, j := range postings {
		if _, ok := known[keys[i]]; !ok {
			newPostings = append(newPostings, j)
		}
	}

	targets, lookups, err := FindMergeCandidates(ctx, q, newPostings)
	if err != nil {
		return Classification{}, 0, err
	}
	statements += lookups

	return Classification{Known: known, NewPostings: newPostings, MergeTargets: targets}, statements, nil
}

func insertNew(ctx context.Context, q domain.BatchRepository, batch PostingBatch, classified Classification) ([]InsertedJob, int, error) {
	params := sqlcgen.BulkInsertJobsParams{SubscriptionID: batch.SubscriptionID}
	candidates := make(map[string]dto.NormalizedJob, len(classified.NewPostings))

	for _, j := range classified.NewPostings {
		key := DedupeKey(j.Company, j.Title, j.URL)
		if _, merged := classified.MergeTargets[key]; merged {
			continue
		}
		raw, err := json.Marshal(j.Raw)
		if err != nil {
			raw = []byte("{}")
		}
		candidates[key] = j
		params.DedupeKeys = append(params.DedupeKeys, key)
		params.SourceKeys = append(params.SourceKeys, j.SourceKey)
		params.ExternalIds = append(params.ExternalIds, deref(j.ExternalID))
		params.Titles = append(params.Titles, j.Title)
		params.Companies = append(params.Companies, j.Company)
		params.Locations = append(params.Locations, deref(j.Location))
		params.Remotes = append(params.Remotes, j.Remote)
		params.SalaryRaws = append(params.SalaryRaws, deref(j.SalaryRaw))
		params.Urls = append(params.Urls, j.URL)
		params.Descriptions = append(params.Descriptions, j.Description)
		params.Raws = append(params.Raws, raw)
		params.PostedAts = append(params.PostedAts, dbutil.TimestampFromPtr(j.PostedAt))
	}

	if len(params.DedupeKeys) == 0 {
		return nil, 0, nil
	}

	rows, err := q.BulkInsertJobs(ctx, params)
	if err != nil {
		return nil, 0, fmt.Errorf("ingestion: insert jobs: %w", err)
	}

	inserted := make([]InsertedJob, 0, len(rows))
	for _, row := range rows {
		posting, ok := candidates[row.DedupeKey]
		if !ok {
			continue
		}
		inserted = append(inserted, InsertedJob{JobID: row.ID, DedupeKey: row.DedupeKey, Posting: posting})
	}
	return inserted, 1, nil
}

func mergeBoards(ctx context.Context, q domain.BatchRepository, postings []dto.NormalizedJob, targets map[string]pgtype.UUID) (int, error) {
	var params sqlcgen.BulkMergeJobBoardsParams
	for _, j := range postings {
		id, ok := targets[DedupeKey(j.Company, j.Title, j.URL)]
		if !ok {
			continue
		}
		params.Ids = append(params.Ids, id)
		params.Urls = append(params.Urls, j.URL)
		params.SourceKeys = append(params.SourceKeys, j.SourceKey)
	}
	if len(params.Ids) == 0 {
		return 0, nil
	}
	affected, err := q.BulkMergeJobBoards(ctx, params)
	if err != nil {
		return 0, fmt.Errorf("ingestion: merge job boards: %w", err)
	}
	return int(affected), nil
}

func attachActivities(ctx context.Context, q domain.BatchRepository, batch PostingBatch, inserted []InsertedJob) error {
	var params sqlcgen.BulkInsertActivitiesParams
	add := func(op, label, sourceKey string, jobID pgtype.UUID) {
		params.Ops = append(params.Ops, op)
		params.Labels = append(params.Labels, label)
		params.JobIds = append(params.JobIds, jobID)
		params.SourceKeys = append(params.SourceKeys, sourceKey)
	}

	for _, ins := range inserted {
		label := fmt.Sprintf("%s — %s", ins.Posting.Company, ins.Posting.Title)
		if batch.NeedsDetail {
			add(OpEnrich, label, ins.Posting.SourceKey, ins.JobID)
			continue
		}
		add(OpMatch, label, "", ins.JobID)
		add(OpGhost, "ghost score", "", ins.JobID)
	}

	rows, err := q.BulkInsertActivities(ctx, params)
	if err != nil {
		return fmt.Errorf("ingestion: insert activities: %w", err)
	}

	byJobOp := make(map[string]pgtype.UUID, len(rows))
	for _, row := range rows {
		byJobOp[dbutil.UUIDString(row.JobId)+"|"+row.Op] = row.ID
	}
	for i := range inserted {
		jobID := dbutil.UUIDString(inserted[i].JobID)
		for _, op := range []string{OpEnrich, OpMatch, OpGhost} {
			if id, ok := byJobOp[jobID+"|"+op]; ok {
				if inserted[i].ActivityIDs == nil {
					inserted[i].ActivityIDs = make(map[string]pgtype.UUID, 2)
				}
				inserted[i].ActivityIDs[op] = id
			}
		}
	}
	return nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
