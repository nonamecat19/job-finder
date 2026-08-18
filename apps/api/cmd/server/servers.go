package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/job-finder/api/internal/db"
	"github.com/job-finder/api/internal/events"
	"github.com/job-finder/api/internal/generation"
	"github.com/job-finder/api/internal/ghostjob"
	"github.com/job-finder/api/internal/httpapi"
	"github.com/job-finder/api/internal/jobsources/interfaces/worker"
	"github.com/job-finder/api/internal/matching"
	"github.com/job-finder/api/internal/queue"
	"github.com/job-finder/api/internal/salary"
)

type Servers struct {
	HTTP      *http.Server
	Consumers []namedConsumer
}

type namedConsumer struct {
	name string
	c    *events.Consumer
}

// consumer builds one work type's Consumer: an events.Consumer wired to
// this work type's queue, wrapped in the same Gate/DeadlineMiddleware
// pipeline the asynq workers used, and translating a handler error into a
// durable retry/dead-letter publish before ack (consumer.go's
// ack-only-after-durable-handling contract, M3-2).
func (p *Platform) consumer(name string, policy queue.TaskPolicy, handler func(context.Context, *queue.Task) error) namedConsumer {
	gate := queue.NewGate(policy)
	deadline := queue.NewDeadlineMiddleware(policy, p.DB.Queries, p.Config.ActivityHeartbeatInterval)
	wrapped := gate.Middleware(deadline.Middleware(handler))

	return namedConsumer{
		name: name,
		c: &events.Consumer{
			Dial:        func() (*amqp.Connection, error) { return amqp.Dial(p.Config.RabbitMQURL) },
			Queue:       "work." + policy.TaskType,
			Concurrency: policy.PoolSize(),
			Logger:      p.Logger,
			HandlerFunc: func(ctx context.Context, d amqp.Delivery) error {
				attempt := headerInt(d.Headers, events.HeaderAttempt)
				ctx = queue.ContextWithRetry(ctx, attempt, events.MaxAttempts(policy.TaskType)-1)

				task := queue.NewTask(policy.TaskType, d.Body)
				if err := wrapped(ctx, task); err != nil {
					failure := classifyWorkError(err)
					// A fresh, uncancelled context: the durable retry/DLQ
					// publish must complete even if the delivery's own
					// context is on its way out (e.g. deadline exceeded).
					return events.HandleFailure(context.Background(), p.Publisher, policy.TaskType, d.Body, d.Headers, attempt, failure)
				}
				return nil
			},
		},
	}
}

// resultConcurrency bounds how many result deliveries results.backend
// processes at once. Results are cheap persistence writes, not model calls,
// so a small fixed pool is enough; there is no per-work-type policy to
// derive it from the way there.consumer has.
const resultConcurrency = 4

// resultConsumer builds the Consumer for results.backend (T061,
// contracts/messaging.md M5, M6): one queue shared by every capability's
// result, dispatched to the per-capability handler registry.HandleResultDelivery
// resolves (events/results.go).
func (p *Platform) resultConsumer(registry events.ResultRegistry) namedConsumer {
	return namedConsumer{
		name: "results",
		c: &events.Consumer{
			Dial:        func() (*amqp.Connection, error) { return amqp.Dial(p.Config.RabbitMQURL) },
			Queue:       events.ResultQueue,
			Concurrency: resultConcurrency,
			Logger:      p.Logger,
			HandlerFunc: registry.HandleResultDelivery(p.Logger),
		},
	}
}

func headerInt(h amqp.Table, key string) int {
	if h == nil {
		return 0
	}
	switch v := h[key].(type) {
	case int32:
		return int(v)
	case int64:
		return int(v)
	case int:
		return v
	}
	return 0
}

// classifyWorkError maps a Go-handled work type's handler error onto a
// result-event Failure category (E5). These work types (ingest, enrich,
// match, generate, salary, ghost) are not yet migrated to the AI service's
// richer LLM-specific categories (rate_limited, credential_rejected, ...) —
// this is deliberately coarse: a handler that wraps queue.ErrSkipRetry is
// non-retryable (invalid_input), a deadline is a retryable timeout,
// anything else is a retryable internal failure.
func classifyWorkError(err error) events.Failure {
	msg := err.Error()
	if errors.Is(err, queue.ErrSkipRetry) {
		return events.Failure{Category: events.FailureInvalidInput, Retryable: false, Message: msg}
	}
	if errors.Is(err, queue.ErrDeadlineExceeded) {
		return events.Failure{Category: events.FailureTimeout, Retryable: events.FailureTimeout.DefaultRetryable(), Message: msg}
	}
	return events.Failure{Category: events.FailureInternal, Retryable: events.FailureInternal.DefaultRetryable(), Message: msg}
}

