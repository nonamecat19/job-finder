package application

import (
	"context"
	"errors"
	"testing"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/llmsettings/domain"
)

type fakeRepo struct {
	rows map[string]sqlcgen.LlmTaskSetting
}

func newFakeRepo() *fakeRepo {
	rows := make(map[string]sqlcgen.LlmTaskSetting, len(domain.TaskKeys))
	for _, k := range domain.TaskKeys {
		rows[k] = sqlcgen.LlmTaskSetting{TaskKey: k, Provider: "ollama", Model: ""}
	}
	return &fakeRepo{rows: rows}
}

func (f *fakeRepo) ListLlmTaskSettings(ctx context.Context) ([]sqlcgen.LlmTaskSetting, error) {
	out := make([]sqlcgen.LlmTaskSetting, 0, len(domain.TaskKeys))
	for _, k := range domain.TaskKeys {
		out = append(out, f.rows[k])
	}
	return out, nil
}

func (f *fakeRepo) UpsertLlmTaskSetting(ctx context.Context, arg sqlcgen.UpsertLlmTaskSettingParams) (sqlcgen.LlmTaskSetting, error) {
	row := sqlcgen.LlmTaskSetting{TaskKey: arg.TaskKey, Provider: arg.Provider, Model: arg.Model}
	f.rows[arg.TaskKey] = row
	return row, nil
}

func TestNewServiceLoadsSeededDefaults(t *testing.T) {
	svc, err := NewService(context.Background(), newFakeRepo(), false)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	state := svc.Get()
	if state.CredentialConfigured {
		t.Error("CredentialConfigured should be false")
	}
	if len(state.Tasks) != len(domain.TaskKeys) {
		t.Fatalf("Tasks len = %d, want %d", len(state.Tasks), len(domain.TaskKeys))
	}
	for _, task := range state.Tasks {
		if task.Provider != "ollama" {
			t.Errorf("task %q provider = %q, want ollama", task.TaskKey, task.Provider)
		}
	}
}

func TestUpdatePersistsAndReloadsSnapshot(t *testing.T) {
	repo := newFakeRepo()
	svc, err := NewService(context.Background(), repo, true)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	state, err := svc.Update(context.Background(), []domain.TaskUpdate{
		{TaskKey: "generation", Provider: "cerebras", Model: "gpt-oss-120b"},
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}

	var found bool
	for _, task := range state.Tasks {
		if task.TaskKey == "generation" {
			found = true
			if task.Provider != "cerebras" || task.Model != "gpt-oss-120b" {
				t.Errorf("generation task = %+v, want cerebras/gpt-oss-120b", task)
			}
		}
	}
	if !found {
		t.Fatal("generation task missing from state")
	}

	// The shared holder must reflect the change immediately (FR-005).
	snap := svc.Holder().Load()
	if snap.Tasks["generation"].Provider != "cerebras" {
		t.Error("snapshot holder was not updated after Update")
	}
}

func TestUpdateRejectsUnknownTaskKey(t *testing.T) {
	svc, _ := NewService(context.Background(), newFakeRepo(), true)
	_, err := svc.Update(context.Background(), []domain.TaskUpdate{{TaskKey: "bogus", Provider: "ollama"}})
	if !errors.Is(err, domain.ErrUnknownTaskKey) {
		t.Errorf("err = %v, want domain.ErrUnknownTaskKey", err)
	}
}

func TestUpdateRejectsInvalidProvider(t *testing.T) {
	svc, _ := NewService(context.Background(), newFakeRepo(), true)
	_, err := svc.Update(context.Background(), []domain.TaskUpdate{{TaskKey: "match", Provider: "openai"}})
	if !errors.Is(err, domain.ErrInvalidProvider) {
		t.Errorf("err = %v, want domain.ErrInvalidProvider", err)
	}
}

func TestUpdateRejectsUnsupportedCerebrasModel(t *testing.T) {
	svc, _ := NewService(context.Background(), newFakeRepo(), true)
	_, err := svc.Update(context.Background(), []domain.TaskUpdate{
		{TaskKey: "match", Provider: "cerebras", Model: "not-a-real-model"},
	})
	if !errors.Is(err, domain.ErrInvalidModel) {
		t.Errorf("err = %v, want domain.ErrInvalidModel", err)
	}
}

func TestUpdateAllowsCerebrasWithoutCredential(t *testing.T) {
	svc, _ := NewService(context.Background(), newFakeRepo(), false)
	state, err := svc.Update(context.Background(), []domain.TaskUpdate{
		{TaskKey: "ghost", Provider: "cerebras", Model: ""},
	})
	if err != nil {
		t.Fatalf("Update should persist even without a configured credential: %v", err)
	}
	if state.CredentialConfigured {
		t.Error("CredentialConfigured should stay false")
	}
	for _, task := range state.Tasks {
		if task.TaskKey == "ghost" && task.Provider != "cerebras" {
			t.Errorf("ghost provider = %q, want cerebras (persisted choice, Router falls back at call time)", task.Provider)
		}
	}
}

func TestUpdateLeavesOmittedTasksUnchanged(t *testing.T) {
	svc, _ := NewService(context.Background(), newFakeRepo(), true)
	if _, err := svc.Update(context.Background(), []domain.TaskUpdate{
		{TaskKey: "match", Provider: "cerebras", Model: "gpt-oss-120b"},
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	state := svc.Get()
	for _, task := range state.Tasks {
		if task.TaskKey != "match" && task.Provider != "ollama" {
			t.Errorf("task %q provider = %q, want unchanged ollama", task.TaskKey, task.Provider)
		}
	}
}
