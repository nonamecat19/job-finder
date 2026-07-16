package enrichment

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/activity"
	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbutil"
	"github.com/job-finder/api/internal/jobsources"
	"github.com/job-finder/api/internal/jobsources/adapters"
	"github.com/job-finder/api/internal/queue"
)

type Handler struct {
	q            *sqlcgen.Queries
	sources      *jobsources.Service
	djinni       adapters.DjinniAdapter
	dou          adapters.DouAdapter
	workua       adapters.WorkUaAdapter
	client       *asynq.Client
	defaultDelay time.Duration
	delays       map[string]time.Duration
}

func NewHandler(q *sqlcgen.Queries, sources *jobsources.Service, djinni adapters.DjinniAdapter, dou adapters.DouAdapter, workua adapters.WorkUaAdapter, client *asynq.Client, defaultDelay time.Duration, delays map[string]time.Duration) *Handler {
	return &Handler{
		q: q, sources: sources,
		djinni: djinni, dou: dou, workua: workua,
		client: client,
		defaultDelay: defaultDelay,
		delays:       delays,
	}
}

func (h *Handler) delayFor(sourceKey string) time.Duration {
	if h.delays != nil {
		if d, ok := h.delays[sourceKey]; ok {
			return d
		}
	}
	return h.defaultDelay
}

func (h *Handler) ProcessTask(ctx context.Context, t *asynq.Task) (err error) {
	var payload queue.EnrichPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("enrichment: invalid payload: %w", err)
	}

	var rec *activity.Recorder
	if payload.ActivityID != nil && *payload.ActivityID != "" {
		rec = activity.FromID(h.q, *payload.ActivityID)
	}

	if rec != nil {
		rec.Start(ctx)
	}

	defer func() {
		if rec != nil {
			if err != nil {
				rec.Fail(ctx, err)
			} else {
				rec.Ok(ctx, "", nil)
			}
		}
	}()

	uid, err := dbutil.ParseUUID(payload.JobID)
	if err != nil {
		return err
	}
	job, err := h.q.GetJobByID(ctx, uid)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	}
	if job.DetailScrapedAt.Valid {
		return nil
	}

	if rec != nil {
		rec.Step(ctx, fmt.Sprintf("fetching detail (%s)", job.SourceKey), nil)
	}

	switch job.SourceKey {
	case "djinni":
		err = h.enrichDjinni(ctx, payload, uid, job)
		return err
	case "dou":
		err = h.enrichDOU(ctx, payload, uid, job)
		return err
	case "workua":
		err = h.enrichWorkUa(ctx, payload, uid, job)
		return err
	default:
		return nil
	}
}

func (h *Handler) enrichDjinni(ctx context.Context, payload queue.EnrichPayload, uid pgtype.UUID, job sqlcgen.Job) error {
	source, err := h.sources.GetByKey(ctx, job.SourceKey)
	if err != nil {
		return err
	}
	config := h.sources.DecryptConfig(source.Config)

	if delay := h.delayFor("djinni"); delay > 0 {
		time.Sleep(delay)
	}

	patch, err := h.djinni.FetchDetail(ctx, job.Url, config)
	if err != nil {
		slog.Warn("enrichment: djinni fetch detail failed", "job", payload.JobID, "url", job.Url, "error", err)
		return nil
	}

	raw, err := json.Marshal(patch.Raw)
	if err != nil {
		raw = []byte("{}")
	}
	if _, err := h.q.UpdateJobDetail(ctx, sqlcgen.UpdateJobDetailParams{
		ID:          uid,
		Description: patch.Description,
		SalaryRaw:   patch.SalaryRaw,
		Location:    patch.Location,
		Remote:      patch.Remote,
		Raw:         raw,
		PostedAt:    dbutil.TimestampFromPtr(patch.PostedAt),
	}); err != nil {
		return fmt.Errorf("enrichment: update djinni job detail: %w", err)
	}

	h.enqueueMatch(ctx, payload.JobID, job)
	slog.Info("enrichment: djinni complete", "job", payload.JobID)
	return nil
}

