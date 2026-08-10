package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbutil"
	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/jobsources/application/ingest"
	jobsources "github.com/job-finder/api/internal/jobsources/domain"
	"github.com/job-finder/api/internal/manualadd/domain"
	"github.com/job-finder/api/internal/queue"
)

// AddTimeout is the whole budget for one manual add: resolution, per-host
// pacing, fetch, parse and persist. One number, because that is the only one
// the operator cares about (FR-003b, D6).
const AddTimeout = 30 * time.Second

// manualTrigger marks the SourceRun a manual add writes. Runs carrying it are
// excluded from source-health accounting (FR-017g).
const manualTrigger = "manual"

// ManualSourceKey backs hand-entered vacancies on hosts no adapter reads.
const ManualSourceKey = "manual"

type Service struct {
	q        domain.Repository
	tx       domain.TxRunner
	sources  domain.SourceProvider
	subs     domain.SubscriptionEnsurer
	registry domain.AdapterRegistry
	jobs     domain.JobReader
	client   domain.Enqueuer

	// fillInEnabled switches no_reader and incomplete from plain failures to
	// 200 responses carrying a draft. It is on once User Story 3's form exists;
	// until then the operator gets a reason and stops (contracts/manual-add.md).
	fillInEnabled bool
}

type Option func(*Service)

// WithFillIn turns on the draft-carrying outcomes (User Story 3).
func WithFillIn() Option {
	return func(s *Service) { s.fillInEnabled = true }
}

func WithTxRunner(tx domain.TxRunner) Option {
	return func(s *Service) { s.tx = tx }
}

func NewService(
	q domain.Repository,
	sources domain.SourceProvider,
	subs domain.SubscriptionEnsurer,
	registry domain.AdapterRegistry,
	jobs domain.JobReader,
	client domain.Enqueuer,
	opts ...Option,
) *Service {
	s := &Service{q: q, sources: sources, subs: subs, registry: registry, jobs: jobs, client: client}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// resolution is what walking the registry produced: either a reader that
// claimed the URL, or the reason nobody did.
type resolution struct {
	adapter   jobsources.Adapter
	reader    jobsources.PostingReader
	sourceKey string
}

// resolve validates the URL before any network call (FR-002), then asks each
// registered PostingReader whether it claims it. A host that is known but whose
// URL is the wrong shape is not_a_posting; a host nobody knows is no_reader —
// two different problems with two different recoveries (D2).
func (s *Service) resolve(rawURL string) (resolution, *domain.Failure) {
	trimmed := strings.TrimSpace(rawURL)
	parsed, failure := parseURL(rawURL)
	if failure != nil {
		return resolution{}, failure
	}

	hostClaimed := false
	for _, adapter := range s.registry.All() {
		reader, ok := jobsources.AsPostingReader(adapter)
		if !ok {
			continue
		}
		if reader.MatchesPostingURL(trimmed) {
			return resolution{adapter: adapter, reader: reader, sourceKey: adapter.Key()}, nil
		}
		if claimsHost, ok := adapter.(hostClaimer); ok && claimsHost.ClaimsHost(parsed.Host) {
			hostClaimed = true
		}
	}

	if hostClaimed {
		return resolution{}, domain.NotAPosting(parsed.Host)
	}
	return resolution{}, domain.NoReader(parsed.Host)
}

// parseURL enforces FR-002: a URL is validated before any network request, and
// only http and https are ever fetched.
func parseURL(rawURL string) (*url.URL, *domain.Failure) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, domain.InvalidURL(rawURL)
	}
	return parsed, nil
}

// hostClaimer lets an adapter say "this host is mine even though this URL is
// not a posting", which is what separates not_a_posting from no_reader. An
// adapter without it can still serve manual add; its non-posting URLs simply
// report no_reader.
type hostClaimer interface {
	ClaimsHost(host string) bool
}