func (p *Platform) policyFor(taskType string) queue.TaskPolicy {
	for _, policy := range p.Policies {
		if policy.TaskType == taskType {
			return policy
		}
	}
	panic("servers: no TaskPolicy for task type " + taskType)
}

func buildServers(p *Platform, app *App) *Servers {
	router := httpapi.NewRouter(
		p.Config.DBAcquireTimeout,
		app.Sources.Mount, app.Roster.Mount, app.Searches.Mount, app.Documents.Mount, app.Generations.Mount,
		app.Profiles.Mount, app.Jobs.Mount, app.Applications.Mount,
		app.Subs.Mount, app.ManualAdd.Mount, app.Activity.Mount, app.Keyword.Mount,
		app.PostAge.Mount, app.Notification.Mount, app.Companies.Mount,
		app.GhostJob.Mount, app.Coach.Mount,
		app.Contacts.Mount,
		app.Outreach.Mount, app.AiFeatures.Mount, app.ResumeShape.Mount, app.SummaryModel.Mount,
		app.InterviewPrep.Mount, app.Health.Mount,
		app.Hosts.Mount,
	)

	srv := &http.Server{
		Addr:              ":" + strconv.Itoa(p.Config.Port),
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}

	consumers := []namedConsumer{
		p.consumer("ingest", p.policyFor(queue.TypeIngest), app.Ingestion.ProcessTask),
		p.consumer("match", p.policyFor(queue.TypeMatch), app.Matching.ProcessTask),
		// generation's Go consumer stays registered unconditionally, even when
		// AI_CAPABILITY_ROUTING routes "generation" to python: the 042
		// workspace protocol (GenerationRunID set) has no python counterpart
		// and always dispatches here (generation.SnapshotEnqueuer, publish.go).
		// Only the legacy merged-resume/cover-letter path is diverted.
		p.consumer("generate", p.policyFor(queue.TypeGenerate), app.Generation.ProcessTask),
		p.consumer("enrich", p.policyFor(queue.TypeEnrich), app.Enrichment.ProcessTask),
	}

	resultRegistry := events.ResultRegistry{}

	// match's Go consumer stays registered unconditionally, even when
	// AI_CAPABILITY_ROUTING routes "match" to python: embedding and
	// similarity prefiltering always run in Go (matching.Service.MatchJob),
	// and MatchJob itself decides whether to call the LLM directly or
	// publish match.requested and defer to this result handler.
	if p.Config.CapabilityRouting(matching.Kind) == "python" {
		resultRegistry[events.EventMatchCompleted] = matching.NewResultHandler(
			p.DB, app.MatchNotifier, app.MatchAutoGenerate, app.MatchGenerator, "jobfinder-ai",
		)
	}

	// ghost's and salary's Go consumers only run while routed to go (C8-3):
	// once AI_CAPABILITY_ROUTING routes a capability to python, the AI
	// service is the sole consumer of its work queue, and this process
	// instead consumes the result off results.backend (registered below).
	if p.Config.CapabilityRouting(ghostjob.Kind) == "python" {
		resultRegistry[events.EventGhostCompleted] = ghostjob.NewResultHandler(p.DB, "jobfinder-ai")
	} else {
		consumers = append(consumers, p.consumer("ghost", p.policyFor(queue.TypeGhostScore), app.Ghost.ProcessTask))
	}

	// salary's Go consumer is gone: its LLM path was deleted once cutover was
	// confirmed (T113), so python is the only mode left and this registration
	// is unconditional now — unlike ghost's, which still has a "go" fallback.
	resultRegistry[events.EventSalaryCompleted] = salary.NewResultHandler(p.DB)

	if p.Config.CapabilityRouting(generation.Kind) == "python" {
		resultRegistry[events.EventGenerateCompleted] = generation.NewResultHandler(
			p.DB, p.GenLate, p.GenLate, app.GenerationRendercvRenderer, app.GenerationHtmlRenderer, "jobfinder-ai",
		)
	}

	consumers = append(consumers, p.resultConsumer(resultRegistry))

	return &Servers{HTTP: srv, Consumers: consumers}
}

func runServers(ctx context.Context, p *Platform, servers *Servers, scheduler *worker.Scheduler) error {
	go func() {
		slog.Info("API listening", "port", p.Config.Port)
		if err := servers.HTTP.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("http server error", "error", err)
		}
	}()

	for _, c := range servers.Consumers {
		c := c
		go func() {
			if err := c.c.Run(ctx); err != nil {
				slog.Error("consumer error", "consumer", c.name, "error", err)
			}
		}()
	}

	go scheduler.Run(ctx)
	go p.Sweeper.Run(ctx)
	go db.NewSaturationSampler(p.DB, slog.Default()).Run(ctx)

	<-ctx.Done()
	slog.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return servers.HTTP.Shutdown(shutdownCtx)
}
