package http_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/llmsettings"
	llmsettingshttp "github.com/job-finder/api/internal/llmsettings/interfaces/http"
	"github.com/job-finder/api/internal/testutil"
)

type fakeLlmSettingsProvider struct {
	state       llmsettings.State
	updateErr   error
	lastUpdates []llmsettings.TaskUpdate
}

func (f *fakeLlmSettingsProvider) Get() llmsettings.State { return f.state }

func (f *fakeLlmSettingsProvider) Update(ctx context.Context, updates []llmsettings.TaskUpdate) (llmsettings.State, error) {
	f.lastUpdates = updates
	if f.updateErr != nil {
		return llmsettings.State{}, f.updateErr
	}
	for _, u := range updates {
		for i, t := range f.state.Tasks {
			if t.TaskKey == u.TaskKey {
				f.state.Tasks[i] = u
			}
		}
	}
	return f.state, nil
}

func seededState(credentialConfigured bool) llmsettings.State {
	tasks := make([]llmsettings.TaskUpdate, 0, len(llmsettings.TaskKeys))
	for _, k := range llmsettings.TaskKeys {
		tasks = append(tasks, llmsettings.TaskUpdate{TaskKey: k, Provider: "ollama", Model: ""})
	}
	return llmsettings.State{
		CredentialConfigured: credentialConfigured,
		Tasks:                tasks,
	}
}

func TestLlmSettingsGet_ReturnsSeededState(t *testing.T) {
	fake := &fakeLlmSettingsProvider{state: seededState(false)}
	h := &llmsettingshttp.LlmSettingsHandler{Settings: fake}
	r := testutil.SetupRouter(h.Mount)

	w := testutil.DoRequestJSON(r, "GET", "/api/v1/settings/llm", nil, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var out dto.LlmSettingsResponseDto
	testutil.ParseJSON(w, &out)
	if out.CredentialConfigured {
		t.Error("CredentialConfigured should be false")
	}
	if len(out.Tasks) != len(llmsettings.TaskKeys) {
		t.Fatalf("Tasks len = %d, want %d", len(out.Tasks), len(llmsettings.TaskKeys))
	}
	for _, task := range out.Tasks {
		if task.Provider != "ollama" {
			t.Errorf("task %q provider = %q, want ollama", task.TaskKey, task.Provider)
		}
	}
}

func TestLlmSettingsPut_SwitchAllToCerebras(t *testing.T) {
	fake := &fakeLlmSettingsProvider{state: seededState(true)}
	h := &llmsettingshttp.LlmSettingsHandler{Settings: fake}
	r := testutil.SetupRouter(h.Mount)

	body := dto.UpdateLlmSettingsRequestDto{}
	for _, k := range llmsettings.TaskKeys {
		body.Tasks = append(body.Tasks, dto.LlmTaskSettingDto{TaskKey: k, Provider: "cerebras", Model: "gpt-oss-120b"})
	}

	w := testutil.DoRequestJSON(r, "PUT", "/api/v1/settings/llm", body, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var out dto.LlmSettingsResponseDto
	testutil.ParseJSON(w, &out)
	for _, task := range out.Tasks {
		if task.Provider != "cerebras" {
			t.Errorf("task %q provider = %q, want cerebras", task.TaskKey, task.Provider)
		}
	}
	if len(fake.lastUpdates) != len(llmsettings.TaskKeys) {
		t.Errorf("Update received %d tasks, want %d", len(fake.lastUpdates), len(llmsettings.TaskKeys))
	}
}

func TestLlmSettingsPut_PartialUpdateAppliesOnlyGivenTasks(t *testing.T) {
	fake := &fakeLlmSettingsProvider{state: seededState(true)}
	h := &llmsettingshttp.LlmSettingsHandler{Settings: fake}
	r := testutil.SetupRouter(h.Mount)

	body := dto.UpdateLlmSettingsRequestDto{Tasks: []dto.LlmTaskSettingDto{
		{TaskKey: "generation", Provider: "cerebras", Model: "gpt-oss-120b"},
	}}
	w := testutil.DoRequestJSON(r, "PUT", "/api/v1/settings/llm", body, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if len(fake.lastUpdates) != 1 || fake.lastUpdates[0].TaskKey != "generation" {
		t.Errorf("Update should receive only the submitted task, got %+v", fake.lastUpdates)
	}
}

func TestLlmSettingsPut_ValidationErrorReturns400(t *testing.T) {
	fake := &fakeLlmSettingsProvider{state: seededState(true), updateErr: llmsettings.ErrInvalidModel}
	h := &llmsettingshttp.LlmSettingsHandler{Settings: fake}
	r := testutil.SetupRouter(h.Mount)

	body := dto.UpdateLlmSettingsRequestDto{Tasks: []dto.LlmTaskSettingDto{
		{TaskKey: "match", Provider: "cerebras", Model: "not-a-model"},
	}}
	w := testutil.DoRequestJSON(r, "PUT", "/api/v1/settings/llm", body, nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestLlmSettingsModels_ReturnsCuratedList(t *testing.T) {
	fake := &fakeLlmSettingsProvider{state: seededState(true)}
	h := &llmsettingshttp.LlmSettingsHandler{Settings: fake}
	r := testutil.SetupRouter(h.Mount)

	w := testutil.DoRequestJSON(r, "GET", "/api/v1/settings/llm/models", nil, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var out dto.LlmModelsResponseDto
	testutil.ParseJSON(w, &out)
	if len(out.Cerebras) == 0 {
		t.Fatal("expected at least one curated Cerebras model")
	}
	var cerebrasDefaults int
	for _, m := range out.Cerebras {
		if m.IsDefault {
			cerebrasDefaults++
		}
	}
	if cerebrasDefaults != 1 {
		t.Errorf("expected exactly one default Cerebras model, got %d", cerebrasDefaults)
	}
}
