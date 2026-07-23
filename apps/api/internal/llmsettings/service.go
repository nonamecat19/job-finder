// Package llmsettings persists and resolves which chat provider/model each
// LLM task (matching, generation, rephrase, ghost-job, default) uses
// (001-cerebras-model-toggle). It owns the llm.SnapshotHolder every
// llm.Router reads from, so a persisted change becomes visible to newly
// started tasks without a restart (FR-005).
package llmsettings

import (
	"context"
	"errors"
	"fmt"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/platform/llm"
)

// TaskKeys are the fixed set of chat tasks that have an independent
// provider/model assignment. "default" covers chat callers that don't have
// a dedicated task (e.g. salary inference, coach), per plan.md.
var TaskKeys = []string{"match", "generation", "rephrase", "ghost", "default"}

func isKnownTaskKey(key string) bool {
	for _, k := range TaskKeys {
		if k == key {
			return true
		}
	}
	return false
}

var (
	ErrUnknownTaskKey  = errors.New("llmsettings: unknown task key")
	ErrInvalidProvider = errors.New("llmsettings: provider must be \"ollama\" or \"cerebras\"")
	ErrInvalidModel    = errors.New("llmsettings: model is not a supported Cerebras free-tier model")
)

// TaskUpdate is one task's requested provider/model assignment (PUT body).
type TaskUpdate struct {
	TaskKey  string
	Provider string
	Model    string
}

// State is the full current settings view returned to callers (GET/PUT
// response per contracts/llm-settings.md).
type State struct {
	CredentialConfigured bool
	Tasks                []TaskUpdate
}

// Service loads persisted per-task settings into a shared llm.SnapshotHolder
// at startup and keeps it in sync on every Update.
type Service struct {
	q                    Repository
	holder               *llm.SnapshotHolder
	credentialConfigured bool
}

// NewService loads all rows via q and builds the initial snapshot.
// credentialConfigured reflects whether config.CerebrasAPIKey was set at
// process start (a restart is required to change the credential itself,
// since it is env-only — see spec FR-013).
func NewService(ctx context.Context, q Repository, credentialConfigured bool) (*Service, error) {
	rows, err := q.ListLlmTaskSettings(ctx)
	if err != nil {
		return nil, fmt.Errorf("llmsettings: load: %w", err)
	}
	holder := llm.NewSnapshotHolder(snapshotFromRows(rows, credentialConfigured))
	return &Service{q: q, holder: holder, credentialConfigured: credentialConfigured}, nil
}

// Holder returns the shared snapshot holder every task's llm.Router reads
// from; main.go passes the same holder to every Router it constructs.
func (s *Service) Holder() *llm.SnapshotHolder { return s.holder }

// Get returns the current settings for every task, from the in-memory
// snapshot (kept authoritative by Update; avoids a DB round trip per read).
func (s *Service) Get() State {
	snap := s.holder.Load()
	tasks := make([]TaskUpdate, 0, len(TaskKeys))
	for _, key := range TaskKeys {
		t := snap.Tasks[key]
		tasks = append(tasks, TaskUpdate{TaskKey: key, Provider: string(t.Provider), Model: t.Model})
	}
	return State{CredentialConfigured: snap.CredentialConfigured, Tasks: tasks}
}

// Update validates and persists the given task assignments (a subset of
// TaskKeys is fine; omitted tasks are unchanged), then reloads the shared
// snapshot so every Router sees the change on its next call (FR-005). A task
// may be set to "cerebras" even when no credential is configured (FR-008) —
// the Router keeps such a task on Ollama until a key is added; State's
// CredentialConfigured tells the caller why.
func (s *Service) Update(ctx context.Context, updates []TaskUpdate) (State, error) {
	for _, u := range updates {
		if !isKnownTaskKey(u.TaskKey) {
			return State{}, fmt.Errorf("%w: %q", ErrUnknownTaskKey, u.TaskKey)
		}
		if u.Provider != string(llm.TaskProviderOllama) && u.Provider != string(llm.TaskProviderCerebras) {
			return State{}, fmt.Errorf("%w: %q", ErrInvalidProvider, u.Provider)
		}
		if u.Provider == string(llm.TaskProviderCerebras) && !llm.IsSupportedCerebrasModel(u.Model) {
			return State{}, fmt.Errorf("%w: %q", ErrInvalidModel, u.Model)
		}
	}

	for _, u := range updates {
		if _, err := s.q.UpsertLlmTaskSetting(ctx, sqlcgen.UpsertLlmTaskSettingParams{
			TaskKey:  u.TaskKey,
			Provider: u.Provider,
			Model:    u.Model,
		}); err != nil {
			return State{}, fmt.Errorf("llmsettings: upsert %q: %w", u.TaskKey, err)
		}
	}

	rows, err := s.q.ListLlmTaskSettings(ctx)
	if err != nil {
		return State{}, fmt.Errorf("llmsettings: reload: %w", err)
	}
	s.holder.Store(snapshotFromRows(rows, s.credentialConfigured))
	return s.Get(), nil
}

func snapshotFromRows(rows []sqlcgen.LlmTaskSetting, credentialConfigured bool) llm.RouterSnapshot {
	tasks := make(map[string]llm.TaskSetting, len(rows))
	for _, r := range rows {
		tasks[r.TaskKey] = llm.TaskSetting{Provider: llm.TaskProvider(r.Provider), Model: r.Model}
	}
	return llm.RouterSnapshot{Tasks: tasks, CredentialConfigured: credentialConfigured}
}
