package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"

	jsadapter "github.com/nonamecat19/jobscraper/adapter"
	"github.com/nonamecat19/jobscraper/ports"

	"github.com/job-finder/api/internal/activity"
	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbutil"
	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/jobsources/application"
	"github.com/job-finder/api/internal/jobsources/application/ingest"
	"github.com/job-finder/api/internal/jobsources/domain"
	"github.com/job-finder/api/internal/queue"
)

const unhealthyAfterConsecutiveFailures = 3

// triggerScheduled marks a SourceRun written by this worker. Manual adds write
// "manual" instead, which keeps them out of the health accounting above.
const triggerScheduled = "scheduled"

func permanent(err error) error {
	return fmt.Errorf("%w: %w", asynq.SkipRetry, err)
}

func lastAttempt(ctx context.Context) bool {
	retried, ok1 := asynq.GetRetryCount(ctx)
	maxRetry, ok2 := asynq.GetMaxRetry(ctx)
	if !ok1 || !ok2 {
		return true
	}
	return retried >= maxRetry
}

type Handler struct {
	q         domain.SearchRepository
	registry  *jsadapter.Registry
	sources   *application.Service
	client    application.Enqueuer
	tx        domain.TxRunner
	chunkSize int
}

type HandlerOption func(*Handler)

func WithTxRunner(tx domain.TxRunner) HandlerOption {
	return func(h *Handler) { h.tx = tx }
}

func WithChunkSize(n int) HandlerOption {
	return func(h *Handler) { h.chunkSize = n }
}

func NewHandler(q domain.SearchRepository, registry *jsadapter.Registry, sources *application.Service, client application.Enqueuer, opts ...HandlerOption) *Handler {
	h := &Handler{q: q, registry: registry, sources: sources, client: client, chunkSize: ingest.DefaultChunkSize}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

func (h *Handler) ProcessTask(ctx context.Context, t *asynq.Task) (err error) {
	var payload queue.IngestPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return permanent(fmt.Errorf("ingestion: invalid payload: %w", err))
	}

	var rec *activity.Recorder
	if payload.ActivityID != nil && *payload.ActivityID != "" {
		rec = activity.FromID(h.q, *payload.ActivityID)
	}

	if rec != nil {
		rec.Start(ctx)
	}

	source, err := h.sources.GetByKey(ctx, payload.SourceKey)
	if err != nil {
		if rec != nil {
			rec.Fail(ctx, err)
		}
		return permanent(err)
	}

	// Parsed before the run is inserted so the run carries its subscription
	// link from the start (041 D7). The subscription row itself is still
	// fetched below, where its URL is needed.
	var subscriptionID pgtype.UUID
	if payload.SubscriptionID != nil {
		uid, parseErr := dbutil.ParseUUID(*payload.SubscriptionID)
		if parseErr != nil {
			if rec != nil {
				rec.Fail(ctx, parseErr)
			}
			return permanent(parseErr)
		}
		subscriptionID = uid
	}

	run, err := h.q.InsertSourceRun(ctx, sqlcgen.InsertSourceRunParams{
		SourceId:       source.ID,
		SearchId:       payload.SearchID,
		SubscriptionId: subscriptionID,
		Trigger:        triggerScheduled,
	})
	if err != nil {
		if rec != nil {
			rec.Fail(ctx, err)
		}
		return fmt.Errorf("ingestion: insert source run: %w", err)
	}

	created := 0
	var jobs []dto.NormalizedJob
	defer func() {
		if rec != nil {
			if err != nil {
				rec.Fail(ctx, err)
			} else {
				rec.Ok(ctx, dbutil.UUIDString(run.ID), map[string]any{
					"found": len(jobs),
					"new":   created,
				})
			}
		}
	}()

	query := dto.SearchQuery{Keywords: ""}
	if payload.SearchID != nil {
		if uid, err := dbutil.ParseUUID(*payload.SearchID); err == nil {
			if search, err := h.q.GetSavedSearch(ctx, uid); err == nil {
				_ = dbutil.UnmarshalJSONB(search.Query, &query)
			}
		}
	}

	var subscription *sqlcgen.Subscription
	if payload.SubscriptionID != nil {
		sub, err := h.q.GetSubscription(ctx, subscriptionID)
		if err != nil {
			h.finishError(ctx, run.ID, source, payload.SourceKey, err)
			return permanent(err)
		}
		subscription = &sub
		query.SubscriptionURL = sub.Url
	}

	adapter, err := h.registry.Get(payload.SourceKey)
	if err != nil {
		h.finishError(ctx, run.ID, source, payload.SourceKey, err)
		return permanent(err)
	}
	config := h.sources.DecryptConfig(source.Config)

	if rec != nil {
		rec.Step(ctx, fmt.Sprintf("scraping %s", payload.SourceKey), nil)
	}

	jobs, err = adapter.Search(ctx, query, config)

	if reporter, ok := adapter.(ports.EmployerReporter); ok {
		if detail, mErr := json.Marshal(reporter.LastRunDetail()); mErr == nil {
			_ = h.q.SetSourceRunEmployerDetail(ctx, sqlcgen.SetSourceRunEmployerDetailParams{ID: run.ID, EmployerDetail: detail})
		}
	}

	if err != nil {
		h.finishError(ctx, run.ID, source, payload.SourceKey, err)
		return err
	}

	if rec != nil {
		rec.Step(ctx, fmt.Sprintf("persisting %d found jobs", len(jobs)), nil)
	}

	needsDetail := jsadapter.NeedsDetail(adapter)

	batch := ingest.NewPostingBatch(jobs, subscriptionID, run.ID, needsDetail)
	started := time.Now()
	result, err := h.persist(ctx, batch)
	if err != nil {
		h.finishError(ctx, run.ID, source, payload.SourceKey, err)
		return err
	}
	created = len(result.Inserted)
	slog.Info("ingest persisted",
		"source", payload.SourceKey, "durationMs", time.Since(started).Milliseconds(),
		"statements", result.Statements, "inserted", created,
		"reposted", result.Reposted, "merged", result.Merged, "skipped", result.Skipped)

	h.enqueueInserted(ctx, result, needsDetail)

	verdict := computeVerdict(len(jobs), nil)
	h.writeVerdict(ctx, run.ID, verdict, 0, "")
	_ = h.q.SetJobSourceHealthy(ctx, sqlcgen.SetJobSourceHealthyParams{Key: payload.SourceKey, Healthy: true})
	if subscription != nil {
		_ = h.q.TouchSubscriptionLastRun(ctx, subscription.ID)
	}
	slog.Info("ingest complete", "source", payload.SourceKey, "found", len(jobs), "new", created)
	return nil
}

