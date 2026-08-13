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

type Servers struct {
	HTTP    *http.Server
	Workers []namedWorker
}

type namedWorker struct {
	name string
	srv  *asynq.Server
	mux  *asynq.ServeMux
}

func (p *Platform) worker(name string, policy queue.TaskPolicy, handler func(context.Context, *asynq.Task) error) namedWorker {
	gate := queue.NewGate(policy)
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
		app.Contacts.Mount, app.Referral.Mount,
		app.Outreach.Mount, app.AiFeatures.Mount, app.ResumeShape.Mount, app.SummaryModel.Mount,
		app.InterviewPrep.Mount, app.Health.Mount,
		app.Hosts.Mount,
	)

	srv := &http.Server{
		Addr:              ":" + strconv.Itoa(p.Config.Port),
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}

	workers := []namedWorker{
		p.worker("ingest", p.policyFor(queue.TypeIngest), app.Ingestion.ProcessTask),
		p.worker("match", p.policyFor(queue.TypeMatch), app.Matching.ProcessTask),
		p.worker("generate", p.policyFor(queue.TypeGenerate), app.Generation.ProcessTask),
		p.worker("enrich", p.policyFor(queue.TypeEnrich), app.Enrichment.ProcessTask),
		p.worker("salary", p.policyFor(queue.TypeSalaryInfer), app.Salary.ProcessTask),
		p.worker("ghost", p.policyFor(queue.TypeGhostScore), app.Ghost.ProcessTask),
	}

	return &Servers{HTTP: srv, Workers: workers}
}

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
