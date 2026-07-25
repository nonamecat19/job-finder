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
	q            Repository
	sources      *jobsources.Service
	djinni       adapters.DjinniAdapter
	dou          adapters.DouAdapter
	workua       adapters.WorkUaAdapter
	indeed       adapters.IndeedAdapter
	remoteok     adapters.RemoteOKAdapter
	glassdoor    adapters.GlassdoorAdapter
	jobleads     adapters.JobLeadsAdapter
	wellfound    adapters.WellfoundAdapter
	client       Enqueuer
	defaultDelay time.Duration
	delays       map[string]time.Duration
}

func NewHandler(q Repository, sources *jobsources.Service, djinni adapters.DjinniAdapter, dou adapters.DouAdapter, workua adapters.WorkUaAdapter, indeed adapters.IndeedAdapter, remoteok adapters.RemoteOKAdapter, glassdoor adapters.GlassdoorAdapter, jobleads adapters.JobLeadsAdapter, wellfound adapters.WellfoundAdapter, client Enqueuer, defaultDelay time.Duration, delays map[string]time.Duration) *Handler {
	return &Handler{
		q: q, sources: sources,
		djinni: djinni, dou: dou, workua: workua, indeed: indeed, remoteok: remoteok, glassdoor: glassdoor, jobleads: jobleads, wellfound: wellfound,
		client:       client,
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
	case "dou":
		err = h.enrichDOU(ctx, payload, uid, job)
	case "workua":
		err = h.enrichWorkUa(ctx, payload, uid, job)
	case "indeed":
		err = h.enrichIndeed(ctx, payload, uid, job)
	case "remoteok":
		err = h.enrichRemoteOK(ctx, payload, uid, job)
	case "glassdoor":
		err = h.enrichGlassdoor(ctx, payload, uid, job)
	case "jobleads":
		err = h.enrichJobLeads(ctx, payload, uid, job)
	case "wellfound":
		err = h.enrichWellfound(ctx, payload, uid, job)
	default:
		// No enrich branch for this source. It still has to reach match and
		// ghost scoring below: for a NeedsDetail adapter, ingestion skipped
		// both on the assumption that this handler would run them, so
		// returning early here would strand the job unscored forever.
		slog.Warn("enrichment: no detail fetcher for source, scoring the listing as-is",
			"job", payload.JobID, "source", job.SourceKey)
	}
	if err != nil {
		return err
	}

	// Reached on every non-error path, including the ones where the detail
	// fetch was skipped or gave up (fetch error, listing delisted). Scoring a
	// stub description is worse than scoring a full one but far better than a
	// job that never gets a score at all and so never surfaces in the
	// score-sorted feed.
	h.enqueueMatch(ctx, payload.JobID, job)
	h.enqueueGhostScore(ctx, payload.JobID)
	return nil
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

	slog.Info("enrichment: workua complete", "job", payload.JobID)
	return nil
}

func (h *Handler) enrichIndeed(ctx context.Context, payload queue.EnrichPayload, uid pgtype.UUID, job sqlcgen.Job) error {
	if delay := h.delayFor("indeed"); delay > 0 {
		time.Sleep(delay)
	}

	patch, err := h.indeed.FetchDetail(ctx, job.Url, nil)
	if err != nil {
		slog.Warn("enrichment: indeed fetch detail failed", "job", payload.JobID, "url", job.Url, "error", err)
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
		return fmt.Errorf("enrichment: update indeed job detail: %w", err)
	}

	slog.Info("enrichment: indeed complete", "job", payload.JobID)
	return nil
}

func (h *Handler) enrichRemoteOK(ctx context.Context, payload queue.EnrichPayload, uid pgtype.UUID, job sqlcgen.Job) error {
	if delay := h.delayFor("remoteok"); delay > 0 {
		time.Sleep(delay)
	}

	patch, err := h.remoteok.FetchDetail(ctx, job.Url, nil)
	if err != nil {
		slog.Warn("enrichment: remoteok fetch detail failed", "job", payload.JobID, "url", job.Url, "error", err)
		return nil
	}
	if !patch.Available {
		slog.Info("enrichment: remoteok listing no longer in feed, leaving existing data untouched", "job", payload.JobID, "url", job.Url)
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
		Location:    nil,
		Remote:      true,
		Raw:         raw,
		PostedAt:    dbutil.TimestampFromPtr(patch.PostedAt),
	}); err != nil {
		return fmt.Errorf("enrichment: update remoteok job detail: %w", err)
	}

	slog.Info("enrichment: remoteok complete", "job", payload.JobID)
	return nil
}