func (h *Handler) finishError(ctx context.Context, runID pgtype.UUID, source sqlcgen.JobSource, sourceKey string, cause error) {
	msg := cause.Error()
	if len(msg) > 1000 {
		msg = msg[:1000]
	}
	_ = h.q.FinishSourceRunError(ctx, sqlcgen.FinishSourceRunErrorParams{ID: runID, Error: &msg})

	verdict, blockedCount, blockReason := computeErrorVerdict(cause)
	h.writeVerdict(ctx, runID, verdict, blockedCount, blockReason)

	if lastAttempt(ctx) {
		h.flagIfUnhealthy(ctx, source, sourceKey)
	}
	slog.Error("ingest failed", "source", sourceKey, "error", msg)
}

func (h *Handler) persist(ctx context.Context, batch ingest.PostingBatch) (ingest.PersistResult, error) {
	if h.tx == nil {
		return ingest.PersistBatch(ctx, h.q, batch, h.chunkSize)
	}
	var result ingest.PersistResult
	err := h.tx.WithinTx(ctx, func(q *sqlcgen.Queries) error {
		var err error
		result, err = ingest.PersistBatch(ctx, q, batch, h.chunkSize)
		return err
	})
	if err != nil {
		return ingest.PersistResult{}, err
	}
	return result, nil
}

func (h *Handler) enqueueInserted(ctx context.Context, result ingest.PersistResult, needsDetail bool) {
	for _, ins := range result.Inserted {
		jobID := dbutil.UUIDString(ins.JobID)
		if needsDetail {
			if err := h.enqueueEnrich(ctx, jobID, ins.Posting, activityID(ins, ingest.OpEnrich)); err != nil {
				slog.Warn("ingestion: enqueue enrich failed", "job", jobID, "error", err)
			}
			continue
		}
		h.enqueueMatch(ctx, jobID, activityID(ins, ingest.OpMatch))
		h.enqueueGhostScore(ctx, jobID, activityID(ins, ingest.OpGhost))
	}
}

func activityID(ins ingest.InsertedJob, op string) *string {
	id, ok := ins.ActivityIDs[op]
	if !ok || !id.Valid {
		return nil
	}
	s := dbutil.UUIDString(id)
	return &s
}

