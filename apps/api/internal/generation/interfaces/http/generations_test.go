//go:build integration

package http_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/job-finder/api/internal/db"
	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbtest"
	"github.com/job-finder/api/internal/dbutil"
	"github.com/job-finder/api/internal/dto"
	generationapp "github.com/job-finder/api/internal/generation/application"
	generationhttp "github.com/job-finder/api/internal/generation/interfaces/http"
	"github.com/job-finder/api/internal/platform/llm"
	profileapp "github.com/job-finder/api/internal/profile/application"
	"github.com/job-finder/api/internal/testutil"
)

type stubProvider struct {
	reply func(prompt string) string
}

func (p *stubProvider) ModelName() string { return "stub" }

func (p *stubProvider) Complete(ctx context.Context, prompt string, opts *llm.CompleteOptions) (string, error) {
	return p.reply(prompt), nil
}

func (p *stubProvider) CompleteJSON(ctx context.Context, prompt string, opts *llm.CompleteOptions) (string, error) {
	return p.reply(prompt), nil
}

func (p *stubProvider) CompleteChat(ctx context.Context, msgs []llm.Message, opts *llm.CompleteOptions) (llm.ChatResult, error) {
	prompt := ""
	if len(msgs) > 0 {
		prompt = msgs[len(msgs)-1].Content
	}
	return llm.ChatResult{Content: p.reply(prompt)}, nil
}

func (p *stubProvider) Embed(context.Context, string) ([]float32, error) { return nil, nil }

func masterJSON(t *testing.T, company string, bullets []string, skillLabel, skillDetails string) []byte {
	t.Helper()
	highlights := make([]any, 0, len(bullets))
	for _, b := range bullets {
		highlights = append(highlights, b)
	}
	master := map[string]any{
		"cv": map[string]any{
			"sections": map[string]any{
				"experience": []any{
					map[string]any{"company": company, "position": "Senior Engineer", "highlights": highlights},
				},
				"skills": []any{
					map[string]any{"label": skillLabel, "details": skillDetails},
				},
			},
		},
	}
	b, err := json.Marshal(master)
	if err != nil {
		t.Fatalf("marshal master: %v", err)
	}
	return b
}

func newWorkspaceHandler(t *testing.T, testDB *db.DB) (*generationhttp.GenerationsHandler, *generationapp.Service) {
	t.Helper()
	profileSvc := profileapp.NewService(testDB.Queries, nil)
	routers := generationapp.GenerationRouters{
		Analyze: &stubProvider{reply: func(string) string {
			b, _ := json.Marshal(map[string]any{
				"requiredSkills": []string{"Go"}, "experienceLevel": "senior",
			})
			return string(b)
		}},
		Summary: &stubProvider{reply: func(string) string {
			b, _ := json.Marshal(map[string]string{"summary": "Senior engineer with a track record of shipping."})
			return string(b)
		}},

		Select: &stubProvider{reply: func(string) string { return `{}` }},
	}
	svc := generationapp.NewService(testDB.Queries, profileSvc, nil, nil, routers, "", "", "moderate", nil)

	svc.SetTxRunner(testDB)
	return &generationhttp.GenerationsHandler{Workspace: svc}, svc
}

func insertTestProfile(ctx context.Context, t *testing.T, testDB *db.DB, config []byte) sqlcgen.Profile {
	t.Helper()
	prof, err := testDB.Queries.CreateProfile(ctx, sqlcgen.CreateProfileParams{
		Name: "Test Profile", Document: []byte(`{}`), RendercvConfig: config,
	})
	if err != nil {
		t.Fatalf("create profile: %v", err)
	}
	return prof
}