// Add is the whole flow: resolve, read, persist, record. Every terminal state
// writes exactly one SourceRun, and every failure leaves no vacancy behind.
func (s *Service) Add(ctx context.Context, rawURL string) (domain.Result, error) {
	started := time.Now()
	ctx, cancel := context.WithTimeout(ctx, AddTimeout)
	defer cancel()

	resolved, failure := s.resolve(rawURL)
	if failure != nil {
		// Nothing was resolved, so there is no source to write a run against.
		// The attempt never reached a host; FR-017e's "one run per attempt"
		// starts at the point an attempt has a source.
		return s.logged(rawURL, "", s.failureResult(rawURL, failure, nil), started), nil
	}

	source, err := s.sources.GetByKey(ctx, resolved.sourceKey)
	if err != nil {
		return domain.Result{}, fmt.Errorf("manualadd: resolve source %s: %w", resolved.sourceKey, err)
	}
	subscription, err := s.subs.EnsureManualSubscription(ctx, resolved.sourceKey)
	if err != nil {
		return domain.Result{}, fmt.Errorf("manualadd: ensure manual subscription: %w", err)
	}

	run, err := s.q.InsertSourceRun(ctx, sqlcgen.InsertSourceRunParams{
		SourceId:       source.ID,
		SubscriptionId: subscription.ID,
		Trigger:        manualTrigger,
	})
	if err != nil {
		return domain.Result{}, fmt.Errorf("manualadd: insert source run: %w", err)
	}

	posting, readErr := resolved.reader.ReadPosting(ctx, strings.TrimSpace(rawURL), s.sources.DecryptConfig(source.Config))
	if readErr != nil {
		failure := classifyReadError(ctx, resolved.sourceKey, readErr)
		s.finishRunError(ctx, run.ID, failure)
		return s.logged(rawURL, resolved.sourceKey, s.failureResult(rawURL, failure, nil), started), nil
	}

	posting.SourceKey = resolved.sourceKey
	if missing := missingRequiredFields(posting); len(missing) > 0 {
		failure := domain.Incomplete(missing)
		s.finishRunError(ctx, run.ID, failure)
		draft := domain.DraftFromPosting(strings.TrimSpace(rawURL), posting)
		return s.logged(rawURL, resolved.sourceKey, s.failureResult(rawURL, failure, draft), started), nil
	}

	result, err := s.persist(ctx, posting, subscription.ID, run.ID, jobsources.NeedsDetail(resolved.adapter))
	if err != nil {
		failure := classifyReadError(ctx, resolved.sourceKey, err)
		s.finishRunError(ctx, run.ID, failure)
		return s.logged(rawURL, resolved.sourceKey, s.failureResult(rawURL, failure, nil), started), nil
	}

	_ = s.q.TouchSubscriptionLastRun(ctx, subscription.ID)
	return s.logged(rawURL, resolved.sourceKey, result, started), nil
}

// persist runs the shared ingest path and turns its outcome into a Result.
// A posting whose dedupe key already exists is a duplicate, not an error
// (FR-007b) — including when a concurrent add won the race (FR-008).
func (s *Service) persist(ctx context.Context, posting dto.NormalizedJob, subscriptionID, runID pgtype.UUID, needsDetail bool) (domain.Result, error) {
	batch := ingest.NewPostingBatch([]dto.NormalizedJob{posting}, subscriptionID, runID, needsDetail)

	persisted, err := s.runPersist(ctx, batch)
	if err != nil {
		return domain.Result{}, err
	}

	dedupeKey := ingest.DedupeKey(posting.Company, posting.Title, posting.URL)
	if len(persisted.Inserted) == 0 {
		existing, lookupErr := s.q.GetJobByDedupeKey(ctx, dedupeKey)
		if lookupErr != nil {
			return domain.Result{}, fmt.Errorf("manualadd: look up existing vacancy: %w", lookupErr)
		}
		job, jobErr := s.jobs.Get(ctx, dbutil.UUIDString(existing))
		if jobErr != nil {
			return domain.Result{}, jobErr
		}
		return domain.Result{
			Outcome: domain.OutcomeDuplicate,
			Job:     &job,
			Reason:  "This vacancy is already in your feed.",
		}, nil
	}

	inserted := persisted.Inserted[0]
	job, err := s.jobs.Get(ctx, dbutil.UUIDString(inserted.JobID))
	if err != nil {
		return domain.Result{}, err
	}

	// Post-ingest work rides a background context on purpose: matching is an
	// LLM call that can take minutes, and the vacancy is useful before it
	// finishes (FR-003d).
	s.enqueueInserted(context.WithoutCancel(ctx), inserted, needsDetail)

	return domain.Result{Outcome: domain.OutcomeCreated, Job: &job}, nil
}

func (s *Service) runPersist(ctx context.Context, batch ingest.PostingBatch) (ingest.PersistResult, error) {
	if s.tx == nil {
		return ingest.PersistBatch(ctx, s.q, batch, ingest.DefaultChunkSize)
	}
	var result ingest.PersistResult
	err := s.tx.WithinTx(ctx, func(q *sqlcgen.Queries) error {
		var err error
		result, err = ingest.PersistBatch(ctx, q, batch, ingest.DefaultChunkSize)
		return err
	})
	if err != nil {
		return ingest.PersistResult{}, err
	}
	return result, nil
}

