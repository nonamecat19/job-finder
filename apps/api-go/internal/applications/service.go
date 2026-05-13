// Package applications ports modules/applications/*: status transitions,
// kanban feed, and the /stats aggregate.
package applications

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api-go/internal/db/sqlcgen"
	"github.com/job-finder/api-go/internal/dbutil"
	"github.com/job-finder/api-go/internal/dto"
)

// ErrNotFound is a sentinel so callers (the HTTP layer) can distinguish
// "no such application" (404) from other Update failures like an invalid
// status value (400) — mirrors NestJS's NotFoundException vs
// BadRequestException split in applications.service.ts:25,28.
var ErrNotFound = errors.New("application not found")

type Service struct {
	q *sqlcgen.Queries
}

func NewService(q *sqlcgen.Queries) *Service {
	return &Service{q: q}
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
		return dto.ApplicationDto{}, fmt.Errorf("application %s not found: %w", id, ErrNotFound)
	}
	if in.Status != nil && !dto.IsValidApplicationStatus(string(*in.Status)) {
		return dto.ApplicationDto{}, fmt.Errorf("invalid status '%s'", *in.Status)
	}

	var events []dto.ApplicationEvent
	_ = dbutil.UnmarshalJSONB(existing.Events, &events)
	params := sqlcgen.UpdateApplicationParams{ID: uid}

	if in.Status != nil && string(*in.Status) != existing.Status {
		events = append(events, dto.ApplicationEvent{Status: string(*in.Status), At: time.Now().UTC().Format(time.RFC3339)})
		s := string(*in.Status)
		params.Status = &s
		eventsJSON, err := json.Marshal(events)
		if err != nil {
			return dto.ApplicationDto{}, err
		}
		params.Events = eventsJSON
		if *in.Status == dto.StatusApplied && !existing.AppliedAt.Valid {
			params.AppliedAt = dbutil.NowTimestamp()
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

	updated, err := s.q.UpdateApplication(ctx, params)
	if err != nil {
		return dto.ApplicationDto{}, err
	}
	if in.Status != nil {
		if _, err := s.q.UpdateJobStatus(ctx, sqlcgen.UpdateJobStatusParams{ID: existing.JobId, Status: string(*in.Status)}); err != nil {
			return dto.ApplicationDto{}, err
		}
	}

	out := toDto(updated)
	if job, err := s.q.GetJobByID(ctx, existing.JobId); err == nil {
		jd := jobDto(job)
		out.Job = &jd
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
