// Package application holds the application-tracking use-case: status
// transitions, kanban feed, and the /stats aggregate. Mirrors
// applications.service.ts.
package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/applications/domain"
	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbutil"
	"github.com/job-finder/api/internal/dto"
)

// ErrNotFound re-exports domain.ErrNotFound for callers that only import
// application (e.g. the httpapi handler).
var ErrNotFound = domain.ErrNotFound

type Service struct {
	q  domain.Repository
	tx domain.TxRunner
}

// NewService builds the use-case. Pass a TxRunner (e.g. *db.DB) to make a
// status change and the outcome event it records commit atomically; omit it
// and the writes run sequentially against q, which is what unit-test fakes do.
func NewService(q domain.Repository, tx ...domain.TxRunner) *Service {
	s := &Service{q: q}
	if len(tx) > 0 {
		s.tx = tx[0]
	}
	return s
}

// inTx runs fn against a transaction-bound Repository when a TxRunner is
// injected, and against the plain repository otherwise. *sqlcgen.Queries
// satisfies Repository structurally, so both paths share one fn body.
func (s *Service) inTx(ctx context.Context, fn func(domain.Repository) error) error {
	if s.tx == nil {
		return fn(s.q)
	}
	return s.tx.WithinTx(ctx, func(q *sqlcgen.Queries) error { return fn(q) })
}

func (s *Service) List(ctx context.Context, status *string) ([]dto.ApplicationDto, error) {
	var statusPattern *string
	if status != nil && *status != "" {
		statusPattern = status
	}
	rows, err := s.q.ListApplications(ctx, statusPattern)
	if err != nil {
		return nil, err
	}
	out := make([]dto.ApplicationDto, 0, len(rows))
	for _, r := range rows {
		out = append(out, listRowToDto(r))
	}
	return out, nil
}

type UpdateInput struct {
	Status *dto.ApplicationStatus
	Notes  **string // outer present = field sent; inner nil = clear notes
}

