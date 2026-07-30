// Package domain holds the llmsettings bounded context's core model: which
// chat provider/model each LLM task (matching, generation, rephrase,
// ghost-job, default) uses (001-cerebras-model-toggle), plus the Repository
// outbound port.
package domain

import (
	"errors"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/platform/llm"
)

// TaskKeys are the fixed set of chat tasks that have an independent
// provider/model assignment. "default" covers chat callers that don't have
// a dedicated task (e.g. salary inference, coach), per plan.md.
var TaskKeys = []string{"match", "generation", "rephrase", "ghost", "default"}

// IsKnownTaskKey reports whether key is one of TaskKeys.
func IsKnownTaskKey(key string) bool {
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

// SnapshotFromRows builds a llm.RouterSnapshot from persisted task-setting
// rows.
func SnapshotFromRows(rows []sqlcgen.LlmTaskSetting, credentialConfigured bool) llm.RouterSnapshot {
	tasks := make(map[string]llm.TaskSetting, len(rows))
	for _, r := range rows {
		tasks[r.TaskKey] = llm.TaskSetting{Provider: llm.TaskProvider(r.Provider), Model: r.Model}
	}
	return llm.RouterSnapshot{Tasks: tasks, CredentialConfigured: credentialConfigured}
}
