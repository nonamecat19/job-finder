// Package jobs ports modules/jobs/*: list/filter/get/shortlist/hide, and
// enqueueing async document generation.
package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/activity"
	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbutil"
	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/queue"
)

// Service is the jobs use-case. It depends on the Repository and Enqueuer
// ports (see ports.go), not on concrete infrastructure.
type Service struct {
	q              Repository
	client         Enqueuer
	salaryFloorUsd int
}

// NewService wires the use-case to its ports. The concrete *sqlcgen.Queries and
// *asynq.Client satisfy the interfaces, so callers pass them directly.
// salaryFloorUsd is SALARY_FLOOR_USD (spec 006, US2); 0 disables the floor
// filter entirely (FR-018).
func NewService(q Repository, client Enqueuer, salaryFloorUsd int) *Service {
	return &Service{q: q, client: client, salaryFloorUsd: salaryFloorUsd}
}

type ListParams struct {
	Sort     string // "score" | "date"
	Source   *string
	MinScore *int
	Status   *string
	Remote   *bool
	Q        *string
	Page     int
	PageSize int
	// ShowBelowFloor, when true, omits the salary-floor predicate so jobs
	// below SALARY_FLOOR_USD are included (FR-016). Default (false) hides
	// them, matching the "filter defaults to on" assumption.
	ShowBelowFloor bool
}

// jobRow is the common shape of ListJobsByScoreRow / ListJobsByDateRow (the
// two sqlc queries only differ in ORDER BY, so their columns are identical).
type jobRow struct {
	Job             sqlcgen.Job
	MrID            pgtype.UUID
	MrSimilarity    *float64
	MrScore         *int32
	MrMatchedSkills []byte
	MrMissingSkills []byte
	MrSummary       *string
	MrRedFlags      []byte
	MrModel         *string
	MrCreatedAt     pgtype.Timestamp
}

