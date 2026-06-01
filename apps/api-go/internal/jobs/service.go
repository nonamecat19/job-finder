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

	"github.com/job-finder/api-go/internal/db/sqlcgen"
	"github.com/job-finder/api-go/internal/dbutil"
	"github.com/job-finder/api-go/internal/dto"
	"github.com/job-finder/api-go/internal/queue"
)

type Service struct {
	q      *sqlcgen.Queries
	client *asynq.Client
}

func NewService(q *sqlcgen.Queries, client *asynq.Client) *Service {
	return &Service{q: q, client: client}
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

	count, err := s.q.CountJobs(ctx, sqlcgen.CountJobsParams{
		Source: params.Source, Status: params.Status, Remote: params.Remote, Q: qPattern, MinScore: minScore,
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
			Offset: offset, Limit: limit,
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
				},
				MrID: x.MrID, MrSimilarity: x.MrSimilarity, MrScore: x.MrScore,
				MrMatchedSkills: x.MrMatchedSkills, MrMissingSkills: x.MrMissingSkills,
				MrSummary: x.MrSummary, MrRedFlags: x.MrRedFlags, MrModel: x.MrModel, MrCreatedAt: x.MrCreatedAt,
			})
		}
	} else {
		r, err := s.q.ListJobsByScore(ctx, sqlcgen.ListJobsByScoreParams{
			Source: params.Source, Status: params.Status, Remote: params.Remote, Q: qPattern, MinScore: minScore,
			Offset: offset, Limit: limit,
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
				},
				MrID: x.MrID, MrSimilarity: x.MrSimilarity, MrScore: x.MrScore,
				MrMatchedSkills: x.MrMatchedSkills, MrMissingSkills: x.MrMissingSkills,
				MrSummary: x.MrSummary, MrRedFlags: x.MrRedFlags, MrModel: x.MrModel, MrCreatedAt: x.MrCreatedAt,
			})
		}
	}

	items := make([]dto.JobDto, 0, len(rows))
	for _, row := range rows {
		items = append(items, rowToDto(row))
	}
	return dto.JobListResponse{Items: items, Total: count, Page: page, PageSize: pageSize}, nil
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
	return jobToDto(updated), nil
}

// EnqueueGeneration enqueues a "generate" asynq task and returns a
// 202-style payload; the dashboard polls documents. Matches
// JobsService.enqueueGeneration.
func (s *Service) EnqueueGeneration(ctx context.Context, id, docType string, profileID *string) (map[string]any, error) {
	if _, err := s.Get(ctx, id); err != nil {
		return nil, err
	}
	payload, err := json.Marshal(queue.GeneratePayload{JobID: id, Type: docType, ProfileID: profileID})
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
	return dto.JobDto{
		ID: dbutil.UUIDString(j.ID), DedupeKey: j.DedupeKey, SourceKey: j.SourceKey,
		Title: j.Title, Company: j.Company, Location: j.Location, Remote: j.Remote,
		SalaryRaw: j.SalaryRaw, URL: j.Url, Description: j.Description,
		PostedAt: dbutil.TimestampPtr(j.PostedAt), IngestedAt: dbutil.Timestamp(j.IngestedAt), Status: j.Status,
	}
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
		ID: dbutil.UUIDString(r.ID), JobID: dbutil.UUIDString(r.JobId), Type: r.Type, Version: int(r.Version),
		Content: content, PdfPath: r.PdfPath, Model: r.Model, CreatedAt: dbutil.Timestamp(r.CreatedAt),
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
