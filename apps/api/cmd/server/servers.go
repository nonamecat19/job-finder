package main

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/hibiken/asynq"

	"github.com/job-finder/api/internal/db"
	"github.com/job-finder/api/internal/httpapi"
	"github.com/job-finder/api/internal/jobsources/interfaces/worker"
	"github.com/job-finder/api/internal/queue"
)

// Servers holds the constructed HTTP server and the per-task asynq workers,
// ready to launch.
type Servers struct {
	HTTP    *http.Server
	Workers []namedWorker
}

type namedWorker struct {
	name string
	srv  *asynq.Server
	mux  *asynq.ServeMux
}

// worker builds one asynq.Server/mux pair. Each task type gets its own server
// (own worker pool + own queue), so its Concurrency is a hard per-queue cap
// rather than a weight shared across task types — matching the BullMQ setup's
// separate queues. Concurrency is sized to policy.PoolSize() (max of local
// and hosted), and the handler is wrapped in the admission gate that enforces
// whichever limit applies to the resolved provider class at run time
// (019-ai-job-throughput, research.md R3). resolver is nil for task types
// with no LLM component, which fall back to a single fixed concurrency.
func (p *Platform) worker(name string, policy queue.TaskPolicy, resolver queue.ClassResolver, handler func(context.Context, *asynq.Task) error) namedWorker {
	gate := queue.NewGate(policy, resolver)
	deadline := queue.NewDeadlineMiddleware(policy, p.DB.Queries, p.Config.ActivityHeartbeatInterval)
	wrapped := gate.Middleware(deadline.Middleware(handler))
	mux := asynq.NewServeMux()
	mux.HandleFunc(policy.TaskType, wrapped)
	return namedWorker{
		name: name,
		srv: asynq.NewServer(p.RedisOpt, asynq.Config{
			Concurrency: policy.PoolSize(),
			Queues:      map[string]int{policy.Queue: 1},
		}),
		mux: mux,
	}
}

// policyFor looks up the validated TaskPolicy for taskType, built once at
// startup (platform.go). Panics on an unknown type: every task type wired in
// buildServers must have a corresponding policy from queue.PoliciesFromConfig.
func (p *Platform) policyFor(taskType string) queue.TaskPolicy {
	for _, policy := range p.Policies {
		if policy.TaskType == taskType {
			return policy
		}
	}
	panic("servers: no TaskPolicy for task type " + taskType)
}

// buildServers mounts the router and constructs the six asynq worker servers.
func buildServers(p *Platform, app *App) *Servers {
	router := httpapi.NewRouter(
		p.Config.DBAcquireTimeout,
		app.Sources.Mount, app.Roster.Mount, app.Searches.Mount, app.Documents.Mount,
		app.Profiles.Mount, app.Jobs.Mount, app.Applications.Mount,
		app.Subs.Mount, app.Activity.Mount, app.Keyword.Mount,
		app.PostAge.Mount, app.Notification.Mount, app.Companies.Mount,
		app.GhostJob.Mount, app.Coach.Mount,
		app.Contacts.Mount, app.Referral.Mount,
		app.Outreach.Mount, app.AiFeatures.Mount, app.ResumeShape.Mount,
		app.InterviewPrep.Mount, app.Health.Mount,
		app.Hosts.Mount,
	)

	srv := &http.Server{
		Addr:              ":" + strconv.Itoa(p.Config.Port),
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}

	// Each worker's pool is sized to max(local, hosted) concurrency from its
	// TaskPolicy, with an admission gate (queue.Gate) enforcing whichever
	// limit applies to the resolved provider class at run time — a
	// gateway-routed or Ollama Cloud task gets AI_CONCURRENCY_CLOUD
	// (default 3), local Ollama keeps AI_CONCURRENCY_LOCAL (default 1)
	// (019-ai-job-throughput, research.md R3; 030-litellm-model-routing fixes
	// the class at Router construction, see application.Router.ProviderClass).
	// ingest/enrich have no LLM component and use a single fixed concurrency
	// (INGEST_CONCURRENCY / ENRICH_CONCURRENCY).
	workers := []namedWorker{
		p.worker("ingest", p.policyFor(queue.TypeIngest), nil, app.Ingestion.ProcessTask),
		p.worker("match", p.policyFor(queue.TypeMatch), app.MatchRouter, app.Matching.ProcessTask),
		p.worker("generate", p.policyFor(queue.TypeGenerate), app.GenerationRouter, app.Generation.ProcessTask),
		p.worker("enrich", p.policyFor(queue.TypeEnrich), nil, app.Enrichment.ProcessTask),
		p.worker("salary", p.policyFor(queue.TypeSalaryInfer), app.DefaultRouter, app.Salary.ProcessTask),
		p.worker("ghost", p.policyFor(queue.TypeGhostScore), app.GhostRouter, app.Ghost.ProcessTask),
	}

	return &Servers{HTTP: srv, Workers: workers}
}

// runServers launches the HTTP server, every worker, and the scheduler as
// goroutines, then blocks until ctx is cancelled and coordinates shutdown.
func runServers(ctx context.Context, p *Platform, servers *Servers, scheduler *worker.Scheduler) error {
	go func() {
		slog.Info("API listening", "port", p.Config.Port)
		if err := servers.HTTP.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("http server error", "error", err)
		}
	}()

	for _, w := range servers.Workers {
		w := w
		go func() {
			if err := w.srv.Run(w.mux); err != nil {
				slog.Error("asynq worker error", "worker", w.name, "error", err)
			}
		}()
	}

	go scheduler.Run(ctx)
	go p.Sweeper.Run(ctx)
	go db.NewSaturationSampler(p.DB, slog.Default()).Run(ctx)

	<-ctx.Done()
	slog.Info("shutting down")
	for _, w := range servers.Workers {
		w.srv.Shutdown()
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return servers.HTTP.Shutdown(shutdownCtx)
}