func (s *Service) List(ctx context.Context, params ListParams) (dto.JobListResponse, error) {
	page := params.Page
	if page < 1 {
		page = 1
	}
	pageSize := params.PageSize
	if pageSize <= 0 {
		pageSize = 25
	}
	if pageSize > 100 {
		pageSize = 100
	}

	var qPattern *string
	if params.Q != nil && *params.Q != "" {
		p := "%" + *params.Q + "%"
		qPattern = &p
	}
	var minScore *int32
	if params.MinScore != nil {
		v := int32(*params.MinScore)
		minScore = &v
	}

	// Floor 0 or the reveal toggle both mean "no predicate" — nil omits it
	// entirely rather than evaluating ">= 0" (FR-018).
	var salaryFloor *int32
	if s.salaryFloorUsd > 0 && !params.ShowBelowFloor {
		v := int32(s.salaryFloorUsd)
		salaryFloor = &v
	}

	count, err := s.q.CountJobs(ctx, sqlcgen.CountJobsParams{
		Source: params.Source, Status: params.Status, Remote: params.Remote, Q: qPattern, MinScore: minScore,
		SalaryFloor: salaryFloor,
	})
	if err != nil {
		return dto.JobListResponse{}, err
	}

	offset := int32((page - 1) * pageSize)
	limit := int32(pageSize)

	var rows []jobRow
	if params.Sort == "date" {
		r, err := s.q.ListJobsByDate(ctx, sqlcgen.ListJobsByDateParams{
			Source: params.Source, Status: params.Status, Remote: params.Remote, Q: qPattern, MinScore: minScore,
			SalaryFloor: salaryFloor, Offset: offset, Limit: limit,
		})
		if err != nil {
			return dto.JobListResponse{}, err
		}
		for _, x := range r {
			rows = append(rows, jobRow{
				Job: sqlcgen.Job{
					ID: x.ID, DedupeKey: x.DedupeKey, SourceKey: x.SourceKey, ExternalId: x.ExternalId,
					Title: x.Title, Company: x.Company, Location: x.Location, Remote: x.Remote,
					SalaryRaw: x.SalaryRaw, Url: x.Url, Description: x.Description, Raw: x.Raw,
					PostedAt: x.PostedAt, IngestedAt: x.IngestedAt, Embedding: x.Embedding, Status: x.Status,
					SalaryMin: x.SalaryMin, SalaryMax: x.SalaryMax, SalaryCurrency: x.SalaryCurrency,
					SalaryConfidence: x.SalaryConfidence, SalarySource: x.SalarySource,
				},
				MrID: x.MrID, MrSimilarity: x.MrSimilarity, MrScore: x.MrScore,
				MrMatchedSkills: x.MrMatchedSkills, MrMissingSkills: x.MrMissingSkills,
				MrSummary: x.MrSummary, MrRedFlags: x.MrRedFlags, MrModel: x.MrModel, MrCreatedAt: x.MrCreatedAt,
			})
		}
	} else {
		r, err := s.q.ListJobsByScore(ctx, sqlcgen.ListJobsByScoreParams{
			Source: params.Source, Status: params.Status, Remote: params.Remote, Q: qPattern, MinScore: minScore,
			SalaryFloor: salaryFloor, Offset: offset, Limit: limit,
		})
		if err != nil {
			return dto.JobListResponse{}, err
		}
		for _, x := range r {
			rows = append(rows, jobRow{
				Job: sqlcgen.Job{
					ID: x.ID, DedupeKey: x.DedupeKey, SourceKey: x.SourceKey, ExternalId: x.ExternalId,
					Title: x.Title, Company: x.Company, Location: x.Location, Remote: x.Remote,
					SalaryRaw: x.SalaryRaw, Url: x.Url, Description: x.Description, Raw: x.Raw,
					PostedAt: x.PostedAt, IngestedAt: x.IngestedAt, Embedding: x.Embedding, Status: x.Status,
					SalaryMin: x.SalaryMin, SalaryMax: x.SalaryMax, SalaryCurrency: x.SalaryCurrency,
					SalaryConfidence: x.SalaryConfidence, SalarySource: x.SalarySource,
				},
				MrID: x.MrID, MrSimilarity: x.MrSimilarity, MrScore: x.MrScore,
				MrMatchedSkills: x.MrMatchedSkills, MrMissingSkills: x.MrMissingSkills,
				MrSummary: x.MrSummary, MrRedFlags: x.MrRedFlags, MrModel: x.MrModel, MrCreatedAt: x.MrCreatedAt,
			})
		}
	}

	items := make([]dto.JobDto, 0, len(rows))
	for _, row := range rows {
		item := rowToDto(row)
		s.markBelowFloor(&item)
		items = append(items, item)
	}
	return dto.JobListResponse{Items: items, Total: count, Page: page, PageSize: pageSize}, nil
}

// markBelowFloor sets SalaryBelowFloor when the job's band is entirely below
// SALARY_FLOOR_USD. Only USD bands are evaluated — a currency the system
// cannot convert must never be filtered or marked (FR-020 fails open).
func (s *Service) markBelowFloor(d *dto.JobDto) {
	if s.salaryFloorUsd <= 0 || d.SalaryMax == nil || d.SalaryCurrency == nil || *d.SalaryCurrency != "USD" {
		return
	}
	d.SalaryBelowFloor = *d.SalaryMax < s.salaryFloorUsd
}

func (s *Service) Get(ctx context.Context, id string) (dto.JobDto, error) {
	uid, err := dbutil.ParseUUID(id)
	if err != nil {
		return dto.JobDto{}, err
	}
	job, err := s.q.GetJobByID(ctx, uid)
	if err != nil {
		return dto.JobDto{}, fmt.Errorf("job %s not found", id)
	}
	out := jobToDto(job)
	s.markBelowFloor(&out)

	if mr, err := s.q.GetMatchResultByJobID(ctx, uid); err == nil {
		md := matchResultDto(mr)
		out.MatchResult = &md
	}
	if docs, err := s.q.GetJobDocuments(ctx, uid); err == nil {
		for _, d := range docs {
			out.Documents = append(out.Documents, documentDto(d))
		}
	}
	if app, err := s.q.GetApplicationByJobID(ctx, uid); err == nil {
		ad := applicationDto(app)
		out.Application = &ad
	}
	return out, nil
}