func (h *Handler) enrichDOU(ctx context.Context, payload queue.EnrichPayload, uid pgtype.UUID, job sqlcgen.Job) error {
	if delay := h.delayFor("dou"); delay > 0 {
		time.Sleep(delay)
	}

	patch, err := h.dou.FetchDetail(ctx, job.Url, nil)
	if err != nil {
		slog.Warn("enrichment: dou fetch detail failed", "job", payload.JobID, "url", job.Url, "error", err)
		return nil
	}

	rawJSON := map[string]any{"detailText": patch.DetailText, "detailHtml": patch.DetailHTML}
	raw, err := json.Marshal(rawJSON)
	if err != nil {
		raw = []byte("{}")
	}

	var postedAt pgtype.Timestamp
	if patch.PostedAt != nil {
		postedAt = pgtype.Timestamp{
			Time:  *patch.PostedAt,
			Valid: true,
		}
	}

	if _, err := h.q.UpdateJobDetail(ctx, sqlcgen.UpdateJobDetailParams{
		ID:          uid,
		Description: patch.DetailText,
		SalaryRaw:   nil,
		Location:    nil,
		Remote:      patch.IsRemote,
		Raw:         raw,
		PostedAt:    postedAt,
	}); err != nil {
		return fmt.Errorf("enrichment: update dou job detail: %w", err)
	}

	h.enqueueMatch(ctx, payload.JobID, job)
	slog.Info("enrichment: dou complete", "job", payload.JobID)
	return nil
}

func (h *Handler) enrichWorkUa(ctx context.Context, payload queue.EnrichPayload, uid pgtype.UUID, job sqlcgen.Job) error {
	delay := h.delayFor("workua")
	if delay < adapters.WorkUaMinDelay {
		delay = adapters.WorkUaMinDelay
	}
	time.Sleep(delay)

	patch, err := h.workua.FetchDetail(ctx, job.Url, nil)
	if err != nil {
		slog.Warn("enrichment: workua fetch detail failed", "job", payload.JobID, "url", job.Url, "error", err)
		return nil
	}

	raw, err := json.Marshal(patch.Raw)
	if err != nil {
		raw = []byte("{}")
	}
	if _, err := h.q.UpdateJobDetail(ctx, sqlcgen.UpdateJobDetailParams{
		ID:          uid,
		Description: patch.Description,
		SalaryRaw:   patch.SalaryRaw,
		Location:    patch.Location,
		Remote:      patch.Remote,
		Raw:         raw,
		PostedAt:    dbutil.TimestampFromPtr(patch.PostedAt),
	}); err != nil {
		return fmt.Errorf("enrichment: update workua job detail: %w", err)
	}

	h.enqueueMatch(ctx, payload.JobID, job)
	slog.Info("enrichment: workua complete", "job", payload.JobID)
	return nil
}

func (h *Handler) enqueueMatch(ctx context.Context, jobID string, job sqlcgen.Job) {
	var actID *string
	matchRec := activity.New(ctx, h.q, "match", fmt.Sprintf("%s — %s", job.Company, job.Title), &jobID, nil, "")
	if matchRec != nil {
		idStr := dbutil.UUIDString(matchRec.ID())
		actID = &idStr
	}

	matchPayload, err := json.Marshal(queue.MatchPayload{JobID: jobID, ActivityID: actID})
	if err != nil {
		return
	}
	if _, err := h.client.EnqueueContext(ctx, asynq.NewTask(queue.TypeMatch, matchPayload),
		asynq.MaxRetry(1), asynq.Queue(queue.QueueMatch)); err != nil {
		slog.Warn("enrichment: enqueue match failed", "job", jobID, "error", err)
	}
}

func (h *Handler) EnqueueBackfill(ctx context.Context, sourceKey string, limit int32) (int, error) {
	jobs, err := h.q.ListJobsNeedingDetail(ctx, sqlcgen.ListJobsNeedingDetailParams{SourceKey: sourceKey, Limit: limit})
	if err != nil {
		return 0, err
	}
	n := 0
	for _, j := range jobs {
		jobID := dbutil.UUIDString(j.ID)
		var actID *string
		enrichRec := activity.New(ctx, h.q, "enrich", fmt.Sprintf("%s — %s", j.Company, j.Title), &jobID, &j.SourceKey, "")
		if enrichRec != nil {
			idStr := dbutil.UUIDString(enrichRec.ID())
			actID = &idStr
		}

		payload, err := json.Marshal(queue.EnrichPayload{JobID: jobID, ActivityID: actID})
		if err != nil {
			continue
		}
		if _, err := h.client.EnqueueContext(ctx, asynq.NewTask(queue.TypeEnrich, payload),
			asynq.MaxRetry(0), asynq.Queue(queue.QueueEnrich)); err != nil {
			return n, fmt.Errorf("enrichment: enqueue backfill: %w", err)
		}
		n++
	}
	return n, nil
}