// enqueueInserted mirrors the crawl path's routing: enrich when the adapter
// needs a detail pass, otherwise match and ghost-score.
func (s *Service) enqueueInserted(ctx context.Context, ins ingest.InsertedJob, needsDetail bool) {
	if s.client == nil {
		return
	}
	jobID := dbutil.UUIDString(ins.JobID)
	if needsDetail {
		s.enqueue(ctx, queue.TypeEnrich, queue.QueueEnrich, jobID, activityID(ins, ingest.OpEnrich),
			queue.EnrichPayload{JobID: jobID, ActivityID: activityID(ins, ingest.OpEnrich)})
		return
	}
	s.enqueue(ctx, queue.TypeMatch, queue.QueueMatch, jobID, activityID(ins, ingest.OpMatch),
		queue.MatchPayload{JobID: jobID, ActivityID: activityID(ins, ingest.OpMatch)})
	s.enqueue(ctx, queue.TypeGhostScore, queue.QueueGhostScore, jobID, activityID(ins, ingest.OpGhost),
		queue.GhostScorePayload{JobID: jobID, ActivityID: activityID(ins, ingest.OpGhost)})
}

func (s *Service) enqueue(ctx context.Context, taskType, queueName, jobID string, actID *string, payload any) {
	body, err := json.Marshal(payload)
	if err != nil {
		return
	}
	opts := []asynq.Option{asynq.MaxRetry(0), asynq.Queue(queueName)}
	if actID != nil {
		opts = append(opts, asynq.TaskID(*actID))
	}
	if _, err := s.client.EnqueueContext(ctx, asynq.NewTask(taskType, body), opts...); err != nil {
		slog.Warn("manualadd: enqueue failed", "task", taskType, "job", jobID, "error", err)
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

func (s *Service) finishRunError(ctx context.Context, runID pgtype.UUID, failure *domain.Failure) {
	msg := failure.Error()
	if len(msg) > 1000 {
		msg = msg[:1000]
	}
	// The deadline that produced a timed_out failure has also cancelled ctx, so
	// the run is closed on a context that outlives it.
	_ = s.q.FinishSourceRunError(context.WithoutCancel(ctx), sqlcgen.FinishSourceRunErrorParams{ID: runID, Error: &msg})
}

// failureResult decides whether a failure dead-ends or offers the fill-in form.
// no_reader and incomplete are recoverable states carrying a draft — but only
// once the form exists to receive it.
func (s *Service) failureResult(rawURL string, failure *domain.Failure, draft *domain.Draft) domain.Result {
	result := domain.Result{
		Outcome: domain.OutcomeFailed,
		Kind:    failure.Kind,
		Reason:  failure.Reason,
	}
	recoverable := failure.Kind == domain.KindNoReader || failure.Kind == domain.KindIncomplete
	if s.fillInEnabled && recoverable {
		result.Outcome = domain.OutcomeNeedsFillIn
		if draft == nil {
			draft = &domain.Draft{URL: strings.TrimSpace(rawURL)}
		}
		result.Draft = draft
	}
	return result
}

func (s *Service) logged(rawURL, sourceKey string, result domain.Result, started time.Time) domain.Result {
	attrs := []any{
		"source", sourceKey,
		"outcome", string(result.Outcome),
		"durationMs", time.Since(started).Milliseconds(),
	}
	if result.Kind != "" {
		attrs = append(attrs, "kind", string(result.Kind))
	}
	if result.Job != nil {
		attrs = append(attrs, "job", result.Job.ID, "dedupeKey", result.Job.DedupeKey)
	}
	if result.Outcome == domain.OutcomeFailed {
		attrs = append(attrs, "url", rawURL)
		slog.Warn("manual add failed", attrs...)
		return result
	}
	slog.Info("manual add", attrs...)
	return result
}

// missingRequiredFields names what the operator would have to supply. A vacancy
// with no title, company or description is not something downstream matching or
// tailoring can work with.
func missingRequiredFields(j dto.NormalizedJob) []string {
	var missing []string
	if strings.TrimSpace(j.Title) == "" {
		missing = append(missing, "title")
	}
	if strings.TrimSpace(j.Company) == "" {
		missing = append(missing, "company")
	}
	if strings.TrimSpace(j.Description) == "" {
		missing = append(missing, "description")
	}
	return missing
}

// classifyReadError maps whatever the retrieval ladder returned onto the FR-018
// taxonomy. The deadline is checked first because a cancelled fetch surfaces as
// a transport error that would otherwise read as unreachable.
func classifyReadError(ctx context.Context, host string, err error) *domain.Failure {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return domain.TimedOut(host, err)
	}
	lower := strings.ToLower(err.Error())
	for _, marker := range []string{"login", "challenge", "blocked", "captcha", "403", "429", "forbidden", "too many requests"} {
		if strings.Contains(lower, marker) {
			return domain.Blocked(host, err)
		}
	}
	return domain.Unreachable(host, err)
}