func (s *Service) Shortlist(ctx context.Context, id string) (dto.JobDto, error) {
	uid, err := dbutil.ParseUUID(id)
	if err != nil {
		return dto.JobDto{}, err
	}
	if _, err := s.Get(ctx, id); err != nil {
		return dto.JobDto{}, err
	}
	if _, err := s.q.UpdateJobStatus(ctx, sqlcgen.UpdateJobStatusParams{ID: uid, Status: "shortlisted"}); err != nil {
		return dto.JobDto{}, err
	}
	events, _ := json.Marshal([]dto.ApplicationEvent{{Status: "shortlisted", At: time.Now().UTC().Format(time.RFC3339)}})
	if err := s.q.UpsertApplicationStatus(ctx, sqlcgen.UpsertApplicationStatusParams{JobId: uid, Status: "shortlisted", Events: events}); err != nil {
		return dto.JobDto{}, err
	}
	return s.Get(ctx, id)
}

func (s *Service) Hide(ctx context.Context, id string) (dto.JobDto, error) {
	uid, err := dbutil.ParseUUID(id)
	if err != nil {
		return dto.JobDto{}, err
	}
	if _, err := s.Get(ctx, id); err != nil {
		return dto.JobDto{}, err
	}
	updated, err := s.q.UpdateJobStatus(ctx, sqlcgen.UpdateJobStatusParams{ID: uid, Status: "hidden"})
	if err != nil {
		return dto.JobDto{}, err
	}
	out := jobToDto(updated)
	s.markBelowFloor(&out)
	return out, nil
}

// DeleteAll wipes every job (and, via ON DELETE cascade, its applications,
// documents, match results, and activity). Returns the number of jobs removed.
func (s *Service) DeleteAll(ctx context.Context) (int64, error) {
	return s.q.DeleteAllJobs(ctx)
}

// EnqueueGeneration enqueues a "generate" asynq task and returns a
// 202-style payload; the dashboard polls documents. Matches
// JobsService.enqueueGeneration.
func (s *Service) EnqueueGeneration(ctx context.Context, id, docType string, profileID *string) (map[string]any, error) {
	jobDto, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	var profID string
	if profileID != nil && *profileID != "" {
		profID = *profileID
	} else {
		p, err := s.q.GetDefaultProfile(ctx)
		if err != nil {
			return nil, fmt.Errorf("precondition failed: no profile exists yet")
		}
		profID = dbutil.UUIDString(p.ID)
	}
	uid, err := dbutil.ParseUUID(profID)
	if err != nil {
		return nil, err
	}
	has, err := s.q.ProfileHasConfig(ctx, uid)
	if err != nil {
		return nil, err
	}
	b, _ := has.(bool)
	if !b {
		return nil, fmt.Errorf("precondition failed: profile has no RenderCV config")
	}

	rec := activity.New(ctx, s.q, "generate", fmt.Sprintf("%s — %s", jobDto.Company, jobDto.Title), &id, nil, "")
	var actID *string
	if rec != nil {
		idStr := dbutil.UUIDString(rec.ID())
		actID = &idStr
	}

	payload, err := json.Marshal(queue.GeneratePayload{JobID: id, Type: docType, ProfileID: profileID, ActivityID: actID})
	if err != nil {
		return nil, err
	}
	info, err := s.client.EnqueueContext(ctx, asynq.NewTask(queue.TypeGenerate, payload),
		asynq.MaxRetry(0), asynq.Queue(queue.QueueGenerate))
	if err != nil {
		return nil, err
	}
	return map[string]any{"queued": true, "queueJobId": info.ID, "type": docType}, nil
}

