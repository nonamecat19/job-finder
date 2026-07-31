package application

import (
	"context"
	"fmt"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/platform/llm"
	"github.com/job-finder/api/internal/llmsettings/domain"
)

// Service loads persisted per-task settings into a shared llm.SnapshotHolder
// at startup and keeps it in sync on every Update.
type Service struct {
	q                    domain.Repository
	holder               *llm.SnapshotHolder
	credentialConfigured bool
}

// NewService loads all rows via q and builds the initial snapshot.
// credentialConfigured reflects whether config.CerebrasAPIKey was set at
// process start (a restart is required to change the credential itself,
// since it is env-only — see spec FR-013).
func NewService(ctx context.Context, q domain.Repository, credentialConfigured bool) (*Service, error) {
	rows, err := q.ListLlmTaskSettings(ctx)
	if err != nil {
		return nil, fmt.Errorf("llmsettings: load: %w", err)
	}
	holder := llm.NewSnapshotHolder(domain.SnapshotFromRows(rows, credentialConfigured))
	return &Service{q: q, holder: holder, credentialConfigured: credentialConfigured}, nil
}

// Holder returns the shared snapshot holder every task's llm.Router reads
// from; main.go passes the same holder to every Router it constructs.
func (s *Service) Holder() *llm.SnapshotHolder { return s.holder }

// Get returns the current settings for every task, from the in-memory
// snapshot (kept authoritative by Update; avoids a DB round trip per read).
func (s *Service) Get() domain.State {
	snap := s.holder.Load()
	tasks := make([]domain.TaskUpdate, 0, len(domain.TaskKeys))
	for _, key := range domain.TaskKeys {
		t := snap.Tasks[key]
		tasks = append(tasks, domain.TaskUpdate{TaskKey: key, Provider: string(t.Provider), Model: t.Model})
	}
	return domain.State{CredentialConfigured: snap.CredentialConfigured, Tasks: tasks}
}

// Update validates and persists the given task assignments (a subset of
// TaskKeys is fine; omitted tasks are unchanged), then reloads the shared
// snapshot so every Router sees the change on its next call (FR-005). A task
// may be set to "cerebras" even when no credential is configured (FR-008) —
// the Router keeps such a task on Ollama until a key is added; State's
// CredentialConfigured tells the caller why.
func (s *Service) Update(ctx context.Context, updates []domain.TaskUpdate) (domain.State, error) {
	for _, u := range updates {
		if !domain.IsKnownTaskKey(u.TaskKey) {
			return domain.State{}, fmt.Errorf("%w: %q", domain.ErrUnknownTaskKey, u.TaskKey)
		}
		if u.Provider != string(llm.TaskProviderOllama) &&
			u.Provider != string(llm.TaskProviderCerebras) &&
			u.Provider != string(llm.TaskProviderGateway) {
			return domain.State{}, fmt.Errorf("%w: %q", domain.ErrInvalidProvider, u.Provider)
		}
		if u.Provider == string(llm.TaskProviderCerebras) && !llm.IsSupportedCerebrasModel(u.Model) {
			return domain.State{}, fmt.Errorf("%w: %q", domain.ErrInvalidModel, u.Model)
		}
	}

	for _, u := range updates {
		if _, err := s.q.UpsertLlmTaskSetting(ctx, sqlcgen.UpsertLlmTaskSettingParams{
			TaskKey:  u.TaskKey,
			Provider: u.Provider,
			Model:    u.Model,
		}); err != nil {
			return domain.State{}, fmt.Errorf("llmsettings: upsert %q: %w", u.TaskKey, err)
		}
	}

	rows, err := s.q.ListLlmTaskSettings(ctx)
	if err != nil {
		return domain.State{}, fmt.Errorf("llmsettings: reload: %w", err)
	}
	s.holder.Store(domain.SnapshotFromRows(rows, s.credentialConfigured))
	return s.Get(), nil
}
