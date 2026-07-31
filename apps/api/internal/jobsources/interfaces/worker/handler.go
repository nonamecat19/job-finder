// Package worker holds the jobsources bounded context's inbound worker
// adapters: the asynq ingest task handler (adapter.Search -> dedupe ->
// persist -> enqueue match) and the due-since-lastRunAt cron Scheduler that
// triggers it. Mirrors ingestion.processor.ts / ingestion.scheduler.ts.
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

	"github.com/job-finder/api/internal/activity"
	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbutil"
	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/jobsources/application"
	"github.com/job-finder/api/internal/jobsources/domain"
	"github.com/job-finder/api/internal/queue"
)

// unhealthyAfterConsecutiveFailures is how many consecutive failed source
// runs flip a JobSource to unhealthy. Since ingest tasks retry (see
// IngestMaxRetry), a full retry cycle that never succeeds produces this many
// failed run rows on its own, so the flag now means "one scrape window failed
// outright" rather than "three scheduled windows failed". It self-clears:
// SetJobSourceHealthy(true) runs on the next successful run.
const unhealthyAfterConsecutiveFailures = 3

// permanent wraps an error so asynq drops the task instead of retrying it.
// Use it for failures no amount of waiting can fix — an unparseable payload,
// a source or adapter that isn't registered, a subscription that's gone. The
// transient cases (HTTP 5xx, timeouts, rate limits, a login that needs
// redoing) stay retryable, which is the whole point of retrying ingest: a
// single blip used to cost the source its entire cron window.
func permanent(err error) error {
	return fmt.Errorf("%w: %w", asynq.SkipRetry, err)
}

// lastAttempt reports whether the current asynq delivery is the final one, so
// bookkeeping that must not run per-attempt (flagging the source unhealthy)
// happens once the retries are actually exhausted. Outside a worker context
// both lookups fail and it reports true — a direct ProcessTask call in a test
// is its own last attempt.
func lastAttempt(ctx context.Context) bool {
	retried, ok1 := asynq.GetRetryCount(ctx)
	maxRetry, ok2 := asynq.GetMaxRetry(ctx)
	if !ok1 || !ok2 {
		return true
	}
	return retried >= maxRetry
}

// Handler processes "ingest" asynq tasks: adapter.Search -> dedupe -> persist
// -> enqueue match. Mirrors ingestion.processor.ts.
type Handler struct {
	q         domain.SearchRepository
	registry  *domain.Registry
	sources   *application.Service
	client    application.Enqueuer
	tx        domain.TxRunner
	chunkSize int
}

// HandlerOption configures optional collaborators. They are options rather
// than constructor parameters so the unit tests, which need neither a real
// transaction runner nor a tuned chunk size, keep constructing the handler
// from its ports alone.
type HandlerOption func(*Handler)

// WithTxRunner makes the persist phase atomic. Without one the handler falls
// back to sequential writes against its own repository.
func WithTxRunner(tx domain.TxRunner) HandlerOption {
	return func(h *Handler) { h.tx = tx }
}

// WithChunkSize overrides the number of postings stored per statement batch.
func WithChunkSize(n int) HandlerOption {
	return func(h *Handler) { h.chunkSize = n }
}

