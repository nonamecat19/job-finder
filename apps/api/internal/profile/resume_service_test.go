package profile_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbutil"
	"github.com/job-finder/api/internal/platform/llm"
	"github.com/job-finder/api/internal/profile"
)

type resumeFakeRepo struct {
	profile.Repository
	row sqlcgen.Profile
}

func (f *resumeFakeRepo) GetProfile(ctx context.Context, id pgtype.UUID) (sqlcgen.Profile, error) {
	return f.row, nil
}

func (f *resumeFakeRepo) UpdateProfile(ctx context.Context, params sqlcgen.UpdateProfileParams) error {
	if v := params.RendercvYaml; v != nil {
		f.row.RendercvYaml = v
	}
	if v := params.RendercvConfig; v != nil {
		f.row.RendercvConfig = v
	}
	return nil
}

func TestGetResume_SocialNetworksSurfaced(t *testing.T) {
	master := map[string]any{
		"cv": map[string]any{
			"name": "Test User",
			"social_networks": []map[string]any{
				{"network": "LinkedIn", "username": "testuser"},
				{"network": "GitHub", "username": "testuser"},
				{"network": "Telegram", "username": "testuser"},
			},
		},
	}
	configJSON, err := json.Marshal(master)
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}

	uid, _ := dbutil.ParseUUID("00000000-0000-0000-0000-000000000001")
	repo := &resumeFakeRepo{row: sqlcgen.Profile{
		ID: uid, Name: "Test User", RendercvConfig: configJSON,
	}}
	svc := profile.NewService(repo, nil, "", "")

	resume, err := svc.GetResume(context.Background(), "00000000-0000-0000-0000-000000000001")
	if err != nil {
		t.Fatalf("GetResume: %v", err)
	}
	if len(resume.SocialNetworks) != 3 {
		t.Fatalf("social networks = %+v, want 3 entries", resume.SocialNetworks)
	}
}

type createFakeRepo struct {
	profile.Repository
	created sqlcgen.CreateProfileParams
	row     sqlcgen.Profile
}

func (f *createFakeRepo) CreateProfile(ctx context.Context, params sqlcgen.CreateProfileParams) (sqlcgen.Profile, error) {
	f.created = params
	f.row = sqlcgen.Profile{
		ID:             f.row.ID,
		Name:           params.Name,
		RendercvYaml:   params.RendercvYaml,
		RendercvConfig: params.RendercvConfig,
	}
	return f.row, nil
}

func (f *createFakeRepo) GetProfile(ctx context.Context, id pgtype.UUID) (sqlcgen.Profile, error) {
	return f.row, nil
}

func (f *createFakeRepo) UpdateProfileEmbedding(ctx context.Context, params sqlcgen.UpdateProfileEmbeddingParams) error {
	return nil
}

type stubEmbedder struct{ llm.Provider }

func (stubEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	return []float32{0}, nil
}

// A blank profile (name only, no uploaded config) is a valid starting point —
// the dashboard creates one silently so the user lands on the editable form.
func TestCreate_WithoutYamlSeedsMinimalConfig(t *testing.T) {
	uid, _ := dbutil.ParseUUID("00000000-0000-0000-0000-000000000002")
	repo := &createFakeRepo{row: sqlcgen.Profile{ID: uid}}
	svc := profile.NewService(repo, stubEmbedder{}, "", "")

	out, err := svc.Create(context.Background(), "My Profile", "", nil)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !out.HasConfig {
		t.Fatalf("HasConfig = false, want seeded config")
	}
	if repo.created.RendercvYaml == nil || *repo.created.RendercvYaml == "" {
		t.Fatalf("rendercvYaml = %v, want seeded yaml", repo.created.RendercvYaml)
	}
	var master map[string]any
	if err := json.Unmarshal(repo.created.RendercvConfig, &master); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}
	cv, _ := master["cv"].(map[string]any)
	if cv["name"] != "My Profile" {
		t.Fatalf("cv.name = %v, want \"My Profile\"", cv["name"])
	}
}
