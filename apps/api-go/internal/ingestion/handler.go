package ingestion

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api-go/internal/db/sqlcgen"
	"github.com/job-finder/api-go/internal/dbutil"
	"github.com/job-finder/api-go/internal/dto"
	"github.com/job-finder/api-go/internal/jobsources"
	"github.com/job-finder/api-go/internal/queue"
)

const unhealthyAfterConsecutiveFailures = 3

// Handler processes "ingest" asynq tasks: adapter.Search -> dedupe -> persist
// -> enqueue match. Mirrors ingestion.processor.ts.
type Handler struct {
	q        *sqlcgen.Queries
	registry *jobsources.Registry
	sources  *jobsources.Service
	client   *asynq.Client
}

func NewHandler(q *sqlcgen.Queries, registry *jobsources.Registry, sources *jobsources.Service, client *asynq.Client) *Handler {
	return &Handler{q: q, registry: registry, sources: sources, client: client}
}

func (h *Handler) ProcessTask(ctx context.Context, t *asynq.Task) error {
	var payload queue.IngestPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("ingestion: invalid payload: %w", err)
	}

	source, err := h.sources.GetByKey(ctx, payload.SourceKey)
	if err != nil {
		return err
	}

	run, err := h.q.InsertSourceRun(ctx, sqlcgen.InsertSourceRunParams{
		SourceId: source.ID,
		SearchId: payload.SearchID,
	})
	if err != nil {
		return fmt.Errorf("ingestion: insert source run: %w", err)
	}

	query := dto.SearchQuery{Keywords: ""}
	if payload.SearchID != nil {
		if uid, err := dbutil.ParseUUID(*payload.SearchID); err == nil {
			if search, err := h.q.GetSavedSearch(ctx, uid); err == nil {
				_ = dbutil.UnmarshalJSONB(search.Query, &query)
			}
		}
	}

	adapter, err := h.registry.Get(payload.SourceKey)
	if err != nil {
		h.finishError(ctx, run.ID, source, payload.SourceKey, err)
		return err
	}
	config := h.sources.DecryptConfig(source.Config)

	jobs, err := adapter.Search(ctx, query, config)
	if err != nil {
		h.finishError(ctx, run.ID, source, payload.SourceKey, err)
		return err
	}

	created := 0
	for _, j := range jobs {
		isNew, err := h.persistIfNew(ctx, j)
		if err != nil {
			h.finishError(ctx, run.ID, source, payload.SourceKey, err)
			return err
		}
		if isNew {
			created++
		}
	}

	if err := h.q.FinishSourceRunOk(ctx, sqlcgen.FinishSourceRunOkParams{
		ID: run.ID, Found: int32(len(jobs)), New: int32(created),
	}); err != nil {
		return err
	}
	_ = h.q.SetJobSourceHealthy(ctx, sqlcgen.SetJobSourceHealthyParams{Key: payload.SourceKey, Healthy: true})
	slog.Info("ingest complete", "source", payload.SourceKey, "found", len(jobs), "new", created)
	return nil
}

func (h *Handler) finishError(ctx context.Context, runID pgtype.UUID, source sqlcgen.JobSource, sourceKey string, cause error) {
	msg := cause.Error()
	if len(msg) > 1000 {
		msg = msg[:1000]
	}
	_ = h.q.FinishSourceRunError(ctx, sqlcgen.FinishSourceRunErrorParams{ID: runID, Error: &msg})
	h.flagIfUnhealthy(ctx, source, sourceKey)
	slog.Error("ingest failed", "source", sourceKey, "error", msg)
}

// persistIfNew dedupes by sha256(lower(company)|lower(title)|canonicalUrl)
// where canonicalUrl strips the query string and trailing slashes — must
// match ingestion.processor.ts:74 exactly.
func (h *Handler) persistIfNew(ctx context.Context, j dto.NormalizedJob) (bool, error) {
	dedupeKey := DedupeKey(j.Company, j.Title, j.URL)

	_, err := h.q.GetJobByDedupeKey(ctx, dedupeKey)
	if err == nil {
		return false, nil // already exists
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return false, err
	}

	raw, err := json.Marshal(j.Raw)
	if err != nil {
		raw = []byte("{}")
	}

	created, err := h.q.InsertJob(ctx, sqlcgen.InsertJobParams{
		DedupeKey:   dedupeKey,
		SourceKey:   j.SourceKey,
		ExternalId:  j.ExternalID,
		Title:       j.Title,
		Company:     j.Company,
		Location:    j.Location,
		Remote:      j.Remote,
		SalaryRaw:   j.SalaryRaw,
		Url:         j.URL,
		Description: j.Description,
		Raw:         raw,
		PostedAt:    dbutil.TimestampFromPtr(j.PostedAt),
	})
	if err != nil {
		return false, fmt.Errorf("ingestion: insert job: %w", err)
	}

	payload, err := json.Marshal(queue.MatchPayload{JobID: dbutil.UUIDString(created.ID)})
	if err != nil {
		return true, err
	}
	// attempts: 2 with exponential backoff, matching matchQueue.add's options.
	if _, err := h.client.EnqueueContext(ctx, asynq.NewTask(queue.TypeMatch, payload), asynq.MaxRetry(1)); err != nil {
		return true, fmt.Errorf("ingestion: enqueue match: %w", err)
	}
	return true, nil
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