func (h *Handler) enrichJobLeads(ctx context.Context, payload queue.EnrichPayload, uid pgtype.UUID, job sqlcgen.Job) error {
	if delay := h.delayFor("jobleads"); delay > 0 {
		time.Sleep(delay)
	}

	patch, err := h.jobleads.FetchDetail(ctx, job.Url, nil)
	if err != nil {
		slog.Warn("enrichment: jobleads fetch detail failed", "job", payload.JobID, "url", job.Url, "error", err)
		return nil
	}
	if !patch.Available {
		slog.Info("enrichment: jobleads listing no longer available, leaving existing data untouched", "job", payload.JobID, "url", job.Url)
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
		Location:    job.Location,
		Remote:      job.Remote,
		Raw:         raw,
		PostedAt:    dbutil.TimestampFromPtr(patch.PostedAt),
	}); err != nil {
		return fmt.Errorf("enrichment: update jobleads job detail: %w", err)
	}

	slog.Info("enrichment: jobleads complete", "job", payload.JobID)
	return nil
}

func (h *Handler) enrichGlassdoor(ctx context.Context, payload queue.EnrichPayload, uid pgtype.UUID, job sqlcgen.Job) error {
	if delay := h.delayFor("glassdoor"); delay > 0 {
		time.Sleep(delay)
	}

	patch, err := h.glassdoor.FetchDetail(ctx, job.Url, nil)
	if err != nil {
		slog.Warn("enrichment: glassdoor fetch detail failed", "job", payload.JobID, "url", job.Url, "error", err)
		return nil
	}
	if !patch.Available {
		slog.Info("enrichment: glassdoor listing no longer available, leaving existing data untouched", "job", payload.JobID, "url", job.Url)
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
		Location:    job.Location,
		Remote:      job.Remote,
		Raw:         raw,
		PostedAt:    dbutil.TimestampFromPtr(patch.PostedAt),
	}); err != nil {
		return fmt.Errorf("enrichment: update glassdoor job detail: %w", err)
	}

	slog.Info("enrichment: glassdoor complete", "job", payload.JobID)
	return nil
}

func (h *Handler) enrichWellfound(ctx context.Context, payload queue.EnrichPayload, uid pgtype.UUID, job sqlcgen.Job) error {
	if delay := h.delayFor("wellfound"); delay > 0 {
		time.Sleep(delay)
	}

	patch, err := h.wellfound.FetchDetail(ctx, job.Url, nil)
	if err != nil {
		slog.Warn("enrichment: wellfound fetch detail failed", "job", payload.JobID, "url", job.Url, "error", err)
		return nil
	}
	if !patch.Available {
		slog.Info("enrichment: wellfound listing no longer available, leaving existing data untouched", "job", payload.JobID, "url", job.Url)
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
		Location:    job.Location,
		Remote:      job.Remote,
		Raw:         raw,
		PostedAt:    dbutil.TimestampFromPtr(patch.PostedAt),
	}); err != nil {
		return fmt.Errorf("enrichment: update wellfound job detail: %w", err)
	}

	h.enqueueMatch(ctx, payload.JobID, job)
	slog.Info("enrichment: wellfound complete", "job", payload.JobID)
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
	opts := []asynq.Option{asynq.MaxRetry(1), asynq.Queue(queue.QueueMatch)}
	if actID != nil {
		opts = append(opts, asynq.TaskID(*actID))
	}
	if _, err := h.client.EnqueueContext(ctx, asynq.NewTask(queue.TypeMatch, matchPayload), opts...); err != nil {
		slog.Warn("enrichment: enqueue match failed", "job", jobID, "error", err)
	}
}

// enqueueGhostScore queues the ghost-job detector (005) once the real
// description has landed. Ingestion no longer does this for NeedsDetail
// sources: the detector's cross-board signal is unmeasurable against a teaser
// description (ghostjob.measureCrossBoard), so scoring at ingest time threw
// away the signal it exists to read. As in ingestion, a failure here is
// logged and swallowed — ghost scoring never blocks the job's own record.
func (h *Handler) enqueueGhostScore(ctx context.Context, jobID string) {
	var actID *string
	rec := activity.New(ctx, h.q, "ghost_score", "ghost score", &jobID, nil, "")
	if rec != nil {
		idStr := dbutil.UUIDString(rec.ID())
		actID = &idStr
	}

	payload, err := json.Marshal(queue.GhostScorePayload{JobID: jobID, ActivityID: actID})
	if err != nil {
		return
	}
	opts := []asynq.Option{asynq.MaxRetry(0), asynq.Queue(queue.QueueGhostScore)}
	if actID != nil {
		opts = append(opts, asynq.TaskID(*actID))
	}
	if _, err := h.client.EnqueueContext(ctx, asynq.NewTask(queue.TypeGhostScore, payload), opts...); err != nil {
		slog.Warn("enrichment: enqueue ghost score failed", "job", jobID, "error", err)
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
		opts := []asynq.Option{asynq.MaxRetry(0), asynq.Queue(queue.QueueEnrich)}
		if actID != nil {
			opts = append(opts, asynq.TaskID(*actID))
		}
		if _, err := h.client.EnqueueContext(ctx, asynq.NewTask(queue.TypeEnrich, payload), opts...); err != nil {
			return n, fmt.Errorf("enrichment: enqueue backfill: %w", err)
		}
		n++
	}
	return n, nil
}
