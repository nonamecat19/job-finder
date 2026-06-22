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

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbutil"
	"github.com/job-finder/api/internal/jobsources"
	"github.com/job-finder/api/internal/jobsources/adapters"
	"github.com/job-finder/api/internal/queue"
)

type Handler struct {
	q       *sqlcgen.Queries
	sources *jobsources.Service
	djinni  adapters.DjinniAdapter
	dou     adapters.DouAdapter
	client  *asynq.Client
	delay   time.Duration
}

func NewHandler(q *sqlcgen.Queries, sources *jobsources.Service, djinni adapters.DjinniAdapter, dou adapters.DouAdapter, client *asynq.Client, delay time.Duration) *Handler {
	return &Handler{q: q, sources: sources, djinni: djinni, dou: dou, client: client, delay: delay}
}

func (h *Handler) ProcessTask(ctx context.Context, t *asynq.Task) error {
	var payload queue.EnrichPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("enrichment: invalid payload: %w", err)
	}

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

	switch job.SourceKey {
	case "djinni":
		return h.enrichDjinni(ctx, payload, uid, job)
	case "dou":
		return h.enrichDOU(ctx, payload, uid, job)
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

	if h.delay > 0 {
		time.Sleep(h.delay)
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

	h.enqueueMatch(ctx, payload.JobID)
	slog.Info("enrichment: djinni complete", "job", payload.JobID)
	return nil
}

func (h *Handler) enrichDOU(ctx context.Context, payload queue.EnrichPayload, uid pgtype.UUID, job sqlcgen.Job) error {
	if h.delay > 0 {
		time.Sleep(h.delay)
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

	h.enqueueMatch(ctx, payload.JobID)
	slog.Info("enrichment: dou complete", "job", payload.JobID)
	return nil
}

func (h *Handler) enqueueMatch(ctx context.Context, jobID string) {
	matchPayload, err := json.Marshal(queue.MatchPayload{JobID: jobID})
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
		payload, err := json.Marshal(queue.EnrichPayload{JobID: dbutil.UUIDString(j.ID)})
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
