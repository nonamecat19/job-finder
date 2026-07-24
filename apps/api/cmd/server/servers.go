package main

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/hibiken/asynq"

	"github.com/job-finder/api/internal/httpapi"
	"github.com/job-finder/api/internal/ingestion"
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
// separate queues.
func (p *Platform) worker(name, taskType, queueName string, concurrency int, handler func(context.Context, *asynq.Task) error) namedWorker {
	mux := asynq.NewServeMux()
	mux.HandleFunc(taskType, handler)
	return namedWorker{
		name: name,
		srv: asynq.NewServer(p.RedisOpt, asynq.Config{
			Concurrency: concurrency,
			Queues:      map[string]int{queueName: 1},
		}),
		mux: mux,
	}
}

// buildServers mounts the router and constructs the six asynq worker servers.
func buildServers(p *Platform, app *App) *Servers {
	router := httpapi.NewRouter(
		app.Sources.Mount, app.Searches.Mount, app.Documents.Mount,
		app.Profiles.Mount, app.Jobs.Mount, app.Applications.Mount,
		app.Subs.Mount, app.Activity.Mount, app.Keyword.Mount,
		app.PostAge.Mount, app.Notification.Mount, app.Companies.Mount,
		app.GhostJob.Mount, app.Coach.Mount,
		app.ExtAuth.Mount, app.ExtProfile.Mount,
		app.Contacts.Mount, app.Referral.Mount,
		app.Outreach.Mount, app.LlmSettings.Mount, app.AiFeatures.Mount,
		app.InterviewPrep.Mount, app.Health.Mount,
	)

	srv := &http.Server{
		Addr:              ":" + strconv.Itoa(p.Config.Port),
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}

	// ingest concurrency=2 (ingestion.processor.ts); match and generate
	// concurrency=1 each ("local LLM handles one request at a time
	// comfortably"). enrich concurrency=1: it hits an authenticated personal
	// djinni page per job, serialized + delayed to stay polite. salary and
	// ghost concurrency=1: same local-LLM contention reasoning as match/generate.
	workers := []namedWorker{
		p.worker("ingest", queue.TypeIngest, queue.QueueIngest, 2, app.Ingestion.ProcessTask),
		p.worker("match", queue.TypeMatch, queue.QueueMatch, 1, app.Matching.ProcessTask),
		p.worker("generate", queue.TypeGenerate, queue.QueueGenerate, 1, app.Generation.ProcessTask),
		p.worker("enrich", queue.TypeEnrich, queue.QueueEnrich, 1, app.Enrichment.ProcessTask),
		p.worker("salary", queue.TypeSalaryInfer, queue.QueueSalaryInfer, 1, app.Salary.ProcessTask),
		p.worker("ghost", queue.TypeGhostScore, queue.QueueGhostScore, 1, app.Ghost.ProcessTask),
	}

	return &Servers{HTTP: srv, Workers: workers}
}

// runServers launches the HTTP server, every worker, and the scheduler as
// goroutines, then blocks until ctx is cancelled and coordinates shutdown.
func runServers(ctx context.Context, p *Platform, servers *Servers, scheduler *ingestion.Scheduler) error {
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

	<-ctx.Done()
	slog.Info("shutting down")
	for _, w := range servers.Workers {
		w.srv.Shutdown()
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return servers.HTTP.Shutdown(shutdownCtx)
}
