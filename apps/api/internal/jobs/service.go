package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/activity"
	"github.com/job-finder/api/internal/apperr"
	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbutil"
	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/queue"
)

type Service struct {
	q              Repository
	client         Enqueuer
	salaryFloorUsd int
}

func NewService(q Repository, client Enqueuer, salaryFloorUsd int) *Service {
	return &Service{q: q, client: client, salaryFloorUsd: salaryFloorUsd}
}

const ghostSignalKind = "ghost"

type ListParams struct {
	Sort           string
	Source         *string
	SubscriptionID *string
	MinScore       *int
	Status         *string
	IncludeHidden  bool
	IncludeApplied bool
	Remote         *bool
	Q              *string
	Page           int
	PageSize       int
	ShowBelowFloor bool
	OnlyHidden     bool
	OnlyApplied    bool
	OnlyBelowFloor bool
	OnlyManual     bool
}

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
	var subscriptionID pgtype.UUID
	if params.SubscriptionID != nil {
		if uid, err := dbutil.ParseUUID(*params.SubscriptionID); err == nil {
			subscriptionID = uid
		}
	}

	var salaryFloor *int32
	if s.salaryFloorUsd > 0 && (!params.ShowBelowFloor || params.OnlyBelowFloor) {
		v := int32(s.salaryFloorUsd)
		salaryFloor = &v
	}

	onlyBelowFloor := &params.OnlyBelowFloor
	onlyHidden := &params.OnlyHidden
	onlyApplied := &params.OnlyApplied
	includeHidden := &params.IncludeHidden
	includeApplied := &params.IncludeApplied
	onlyManual := &params.OnlyManual

	count, err := s.q.CountJobs(ctx, sqlcgen.CountJobsParams{
		Source: params.Source, SubscriptionID: subscriptionID, Status: params.Status, Remote: params.Remote, Q: qPattern, MinScore: minScore,
		SalaryFloor: salaryFloor, IncludeHidden: includeHidden, IncludeApplied: includeApplied,
		OnlyHidden: onlyHidden, OnlyApplied: onlyApplied, OnlyBelowFloor: onlyBelowFloor, OnlyManual: onlyManual,
	})
	if err != nil {
		return dto.JobListResponse{}, err
	}

	offset := int32((page - 1) * pageSize)
	limit := int32(pageSize)

	var rows []jobRow
	if params.Sort == "date" {
		r, err := s.q.ListJobsByDate(ctx, sqlcgen.ListJobsByDateParams{
			Source: params.Source, SubscriptionID: subscriptionID, Status: params.Status, Remote: params.Remote, Q: qPattern, MinScore: minScore,
			SalaryFloor: salaryFloor, IncludeHidden: includeHidden, IncludeApplied: includeApplied,
			OnlyHidden: onlyHidden, OnlyApplied: onlyApplied, OnlyBelowFloor: onlyBelowFloor, OnlyManual: onlyManual,
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
					SalaryMin: x.SalaryMin, SalaryMax: x.SalaryMax, SalaryCurrency: x.SalaryCurrency,
					SalaryConfidence: x.SalaryConfidence, SalarySource: x.SalarySource, SubscriptionId: x.SubscriptionId,
				},
				MrID: x.MrID, MrSimilarity: x.MrSimilarity, MrScore: x.MrScore,
				MrMatchedSkills: x.MrMatchedSkills, MrMissingSkills: x.MrMissingSkills,
				MrSummary: x.MrSummary, MrRedFlags: x.MrRedFlags, MrModel: x.MrModel, MrCreatedAt: x.MrCreatedAt,
			})
		}
	} else {
		r, err := s.q.ListJobsByScore(ctx, sqlcgen.ListJobsByScoreParams{
			Source: params.Source, SubscriptionID: subscriptionID, Status: params.Status, Remote: params.Remote, Q: qPattern, MinScore: minScore,
			SalaryFloor: salaryFloor, IncludeHidden: includeHidden, IncludeApplied: includeApplied,
			OnlyHidden: onlyHidden, OnlyApplied: onlyApplied, OnlyBelowFloor: onlyBelowFloor, OnlyManual: onlyManual,
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
					SalaryMin: x.SalaryMin, SalaryMax: x.SalaryMax, SalaryCurrency: x.SalaryCurrency,
					SalaryConfidence: x.SalaryConfidence, SalarySource: x.SalarySource, SubscriptionId: x.SubscriptionId,
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

	if len(items) > 0 {
		jobIDs := make([]pgtype.UUID, len(rows))
		for i, row := range rows {
			jobIDs[i] = row.Job.ID
		}
		if signals, err := s.q.ListJobSignalsByJobIds(ctx, sqlcgen.ListJobSignalsByJobIdsParams{
			JobIds: jobIDs, Kind: ghostSignalKind,
		}); err == nil {
			byJobID := make(map[string]sqlcgen.JobSignal, len(signals))
			for _, sig := range signals {
				byJobID[dbutil.UUIDString(sig.JobId)] = sig
			}
			for i, item := range items {
				if sig, ok := byJobID[item.ID]; ok {
					gsd := jobSignalDto(sig)
					items[i].GhostSignal = &gsd
				}
			}
		}
	}

	return dto.JobListResponse{Items: items, Total: count, Page: page, PageSize: pageSize}, nil
}

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
	if gs, err := s.q.GetJobSignal(ctx, sqlcgen.GetJobSignalParams{JobId: uid, Kind: ghostSignalKind}); err == nil {
		gsd := jobSignalDto(gs)
		out.GhostSignal = &gsd
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

func (s *Service) Unhide(ctx context.Context, id string) (dto.JobDto, error) {
	uid, err := dbutil.ParseUUID(id)
	if err != nil {
		return dto.JobDto{}, err
	}
	if _, err := s.Get(ctx, id); err != nil {
		return dto.JobDto{}, err
	}
	updated, err := s.q.UpdateJobStatus(ctx, sqlcgen.UpdateJobStatusParams{ID: uid, Status: "found"})
	if err != nil {
		return dto.JobDto{}, err
	}
	out := jobToDto(updated)
	s.markBelowFloor(&out)
	return out, nil
}

func (s *Service) DeleteAll(ctx context.Context) (int64, error) {
	return s.q.DeleteAllJobs(ctx)
}

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
			return nil, apperr.Precondition("no profile exists yet")
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
		return nil, apperr.Precondition("profile has no RenderCV config")
	}

	rec := activity.New(ctx, s.q, "generate", fmt.Sprintf("%s — %s", jobDto.Company, jobDto.Title), &id, nil, "")
	var actID *string
	if rec != nil {
		idStr := dbutil.UUIDString(rec.ID())
		actID = &idStr
		meta := map[string]any{"docType": docType}
		if profileID != nil {
			meta["profileId"] = *profileID
		}
		rec.Step(ctx, "queued", meta)
	}

	payload, err := json.Marshal(queue.GeneratePayload{JobID: id, Type: docType, ProfileID: profileID, ActivityID: actID})
	if err != nil {
		return nil, err
	}
	genOpts := []asynq.Option{asynq.MaxRetry(0), asynq.Queue(queue.QueueGenerate)}
	if actID != nil {
		genOpts = append(genOpts, asynq.TaskID(*actID))
	}
	info, err := s.client.EnqueueContext(ctx, asynq.NewTask(queue.TypeGenerate, payload), genOpts...)
	if err != nil {
		return nil, err
	}
	return map[string]any{"queued": true, "queueJobId": info.ID, "type": docType}, nil
}

func (s *Service) EnqueueSalaryInfer(ctx context.Context, jobID string) error {
	jobDto, err := s.Get(ctx, jobID)
	if err != nil {
		return err
	}

	rec := activity.New(ctx, s.q, "salary_infer", fmt.Sprintf("%s — %s", jobDto.Company, jobDto.Title), &jobID, nil, "")
	var actID *string
	if rec != nil {
		idStr := dbutil.UUIDString(rec.ID())
		actID = &idStr
	}

	payload, err := json.Marshal(queue.SalaryInferPayload{JobID: jobID, ActivityID: actID})
	if err != nil {
		return err
	}
	opts := []asynq.Option{asynq.MaxRetry(1), asynq.Queue(queue.QueueSalaryInfer)}
	if actID != nil {
		opts = append(opts, asynq.TaskID(*actID))
	}
	_, err = s.client.EnqueueContext(ctx, asynq.NewTask(queue.TypeSalaryInfer, payload), opts...)
	return err
}

func jobToDto(j sqlcgen.Job) dto.JobDto {
	out := dto.JobDto{
		ID: dbutil.UUIDString(j.ID), DedupeKey: j.DedupeKey, SourceKey: j.SourceKey,
		Title: j.Title, Company: j.Company, Location: j.Location, Remote: j.Remote,
		SalaryRaw: j.SalaryRaw, URL: j.Url, Description: j.Description,
		PostedAt: dbutil.TimestampPtr(j.PostedAt), IngestedAt: dbutil.Timestamp(j.IngestedAt), Status: j.Status,
		SalaryMin: int32PtrToIntPtr(j.SalaryMin), SalaryMax: int32PtrToIntPtr(j.SalaryMax),
		SalaryCurrency: j.SalaryCurrency, SalaryConfidence: j.SalaryConfidence, SalarySource: j.SalarySource,
		SubscriptionID:         dbutil.UUIDStringPtr(j.SubscriptionId),
		ExperienceLevel:        j.ExperienceLevel,
		ExperienceMinYears:     int32PtrToIntPtr(j.ExperienceMinYears),
		EnglishLevel:           j.EnglishLevel,
		SalaryEstimateRaw:      j.SalaryEstimateRaw,
		SalaryEstimateMin:      int32PtrToIntPtr(j.SalaryEstimateMin),
		SalaryEstimateMax:      int32PtrToIntPtr(j.SalaryEstimateMax),
		SalaryEstimateCurrency: j.SalaryEstimateCurrency,
		SalaryIsEstimated:      j.SalaryEstimateMin != nil || j.SalaryEstimateMax != nil,
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

func jobSignalDto(r sqlcgen.JobSignal) dto.JobSignalDto {
	var breakdown dto.GhostSignalBreakdownDto
	_ = dbutil.UnmarshalJSONB(r.Signals, &breakdown)
	var model string
	if r.Model != nil {
		model = *r.Model
	}
	return dto.JobSignalDto{
		ID:        dbutil.UUIDString(r.ID),
		JobID:     dbutil.UUIDString(r.JobId),
		Kind:      r.Kind,
		Score:     int(r.Score),
		Model:     model,
		CreatedAt: dbutil.Timestamp(r.CreatedAt),
		Signals:   breakdown,
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