func (h *Handler) enqueueMatch(ctx context.Context, jobID string, actID *string) {
	payload, err := json.Marshal(queue.MatchPayload{JobID: jobID, ActivityID: actID})
	if err != nil {
		return
	}
	opts := []asynq.Option{asynq.MaxRetry(1), asynq.Queue(queue.QueueMatch)}
	if actID != nil {
		opts = append(opts, asynq.TaskID(*actID))
	}
	if _, err := h.client.EnqueueContext(ctx, asynq.NewTask(queue.TypeMatch, payload), opts...); err != nil {
		slog.Warn("ingestion: enqueue match failed", "job", jobID, "error", err)
	}
}

func (h *Handler) enqueueGhostScore(ctx context.Context, jobID string, actID *string) {
	payload, err := json.Marshal(queue.GhostScorePayload{JobID: jobID, ActivityID: actID})
	if err != nil {
		return
	}
	opts := []asynq.Option{asynq.MaxRetry(0), asynq.Queue(queue.QueueGhostScore)}
	if actID != nil {
		opts = append(opts, asynq.TaskID(*actID))
	}
	if _, err := h.client.EnqueueContext(ctx, asynq.NewTask(queue.TypeGhostScore, payload), opts...); err != nil {
		slog.Warn("ingestion: enqueue ghost score failed", "job", jobID, "error", err)
	}
}

func (h *Handler) enqueueEnrich(ctx context.Context, jobID string, j dto.NormalizedJob, actID *string) error {
	payload, err := json.Marshal(queue.EnrichPayload{JobID: jobID, ActivityID: actID})
	if err != nil {
		return err
	}
	opts := []asynq.Option{asynq.MaxRetry(0), asynq.Queue(queue.QueueEnrich)}
	if actID != nil {
		opts = append(opts, asynq.TaskID(*actID))
	}
	if _, err := h.client.EnqueueContext(ctx, asynq.NewTask(queue.TypeEnrich, payload), opts...); err != nil {
		return fmt.Errorf("ingestion: enqueue enrich: %w", err)
	}
	return nil
}

func (h *Handler) flagIfUnhealthy(ctx context.Context, sourceID sqlcgen.JobSource, sourceKey string) {
	recent, err := h.q.RecentSourceRunsForSource(ctx, sqlcgen.RecentSourceRunsForSourceParams{
		SourceId: sourceID.ID,
		Limit:    unhealthyAfterConsecutiveFailures,
	})
	if err != nil || len(recent) != unhealthyAfterConsecutiveFailures {
		return
	}
	allFailed := true
	for _, ok := range recent {
		if ok == nil || *ok {
			allFailed = false
			break
		}
	}
	if allFailed {
		_ = h.q.SetJobSourceHealthy(ctx, sqlcgen.SetJobSourceHealthyParams{Key: sourceKey, Healthy: false})
		slog.Warn("source flagged unhealthy after consecutive failures", "source", sourceKey, "count", unhealthyAfterConsecutiveFailures)
	}
}

func (h *Handler) writeVerdict(ctx context.Context, runID pgtype.UUID, verdict string, blockedCount int32, blockReason string) {
	var reason *string
	if blockReason != "" {
		reason = &blockReason
	}
	v := verdict
	_ = h.q.SetSourceRunVerdict(ctx, sqlcgen.SetSourceRunVerdictParams{
		ID:           runID,
		Verdict:      &v,
		BlockedCount: blockedCount,
		BlockReason:  reason,
	})
}

func computeVerdict(found int, err error) string {
	if err != nil {
		errStr := err.Error()
		if isBlockError(errStr) || isChallengeError(errStr) || isRefusedError(errStr) {
			return "blocked"
		}
		return "blocked"
	}
	if found == 0 {
		return "partial"
	}
	return "success"
}

func computeErrorVerdict(err error) (string, int32, string) {
	if err == nil {
		return "success", 0, ""
	}
	errStr := err.Error()
	blockedCount := int32(1)
	blockReason := errStr
	if len(blockReason) > 500 {
		blockReason = blockReason[:500]
	}
	return "blocked", blockedCount, blockReason
}

func isBlockError(errStr string) bool {
	lower := strings.ToLower(errStr)
	blockMarkers := []string{"blocked", "challenged", "refused", "deferred", "interstitial"}
	for _, m := range blockMarkers {
		if strings.Contains(lower, m) {
			return true
		}
	}
	return false
}

func isChallengeError(errStr string) bool {
	lower := strings.ToLower(errStr)
	return strings.Contains(lower, "challenged") || strings.Contains(lower, "challenge")
}

func isRefusedError(errStr string) bool {
	lower := strings.ToLower(errStr)
	return strings.Contains(lower, "refused")
}