func TestGenerationsStartPollFetch(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	testDB := dbtest.New(t)

	config := masterJSON(t, "Acme Inc.", []string{"Shipped the thing", "Fixed the other thing"}, "Languages", "Go, TypeScript")
	prof := insertTestProfile(ctx, t, testDB, config)

	h, svc := newWorkspaceHandler(t, testDB)
	r := testutil.SetupRouter(h.Mount)

	profileID := dbutil.UUIDString(prof.ID)
	w := testutil.DoRequestJSON(r, "POST", "/api/generations", map[string]any{
		"profileId": profileID,
		"vacancy":   map[string]any{"company": "Acme", "title": "Senior Engineer", "text": "Looking for a Go engineer."},
	}, nil)
	if w.Code != 202 {
		t.Fatalf("expected 202, got %d: %s", w.Code, w.Body.String())
	}
	var started struct {
		RunID      string `json:"runId"`
		ActivityID string `json:"activityId"`
	}
	testutil.ParseJSON(w, &started)
	if started.RunID == "" {
		t.Fatal("expected a non-empty runId")
	}

	pollW := testutil.DoRequest(r, "GET", "/api/generations/"+started.RunID, nil, map[string]string{"runId": started.RunID})
	if pollW.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", pollW.Code, pollW.Body.String())
	}
	var run dto.GenerationRunDto
	testutil.ParseJSON(pollW, &run)
	if run.State != "running" {
		t.Fatalf("expected state running before StartRun, got %q", run.State)
	}

	var expSection *dto.GenerationSectionDto
	for i := range run.Sections {
		if run.Sections[i].Kind == "experience" {
			expSection = &run.Sections[i]
		}
	}
	if expSection == nil {
		t.Fatal("expected an experience section")
	}
	if len(expSection.Items) != 2 {
		t.Fatalf("expected 2 achievement items (every master bullet), got %d", len(expSection.Items))
	}
	if expSection.Items[0].Text != "Shipped the thing" || expSection.Items[1].Text != "Fixed the other thing" {
		t.Fatalf("expected items in master order, got %+v", expSection.Items)
	}
	for _, it := range expSection.Items {
		if it.Origin != "profile" {
			t.Errorf("item origin = %q, want profile", it.Origin)
		}
		if !it.Selected {
			t.Errorf("item %q selected = false, want true (fewer bullets than min)", it.Text)
		}
	}

	if err := svc.StartRun(ctx, started.RunID, nil); err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	fetchW := testutil.DoRequest(r, "GET", "/api/generations/"+started.RunID, nil, map[string]string{"runId": started.RunID})
	var finished dto.GenerationRunDto
	testutil.ParseJSON(fetchW, &finished)
	if finished.State != "ready" {
		t.Fatalf("expected state ready after StartRun, got %q: %+v", finished.State, finished)
	}
	var summarySection *dto.GenerationSectionDto
	for i := range finished.Sections {
		if finished.Sections[i].Kind == "summary" {
			summarySection = &finished.Sections[i]
		}
	}
	if summarySection == nil || len(summarySection.Items) != 1 {
		t.Fatalf("expected exactly one summary item, got %+v", summarySection)
	}
	if summarySection.Items[0].Origin != "ai" {
		t.Errorf("summary item origin = %q, want ai", summarySection.Items[0].Origin)
	}
}

func TestGenerationsStart_ProfileWithNoMasterContent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	testDB := dbtest.New(t)

	prof := insertTestProfile(ctx, t, testDB, nil)
	h, _ := newWorkspaceHandler(t, testDB)
	r := testutil.SetupRouter(h.Mount)

	w := testutil.DoRequestJSON(r, "POST", "/api/generations", map[string]any{
		"profileId": dbutil.UUIDString(prof.ID),
		"vacancy":   map[string]any{"company": "Acme", "title": "Senior Engineer", "text": "Looking for a Go engineer."},
	}, nil)
	if w.Code != 400 {
		t.Fatalf("expected 400 for a profile with no master content, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGenerationsStart_UnknownJobId(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	testDB := dbtest.New(t)

	config := masterJSON(t, "Acme Inc.", []string{"Shipped the thing"}, "Languages", "Go")
	prof := insertTestProfile(ctx, t, testDB, config)
	h, _ := newWorkspaceHandler(t, testDB)
	r := testutil.SetupRouter(h.Mount)

	w := testutil.DoRequestJSON(r, "POST", "/api/generations", map[string]any{
		"profileId": dbutil.UUIDString(prof.ID),
		"jobId":     "00000000-0000-0000-0000-000000000000",
	}, nil)
	if w.Code != 404 {
		t.Fatalf("expected 404 for an unknown jobId, got %d: %s", w.Code, w.Body.String())
	}
}