func NewHandler(q domain.SearchRepository, registry *domain.Registry, sources *application.Service, client application.Enqueuer, opts ...HandlerOption) *Handler {
	h := &Handler{q: q, registry: registry, sources: sources, client: client, chunkSize: defaultChunkSize}
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

	run, err := h.q.InsertSourceRun(ctx, sqlcgen.InsertSourceRunParams{
		SourceId: source.ID,
		SearchId: payload.SearchID,
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
		uid, err := dbutil.ParseUUID(*payload.SubscriptionID)
		if err != nil {
			h.finishError(ctx, run.ID, source, payload.SourceKey, err)
			return permanent(err)
		}
		sub, err := h.q.GetSubscription(ctx, uid)
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

	// ATS board adapters (013) fan one Search call out over many roster
	// employers; capture their per-employer outcomes on the run regardless
	// of whether the overall Search errored (FR-019, FR-020, FR-023) — a
	// run-level error from a board adapter means "zero employers read"
	// (FR-021), not "no detail worth keeping".
	if reporter, ok := adapter.(domain.EmployerReporter); ok {
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

	var subscriptionID pgtype.UUID
	if subscription != nil {
		subscriptionID = subscription.ID
	}

	// Adapters whose Search returns list-only rows get their downstream
	// analysis deferred: enrichment fetches the real description first, then
	// enqueues match + ghost itself. Scoring the teaser here as well would
	// mean two LLM passes per job, the first one on text the job doesn't
	// actually have.
	needsDetail := domain.NeedsDetail(adapter)

	batch := NewPostingBatch(jobs, subscriptionID, run.ID, needsDetail)
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

	// Redis cannot join the Postgres transaction, so downstream work is
	// queued after commit and only for genuinely new rows. A crash in between
	// leaves the job for the ListJobsMissingMatch sweep, exactly as before.
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

	// Only judge the source's health once asynq has stopped retrying: an
	// earlier attempt still has a chance to succeed and clear the flag, and
	// flipping it mid-cycle would show the source as broken in the UI while
	// the retry that fixes it is still queued.
	if lastAttempt(ctx) {
		h.flagIfUnhealthy(ctx, source, sourceKey)
	}
	slog.Error("ingest failed", "source", sourceKey, "error", msg)
}

// persist stores the whole run atomically. The dedupe key is
// sha256(lower(company)|lower(title)|canonicalUrl) where canonicalUrl strips
// the query string and trailing slashes — it must match
// ingestion.processor.ts:74 exactly, and is unchanged by batching, so every
// previously stored posting is still recognised.
//
// Without a transaction runner injected the same code runs against the
// handler's own repository, so unit tests need no database.
func (h *Handler) persist(ctx context.Context, batch PostingBatch) (PersistResult, error) {
	if h.tx == nil {
		return persistBatch(ctx, h.q, batch, h.chunkSize)
	}
	var result PersistResult
	err := h.tx.WithinTx(ctx, func(q *sqlcgen.Queries) error {
		var err error
		result, err = persistBatch(ctx, q, batch, h.chunkSize)
		return err
	})
	if err != nil {
		return PersistResult{}, err
	}
	return result, nil
}

// enqueueInserted queues downstream work for the postings this run actually
// inserted. needsDetail comes from the source's adapter: when true the row is
// a list-only stub, so match and ghost scoring are left to enrichment (which
// runs them once the real description has landed) instead of being enqueued
// twice against two different versions of the same job.
func (h *Handler) enqueueInserted(ctx context.Context, result PersistResult, needsDetail bool) {
	for _, ins := range result.Inserted {
		jobID := dbutil.UUIDString(ins.JobID)
		if needsDetail {
			if err := h.enqueueEnrich(ctx, jobID, ins.Posting, activityID(ins, opEnrich)); err != nil {
				slog.Warn("ingestion: enqueue enrich failed", "job", jobID, "error", err)
			}
			continue
		}
		h.enqueueMatch(ctx, jobID, activityID(ins, opMatch))
		h.enqueueGhostScore(ctx, jobID, activityID(ins, opGhost))
	}
}

// activityID renders the pre-created ActivityRun id for an op, or nil when the
// batch insert produced no row for it — the task is still queued, just
// untracked, matching the old behaviour when activity.New failed.
func activityID(ins InsertedJob, op string) *string {
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
	// attempts: 2 with exponential backoff, matching matchQueue.add's options.
	opts := []asynq.Option{asynq.MaxRetry(1), asynq.Queue(queue.QueueMatch)}
	if actID != nil {
		opts = append(opts, asynq.TaskID(*actID))
	}
	if _, err := h.client.EnqueueContext(ctx, asynq.NewTask(queue.TypeMatch, payload), opts...); err != nil {
		slog.Warn("ingestion: enqueue match failed", "job", jobID, "error", err)
	}
}

// enqueueGhostScore queues the ghost-job detector (005) to score this job
// right after ingestion. The activity record is tracked for retry/visibility
// only — a scoring failure here must never affect ingestion or the job's own
// record (FR-018); the handler always returns nil to asynq regardless.
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

// enqueueEnrich queues a detail-fetch task for a shallow (list-only) job
// row. No retry: a fetch failure just leaves detailScrapedAt NULL, and the
// next backfill sweep (POST /sources/djinni/enrich) picks it up again.
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

// flagIfUnhealthy marks the source unhealthy after N consecutive failed runs,
// matching flagIfUnhealthy in ingestion.processor.ts.
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