func (s *Service) Update(ctx context.Context, id string, in UpdateInput) (dto.ApplicationDto, error) {
	uid, err := dbutil.ParseUUID(id)
	if err != nil {
		return dto.ApplicationDto{}, err
	}
	existing, err := s.q.GetApplicationByID(ctx, uid)
	if err != nil {
		return dto.ApplicationDto{}, fmt.Errorf("application %s not found: %w", id, domain.ErrNotFound)
	}
	if in.Status != nil && !dto.IsValidApplicationStatus(string(*in.Status)) {
		return dto.ApplicationDto{}, fmt.Errorf("invalid status '%s'", *in.Status)
	}

	var events []dto.ApplicationEvent
	_ = dbutil.UnmarshalJSONB(existing.Events, &events)
	params := sqlcgen.UpdateApplicationParams{ID: uid}

	// One instant shared by the jsonb annotation, the outcome event's
	// "occurredAt", and "appliedAt" — the post-age signal reads "appliedAt" and
	// must see the same moment the `applied` event carries.
	occurredAt := time.Now().UTC()
	var outcome *dto.OutcomeEventType

	if in.Status != nil && string(*in.Status) != existing.Status {
		events = append(events, dto.ApplicationEvent{Status: string(*in.Status), At: occurredAt.Format(time.RFC3339)})
		s := string(*in.Status)
		params.Status = &s
		eventsJSON, err := json.Marshal(events)
		if err != nil {
			return dto.ApplicationDto{}, err
		}
		params.Events = eventsJSON
		if *in.Status == dto.StatusApplied && !existing.AppliedAt.Valid {
			params.AppliedAt = pgtype.Timestamp{Time: occurredAt, Valid: true}
		}
		if et, ok := dto.OutcomeEventForStatus(*in.Status); ok {
			outcome = &et
		}
	} else {
		// keep existing events unchanged (params.Events must still be a valid
		// jsonb value since the column is NOT NULL — COALESCE isn't used here,
		// mirroring the direct `set: {...}` object the TS code builds).
		eventsJSON, _ := json.Marshal(events)
		params.Events = eventsJSON
	}
	if in.Notes != nil {
		params.NotesSet = boolPtr(true)
		params.Notes = *in.Notes
	}

	// Status write, outcome-event append, and the mirrored job status all commit
	// together — the current-state column and the event log must never diverge.
	var updated sqlcgen.Application
	err = s.inTx(ctx, func(q domain.Repository) error {
		var err error
		if updated, err = q.UpdateApplication(ctx, params); err != nil {
			return err
		}
		if outcome != nil {
			_, err := q.InsertApplicationOutcome(ctx, sqlcgen.InsertApplicationOutcomeParams{
				ApplicationId: uid,
				EventType:     string(*outcome),
				OccurredAt:    pgtype.Timestamp{Time: occurredAt, Valid: true},
			})
			// No row means the partial unique index rejected a duplicate
			// terminal-once event ('applied'/'offer'/'rejected'). That is the
			// specified idempotent no-op, not a failure.
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
		}
		if in.Status != nil {
			if _, err := q.UpdateJobStatus(ctx, sqlcgen.UpdateJobStatusParams{ID: existing.JobId, Status: string(*in.Status)}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return dto.ApplicationDto{}, err
	}

	out := toDto(updated)
	if job, err := s.q.GetJobByID(ctx, existing.JobId); err == nil {
		jd := jobDto(job)
		out.Job = &jd
	}
	return out, nil
}

// Timeline returns the application's outcome events oldest-first. The log is
// append-only, so a status that regressed (offer back to screen) still shows
// every transition in the order it happened rather than a rewritten linear
// history.
func (s *Service) Timeline(ctx context.Context, id string) ([]dto.ApplicationOutcomeDto, error) {
	uid, err := dbutil.ParseUUID(id)
	if err != nil {
		return nil, err
	}
	rows, err := s.q.ListApplicationOutcomes(ctx, uid)
	if err != nil {
		return nil, err
	}
	out := make([]dto.ApplicationOutcomeDto, 0, len(rows))
	for _, r := range rows {
		out = append(out, dto.ApplicationOutcomeDto{
			ID:            dbutil.UUIDString(r.ID),
			ApplicationID: dbutil.UUIDString(r.ApplicationId),
			EventType:     dto.OutcomeEventType(r.EventType),
			OccurredAt:    dbutil.Timestamp(r.OccurredAt),
			RecordedAt:    dbutil.Timestamp(r.RecordedAt),
			Note:          r.Note,
		})
	}
	return out, nil
}

func (s *Service) Stats(ctx context.Context) (dto.StatsDto, error) {
	jobsTotal, err := s.q.StatsJobsTotal(ctx)
	if err != nil {
		return dto.StatsDto{}, err
	}
	since := pgtype.Timestamp{Time: time.Now().Add(-24 * time.Hour).UTC(), Valid: true}
	jobsLast24h, err := s.q.StatsJobsLast24h(ctx, since)
	if err != nil {
		return dto.StatsDto{}, err
	}
	highFit, err := s.q.StatsHighFit(ctx)
	if err != nil {
		return dto.StatsDto{}, err
	}
	pipelineRows, err := s.q.StatsPipeline(ctx)
	if err != nil {
		return dto.StatsDto{}, err
	}
	pipeline := make(map[string]int64, len(pipelineRows))
	for _, r := range pipelineRows {
		pipeline[r.Status] = r.Count
	}
	runs, err := s.q.RecentRunsJoined(ctx, 10)
	if err != nil {
		return dto.StatsDto{}, err
	}
	recentRuns := make([]dto.SourceRunDto, 0, len(runs))
	for _, r := range runs {
		recentRuns = append(recentRuns, dto.SourceRunDto{
			ID: dbutil.UUIDString(r.ID), SourceKey: r.SourceKey, SearchID: r.SearchID,
			StartedAt: dbutil.Timestamp(r.StartedAt), FinishedAt: dbutil.TimestampPtr(r.FinishedAt),
			OK: r.Ok, Found: int(r.Found), New: int(r.New), Error: r.Error,
			Verdict: r.Verdict, BlockedCount: int(r.BlockedCount), BlockReason: r.BlockReason,
		})
	}

	return dto.StatsDto{
		JobsTotal: jobsTotal, JobsLast24h: jobsLast24h, HighFit: highFit,
		Pipeline: pipeline, RecentRuns: recentRuns,
	}, nil
}

func boolPtr(b bool) *bool { return &b }

func toDto(a sqlcgen.Application) dto.ApplicationDto {
	var events []dto.ApplicationEvent
	_ = dbutil.UnmarshalJSONB(a.Events, &events)
	if events == nil {
		events = []dto.ApplicationEvent{}
	}
	return dto.ApplicationDto{
		ID: dbutil.UUIDString(a.ID), JobID: dbutil.UUIDString(a.JobId), Status: dto.ApplicationStatus(a.Status),
		Notes: a.Notes, AppliedAt: dbutil.TimestampPtr(a.AppliedAt), Events: events, UpdatedAt: dbutil.Timestamp(a.UpdatedAt),
	}
}

func jobDto(j sqlcgen.Job) dto.JobDto {
	return dto.JobDto{
		ID: dbutil.UUIDString(j.ID), DedupeKey: j.DedupeKey, SourceKey: j.SourceKey,
		Title: j.Title, Company: j.Company, Location: j.Location, Remote: j.Remote,
		SalaryRaw: j.SalaryRaw, URL: j.Url, Description: j.Description,
		PostedAt: dbutil.TimestampPtr(j.PostedAt), IngestedAt: dbutil.Timestamp(j.IngestedAt), Status: j.Status,
	}
}

// listRowToDto converts a ListApplications joined row (application + job +
// left-joined match result) into an ApplicationDto with its embedded JobDto,
// matching `with: { job: { with: { matchResult: true } } }` in
// applications.service.ts's list().
func listRowToDto(r sqlcgen.ListApplicationsRow) dto.ApplicationDto {
	var events []dto.ApplicationEvent
	_ = dbutil.UnmarshalJSONB(r.Events, &events)
	if events == nil {
		events = []dto.ApplicationEvent{}
	}

	job := dto.JobDto{
		ID: dbutil.UUIDString(r.JobIDFull), DedupeKey: r.JobDedupeKey, SourceKey: r.JobSourceKey,
		Title: r.JobTitle, Company: r.JobCompany, Location: r.JobLocation, Remote: r.JobRemote,
		SalaryRaw: r.JobSalaryRaw, URL: r.JobUrl, Description: r.JobDescription,
		PostedAt: dbutil.TimestampPtr(r.JobPostedAt), IngestedAt: dbutil.Timestamp(r.JobIngestedAt), Status: r.JobStatus,
	}
	if r.MrID.Valid {
		var score *int
		if r.MrScore != nil {
			v := int(*r.MrScore)
			score = &v
		}
		var matched, missing, redFlags *[]string
		var m, mi, rf []string
		if dbutil.UnmarshalJSONB(r.MrMatchedSkills, &m) == nil && r.MrMatchedSkills != nil && string(r.MrMatchedSkills) != "null" {
			matched = &m
		}
		if dbutil.UnmarshalJSONB(r.MrMissingSkills, &mi) == nil && r.MrMissingSkills != nil && string(r.MrMissingSkills) != "null" {
			missing = &mi
		}
		if dbutil.UnmarshalJSONB(r.MrRedFlags, &rf) == nil && r.MrRedFlags != nil && string(r.MrRedFlags) != "null" {
			redFlags = &rf
		}
		model := ""
		if r.MrModel != nil {
			model = *r.MrModel
		}
		var similarity float64
		if r.MrSimilarity != nil {
			similarity = *r.MrSimilarity
		}
		job.MatchResult = &dto.MatchResultDto{
			ID: dbutil.UUIDString(r.MrID), JobID: job.ID, Similarity: similarity, Score: score,
			MatchedSkills: matched, MissingSkills: missing, Summary: r.MrSummary, RedFlags: redFlags,
			Model: model, CreatedAt: dbutil.Timestamp(r.MrCreatedAt),
		}
	}

	return dto.ApplicationDto{
		ID: dbutil.UUIDString(r.ID), JobID: dbutil.UUIDString(r.JobId), Status: dto.ApplicationStatus(r.Status),
		Notes: r.Notes, AppliedAt: dbutil.TimestampPtr(r.AppliedAt), Events: events,
		UpdatedAt: dbutil.Timestamp(r.UpdatedAt), Job: &job,
	}
}