func jobToDto(j sqlcgen.Job) dto.JobDto {
	out := dto.JobDto{
		ID: dbutil.UUIDString(j.ID), DedupeKey: j.DedupeKey, SourceKey: j.SourceKey,
		Title: j.Title, Company: j.Company, Location: j.Location, Remote: j.Remote,
		SalaryRaw: j.SalaryRaw, URL: j.Url, Description: j.Description,
		PostedAt: dbutil.TimestampPtr(j.PostedAt), IngestedAt: dbutil.Timestamp(j.IngestedAt), Status: j.Status,
		SalaryMin: int32PtrToIntPtr(j.SalaryMin), SalaryMax: int32PtrToIntPtr(j.SalaryMax),
		SalaryCurrency: j.SalaryCurrency, SalaryConfidence: j.SalaryConfidence, SalarySource: j.SalarySource,
	}
	var rawFields struct {
		DetailHTML *string `json:"detailHtml"`
	}
	if dbutil.UnmarshalJSONB(j.Raw, &rawFields) == nil && rawFields.DetailHTML != nil {
		out.DescriptionHtml = rawFields.DetailHTML
	}
	return out
}

func int32PtrToIntPtr(v *int32) *int {
	if v == nil {
		return nil
	}
	n := int(*v)
	return &n
}

func rowToDto(row jobRow) dto.JobDto {
	out := jobToDto(row.Job)
	if row.MrID.Valid {
		md := matchResultDto(sqlcgen.MatchResult{
			ID: row.MrID, JobId: row.Job.ID, Similarity: derefF(row.MrSimilarity), Score: row.MrScore,
			MatchedSkills: row.MrMatchedSkills, MissingSkills: row.MrMissingSkills, Summary: row.MrSummary,
			RedFlags: row.MrRedFlags, Model: derefS(row.MrModel), CreatedAt: row.MrCreatedAt,
		})
		out.MatchResult = &md
	}
	return out
}

func derefF(v *float64) float64 {
	if v == nil {
		return 0
	}
	return *v
}
func derefS(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func matchResultDto(r sqlcgen.MatchResult) dto.MatchResultDto {
	var score *int
	if r.Score != nil {
		v := int(*r.Score)
		score = &v
	}
	var matched, missing, redFlags *[]string
	var m, mi, rf []string
	if dbutil.UnmarshalJSONB(r.MatchedSkills, &m) == nil && r.MatchedSkills != nil && string(r.MatchedSkills) != "null" {
		matched = &m
	}
	if dbutil.UnmarshalJSONB(r.MissingSkills, &mi) == nil && r.MissingSkills != nil && string(r.MissingSkills) != "null" {
		missing = &mi
	}
	if dbutil.UnmarshalJSONB(r.RedFlags, &rf) == nil && r.RedFlags != nil && string(r.RedFlags) != "null" {
		redFlags = &rf
	}
	return dto.MatchResultDto{
		ID: dbutil.UUIDString(r.ID), JobID: dbutil.UUIDString(r.JobId), Similarity: r.Similarity,
		Score: score, MatchedSkills: matched, MissingSkills: missing, Summary: r.Summary, RedFlags: redFlags,
		Model: r.Model, CreatedAt: dbutil.Timestamp(r.CreatedAt),
	}
}

func documentDto(r sqlcgen.GeneratedDocument) dto.GeneratedDocumentDto {
	var content any
	_ = dbutil.UnmarshalJSONB(r.Content, &content)
	return dto.GeneratedDocumentDto{
		ID: dbutil.UUIDString(r.ID), JobID: dbutil.UUIDStringPtr(r.JobId), Type: r.Type, Version: int(r.Version),
		Content: content, PdfPath: r.PdfPath, Model: r.Model, Company: r.Company, Title: r.Title, Vacancy: r.Vacancy,
		CreatedAt: dbutil.Timestamp(r.CreatedAt),
	}
}

func applicationDto(a sqlcgen.Application) dto.ApplicationDto {
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
